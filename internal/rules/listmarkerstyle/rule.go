// Package listmarkerstyle implements MDS045, which pins the bullet
// character for unordered lists. CommonMark accepts `-`, `*`, and `+`
// interchangeably; this rule requires a single marker (or a rotation by
// depth) to reduce diff noise and aid visual scanning.
package listmarkerstyle

import (
	"fmt"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/yuin/goldmark/ast"
)

// Style values for the rule's `style` setting.
const (
	StyleDash     = "dash"
	StyleAsterisk = "asterisk"
	StylePlus     = "plus"
)

func init() {
	rule.Register(&Rule{Style: StyleDash})
}

// Rule pins the marker character for unordered lists.
type Rule struct {
	Style  string
	Nested []string
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS045" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "list-marker-style" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "list" }

// EnabledByDefault implements rule.Defaultable. The rule is opt-in:
// users pick a project convention and turn the rule on.
func (r *Rule) EnabledByDefault() bool { return false }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	var diags []lint.Diagnostic

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		list, ok := n.(*ast.List)
		if !ok || list.IsOrdered() {
			return ast.WalkContinue, nil
		}
		if d, ok := r.checkList(f, list); ok {
			diags = append(diags, d)
		}
		return ast.WalkContinue, nil
	})

	return diags
}

// checkList emits a diagnostic when the list's marker does not match
// the expected marker for its depth. Returns the diagnostic and true
// if one was produced, false otherwise.
func (r *Rule) checkList(f *lint.File, list *ast.List) (lint.Diagnostic, bool) {
	depth := r.computeDepth(list)
	expected := r.expectedMarker(depth)
	actual := list.Marker

	if actual == expected {
		return lint.Diagnostic{}, false
	}

	// Emit diagnostic at the first list item's line
	firstLine := 0
	if list.FirstChild() != nil {
		item := list.FirstChild().(*ast.ListItem)
		firstLine = r.firstLineOfListItem(f, item)
	}

	msg := r.formatMessage(actual, expected, depth)
	return lint.Diagnostic{
		File:     f.Path,
		Line:     firstLine,
		Column:   1,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Warning,
		Message:  msg,
	}, true
}

// computeDepth counts the number of *ast.List ancestors of the given node.
func (r *Rule) computeDepth(n ast.Node) int {
	depth := 0
	for p := n.Parent(); p != nil; p = p.Parent() {
		if _, ok := p.(*ast.List); ok {
			depth++
		}
	}
	return depth
}

// expectedMarker returns the marker byte that should be used at the
// given depth according to the rule's configuration.
func (r *Rule) expectedMarker(depth int) byte {
	if len(r.Nested) == 0 {
		return styleToMarker(r.Style)
	}
	idx := depth % len(r.Nested)
	return styleToMarker(r.Nested[idx])
}

// styleToMarker converts a style string to its marker byte.
func styleToMarker(style string) byte {
	switch style {
	case StyleDash:
		return '-'
	case StyleAsterisk:
		return '*'
	case StylePlus:
		return '+'
	default:
		return '-'
	}
}

// markerToStyle converts a marker byte to its style string.
func markerToStyle(marker byte) string {
	switch marker {
	case '-':
		return StyleDash
	case '*':
		return StyleAsterisk
	case '+':
		return StylePlus
	default:
		return "unknown"
	}
}

// formatMessage creates the diagnostic message.
func (r *Rule) formatMessage(actual, expected byte, depth int) string {
	if len(r.Nested) > 0 {
		return fmt.Sprintf(
			"unordered list at depth %d uses %s; expected %s",
			depth, markerToStyle(actual), markerToStyle(expected),
		)
	}
	return fmt.Sprintf(
		"unordered list uses %s; configured style is %s",
		markerToStyle(actual), markerToStyle(expected),
	)
}

// firstLineOfListItem returns the 1-based source line of an item's
// marker. When the ListItem carries line segments, the first segment's
// start offset gives the marker line directly. Otherwise the marker
// line is derived from the first block child.
func (r *Rule) firstLineOfListItem(f *lint.File, li *ast.ListItem) int {
	if li.Lines().Len() > 0 {
		seg := li.Lines().At(0)
		return f.LineOfOffset(seg.Start)
	}
	for c := li.FirstChild(); c != nil; c = c.NextSibling() {
		if line := blockFirstLine(f, c); line > 0 {
			return line
		}
	}
	return 0
}

// blockFirstLine returns the first source line of a block node.
// Recurses only through container blocks (whose Lines() is empty).
func blockFirstLine(f *lint.File, n ast.Node) int {
	if n.Lines().Len() > 0 {
		return f.LineOfOffset(n.Lines().At(0).Start)
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if l := blockFirstLine(f, c); l > 0 {
			return l
		}
	}
	return 0
}

// Fix implements rule.FixableRule.
func (r *Rule) Fix(f *lint.File) []byte {
	// Map of line number to new marker byte
	markerEdits := map[int]byte{}

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		list, ok := n.(*ast.List)
		if !ok || list.IsOrdered() {
			return ast.WalkContinue, nil
		}
		r.collectListEdits(f, list, markerEdits)
		return ast.WalkContinue, nil
	})

	if len(markerEdits) == 0 {
		out := make([]byte, len(f.Source))
		copy(out, f.Source)
		return out
	}

	// Apply edits line by line
	resultLines := make([][]byte, len(f.Lines))
	for i, line := range f.Lines {
		lineNum := i + 1
		newLine := append([]byte(nil), line...)
		if newMarker, ok := markerEdits[lineNum]; ok {
			newLine = replaceMarker(newLine, newMarker)
		}
		resultLines[i] = newLine
	}

	return joinLines(resultLines)
}

// collectListEdits records marker replacements for all items in a list.
func (r *Rule) collectListEdits(f *lint.File, list *ast.List, markerEdits map[int]byte) {
	depth := r.computeDepth(list)
	expected := r.expectedMarker(depth)
	actual := list.Marker

	if actual == expected {
		return
	}

	// Collect edits for each list item
	for c := list.FirstChild(); c != nil; c = c.NextSibling() {
		item := c.(*ast.ListItem)
		line := r.firstLineOfListItem(f, item)
		if line > 0 {
			markerEdits[line] = expected
		}
	}
}

// replaceMarker replaces the list marker character in a line.
// The marker is the first non-space character that is -, *, or +.
func replaceMarker(line []byte, newMarker byte) []byte {
	for i := 0; i < len(line); i++ {
		if line[i] == ' ' || line[i] == '\t' {
			continue
		}
		if line[i] == '-' || line[i] == '*' || line[i] == '+' {
			newLine := make([]byte, len(line))
			copy(newLine, line)
			newLine[i] = newMarker
			return newLine
		}
		break
	}
	return line
}

// joinLines joins lines with newline separators.
func joinLines(lines [][]byte) []byte {
	if len(lines) == 0 {
		return []byte{}
	}
	totalLen := 0
	for _, line := range lines {
		totalLen += len(line) + 1 // +1 for newline
	}
	totalLen-- // last line doesn't need newline at end

	result := make([]byte, 0, totalLen)
	for i, line := range lines {
		result = append(result, line...)
		if i < len(lines)-1 {
			result = append(result, '\n')
		}
	}
	return result
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(s map[string]any) error {
	for k, v := range s {
		switch k {
		case "style":
			str, ok := v.(string)
			if !ok {
				return fmt.Errorf("list-marker-style: style must be a string, got %T", v)
			}
			if !isValidStyle(str) {
				return fmt.Errorf("list-marker-style: invalid style %q (valid: dash, asterisk, plus)", str)
			}
			r.Style = str
		case "nested":
			slice, ok := v.([]any)
			if !ok {
				return fmt.Errorf("list-marker-style: nested must be a list, got %T", v)
			}
			nested := make([]string, len(slice))
			for i, item := range slice {
				str, ok := item.(string)
				if !ok {
					return fmt.Errorf("list-marker-style: nested[%d] must be a string, got %T", i, item)
				}
				if !isValidStyle(str) {
					return fmt.Errorf("list-marker-style: invalid nested[%d] style %q", i, str)
				}
				nested[i] = str
			}
			r.Nested = nested
		default:
			return fmt.Errorf("list-marker-style: unknown setting %q", k)
		}
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"style":  StyleDash,
		"nested": []string{},
	}
}

func isValidStyle(s string) bool {
	return s == StyleDash || s == StyleAsterisk || s == StylePlus
}

var (
	_ rule.Configurable = (*Rule)(nil)
	_ rule.FixableRule  = (*Rule)(nil)
	_ rule.Defaultable  = (*Rule)(nil)
)
