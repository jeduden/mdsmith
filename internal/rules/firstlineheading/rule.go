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

// ID implements rule.Rule.
func (r *Rule) ID() string { return "TM004" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "first-line-heading" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "heading" }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	level := r.Level
	if level == 0 {
		level = 1
	}

	missingMsg := fmt.Sprintf("first line should be a level %d heading", level)

	if len(f.Source) == 0 {
		return r.diag(f, missingMsg)
	}

	firstChild := f.AST.FirstChild()
	if firstChild == nil {
		return r.diag(f, missingMsg)
	}

	heading, ok := firstChild.(*ast.Heading)
	if !ok {
		return r.diag(f, missingMsg)
	}

	if headingLine(heading, f) != 1 {
		return r.diag(f, missingMsg)
	}

	if heading.Level != level {
		return r.diag(f, fmt.Sprintf("first heading should be level %d, got %d", level, heading.Level))
	}

	return nil
}

// diag returns a single-element diagnostic slice for a line-1 issue.
func (r *Rule) diag(f *lint.File, msg string) []lint.Diagnostic {
	return []lint.Diagnostic{{
		File:     f.Path,
		Line:     1,
		Column:   1,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Warning,
		Message:  msg,
	}}
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(settings map[string]any) error {
	for k, v := range settings {
		switch k {
		case "level":
			n, ok := toInt(v)
			if !ok {
				return fmt.Errorf("first-line-heading: level must be an integer, got %T", v)
			}
			if n < 1 || n > 6 {
				return fmt.Errorf("first-line-heading: level must be 1-6, got %d", n)
			}
			r.Level = n
		default:
			return fmt.Errorf("first-line-heading: unknown setting %q", k)
		}
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"level": 1,
	}
}

// toInt converts a value to int.
func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case float64:
		return int(n), true
	case int64:
		return int(n), true
	}
	return 0, false
}

var _ rule.Configurable = (*Rule)(nil)

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
