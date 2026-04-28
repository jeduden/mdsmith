package listmarkerstyle

import (
	"fmt"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/yuin/goldmark/ast"
)

func init() {
	rule.Register(&Rule{})
}

// Rule checks that unordered lists use a consistent marker character.
type Rule struct {
	// Style is the required bullet character: "dash", "asterisk", or "plus".
	// An empty string disables the rule.
	Style string
	// Nested is an optional ordered list of marker names cycled by depth.
	// When non-empty, depth 0 uses Nested[0], depth 1 uses Nested[1], etc.
	// (cycling if depth >= len(Nested)).
	Nested []string
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS045" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "list-marker-style" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "list" }

// EnabledByDefault implements rule.Defaultable.
func (r *Rule) EnabledByDefault() bool { return false }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	if r.Style == "" {
		return nil
	}

	var diags []lint.Diagnostic

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		list, ok := n.(*ast.List)
		if !ok || list.IsOrdered() {
			return ast.WalkContinue, nil
		}

		depth := listDepth(list)
		expectedMarker := r.expectedMarker(depth)
		actualMarker := string([]byte{list.Marker})

		if actualMarker == expectedMarker {
			return ast.WalkContinue, nil
		}

		line := firstLineOfList(f, list)
		if line < 1 {
			return ast.WalkContinue, nil
		}

		var msg string
		if len(r.Nested) > 0 {
			msg = fmt.Sprintf(
				"unordered list at depth %d uses %s; expected %s",
				depth, actualMarker, expectedMarker,
			)
		} else {
			msg = fmt.Sprintf(
				"unordered list uses %s; configured style is %s",
				actualMarker, expectedMarker,
			)
		}

		diags = append(diags, lint.Diagnostic{
			File:     f.Path,
			Line:     line,
			Column:   1,
			RuleID:   r.ID(),
			RuleName: r.Name(),
			Severity: lint.Warning,
			Message:  msg,
		})

		return ast.WalkContinue, nil
	})

	return diags
}

// Fix implements rule.FixableRule.
func (r *Rule) Fix(f *lint.File) []byte {
	if r.Style == "" {
		return f.Source
	}

	// Build a map from byte offset -> replacement marker byte.
	replacements := make(map[int]byte)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		list, ok := n.(*ast.List)
		if !ok || list.IsOrdered() {
			return ast.WalkContinue, nil
		}

		depth := listDepth(list)
		expectedMarkerStr := r.expectedMarker(depth)
		expectedByte := markerByte(expectedMarkerStr)
		actualByte := list.Marker

		if actualByte == expectedByte {
			return ast.WalkContinue, nil
		}

		// Replace the marker byte at each list item's start.
		for item := list.FirstChild(); item != nil; item = item.NextSibling() {
			li, ok := item.(*ast.ListItem)
			if !ok {
				continue
			}
			offset := firstByteOfListItem(f, li)
			if offset < 0 {
				continue
			}
			replacements[offset] = expectedByte
		}

		return ast.WalkContinue, nil
	})

	if len(replacements) == 0 {
		result := make([]byte, len(f.Source))
		copy(result, f.Source)
		return result
	}

	result := make([]byte, len(f.Source))
	copy(result, f.Source)
	for offset, b := range replacements {
		result[offset] = b
	}
	return result
}

// listDepth returns the number of *ast.List ancestors of n.
// A top-level list returns 0.
func listDepth(n *ast.List) int {
	depth := 0
	for p := n.Parent(); p != nil; p = p.Parent() {
		if _, ok := p.(*ast.List); ok {
			depth++
		}
	}
	return depth
}

// expectedMarker returns the expected marker string for the given depth.
func (r *Rule) expectedMarker(depth int) string {
	if len(r.Nested) > 0 {
		return markerString(r.Nested[depth%len(r.Nested)])
	}
	return markerString(r.Style)
}

// markerString converts a marker name ("dash", "asterisk", "plus") to the
// corresponding single-character string.
func markerString(name string) string {
	switch name {
	case "dash":
		return "-"
	case "asterisk":
		return "*"
	case "plus":
		return "+"
	}
	return name
}

// markerByte converts a marker string ("-", "*", "+") to its byte value.
func markerByte(s string) byte {
	if len(s) > 0 {
		return s[0]
	}
	return '-'
}

// firstLineOfList returns the 1-based line number of the first item in list.
func firstLineOfList(f *lint.File, list *ast.List) int {
	for item := list.FirstChild(); item != nil; item = item.NextSibling() {
		li, ok := item.(*ast.ListItem)
		if !ok {
			continue
		}
		offset := firstByteOfListItem(f, li)
		if offset >= 0 {
			return f.LineOfOffset(offset)
		}
	}
	return 0
}

// firstByteOfListItem returns the byte offset of the marker character of the
// given list item, or -1 if not found.
func firstByteOfListItem(f *lint.File, li *ast.ListItem) int {
	// Try Lines() first (block content).
	if li.Lines().Len() > 0 {
		seg := li.Lines().At(0)
		// Walk backwards from seg.Start to find the marker byte on the same line.
		pos := seg.Start - 1
		// Skip whitespace between marker and content.
		for pos >= 0 && (f.Source[pos] == ' ' || f.Source[pos] == '\t') {
			pos--
		}
		// pos should now point at the marker byte.
		if pos >= 0 && isMarkerByte(f.Source[pos]) {
			return pos
		}
		// Fall back: find the start of the line and look for the marker.
		return findMarkerOnLine(f.Source, seg.Start)
	}

	// Try children.
	if li.HasChildren() {
		for c := li.FirstChild(); c != nil; c = c.NextSibling() {
			offset := firstByteOfChild(f, c)
			if offset >= 0 {
				return offset
			}
		}
	}
	return -1
}

// firstByteOfChild finds the first content byte of a child node, then
// finds the marker on that line.
func firstByteOfChild(f *lint.File, n ast.Node) int {
	switch t := n.(type) {
	case *ast.Text:
		return findMarkerOnLine(f.Source, t.Segment.Start)
	}
	if n.Lines().Len() > 0 {
		return findMarkerOnLine(f.Source, n.Lines().At(0).Start)
	}
	if n.HasChildren() {
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			if off := firstByteOfChild(f, c); off >= 0 {
				return off
			}
		}
	}
	return -1
}

// findMarkerOnLine scans backwards from pos to the start of the line,
// then searches forward for the first marker character.
func findMarkerOnLine(src []byte, pos int) int {
	// Find the start of the current line.
	lineStart := pos
	for lineStart > 0 && src[lineStart-1] != '\n' {
		lineStart--
	}
	// Scan forward from lineStart for a marker character.
	for i := lineStart; i < pos && i < len(src); i++ {
		if isMarkerByte(src[i]) {
			return i
		}
	}
	return -1
}

// isMarkerByte reports whether b is a list marker character.
func isMarkerByte(b byte) bool {
	return b == '-' || b == '*' || b == '+'
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(settings map[string]any) error {
	for k, v := range settings {
		switch k {
		case "style":
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("list-marker-style: style must be a string, got %T", v)
			}
			if s != "dash" && s != "asterisk" && s != "plus" {
				return fmt.Errorf(
					"list-marker-style: invalid style %q (valid: dash, asterisk, plus)", s,
				)
			}
			r.Style = s
		case "nested":
			list, ok := v.([]any)
			if !ok {
				return fmt.Errorf(
					"list-marker-style: nested must be a list of strings, got %T", v,
				)
			}
			markers := make([]string, 0, len(list))
			for i, item := range list {
				s, ok := item.(string)
				if !ok {
					return fmt.Errorf(
						"list-marker-style: nested[%d] must be a string, got %T", i, item,
					)
				}
				if s != "dash" && s != "asterisk" && s != "plus" {
					return fmt.Errorf(
						"list-marker-style: invalid nested marker %q (valid: dash, asterisk, plus)", s,
					)
				}
				markers = append(markers, s)
			}
			r.Nested = markers
		default:
			return fmt.Errorf("list-marker-style: unknown setting %q", k)
		}
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"style":  "",
		"nested": []string{},
	}
}

var _ rule.Configurable = (*Rule)(nil)
var _ rule.Defaultable = (*Rule)(nil)
