// Package blockquotewhitespace implements MDS059, which flags two blockquote
// defects: more than one space after the > marker (MD027) and a blank line
// between two adjacent sibling blockquote nodes (MD028).
package blockquotewhitespace

import (
	"bytes"
	"regexp"
	"strings"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/yuin/goldmark/ast"
)

func init() {
	rule.Register(&Rule{})
}

// Rule flags two blockquote whitespace defects.
// MD027: more than one space after the > marker; autofix collapses to one.
// MD028: blank line(s) between two adjacent sibling blockquotes; flag only.
type Rule struct{}

var (
	// reBlockquoteLine matches lines that begin with an optional blockquote
	// prefix so the MD027 check skips > inside code spans or list text.
	reBlockquoteLine = regexp.MustCompile(`^\s{0,3}(?:>\s*)*>`)
	reMultiSpace     = regexp.MustCompile(`> {2,}`)
)

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS059" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "blockquote-whitespace" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "whitespace" }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	var diags []lint.Diagnostic
	codeLines := lint.CollectCodeBlockLines(f)

	// MD027: flag blockquote lines that have > followed by 2+ spaces.
	// Restrict to lines that start with the blockquote marker pattern so
	// that > inside code spans or list text is not flagged.
	for i, line := range f.Lines {
		lineNum := i + 1
		if codeLines[lineNum] {
			continue
		}
		if !reBlockquoteLine.Match(line) {
			continue
		}
		if loc := reMultiSpace.FindIndex(line); loc != nil {
			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     lineNum,
				Column:   loc[0] + 1,
				RuleID:   r.ID(),
				RuleName: r.Name(),
				Severity: lint.Warning,
				Message:  "multiple spaces after blockquote marker",
			})
		}
	}

	// MD028: flag blank-line gaps between adjacent sibling blockquote nodes.
	diags = append(diags, r.checkBlankBetween(f)...)
	return diags
}

func (r *Rule) checkBlankBetween(f *lint.File) []lint.Diagnostic {
	var diags []lint.Diagnostic
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		bq, ok := n.(*ast.Blockquote)
		if !ok {
			return ast.WalkContinue, nil
		}
		nextBq, ok := bq.NextSibling().(*ast.Blockquote)
		if !ok {
			return ast.WalkContinue, nil
		}
		last := nodeLastLine(f, bq)
		first := nodeFirstLine(f, nextBq)
		if last <= 0 || first <= 0 || first <= last+1 {
			return ast.WalkContinue, nil
		}
		for gap := last + 1; gap < first; gap++ {
			if !isBlankLine(f, gap) {
				return ast.WalkContinue, nil
			}
		}
		diags = append(diags, lint.Diagnostic{
			File:     f.Path,
			Line:     last + 1,
			Column:   1,
			RuleID:   r.ID(),
			RuleName: r.Name(),
			Severity: lint.Warning,
			Message:  "blank line between blockquotes",
		})
		return ast.WalkContinue, nil
	})
	return diags
}

// Fix implements rule.FixableRule. Collapses multiple spaces after > to one
// space on every non-code-block line. MD028 violations are not auto-fixed
// because the intent (one quote vs two) is ambiguous.
func (r *Rule) Fix(f *lint.File) []byte {
	codeLines := lint.CollectCodeBlockLines(f)
	lines := make([]string, len(f.Lines))
	for i, line := range f.Lines {
		lineNum := i + 1
		if codeLines[lineNum] || !reBlockquoteLine.Match(line) {
			lines[i] = string(line)
			continue
		}
		lines[i] = string(reMultiSpace.ReplaceAll(line, []byte("> ")))
	}
	return []byte(strings.Join(lines, "\n"))
}

// nodeFirstLine returns the 1-based source line of n's first content byte.
// For container nodes (e.g. Blockquote) it recurses into children.
func nodeFirstLine(f *lint.File, n ast.Node) int {
	if t, ok := n.(*ast.Text); ok {
		return f.LineOfOffset(t.Segment.Start)
	}
	if n.Lines().Len() > 0 {
		return f.LineOfOffset(n.Lines().At(0).Start)
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if line := nodeFirstLine(f, c); line > 0 {
			return line
		}
	}
	return 0
}

// nodeLastLine returns the 1-based source line of n's last content byte.
// For container nodes it recurses into children in reverse order.
func nodeLastLine(f *lint.File, n ast.Node) int {
	if t, ok := n.(*ast.Text); ok {
		if t.Segment.Stop > 0 {
			return f.LineOfOffset(t.Segment.Stop - 1)
		}
		return f.LineOfOffset(t.Segment.Start)
	}
	if n.Lines().Len() > 0 {
		last := n.Lines().At(n.Lines().Len() - 1)
		return f.LineOfOffset(last.Start)
	}
	for c := n.LastChild(); c != nil; c = c.PreviousSibling() {
		if line := nodeLastLine(f, c); line > 0 {
			return line
		}
	}
	return 0
}

func isBlankLine(f *lint.File, lineNum int) bool {
	idx := lineNum - 1
	if idx < 0 || idx >= len(f.Lines) {
		return false
	}
	return len(bytes.TrimSpace(f.Lines[idx])) == 0
}

var _ rule.FixableRule = (*Rule)(nil)
