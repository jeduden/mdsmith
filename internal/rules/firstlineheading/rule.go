package firstlineheading

import (
	"fmt"

	"github.com/jeduden/tidymark/internal/lint"
	"github.com/jeduden/tidymark/internal/rule"
	"github.com/yuin/goldmark/ast"
)

func init() {
	rule.Register(&Rule{Level: 1})
}

// Rule checks that the first line of the file is a heading of the configured level.
type Rule struct {
	Level int // expected heading level (default: 1)
}

func (r *Rule) ID() string   { return "TM004" }
func (r *Rule) Name() string { return "first-line-heading" }

func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	level := r.Level
	if level == 0 {
		level = 1
	}

	// Empty file
	if len(f.Source) == 0 {
		return []lint.Diagnostic{{
			File:     f.Path,
			Line:     1,
			Column:   1,
			RuleID:   r.ID(),
			RuleName: r.Name(),
			Severity: lint.Warning,
			Message:  fmt.Sprintf("first line should be a level %d heading", level),
		}}
	}

	// Find the first child node of the document
	firstChild := f.AST.FirstChild()
	if firstChild == nil {
		return []lint.Diagnostic{{
			File:     f.Path,
			Line:     1,
			Column:   1,
			RuleID:   r.ID(),
			RuleName: r.Name(),
			Severity: lint.Warning,
			Message:  fmt.Sprintf("first line should be a level %d heading", level),
		}}
	}

	heading, ok := firstChild.(*ast.Heading)
	if !ok {
		return []lint.Diagnostic{{
			File:     f.Path,
			Line:     1,
			Column:   1,
			RuleID:   r.ID(),
			RuleName: r.Name(),
			Severity: lint.Warning,
			Message:  fmt.Sprintf("first line should be a level %d heading", level),
		}}
	}

	// Check that the heading is on line 1
	line := headingLine(heading, f)
	if line != 1 {
		return []lint.Diagnostic{{
			File:     f.Path,
			Line:     1,
			Column:   1,
			RuleID:   r.ID(),
			RuleName: r.Name(),
			Severity: lint.Warning,
			Message:  fmt.Sprintf("first line should be a level %d heading", level),
		}}
	}

	// Check heading level
	if heading.Level != level {
		return []lint.Diagnostic{{
			File:     f.Path,
			Line:     1,
			Column:   1,
			RuleID:   r.ID(),
			RuleName: r.Name(),
			Severity: lint.Warning,
			Message:  fmt.Sprintf("first heading should be level %d, got %d", level, heading.Level),
		}}
	}

	return nil
}

func headingLine(heading *ast.Heading, f *lint.File) int {
	lines := heading.Lines()
	if lines.Len() > 0 {
		return f.LineOfOffset(lines.At(0).Start)
	}
	for c := heading.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			return f.LineOfOffset(t.Segment.Start)
		}
	}
	return 1
}
