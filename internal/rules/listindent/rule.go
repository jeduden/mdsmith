package listindent

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/jeduden/tidymark/internal/lint"
	"github.com/jeduden/tidymark/internal/rule"
	"github.com/yuin/goldmark/ast"
)

func init() {
	rule.Register(&Rule{Spaces: 2})
}

// Rule checks that nested list items are indented by the configured number of
// spaces per nesting level.
type Rule struct {
	Spaces int
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "TM016" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "list-indent" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "list" }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	var diags []lint.Diagnostic
	spaces := r.Spaces
	if spaces <= 0 {
		spaces = 2
	}

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		listItem, ok := n.(*ast.ListItem)
		if !ok {
			return ast.WalkContinue, nil
		}

		level := nestingLevel(listItem)
		if level == 0 {
			return ast.WalkContinue, nil
		}

		expectedIndent := level * spaces
		line := firstLineOfListItem(f, listItem)
		if line < 1 || line > len(f.Lines) {
			return ast.WalkContinue, nil
		}

		actualIndent := countLeadingSpaces(f.Lines[line-1])
		if actualIndent != expectedIndent {
			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     line,
				Column:   1,
				RuleID:   r.ID(),
				RuleName: r.Name(),
				Severity: lint.Warning,
				Message:  "list indent should be " + itoa(expectedIndent) + " spaces, found " + itoa(actualIndent),
			})
		}

		return ast.WalkContinue, nil
	})

	return diags
}

// nestingLevel returns the nesting depth of a ListItem. A top-level list item
// returns 0. A list item inside a nested list returns 1, etc.
func nestingLevel(li *ast.ListItem) int {
	level := 0
	for p := li.Parent(); p != nil; p = p.Parent() {
		if _, ok := p.(*ast.ListItem); ok {
			level++
		}
	}
	return level
}

func firstLineOfListItem(f *lint.File, li *ast.ListItem) int {
	if li.Lines().Len() > 0 {
		seg := li.Lines().At(0)
		return f.LineOfOffset(seg.Start)
	}
	// Try children.
	if li.HasChildren() {
		for c := li.FirstChild(); c != nil; c = c.NextSibling() {
			line := firstLineOfChild(f, c)
			if line > 0 {
				return line
			}
		}
	}
	return 0
}

// isInlineNode returns true for inline AST nodes whose Lines() method panics.
func isInlineNode(n ast.Node) bool {
	switch n.(type) {
	case *ast.Text, *ast.String, *ast.CodeSpan, *ast.Emphasis,
		*ast.Link, *ast.Image, *ast.AutoLink, *ast.RawHTML:
		return true
	}
	return false
}

func firstLineOfChild(f *lint.File, n ast.Node) int {
	if t, ok := n.(*ast.Text); ok {
		return f.LineOfOffset(t.Segment.Start)
	}
	if isInlineNode(n) {
		if n.HasChildren() {
			for c := n.FirstChild(); c != nil; c = c.NextSibling() {
				line := firstLineOfChild(f, c)
				if line > 0 {
					return line
				}
			}
		}
		return 0
	}
	if n.Lines().Len() > 0 {
		seg := n.Lines().At(0)
		return f.LineOfOffset(seg.Start)
	}
	if n.HasChildren() {
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			line := firstLineOfChild(f, c)
			if line > 0 {
				return line
			}
		}
	}
	return 0
}

func countLeadingSpaces(line []byte) int {
	count := 0
	for _, b := range line {
		if b == ' ' {
			count++
		} else {
			break
		}
	}
	return count
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	if neg {
		s = "-" + s
	}
	return s
}

// Fix implements rule.FixableRule.
func (r *Rule) Fix(f *lint.File) []byte {
	spaces := r.Spaces
	if spaces <= 0 {
		spaces = 2
	}

	type lineAdjust struct {
		line           int // 1-based
		expectedIndent int
	}
	var adjustments []lineAdjust

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		listItem, ok := n.(*ast.ListItem)
		if !ok {
			return ast.WalkContinue, nil
		}

		level := nestingLevel(listItem)
		if level == 0 {
			return ast.WalkContinue, nil
		}

		expectedIndent := level * spaces
		line := firstLineOfListItem(f, listItem)
		if line < 1 || line > len(f.Lines) {
			return ast.WalkContinue, nil
		}

		actualIndent := countLeadingSpaces(f.Lines[line-1])
		if actualIndent != expectedIndent {
			adjustments = append(adjustments, lineAdjust{
				line:           line,
				expectedIndent: expectedIndent,
			})
		}

		return ast.WalkContinue, nil
	})

	if len(adjustments) == 0 {
		result := make([]byte, len(f.Source))
		copy(result, f.Source)
		return result
	}

	// Build a map for quick lookup.
	adjMap := make(map[int]int) // line -> expected indent
	for _, a := range adjustments {
		adjMap[a.line] = a.expectedIndent
	}

	var resultLines []string
	for i, line := range f.Lines {
		lineNum := i + 1
		if expected, ok := adjMap[lineNum]; ok {
			trimmed := bytes.TrimLeft(line, " ")
			newLine := strings.Repeat(" ", expected) + string(trimmed)
			resultLines = append(resultLines, newLine)
		} else {
			resultLines = append(resultLines, string(line))
		}
	}

	return []byte(strings.Join(resultLines, "\n"))
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(settings map[string]any) error {
	for k, v := range settings {
		switch k {
		case "spaces":
			n, ok := toIntSetting(v)
			if !ok {
				return fmt.Errorf("list-indent: spaces must be an integer, got %T", v)
			}
			r.Spaces = n
		default:
			return fmt.Errorf("list-indent: unknown setting %q", k)
		}
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"spaces": 2,
	}
}

// toIntSetting converts a value to int.
func toIntSetting(v any) (int, bool) {
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
