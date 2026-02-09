package nobareurls

import (
	"regexp"

	"github.com/jeduden/tidymark/internal/lint"
	"github.com/jeduden/tidymark/internal/rule"
	"github.com/yuin/goldmark/ast"
)

func init() {
	rule.Register(&Rule{})
}

var urlPattern = regexp.MustCompile(`https?://[^\s)>\]]+`)

// Rule checks that bare URLs in text are flagged.
// URLs inside links, code blocks, code spans, autolinks, or reference
// definitions are not considered bare.
type Rule struct{}

func (r *Rule) ID() string   { return "TM012" }
func (r *Rule) Name() string { return "no-bare-urls" }

func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	var diags []lint.Diagnostic

	ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		textNode, ok := n.(*ast.Text)
		if !ok {
			return ast.WalkContinue, nil
		}

		// Skip text nodes inside links.
		if isInsideNonBareContext(n) {
			return ast.WalkContinue, nil
		}

		seg := textNode.Segment
		content := seg.Value(f.Source)
		matches := urlPattern.FindAllIndex(content, -1)
		for _, m := range matches {
			offset := seg.Start + m[0]
			line := f.LineOfOffset(offset)
			// Compute column: find the start of this line.
			lineStart := 0
			count := 1
			for i := 0; i < offset && i < len(f.Source); i++ {
				if f.Source[i] == '\n' {
					lineStart = i + 1
					count++
					_ = count
				}
			}
			col := offset - lineStart + 1

			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     line,
				Column:   col,
				RuleID:   r.ID(),
				RuleName: r.Name(),
				Severity: lint.Warning,
				Message:  "bare URL found; use angle brackets or a link",
			})
		}

		return ast.WalkContinue, nil
	})

	return diags
}

// isInsideNonBareContext checks if a node is a descendant of an ast.Link,
// ast.AutoLink, or ast.CodeSpan (where URLs should not be flagged).
func isInsideNonBareContext(n ast.Node) bool {
	for p := n.Parent(); p != nil; p = p.Parent() {
		switch p.(type) {
		case *ast.Link, *ast.AutoLink, *ast.CodeSpan:
			return true
		}
	}
	return false
}

func (r *Rule) Fix(f *lint.File) []byte {
	type replacement struct {
		start int
		end   int
		url   []byte
	}
	var replacements []replacement

	ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		textNode, ok := n.(*ast.Text)
		if !ok {
			return ast.WalkContinue, nil
		}

		if isInsideNonBareContext(n) {
			return ast.WalkContinue, nil
		}

		seg := textNode.Segment
		content := seg.Value(f.Source)
		matches := urlPattern.FindAllIndex(content, -1)
		for _, m := range matches {
			absStart := seg.Start + m[0]
			absEnd := seg.Start + m[1]
			replacements = append(replacements, replacement{
				start: absStart,
				end:   absEnd,
				url:   f.Source[absStart:absEnd],
			})
		}

		return ast.WalkContinue, nil
	})

	if len(replacements) == 0 {
		result := make([]byte, len(f.Source))
		copy(result, f.Source)
		return result
	}

	// Build result by applying replacements in order (they are already in
	// document order from the AST walk).
	var result []byte
	prev := 0
	for _, rep := range replacements {
		result = append(result, f.Source[prev:rep.start]...)
		result = append(result, '<')
		result = append(result, rep.url...)
		result = append(result, '>')
		prev = rep.end
	}
	result = append(result, f.Source[prev:]...)
	return result
}
