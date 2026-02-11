package headingstyle

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/jeduden/tidymark/internal/lint"
	"github.com/jeduden/tidymark/internal/rule"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

func init() {
	rule.Register(&Rule{Style: "atx"})
}

// Rule checks that all headings use a consistent style (atx or setext).
type Rule struct {
	Style string // "atx" or "setext"
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "TM002" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "heading-style" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "heading" }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	var diags []lint.Diagnostic
	style := r.Style
	if style == "" {
		style = "atx"
	}

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		heading, ok := n.(*ast.Heading)
		if !ok {
			return ast.WalkContinue, nil
		}

		isATX := isATXHeading(heading, f.Source)

		if style == "atx" && !isATX {
			line := headingLine(heading, f)
			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     line,
				Column:   1,
				RuleID:   r.ID(),
				RuleName: r.Name(),
				Severity: lint.Warning,
				Message:  "heading style should be atx",
			})
		} else if style == "setext" && isATX {
			// setext only supports levels 1 and 2; levels 3-6 must use atx
			if heading.Level <= 2 {
				line := headingLine(heading, f)
				diags = append(diags, lint.Diagnostic{
					File:     f.Path,
					Line:     line,
					Column:   1,
					RuleID:   r.ID(),
					RuleName: r.Name(),
					Severity: lint.Warning,
					Message:  "heading style should be setext",
				})
			}
		}

		return ast.WalkContinue, nil
	})

	return diags
}

// Fix implements rule.FixableRule.
func (r *Rule) Fix(f *lint.File) []byte {
	style := r.Style
	if style == "" {
		style = "atx"
	}

	result := make([]byte, len(f.Source))
	copy(result, f.Source)

	type replacement struct {
		start, end int
		newText    string
	}
	var replacements []replacement

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		heading, ok := n.(*ast.Heading)
		if !ok {
			return ast.WalkContinue, nil
		}

		isATX := isATXHeading(heading, f.Source)
		hText := headingText(heading, f.Source)

		if style == "atx" && !isATX {
			// Convert setext to atx
			start, end := headingByteRange(heading, f.Source)
			prefix := strings.Repeat("#", heading.Level)
			replacements = append(replacements, replacement{
				start:   start,
				end:     end,
				newText: prefix + " " + hText,
			})
		} else if style == "setext" && isATX && heading.Level <= 2 {
			// Convert atx to setext
			start, end := headingByteRange(heading, f.Source)
			underChar := "="
			if heading.Level == 2 {
				underChar = "-"
			}
			underline := strings.Repeat(underChar, len(hText))
			if len(hText) == 0 {
				underline = strings.Repeat(underChar, 3)
			}
			replacements = append(replacements, replacement{
				start:   start,
				end:     end,
				newText: hText + "\n" + underline,
			})
		}

		return ast.WalkContinue, nil
	})

	// Apply replacements in reverse order to preserve offsets
	for i := len(replacements) - 1; i >= 0; i-- {
		rep := replacements[i]
		before := result[:rep.start]
		after := result[rep.end:]
		result = append(before, append([]byte(rep.newText), after...)...)
	}

	return result
}

// isATXHeading checks whether a heading uses ATX style (starts with #).
func isATXHeading(heading *ast.Heading, source []byte) bool {
	lines := heading.Lines()
	if lines.Len() == 0 {
		// For ATX headings in goldmark, the heading node has no Lines().
		// We need to find the actual source line via child text nodes.
		if heading.FirstChild() != nil {
			var seg text.Segment
			switch c := heading.FirstChild().(type) {
			case *ast.Text:
				seg = c.Segment
			default:
				// Walk to find a text node
				_ = ast.Walk(heading, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
					if entering {
						if t, ok := n.(*ast.Text); ok {
							seg = t.Segment
							return ast.WalkStop, nil
						}
					}
					return ast.WalkContinue, nil
				})
			}
			if seg.Start > 0 || seg.Stop > 0 {
				// Find the beginning of this line
				lineStart := seg.Start
				for lineStart > 0 && source[lineStart-1] != '\n' {
					lineStart--
				}
				if lineStart < len(source) && source[lineStart] == '#' {
					return true
				}
			}
			return true // default to atx if we can't determine
		}
		return true // no lines, no children - assume atx
	}

	// If Lines() > 0, it could be setext. Check if the source line starts with #.
	seg := lines.At(0)
	lineStart := seg.Start
	for lineStart > 0 && source[lineStart-1] != '\n' {
		lineStart--
	}
	if lineStart < len(source) && source[lineStart] == '#' {
		return true
	}
	return false
}

func headingLine(heading *ast.Heading, f *lint.File) int {
	lines := heading.Lines()
	if lines.Len() > 0 {
		return f.LineOfOffset(lines.At(0).Start)
	}
	// For ATX headings, find the line via child text nodes
	for c := heading.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			return f.LineOfOffset(t.Segment.Start)
		}
		// Check inline children
		var found int
		_ = ast.Walk(c, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
			if entering {
				if t, ok := n.(*ast.Text); ok {
					found = f.LineOfOffset(t.Segment.Start)
					return ast.WalkStop, nil
				}
			}
			return ast.WalkContinue, nil
		})
		if found > 0 {
			return found
		}
	}
	return 1
}

func headingByteRange(heading *ast.Heading, source []byte) (int, int) {
	// Find the start of the heading in source
	lines := heading.Lines()
	var start int

	if lines.Len() > 0 {
		start = lines.At(0).Start
	} else {
		// ATX heading - find via children
		for c := heading.FirstChild(); c != nil; c = c.NextSibling() {
			if t, ok := c.(*ast.Text); ok {
				start = t.Segment.Start
				break
			}
		}
	}

	// Go to the start of the line
	for start > 0 && source[start-1] != '\n' {
		start--
	}

	isATX := isATXHeading(heading, source)

	if isATX {
		// ATX heading is a single line
		end := start
		for end < len(source) && source[end] != '\n' {
			end++
		}
		return start, end
	}

	// Setext heading spans multiple lines (text + underline)
	// Find end of text line
	endText := start
	for endText < len(source) && source[endText] != '\n' {
		endText++
	}
	// Skip past newline to underline
	endUnderline := endText + 1
	for endUnderline < len(source) && source[endUnderline] != '\n' {
		endUnderline++
	}
	return start, endUnderline
}

func headingText(heading *ast.Heading, source []byte) string {
	var buf bytes.Buffer
	for c := heading.FirstChild(); c != nil; c = c.NextSibling() {
		extractText(c, source, &buf)
	}
	return buf.String()
}

func extractText(n ast.Node, source []byte, buf *bytes.Buffer) {
	if t, ok := n.(*ast.Text); ok {
		buf.Write(t.Segment.Value(source))
		return
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		extractText(c, source, buf)
	}
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(settings map[string]any) error {
	for k, v := range settings {
		switch k {
		case "style":
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("heading-style: style must be a string, got %T", v)
			}
			if s != "atx" && s != "setext" {
				return fmt.Errorf("heading-style: invalid style %q (valid: atx, setext)", s)
			}
			r.Style = s
		default:
			return fmt.Errorf("heading-style: unknown setting %q", k)
		}
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"style": "atx",
	}
}

var _ rule.FixableRule = (*Rule)(nil)
var _ rule.Configurable = (*Rule)(nil)
