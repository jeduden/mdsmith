package blanklinearoundheadings

import (
	"strings"

	"github.com/jeduden/tidymark/internal/lint"
	"github.com/jeduden/tidymark/internal/rule"
	"github.com/yuin/goldmark/ast"
)

func init() {
	rule.Register(&Rule{})
}

// Rule checks that headings have blank lines before and after them.
type Rule struct{}

func (r *Rule) ID() string   { return "TM013" }
func (r *Rule) Name() string { return "blank-line-around-headings" }

func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	var diags []lint.Diagnostic

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		heading, ok := n.(*ast.Heading)
		if !ok {
			return ast.WalkContinue, nil
		}

		line := headingLine(heading, f)
		lastLine := headingLastLine(heading, f)

		// Check blank line before (not needed for line 1)
		if line > 1 {
			prevLineIdx := line - 2 // 0-based index
			if prevLineIdx >= 0 && prevLineIdx < len(f.Lines) {
				prevLine := strings.TrimSpace(string(f.Lines[prevLineIdx]))
				if prevLine != "" {
					diags = append(diags, lint.Diagnostic{
						File:     f.Path,
						Line:     line,
						Column:   1,
						RuleID:   r.ID(),
						RuleName: r.Name(),
						Severity: lint.Warning,
						Message:  "heading should have a blank line before",
					})
				}
			}
		}

		// Check blank line after (not needed for last line)
		nextLineIdx := lastLine // 0-based index of line after heading
		if nextLineIdx < len(f.Lines) {
			nextLine := strings.TrimSpace(string(f.Lines[nextLineIdx]))
			if nextLine != "" {
				diags = append(diags, lint.Diagnostic{
					File:     f.Path,
					Line:     line,
					Column:   1,
					RuleID:   r.ID(),
					RuleName: r.Name(),
					Severity: lint.Warning,
					Message:  "heading should have a blank line after",
				})
			}
		}

		return ast.WalkContinue, nil
	})

	return diags
}

func (r *Rule) Fix(f *lint.File) []byte {
	type headingInfo struct {
		line     int // 1-based
		lastLine int // 1-based
	}
	var headings []headingInfo

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		heading, ok := n.(*ast.Heading)
		if !ok {
			return ast.WalkContinue, nil
		}
		line := headingLine(heading, f)
		lastLine := headingLastLine(heading, f)
		headings = append(headings, headingInfo{line: line, lastLine: lastLine})
		return ast.WalkContinue, nil
	})

	if len(headings) == 0 {
		return f.Source
	}

	lines := make([]string, len(f.Lines))
	for i, l := range f.Lines {
		lines[i] = string(l)
	}

	// Track which lines need blank lines inserted before/after
	// We'll work with a set of "insert blank before line N" instructions
	insertBefore := make(map[int]bool) // 1-based line numbers
	insertAfter := make(map[int]bool)  // 1-based line numbers

	for _, h := range headings {
		// Check blank line before
		if h.line > 1 {
			prevLineIdx := h.line - 2
			if prevLineIdx >= 0 && prevLineIdx < len(lines) {
				if strings.TrimSpace(lines[prevLineIdx]) != "" {
					insertBefore[h.line] = true
				}
			}
		}
		// Check blank line after
		nextLineIdx := h.lastLine
		if nextLineIdx < len(lines) {
			if strings.TrimSpace(lines[nextLineIdx]) != "" {
				insertAfter[h.lastLine] = true
			}
		}
	}

	var result []string
	for i, line := range lines {
		lineNum := i + 1
		if insertBefore[lineNum] {
			result = append(result, "")
		}
		result = append(result, line)
		if insertAfter[lineNum] {
			result = append(result, "")
		}
	}

	return []byte(strings.Join(result, "\n"))
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

func headingLastLine(heading *ast.Heading, f *lint.File) int {
	lines := heading.Lines()
	if lines.Len() > 0 {
		// Setext headings: the underline is on the line after the text
		lastSeg := lines.At(lines.Len() - 1)
		textLine := f.LineOfOffset(lastSeg.Start)
		// Check if next line is an underline (setext)
		if isSetextHeading(heading, f.Source) {
			return textLine + 1
		}
		return textLine
	}
	// ATX heading is a single line
	return headingLine(heading, f)
}

func isSetextHeading(heading *ast.Heading, source []byte) bool {
	lines := heading.Lines()
	if lines.Len() == 0 {
		return false
	}
	seg := lines.At(0)
	lineStart := seg.Start
	for lineStart > 0 && source[lineStart-1] != '\n' {
		lineStart--
	}
	if lineStart < len(source) && source[lineStart] == '#' {
		return false
	}
	return true
}

var _ rule.FixableRule = (*Rule)(nil)
