package blanklinearoundlists

import (
	"bytes"

	"github.com/jeduden/tidymark/internal/lint"
	"github.com/jeduden/tidymark/internal/rule"
	"github.com/yuin/goldmark/ast"
)

func init() {
	rule.Register(&Rule{})
}

// Rule checks that lists have blank lines before and after them.
type Rule struct{}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "TM014" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "blank-line-around-lists" }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	var diags []lint.Diagnostic
	codeLines := lint.CollectCodeBlockLines(f)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		list, ok := n.(*ast.List)
		if !ok {
			return ast.WalkContinue, nil
		}

		// Skip nested lists: if parent is a ListItem, this is an inner list.
		if _, isListItem := list.Parent().(*ast.ListItem); isListItem {
			return ast.WalkContinue, nil
		}

		// Get the line of the first line of the list.
		listStartLine := lineOfNode(f, list)
		listEndLine := lastLineOfNode(f, list)

		// Skip lists whose lines overlap with code block regions.
		if codeLines[listStartLine] || codeLines[listEndLine] {
			return ast.WalkContinue, nil
		}
		totalLines := len(f.Lines)

		// Check blank line before (unless list starts at line 1).
		if listStartLine > 1 {
			prevLine := listStartLine - 1 // 1-based
			if prevLine >= 1 && prevLine <= totalLines {
				if !isBlank(f.Lines[prevLine-1]) {
					diags = append(diags, lint.Diagnostic{
						File:     f.Path,
						Line:     listStartLine,
						Column:   1,
						RuleID:   r.ID(),
						RuleName: r.Name(),
						Severity: lint.Warning,
						Message:  "list should be preceded by a blank line",
					})
				}
			}
		}

		// Check blank line after (unless list ends at last line).
		if listEndLine < totalLines {
			nextLine := listEndLine + 1 // 1-based
			if nextLine >= 1 && nextLine <= totalLines {
				if !isBlank(f.Lines[nextLine-1]) {
					diags = append(diags, lint.Diagnostic{
						File:     f.Path,
						Line:     listEndLine,
						Column:   1,
						RuleID:   r.ID(),
						RuleName: r.Name(),
						Severity: lint.Warning,
						Message:  "list should be followed by a blank line",
					})
				}
			}
		}

		return ast.WalkContinue, nil
	})

	return diags
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

func lineOfNode(f *lint.File, n ast.Node) int {
	// Inline nodes do not have Lines(); use Segment for Text nodes.
	if t, ok := n.(*ast.Text); ok {
		return f.LineOfOffset(t.Segment.Start)
	}
	if isInlineNode(n) {
		// For other inline nodes, recurse into children.
		if n.HasChildren() {
			for c := n.FirstChild(); c != nil; c = c.NextSibling() {
				line := lineOfNode(f, c)
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
	// For container nodes, find the first child with lines.
	if n.HasChildren() {
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			line := lineOfNode(f, c)
			if line > 0 {
				return line
			}
		}
	}
	return 0
}

func lastLineOfNode(f *lint.File, n ast.Node) int {
	// Inline nodes do not have Lines(); use Segment for Text nodes.
	if t, ok := n.(*ast.Text); ok {
		return f.LineOfOffset(t.Segment.Stop - 1)
	}
	if isInlineNode(n) {
		if n.HasChildren() {
			for c := n.LastChild(); c != nil; c = c.PreviousSibling() {
				line := lastLineOfNode(f, c)
				if line > 0 {
					return line
				}
			}
		}
		return 0
	}
	if n.Lines().Len() > 0 {
		seg := n.Lines().At(n.Lines().Len() - 1)
		return f.LineOfOffset(seg.Start)
	}
	// For container nodes, find the last child with lines.
	if n.HasChildren() {
		for c := n.LastChild(); c != nil; c = c.PreviousSibling() {
			line := lastLineOfNode(f, c)
			if line > 0 {
				return line
			}
		}
	}
	return 0
}

func isBlank(line []byte) bool {
	return len(bytes.TrimSpace(line)) == 0
}

// Fix implements rule.FixableRule.
func (r *Rule) Fix(f *lint.File) []byte {
	var insertBefore []int
	var insertAfter []int
	codeLines := lint.CollectCodeBlockLines(f)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		list, ok := n.(*ast.List)
		if !ok {
			return ast.WalkContinue, nil
		}

		if _, isListItem := list.Parent().(*ast.ListItem); isListItem {
			return ast.WalkContinue, nil
		}

		listStartLine := lineOfNode(f, list)
		listEndLine := lastLineOfNode(f, list)

		// Skip lists whose lines overlap with code block regions.
		if codeLines[listStartLine] || codeLines[listEndLine] {
			return ast.WalkContinue, nil
		}
		totalLines := len(f.Lines)

		if listStartLine > 1 {
			prevLine := listStartLine - 1
			if prevLine >= 1 && prevLine <= totalLines {
				if !isBlank(f.Lines[prevLine-1]) {
					insertBefore = append(insertBefore, listStartLine)
				}
			}
		}

		if listEndLine < totalLines {
			nextLine := listEndLine + 1
			if nextLine >= 1 && nextLine <= totalLines {
				if !isBlank(f.Lines[nextLine-1]) {
					insertAfter = append(insertAfter, listEndLine)
				}
			}
		}

		return ast.WalkContinue, nil
	})

	if len(insertBefore) == 0 && len(insertAfter) == 0 {
		result := make([]byte, len(f.Source))
		copy(result, f.Source)
		return result
	}

	// Build a set of lines before/after which we need to insert blanks.
	beforeSet := make(map[int]bool)
	afterSet := make(map[int]bool)
	for _, l := range insertBefore {
		beforeSet[l] = true
	}
	for _, l := range insertAfter {
		afterSet[l] = true
	}

	var resultLines [][]byte
	for i, line := range f.Lines {
		lineNum := i + 1
		if beforeSet[lineNum] {
			resultLines = append(resultLines, []byte{})
		}
		resultLines = append(resultLines, line)
		if afterSet[lineNum] {
			resultLines = append(resultLines, []byte{})
		}
	}

	return bytes.Join(resultLines, []byte("\n"))
}
