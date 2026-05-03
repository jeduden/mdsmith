// Package nospaceincodespans implements MDS052, which flags inline code
// spans with leading or trailing whitespace inside the backticks.
package nospaceincodespans

import (
	"bytes"
	"sort"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/yuin/goldmark/ast"
)

func init() {
	rule.Register(&Rule{})
}

// Rule flags inline code spans with leading or trailing whitespace.
type Rule struct{}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS052" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "no-space-in-code-spans" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "whitespace" }

// EnabledByDefault implements rule.Defaultable.
func (r *Rule) EnabledByDefault() bool { return false }

const (
	msgLeading  = "code span has leading whitespace"
	msgTrailing = "code span has trailing whitespace"
)

// Check implements rule.Rule.
//
// Detection uses the goldmark text-segment bytes, which already reflect
// CommonMark's single-space-trim rule (one space stripped from each side
// when both sides have one and the content is not all spaces). Inspecting
// the post-trim segment avoids false positives on spans like “ `  x ` “
// where only the leading double-space is visible after CommonMark normalises
// the source.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	var diags []lint.Diagnostic
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		cs, ok := n.(*ast.CodeSpan)
		if !ok {
			return ast.WalkContinue, nil
		}
		first, last, ok2 := spanBounds(cs)
		if !ok2 || last == first {
			return ast.WalkContinue, nil
		}
		seg := f.Source[first:last]

		// Find opening backtick offset for position reporting.
		// recoverContentBounds undoes the CommonMark leading-space strip so we
		// step back to the raw content boundary before walking through backticks.
		rawStart, _ := recoverContentBounds(first, last, f.Source)
		btStart := rawStart
		for btStart > 0 && f.Source[btStart-1] == '`' {
			btStart--
		}
		line := f.LineOfOffset(btStart)
		col := f.ColumnOfOffset(btStart)

		if isASCIIWhitespace(seg[0]) {
			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     line,
				Column:   col,
				RuleID:   r.ID(),
				RuleName: r.Name(),
				Severity: lint.Warning,
				Message:  msgLeading,
			})
		}
		if isASCIIWhitespace(seg[len(seg)-1]) {
			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     line,
				Column:   col,
				RuleID:   r.ID(),
				RuleName: r.Name(),
				Severity: lint.Warning,
				Message:  msgTrailing,
			})
		}
		return ast.WalkContinue, nil
	})
	return diags
}

// Fix implements rule.FixableRule. It trims leading and trailing whitespace
// from code span content in the source file. Spans that become empty after
// trimming, or whose trimmed content starts or ends with a backtick (which
// would merge with the delimiter run and change the parsed value), are left
// unchanged.
func (r *Rule) Fix(f *lint.File) []byte {
	type cut struct {
		start, end int
		repl       []byte
	}
	var cuts []cut

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		cs, ok := n.(*ast.CodeSpan)
		if !ok {
			return ast.WalkContinue, nil
		}
		first, last, ok2 := spanBounds(cs)
		if !ok2 || last == first {
			return ast.WalkContinue, nil
		}
		// Use the goldmark segment (post-CommonMark-trim) to decide whether
		// this span needs fixing.
		seg := f.Source[first:last]
		if !isASCIIWhitespace(seg[0]) && !isASCIIWhitespace(seg[len(seg)-1]) {
			return ast.WalkContinue, nil
		}
		// Recover the full raw content (pre-trim) from the source.
		start, end := recoverContentBounds(first, last, f.Source)
		raw := f.Source[start:end]

		// Use bytes.Trim with an explicit ASCII cutset (not TrimFunc) to
		// avoid the rune-to-byte truncation hazard with non-ASCII content.
		trimmed := bytes.Trim(raw, " \t\n\r")
		if len(trimmed) == 0 {
			return ast.WalkContinue, nil
		}
		// If the trimmed content starts or ends with a backtick, naively
		// removing the surrounding spaces would merge those backticks into
		// the delimiter run and change the rendered code span. Preserve one
		// protective space on the affected side instead.
		if trimmed[0] == '`' {
			trimmed = append([]byte{' '}, trimmed...)
		}
		if trimmed[len(trimmed)-1] == '`' {
			trimmed = append(trimmed, ' ')
		}
		if bytes.Equal(trimmed, raw) {
			return ast.WalkContinue, nil
		}
		cuts = append(cuts, cut{start: start, end: end, repl: trimmed})
		return ast.WalkContinue, nil
	})

	if len(cuts) == 0 {
		out := make([]byte, len(f.Source))
		copy(out, f.Source)
		return out
	}

	sort.Slice(cuts, func(i, j int) bool { return cuts[i].start < cuts[j].start })
	var out bytes.Buffer
	prev := 0
	for _, c := range cuts {
		out.Write(f.Source[prev:c.start])
		out.Write(c.repl)
		prev = c.end
	}
	out.Write(f.Source[prev:])
	return out.Bytes()
}

// recoverContentBounds returns the [start, end) byte range of a code span's
// raw content, undoing the CommonMark single-space trim that goldmark applies
// before recording the text-child segments.
func recoverContentBounds(first, last int, source []byte) (start, end int) {
	start = first
	// If the byte before the segment is a space and the byte before that is
	// a backtick, the leading space was stripped by CommonMark.
	if start > 1 && source[start-1] == ' ' && source[start-2] == '`' {
		start--
	}

	end = last
	// Similarly for the trailing side.
	if end+1 < len(source) && source[end] == ' ' && source[end+1] == '`' {
		end++
	}
	return start, end
}

// spanBounds returns the [start, end) byte range of a CodeSpan's content
// as reported by goldmark (post-CommonMark-trim) by walking text children.
func spanBounds(cs *ast.CodeSpan) (first, last int, ok bool) {
	first = -1
	last = -1
	for c := cs.FirstChild(); c != nil; c = c.NextSibling() {
		t, ok2 := c.(*ast.Text)
		if !ok2 {
			continue
		}
		if first < 0 || t.Segment.Start < first {
			first = t.Segment.Start
		}
		if t.Segment.Stop > last {
			last = t.Segment.Stop
		}
	}
	return first, last, first >= 0 && last >= first
}

func isASCIIWhitespace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

var (
	_ rule.FixableRule = (*Rule)(nil)
	_ rule.Defaultable = (*Rule)(nil)
)
