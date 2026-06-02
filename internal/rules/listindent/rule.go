package listindent

import (
	"bytes"
	"fmt"
	"strconv"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
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
func (r *Rule) ID() string { return "MDS016" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "list-indent" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "list" }

// Check implements rule.Rule. The per-list-item logic is pure and
// stateless, so it is expressed as CheckNode and the engine can fold
// this rule into one shared AST walk; a direct call still works via
// rule.WalkNodes.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	return rule.WalkNodes(r, f)
}

// CheckNode implements rule.NodeChecker.
func (r *Rule) CheckNode(n ast.Node, entering bool, f *lint.File) []lint.Diagnostic {
	if !entering {
		return nil
	}
	listItem, ok := n.(*ast.ListItem)
	if !ok {
		return nil
	}
	spaces := r.Spaces
	if spaces <= 0 {
		spaces = 2
	}
	level := nestingLevel(listItem)
	if level == 0 {
		return nil
	}
	expectedIndent := level * spaces
	line := firstLineOfListItem(f, listItem)
	if line < 1 || line > len(f.Lines) {
		return nil
	}
	actualIndent := countLeadingSpaces(f.Lines[line-1])
	if actualIndent == expectedIndent {
		return nil
	}
	return []lint.Diagnostic{{
		File:     f.Path,
		Line:     line,
		Column:   1,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Warning,
		Message: "list indent should be " + strconv.Itoa(expectedIndent) +
			" spaces, found " + strconv.Itoa(actualIndent),
	}}
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

// Fix implements rule.FixableRule.
func (r *Rule) Fix(f *lint.File) []byte {
	spaces := r.Spaces
	if spaces <= 0 {
		spaces = 2
	}

	adjMap := collectIndentAdjustments(f, spaces)

	if len(adjMap) == 0 {
		result := make([]byte, len(f.Source))
		copy(result, f.Source)
		return result
	}

	var out bytes.Buffer
	out.Grow(len(f.Source))
	for i, line := range f.Lines {
		if i > 0 {
			out.WriteByte('\n')
		}
		lineNum := i + 1
		if expected, ok := adjMap[lineNum]; ok {
			trimmed := bytes.TrimLeft(line, " ")
			for j := 0; j < expected; j++ {
				out.WriteByte(' ')
			}
			out.Write(trimmed)
		} else {
			out.Write(line)
		}
	}
	return out.Bytes()
}

// collectIndentAdjustments walks the AST and returns a map from 1-based line
// number to the expected indent for lines that need adjustment.
func collectIndentAdjustments(f *lint.File, spaces int) map[int]int {
	adjMap := make(map[int]int)

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

		if countLeadingSpaces(f.Lines[line-1]) != expectedIndent {
			adjMap[line] = expectedIndent
		}

		return ast.WalkContinue, nil
	})

	return adjMap
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
var _ rule.NodeChecker = (*Rule)(nil)
