package notrailingpunctuation

import (
	"fmt"
	"strings"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/astutil"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
)

func init() {
	rule.Register(&Rule{})
}

// Rule checks that heading text does not end with trailing punctuation.
type Rule struct{}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS017" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "no-trailing-punctuation-in-heading" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "heading" }

// flaggedPunctuation contains the punctuation characters that are not allowed
// at the end of a heading.
const flaggedPunctuation = ".,;:!"

// Check implements rule.Rule. The per-heading logic is pure and
// stateless, so it is expressed as CheckNode and the engine can fold
// this rule into one shared AST walk; a direct call still works via
// rule.WalkNodes. On the parse-skipped path (f.AST nil) the AST walk
// surfaces no headings, so the same per-heading verdict runs over the
// headings of the shared run-grouped inline parse (lint.InlineBlocks),
// each heading's run-local segment offsets mapped back to the document so
// the flagged text and line stay byte-identical to the AST walk.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	if f != nil && f.AST == nil {
		var diags []lint.Diagnostic
		lint.WalkInlineNodes(f, func(n ast.Node, base int) {
			heading, ok := n.(*ast.Heading)
			if !ok {
				return
			}
			text := astutil.HeadingTextBase(heading, f.Source, base)
			if d, ok := r.verdict(f, text, astutil.HeadingLineBase(heading, f, base)); ok {
				diags = append(diags, d)
			}
		})
		return diags
	}
	return rule.WalkNodes(r, f)
}

// InlineCapable implements rule.InlineChecker: Check serves the nil-AST path
// from lint.WalkInlineNodes (which reads lint.InlineBlocks).
func (r *Rule) InlineCapable() bool { return true }

var _ rule.InlineChecker = (*Rule)(nil)

// CheckNode implements rule.NodeChecker.
func (r *Rule) CheckNode(n ast.Node, entering bool, f *lint.File) []lint.Diagnostic {
	if !entering {
		return nil
	}
	heading, ok := n.(*ast.Heading)
	if !ok {
		return nil
	}
	if d, ok := r.verdict(f, astutil.HeadingText(heading, f.Source), astutil.HeadingLine(heading, f)); ok {
		return []lint.Diagnostic{d}
	}
	return nil
}

// verdict applies the trailing-punctuation check to one heading. text is the
// heading's flattened content (untrimmed; verdict trims it) and line its
// 1-based source line. Both the AST path (CheckNode) and the nil-AST path
// (Check) drive it, so the diagnostic is byte-identical.
func (r *Rule) verdict(f *lint.File, text string, line int) (lint.Diagnostic, bool) {
	text = strings.TrimSpace(text)
	if len(text) == 0 {
		return lint.Diagnostic{}, false
	}
	if text == "..." {
		// Reserved wildcard marker for required-structure prototypes.
		return lint.Diagnostic{}, false
	}

	lastChar := text[len(text)-1]
	if !strings.ContainsRune(flaggedPunctuation, rune(lastChar)) {
		return lint.Diagnostic{}, false
	}
	return lint.Diagnostic{
		File:     f.Path,
		Line:     line,
		Column:   1,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Warning,
		Message:  fmt.Sprintf("heading should not end with punctuation %q", string(lastChar)),
	}, true
}

var _ rule.NodeChecker = (*Rule)(nil)

// enteringKinds is the static node-kind interest CheckNode declares
// via rule.KindScopedChecker; package-level so EnteringKinds returns
// it without allocating.
var enteringKinds = []ast.NodeKind{ast.KindHeading}

// EnteringKinds implements rule.KindScopedChecker: CheckNode only
// reacts to these node kinds, entering visits only.
func (r *Rule) EnteringKinds() []ast.NodeKind { return enteringKinds }

var _ rule.KindScopedChecker = (*Rule)(nil)
