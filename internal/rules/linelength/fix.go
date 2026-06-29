package linelength

import (
	"bytes"
	"unicode/utf8"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
)

// FixTitle implements rule.QuickFixTitler so the editor lightbulb reads
// the concrete edit rather than the generic "Fix all ...".
func (r *Rule) FixTitle() string { return "Reflow long lines" }

// Fix implements rule.FixableRule. It rewraps over-long top-level prose
// paragraphs to Max, but only when the rule is configured with
// reflow: true. With reflow off it returns the source unchanged, so
// `mdsmith fix` never reflows paragraphs unless the project opts in.
//
// Reflow is deliberately conservative. It touches only paragraphs whose
// parent is the document (not list items or block quotes), and skips any
// paragraph that is a table, sits inside a generated section, carries a
// hard line break, or contains inline raw HTML. Inline code spans are
// preserved verbatim as atomic tokens. A word wider than Max — a long URL
// or link — is left on its own over-long line, so the fixer is a true
// fixpoint: re-running it produces identical bytes.
func (r *Rule) Fix(f *lint.File) []byte {
	if !r.Reflow || f.AST == nil {
		return cloneBytes(f.Source)
	}
	width := r.Max
	if width <= 0 {
		width = 80
	}

	type replacement struct {
		startLine int
		endLine   int
		lines     []string
	}
	var repls []replacement
	spans := f.CodeSpanLiteralRanges()
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		para, ok := n.(*ast.Paragraph)
		if !ok {
			return ast.WalkContinue, nil
		}
		if sl, el, out, reflowed := r.reflowParagraph(f, para, width, spans); reflowed {
			repls = append(repls, replacement{startLine: sl, endLine: el, lines: out})
		}
		return ast.WalkSkipChildren, nil
	})
	if len(repls) == 0 {
		return cloneBytes(f.Source)
	}

	out := make([][]byte, 0, len(f.Lines))
	li := 0 // 0-based index into f.Lines
	for _, rp := range repls {
		for li < rp.startLine-1 {
			out = append(out, f.Lines[li])
			li++
		}
		for _, s := range rp.lines {
			out = append(out, []byte(s))
		}
		li = rp.endLine // skip the paragraph's original lines
	}
	for li < len(f.Lines) {
		out = append(out, f.Lines[li])
		li++
	}
	return bytes.Join(out, []byte("\n"))
}

// reflowParagraph computes replacement lines for one paragraph. It
// returns the paragraph's 1-based start and end source lines, the
// rewrapped lines, and whether a reflow should be applied. reflowed is
// false when the paragraph is out of scope (not top-level, a table,
// generated, hard-broken, raw-HTML-bearing) or has no line that the rule
// would actually flag as too long.
func (r *Rule) reflowParagraph(
	f *lint.File, para *ast.Paragraph, width int, spans []lint.Range,
) (startLine, endLine int, out []string, reflowed bool) {
	segs := para.Lines()
	if segs.Len() == 0 {
		return 0, 0, nil, false
	}
	first := segs.At(0)
	last := segs.At(segs.Len() - 1)
	startLine = f.LineOfOffset(first.Start)
	endLine = f.LineOfOffset(last.Start)
	if startLine < 1 || endLine > len(f.Lines) || endLine < startLine {
		return 0, 0, nil, false
	}

	parent := para.Parent()
	if parent == nil || parent.Kind() != ast.KindDocument {
		return 0, 0, nil, false
	}
	if isTableLineStart(f.Lines[startLine-1]) {
		return 0, 0, nil, false
	}
	if overlapsGeneratedRange(f, startLine, endLine) {
		return 0, 0, nil, false
	}
	if paragraphHasRawHTML(para) {
		return 0, 0, nil, false
	}
	if !r.paragraphHasFlaggedLine(f, startLine, endLine, width) {
		return 0, 0, nil, false
	}

	indent := leadingWhitespace(f.Lines[startLine-1])
	tokens := tokenizeParagraph(f.Source, first.Start, last.Stop, spans)
	if len(tokens) == 0 {
		return 0, 0, nil, false
	}
	out = wrapTokens(tokens, indent, width, r.isAbbrev)
	return startLine, endLine, out, true
}

// paragraphHasFlaggedLine reports whether any source line of the
// paragraph would be flagged by Check at the given width: longer than
// width runes and not skipped by the rule's url/stern exclusions. A
// paragraph carrying a hard line break is reported as not flagged so the
// caller leaves the intentional break alone.
func (r *Rule) paragraphHasFlaggedLine(f *lint.File, startLine, endLine, width int) bool {
	for i := startLine - 1; i < endLine; i++ {
		line := f.Lines[i]
		if hasHardLineBreak(line) {
			return false
		}
		if utf8.RuneCount(line) <= width {
			continue
		}
		if r.isExcluded("urls") && isURLOnlyLine(line) {
			continue
		}
		if r.Stern && !hasSpacePastLimit(line, width) {
			continue
		}
		return true
	}
	return false
}

// overlapsGeneratedRange reports whether the inclusive line span
// [startLine, endLine] intersects any generated-section range, which the
// host file must not rewrite — the directive source owns those bytes.
func overlapsGeneratedRange(f *lint.File, startLine, endLine int) bool {
	for _, gr := range f.GeneratedRanges {
		if startLine <= gr.To && gr.From <= endLine {
			return true
		}
	}
	return false
}

// cloneBytes returns a copy of b so callers never alias the file source.
func cloneBytes(b []byte) []byte {
	out := make([]byte, len(b))
	copy(out, b)
	return out
}

var (
	_ rule.FixableRule    = (*Rule)(nil)
	_ rule.QuickFixTitler = (*Rule)(nil)
	_ rule.ListMerger     = (*Rule)(nil)
)
