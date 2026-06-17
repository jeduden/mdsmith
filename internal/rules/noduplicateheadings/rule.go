package noduplicateheadings

import (
	"strconv"
	"strings"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/astutil"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
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

// Check implements rule.Rule. On the parse-skipped path (f.AST nil) the
// AST walk surfaces no headings, so the same first-seen-wins duplicate
// scan runs over the headings of the shared run-grouped inline parse
// (lint.InlineBlocks), with each heading's run-local segment offsets
// mapped back to the document. WalkInlineNodes visits the runs in
// document order, so the first-occurrence line each duplicate names is
// byte-identical to the AST walk's.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	if f != nil && f.AST == nil {
		return r.checkFromInline(f)
	}

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

		text := astutil.HeadingText(heading, f.Source)
		line := astutil.HeadingLine(heading, f)
		if d, ok := r.verdict(f, text, line, seen); ok {
			diags = append(diags, d)
		}

		return ast.WalkContinue, nil
	})

	return diags
}

// checkFromInline runs the duplicate scan over the re-parsed inline runs
// for the nil-AST path. base maps each heading's run-local segment offsets
// back to the document so the heading text and line match the AST walk.
func (r *Rule) checkFromInline(f *lint.File) []lint.Diagnostic {
	var diags []lint.Diagnostic
	seen := make(map[string]int) // text -> first occurrence line
	lint.WalkInlineNodes(f, func(n ast.Node, base int) {
		heading, ok := n.(*ast.Heading)
		if !ok {
			return
		}
		text := astutil.HeadingTextBase(heading, f.Source, base)
		line := astutil.HeadingLineBase(heading, f, base)
		if d, ok := r.verdict(f, text, line, seen); ok {
			diags = append(diags, d)
		}
	})
	return diags
}

// verdict applies the first-seen-wins duplicate check to one heading. text
// is the heading's flattened content, line its 1-based source line, and
// seen the running first-occurrence map (mutated to record a new text). It
// returns the diagnostic for a repeat heading, or ok == false when the
// heading is the first of its text or the reserved `...` wildcard.
func (r *Rule) verdict(f *lint.File, text string, line int, seen map[string]int) (lint.Diagnostic, bool) {
	if strings.TrimSpace(text) == "..." {
		// Reserved wildcard marker for required-structure prototypes.
		return lint.Diagnostic{}, false
	}
	if firstLine, exists := seen[text]; exists {
		return lint.Diagnostic{
			File:     f.Path,
			Line:     line,
			Column:   1,
			RuleID:   r.ID(),
			RuleName: r.Name(),
			Severity: lint.Warning,
			Message: "duplicate heading " + strconv.Quote(text) +
				" (first defined on line " + strconv.Itoa(firstLine) + ")",
		}, true
	}
	seen[text] = line
	return lint.Diagnostic{}, false
}
