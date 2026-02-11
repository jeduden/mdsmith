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

// Category implements rule.Rule.
func (r *Rule) Category() string { return "list" }

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

		if _, isListItem := list.Parent().(*ast.ListItem); isListItem {
			return ast.WalkContinue, nil
		}

		listStartLine := lineOfNode(f, list)
		listEndLine := lastLineOfNode(f, list)

		if codeLines[listStartLine] || codeLines[listEndLine] {
			return ast.WalkContinue, nil
		}

		if d, ok := r.checkAdjacentBlank(f, listStartLine, -1, "list should be preceded by a blank line"); ok {
			diags = append(diags, d)
		}
		if d, ok := r.checkAdjacentBlank(f, listEndLine, +1, "list should be followed by a blank line"); ok {
			diags = append(diags, d)
		}

		return ast.WalkContinue, nil
	})

	return diags
}

// checkAdjacentBlank checks whether the line adjacent to targetLine (offset -1 for before,
// +1 for after) is non-blank and returns a diagnostic if so.
func (r *Rule) checkAdjacentBlank(f *lint.File, targetLine, direction int, msg string) (lint.Diagnostic, bool) {
	totalLines := len(f.Lines)
	adjLine := targetLine + direction
	if adjLine < 1 || adjLine > totalLines {
		return lint.Diagnostic{}, false
	}
	if isBlank(f.Lines[adjLine-1]) {
		return lint.Diagnostic{}, false
	}
	return lint.Diagnostic{
		File:     f.Path,
		Line:     targetLine,
		Column:   1,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Warning,
		Message:  msg,
	}, true
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
	beforeSet, afterSet := r.collectBlankLineInsertions(f)

	if len(beforeSet) == 0 && len(afterSet) == 0 {
		result := make([]byte, len(f.Source))
		copy(result, f.Source)
		return result
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

// collectBlankLineInsertions walks the AST and returns sets of 1-based line numbers
// that need a blank line inserted before or after them.
func (r *Rule) collectBlankLineInsertions(f *lint.File) (beforeSet, afterSet map[int]bool) {
	beforeSet = make(map[int]bool)
	afterSet = make(map[int]bool)
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

		if codeLines[listStartLine] || codeLines[listEndLine] {
			return ast.WalkContinue, nil
		}

		if needsBlankAdjacent(f, listStartLine, -1) {
			beforeSet[listStartLine] = true
		}
		if needsBlankAdjacent(f, listEndLine, +1) {
			afterSet[listEndLine] = true
		}

		return ast.WalkContinue, nil
	})

	return beforeSet, afterSet
}

// needsBlankAdjacent returns true if the line adjacent to targetLine
// (direction -1 for before, +1 for after) exists and is non-blank.
func needsBlankAdjacent(f *lint.File, targetLine, direction int) bool {
	adjLine := targetLine + direction
	if adjLine < 1 || adjLine > len(f.Lines) {
		return false
	}
	return !isBlank(f.Lines[adjLine-1])
}
