package linelength

import (
	"bytes"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/punkt"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
)

// abbrevTrimCutset is the trailing clause punctuation stripped before a
// user-list abbreviation lookup, so a configured "e.g." also matches
// "e.g.,". punkt.Storage.IsAbbrevToken applies the same trim internally
// for the trained-model path.
const abbrevTrimCutset = ",;:"

// englishAbbrevStorage lazily loads the trained Punkt model — the same
// abbreviation knowledge the readability rules use to split sentences —
// the first time a reflow fix needs it. Loading is deferred so a plain
// `mdsmith check` (or a fix with reflow off) never pays the english.json
// parse.
var englishAbbrevStorage = sync.OnceValue(func() *punkt.Storage {
	return punkt.NewEnglish().Storage
})

// isAbbrev reports whether tok is an abbreviation that must stay glued
// to the word that follows it during reflow, so reflow never ends a
// wrapped line on it. Detection defers to the trained Punkt model
// (honorifics like "Dr."/"Mr.", reference forms like "vs."/"No.",
// initials like "J.", and dotted forms like "e.g."/"i.e."/"U.S.A.").
// The configured Abbreviations list adds project-specific entries the
// model does not know ("etc.", "approx."); they are matched after
// trimming trailing clause punctuation, the same normalization the
// model applies.
func (r *Rule) isAbbrev(tok string) bool {
	t := strings.TrimRight(tok, abbrevTrimCutset)
	if t == "" {
		return false
	}
	for _, a := range r.Abbreviations {
		if a == t {
			return true
		}
	}
	return englishAbbrevStorage().IsAbbrevToken(tok)
}

// tokenizeParagraph splits the source bytes in [start, end) into
// whitespace-delimited tokens, treating each inline code span as one
// atomic token whose internal bytes are preserved verbatim. spans holds
// the document-absolute literal ranges (backticks included) of every
// code span, in ascending order; a newline inside a span becomes a
// single space, mirroring CommonMark's code-span line-ending rule. A
// code span adjacent to surrounding text with no intervening space stays
// part of the same token (e.g. "pre`code`post").
func tokenizeParagraph(src []byte, start, end int, spans []lint.Range) []string {
	var tokens []string
	var cur []byte
	flush := func() {
		if len(cur) > 0 {
			tokens = append(tokens, string(cur))
			cur = cur[:0]
		}
	}
	si := 0
	for si < len(spans) && spans[si].End <= start {
		si++
	}
	for pos := start; pos < end; {
		for si < len(spans) && spans[si].End <= pos {
			si++
		}
		if si < len(spans) && pos >= spans[si].Start && pos < spans[si].End {
			spanEnd := spans[si].End
			if spanEnd > end {
				spanEnd = end
			}
			for j := pos; j < spanEnd; j++ {
				if b := src[j]; b == '\n' || b == '\r' {
					cur = append(cur, ' ')
				} else {
					cur = append(cur, b)
				}
			}
			pos = spanEnd
			continue
		}
		switch src[pos] {
		case ' ', '\t', '\n', '\r':
			flush()
		default:
			cur = append(cur, src[pos])
		}
		pos++
	}
	flush()
	return tokens
}

// wrapTokens greedily packs tokens into lines no wider than width runes,
// each prefixed with indent. It first coalesces tokens into wrap units:
// a unit is a maximal run of tokens where glue(token) holds at every
// internal boundary, so an abbreviation (and its following word) stays
// in one unit. Units are the atomic wrap elements — a unit never splits
// across lines, and a unit wider than width still occupies its own line.
//
// Wrapping by unit (rather than gluing token by token) means a glued run
// that does not fit breaks *before* the unit instead of overflowing past
// width: "U. S. A." moves to the next line whole rather than dragging
// the line over the limit. Returns nil for an empty token list.
func wrapTokens(tokens []string, indent string, width int, glue func(prev string) bool) []string {
	if len(tokens) == 0 {
		return nil
	}
	units := buildWrapUnits(tokens, glue)
	indentW := utf8.RuneCountInString(indent)
	var lines []string
	cur := indent + units[0]
	curW := indentW + utf8.RuneCountInString(units[0])
	for _, u := range units[1:] {
		uW := utf8.RuneCountInString(u)
		if curW+1+uW <= width {
			cur += " " + u
			curW += 1 + uW
		} else {
			lines = append(lines, cur)
			cur = indent + u
			curW = indentW + uW
		}
	}
	return append(lines, cur)
}

// buildWrapUnits coalesces tokens into space-joined units. A new token
// joins the current unit while glue holds for the unit's last token, so
// an abbreviation never ends a unit (and thus never ends a wrapped
// line). Every token lands in exactly one unit, so the joined units
// reproduce the original word sequence.
func buildWrapUnits(tokens []string, glue func(prev string) bool) []string {
	units := make([]string, 0, len(tokens))
	var b strings.Builder
	for i := 0; i < len(tokens); {
		j := i
		size := len(tokens[i])
		for j < len(tokens)-1 && glue(tokens[j]) {
			j++
			size += 1 + len(tokens[j]) // +1 for the space separator
		}
		if j == i {
			units = append(units, tokens[i])
		} else {
			b.Reset()
			b.Grow(size)
			for k := i; k <= j; k++ {
				if k > i {
					b.WriteByte(' ')
				}
				b.WriteString(tokens[k])
			}
			units = append(units, b.String())
		}
		i = j + 1
	}
	return units
}

// leadingWhitespace returns the run of spaces and tabs at the start of
// line, as a string suitable for re-prefixing reflowed lines.
func leadingWhitespace(line []byte) string {
	i := 0
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	return string(line[:i])
}

// hasHardLineBreak reports whether line ends with a Markdown hard line
// break — a trailing backslash (with no following whitespace) or two or
// more trailing spaces. A paragraph carrying one is left unreflowed so
// the intentional break survives. A single trailing backslash followed
// by a space is an escaped space, not a break, so the raw line is tested
// without trimming.
func hasHardLineBreak(line []byte) bool {
	if bytes.HasSuffix(line, []byte("\\")) {
		return true
	}
	return bytes.HasSuffix(line, []byte("  "))
}

// paragraphHasRawHTML reports whether the paragraph subtree contains an
// inline raw-HTML node. Such paragraphs are skipped: an inline tag like
// <br> is line-break significant and raw markup whitespace can matter, so
// reflow leaves them untouched.
func paragraphHasRawHTML(para ast.Node) bool {
	found := false
	_ = ast.Walk(para, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if n.Kind() == ast.KindRawHTML {
			found = true
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	return found
}
