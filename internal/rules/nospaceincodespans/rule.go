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
// where only one leading space remains visible after CommonMark strips one
// from each side. The per-span logic is pure and stateless, so it is
// expressed as CheckNode and the engine can fold this rule into one
// shared AST walk; a direct call still works via rule.WalkNodes.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	return rule.WalkNodes(r, f)
}

// CheckNode implements rule.NodeChecker.
func (r *Rule) CheckNode(n ast.Node, entering bool, f *lint.File) []lint.Diagnostic {
	if !entering {
		return nil
	}
	cs, ok := n.(*ast.CodeSpan)
	if !ok {
		return nil
	}
	first, last, ok2 := spanBounds(cs)
	if !ok2 || last == first {
		return nil
	}
	seg := f.Source[first:last]
	if !isASCIIWhitespace(seg[0]) && !isASCIIWhitespace(seg[len(seg)-1]) {
		return nil
	}
	btStart := openingBacktickOffset(cs, f.Source)
	line := f.LineOfOffset(btStart)
	if inGeneratedSection(f, line) {
		return nil
	}
	col := f.ColumnOfOffset(btStart)

	var diags []lint.Diagnostic
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
	return diags
}

// Fix implements rule.FixableRule. It trims leading and trailing whitespace
// from code span content in the source file. Spans that become empty after
// trimming are left unchanged. When the trimmed content starts or ends with a
// backtick, balanced spaces are added on both sides so CommonMark's
// single-space trim removes them symmetrically, preserving the rendered
// content without merging the content backtick into the delimiter run.
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
		seg := f.Source[first:last]
		if !isASCIIWhitespace(seg[0]) && !isASCIIWhitespace(seg[len(seg)-1]) {
			return ast.WalkContinue, nil
		}
		if inGeneratedSection(f, f.LineOfOffset(openingBacktickOffset(cs, f.Source))) {
			return ast.WalkContinue, nil
		}
		start, end := recoverContentBounds(first, last, f.Source)
		raw := f.Source[start:end]
		// bytes.Trim with an explicit cutset avoids the rune truncation hazard of TrimFunc.
		trimmed := bytes.Trim(raw, " \t\n\r")
		if len(trimmed) == 0 {
			return ast.WalkContinue, nil
		}
		// If the trimmed content starts or ends with a backtick, naively
		// removing the surrounding spaces would merge those backticks into
		// the delimiter run and change the rendered code span. Add balanced
		// spaces on both sides so CommonMark's single-space trim removes them
		// symmetrically, leaving the correct content.
		if trimmed[0] == '`' || trimmed[len(trimmed)-1] == '`' {
			trimmed = append(append([]byte{' '}, trimmed...), ' ')
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

// openingBacktickOffset returns the byte offset of the opening backtick
// delimiter of cs by walking back through the raw source past any stripped
// leading space and then through the backtick run.
func openingBacktickOffset(cs *ast.CodeSpan, source []byte) int {
	first, last, ok := spanBounds(cs)
	if !ok {
		return 0
	}
	rawStart, _ := recoverContentBounds(first, last, source)
	off := rawStart
	for off > 0 && source[off-1] == '`' {
		off--
	}
	return off
}

// inGeneratedSection reports whether line falls inside any generated section.
func inGeneratedSection(f *lint.File, line int) bool {
	for _, gr := range f.GeneratedRanges {
		if gr.Contains(line) {
			return true
		}
	}
	return false
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
	_ rule.NodeChecker = (*Rule)(nil)
)
