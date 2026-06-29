package linelength

import (
	"bytes"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
)

// defaultAbbreviations is the built-in set of forward-gluing
// abbreviations: tokens that precede content and so must not be left
// dangling at the end of a wrapped line. The reflow wrapper keeps the
// word after one of these on the same line. Initials ("J.") and
// internal-dot abbreviations ("e.g.", "i.e.", "a.m.") are recognised by
// looksLikeAbbrev instead, so they need not be listed here. Users extend
// this set through the `abbreviations` setting (append-merged across
// config layers); they cannot remove a built-in entry.
var defaultAbbreviations = []string{
	"Mr.", "Mrs.", "Ms.", "Dr.", "Prof.", "Sr.", "Jr.", "St.", "Rev.",
	"vs.", "cf.", "No.", "Vol.", "Ch.", "Sec.", "Fig.", "Eq.", "pp.",
}

// abbrevTrimCutset is the trailing punctuation stripped before an
// abbreviation lookup, so "e.g.," and "cf.;" still glue forward.
const abbrevTrimCutset = ",;:"

// isAbbrev reports whether tok is an abbreviation that must stay glued
// to the word that follows it during reflow. It checks the rule's
// configured set (built-ins plus any `abbreviations` additions) and the
// structural looksLikeAbbrev heuristic. Trailing clause punctuation is
// trimmed first so "e.g.," matches "e.g.".
func (r *Rule) isAbbrev(tok string) bool {
	t := strings.TrimRight(tok, abbrevTrimCutset)
	if t == "" {
		return false
	}
	if _, ok := r.abbrevSet()[t]; ok {
		return true
	}
	return looksLikeAbbrev(t)
}

// abbrevSet returns the effective abbreviation set: the built-in
// defaults unioned with the configured Abbreviations list. The built-ins
// are always present so a user `abbreviations` list extends rather than
// replaces them, even on the byte-level config path that does not run
// the append merge.
func (r *Rule) abbrevSet() map[string]struct{} {
	set := make(map[string]struct{}, len(defaultAbbreviations)+len(r.Abbreviations))
	for _, a := range defaultAbbreviations {
		set[a] = struct{}{}
	}
	for _, a := range r.Abbreviations {
		if a != "" {
			set[a] = struct{}{}
		}
	}
	return set
}

// looksLikeAbbrev recognises abbreviations by shape rather than by an
// explicit list: a single-letter initial ("J.", "e.") or a token built
// only of letters and periods carrying an internal period ("e.g.",
// "i.e.", "U.S.A.", "a.m."). A plain word ending a sentence ("cat.",
// "Go.") has one trailing period and more than one letter, so it is not
// matched — reflow may end a line on it.
func looksLikeAbbrev(t string) bool {
	if !strings.HasSuffix(t, ".") {
		return false
	}
	letters, dots := 0, 0
	for _, c := range t {
		switch {
		case c == '.':
			dots++
		case unicode.IsLetter(c):
			letters++
		default:
			return false
		}
	}
	if letters == 0 || letters > 3 {
		return false
	}
	return letters == 1 || dots >= 2
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
// each prefixed with indent. glue(prev) reports whether the token after
// prev must stay on the same line regardless of width, which keeps an
// abbreviation joined to the word that follows it. A single token wider
// than width still occupies its own line (it cannot be broken). Returns
// nil for an empty token list.
func wrapTokens(tokens []string, indent string, width int, glue func(prev string) bool) []string {
	if len(tokens) == 0 {
		return nil
	}
	indentW := utf8.RuneCountInString(indent)
	var lines []string
	cur := indent + tokens[0]
	curW := indentW + utf8.RuneCountInString(tokens[0])
	prev := tokens[0]
	for _, tok := range tokens[1:] {
		tokW := utf8.RuneCountInString(tok)
		if glue(prev) || curW+1+tokW <= width {
			cur += " " + tok
			curW += 1 + tokW
		} else {
			lines = append(lines, cur)
			cur = indent + tok
			curW = indentW + tokW
		}
		prev = tok
	}
	return append(lines, cur)
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
