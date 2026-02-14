package noduplicateheadings

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/yuin/goldmark/ast"
)

func init() {
	rule.Register(&Rule{})
}

// Rule checks that no two headings have the same text content.
type Rule struct{}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS005" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "no-duplicate-headings" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "heading" }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	var diags []lint.Diagnostic
	seen := make(map[string]int) // text -> first occurrence line

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		heading, ok := n.(*ast.Heading)
		if !ok {
			return ast.WalkContinue, nil
		}

		text := headingText(heading, f.Source)
		if strings.TrimSpace(text) == "..." {
			// Reserved wildcard marker for required-structure prototypes.
			return ast.WalkContinue, nil
		}
		line := headingLine(heading, f)

		if firstLine, exists := seen[text]; exists {
			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     line,
				Column:   1,
				RuleID:   r.ID(),
				RuleName: r.Name(),
				Severity: lint.Warning,
				Message:  fmt.Sprintf("duplicate heading %q (first defined on line %d)", text, firstLine),
			})
		} else {
			seen[text] = line
		}

		return ast.WalkContinue, nil
	})

	return diags
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
