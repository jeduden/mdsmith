package notrailingpunctuation

import (
	"fmt"
	"strings"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/astutil"
	"github.com/yuin/goldmark/ast"
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
// rule.WalkNodes.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	return rule.WalkNodes(r, f)
}

// CheckNode implements rule.NodeChecker.
func (r *Rule) CheckNode(n ast.Node, entering bool, f *lint.File) []lint.Diagnostic {
	if !entering {
		return nil
	}
	heading, ok := n.(*ast.Heading)
	if !ok {
		return nil
	}

	text := astutil.HeadingText(heading, f.Source)
	text = strings.TrimSpace(text)
	if len(text) == 0 {
		return nil
	}
	if text == "..." {
		// Reserved wildcard marker for required-structure prototypes.
		return nil
	}

	lastChar := text[len(text)-1]
	if strings.ContainsRune(flaggedPunctuation, rune(lastChar)) {
		line := astutil.HeadingLine(heading, f)
		return []lint.Diagnostic{{
			File:     f.Path,
			Line:     line,
			Column:   1,
			RuleID:   r.ID(),
			RuleName: r.Name(),
			Severity: lint.Warning,
			Message:  fmt.Sprintf("heading should not end with punctuation %q", string(lastChar)),
		}}
	}
	return nil
}

var _ rule.NodeChecker = (*Rule)(nil)
