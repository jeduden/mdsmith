package unclosedcodeblock

import (
	"bytes"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/fencepos"
	"github.com/yuin/goldmark/ast"
)

func init() {
	rule.Register(&Rule{})
}

// Rule detects fenced code blocks that lack a closing fence delimiter.
type Rule struct{}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS031" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "unclosed-code-block" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "code" }

// Check implements rule.Rule. The per-block logic is pure and
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
	fcb, ok := n.(*ast.FencedCodeBlock)
	if !ok {
		return nil
	}
	if hasClosingFence(f, fcb) {
		return nil
	}
	openLine := fencepos.OpenLine(f, fcb)
	return []lint.Diagnostic{{
		File:     f.Path,
		Line:     openLine,
		Column:   1,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Error,
		Message:  "unclosed fenced code block",
	}}
}

var _ rule.NodeChecker = (*Rule)(nil)

// hasClosingFence checks whether a fenced code block has a proper closing
// fence line after its content.
func hasClosingFence(f *lint.File, fcb *ast.FencedCodeBlock) bool {
	openStart, openEnd := fencepos.OpenLineRange(f.Source, fcb)
	if openStart >= len(f.Source) {
		return true
	}

	fenceChar := fencepos.CharAt(f.Source, openStart)
	if fenceChar == 0 {
		return true
	}

	closeStart, closeEnd := fencepos.CloseLineRange(f.Source, fcb, openEnd)

	// No closing line exists (at or past EOF).
	if closeStart >= len(f.Source) {
		return false
	}

	// Require a non-empty closing line; the fence characters are validated below.
	if closeStart == closeEnd {
		return false
	}
	closingLine := bytes.TrimLeft(f.Source[closeStart:closeEnd], " ")
	minFence := []byte{fenceChar, fenceChar, fenceChar}
	return bytes.HasPrefix(closingLine, minFence)
}
