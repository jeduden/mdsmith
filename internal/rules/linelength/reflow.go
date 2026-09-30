package linelength

import (
	"bytes"
	"strings"
	"unicode/utf8"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/mdtext"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
)

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
	t := strings.TrimRight(tok, mdtext.AbbrevTrimCutset)
	if t == "" {
		return false
	}
	for _, a := range r.Abbreviations {
		if a == t {
			return true
		}
	}
	return mdtext.IsAbbrevToken(tok)
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

// wrapTokens packs tokens into lines no wider than width runes, each
// prefixed with indent. It first coalesces tokens into wrap units: a
// unit is a maximal run of tokens where glue(token) holds at every
// internal boundary, so an abbreviation (and its following word) stays
// in one unit. Units are the atomic wrap elements — a unit never splits
// across lines, and a unit wider than width still occupies its own
// line.
//
// Wrapping by unit (rather than gluing token by token) means a glued run
// that does not fit breaks *before* the unit instead of overflowing past
// width: "U. S. A." moves to the next line whole rather than dragging
// the line over the limit.
//
// No line may read as a line that ends the paragraph
// (lint.InterruptsParagraph): a line starting "# " would become a
// heading, "> " a block quote, "1. " a list (issue #844). Lines are as
// full as that allows; see linePlanner. Returns nil for an empty token
// list, when every layout has such a line, or when every layout that
// avoids them has a line more than maxOverflowUnits units past width.
func wrapTokens(tokens []string, indent string, width int, glue func(prev string) bool) []string {
	if len(tokens) == 0 {
		return nil
	}
	p := linePlanner{
		units:   buildWrapUnits(tokens, glue),
		indent:  indent,
		indentW: utf8.RuneCountInString(indent),
		width:   width,
	}
	return p.layout()
}

// maxOverflowUnits caps how far a line may run past width: at most this
// many units after the last one that fits. It keeps the search linear in
// the paragraph length. A paragraph that needs a longer line is left as
// written.
const maxOverflowUnits = 8

// linePlanner lays wrap units out into lines. A line "fits" when it is
// no wider than width or holds a single unit, and it is "safe" when
// lint.InterruptsParagraph is false for it. Every line of a layout is
// safe.
//
// The planner works back to front, so each choice knows what the rest
// of the paragraph allows. For each unit s it picks the line that starts
// there, in this order of preference:
//
//  1. the longest line that fits, after which every line fits;
//  2. the longest line that fits, after which a later line runs past
//     width;
//  3. the shortest line past width, at most maxOverflowUnits units
//     longer than the longest line that fits.
//
// A layout within width is therefore used whenever one exists. Text with
// no block start wraps exactly as greedy packing does, and a marker that
// greedy packing would put at the start of a line pulls the unit before
// it down to lead that line instead.
type linePlanner struct {
	units   []string
	indent  string
	indentW int
	width   int
	buf     []byte // scratch for rendering one candidate line
}

// linePlan is the line picked for one start unit: it holds units[s:end].
// end is 0 when the units from s on have no safe layout. fits reports
// that this line and every later one fit.
type linePlan struct {
	end  int
	fits bool
}

// layout returns the rendered lines, or nil when no layout is safe.
func (p *linePlanner) layout() []string {
	n := len(p.units)
	p.buf = make([]byte, 0, len(p.indent)+p.width)
	plans := make([]linePlan, n+1)
	// The empty remainder needs no line. Its end only has to be nonzero
	// to mark it laid out, and n is at least 1 here.
	plans[n] = linePlan{end: n, fits: true}
	for s := n - 1; s >= 0; s-- {
		plans[s] = p.pick(s, plans)
	}
	if plans[0].end == 0 {
		return nil
	}
	var lines []string
	for s := 0; s < n; s = plans[s].end {
		lines = append(lines, string(p.render(s, plans[s].end)))
	}
	return lines
}

// pick chooses the line that starts at unit s, given the plans of every
// later start. See linePlanner for the order of preference.
func (p *linePlanner) pick(s int, plans []linePlan) linePlan {
	fit := p.fitEnd(s)
	longest := 0
	for e := fit; e > s; e-- {
		if plans[e].end == 0 || p.breaks(s, e) {
			continue
		}
		if plans[e].fits {
			return linePlan{end: e, fits: true}
		}
		if longest == 0 {
			longest = e
		}
	}
	if longest > 0 {
		return linePlan{end: longest}
	}
	last := min(len(p.units), fit+maxOverflowUnits)
	for e := fit + 1; e <= last; e++ {
		if plans[e].end != 0 && !p.breaks(s, e) {
			return linePlan{end: e}
		}
	}
	return linePlan{}
}

// fitEnd returns the largest end such that the line holding
// units[s:end] fits: it is no wider than width, or it holds the one
// unit units[s].
func (p *linePlanner) fitEnd(s int) int {
	w := p.indentW + utf8.RuneCountInString(p.units[s])
	e := s + 1
	for e < len(p.units) {
		uw := utf8.RuneCountInString(p.units[e])
		if w+1+uw > p.width {
			break
		}
		w += 1 + uw
		e++
	}
	return e
}

// breaks reports whether the line holding units[s:e] is unsafe (see
// unsafeLine).
func (p *linePlanner) breaks(s, e int) bool {
	return unsafeLine(p.render(s, e))
}

// unsafeLine reports whether line, placed after a paragraph line, could
// end the paragraph. That is a CommonMark block start
// (lint.InterruptsParagraph) or, whatever flavor is configured, a block
// start of a Markdown extension (lint.ExtensionInterruptsParagraph).
func unsafeLine(line []byte) bool {
	return lint.InterruptsParagraph(line) || lint.ExtensionInterruptsParagraph(line)
}

// render writes the line holding units[s:e], indent first, into the
// scratch buffer and returns it. The result is valid until the next
// render call.
func (p *linePlanner) render(s, e int) []byte {
	p.buf = append(p.buf[:0], p.indent...)
	for i := s; i < e; i++ {
		if i > s {
			p.buf = append(p.buf, ' ')
		}
		p.buf = append(p.buf, p.units[i]...)
	}
	return p.buf
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
