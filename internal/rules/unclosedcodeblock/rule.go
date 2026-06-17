package unclosedcodeblock

import (
	"bytes"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/fencepos"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
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

// Check implements rule.Rule. Whether a fenced block is unclosed is a
// property of its delimiters, not the inline tree, so the rule is a
// rule.BlockChecker: on a parsed File it folds into the engine's shared
// AST walk (rule.WalkNodes); on a parse-skipped File (f.AST nil) it reads
// the Layer 0 block scan, whose fenced spans carry the same closure bit
// (rule.WalkBlocks).
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	if f != nil && f.AST == nil {
		return rule.WalkBlocks(r, f)
	}
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
	return r.diag(f, fencepos.OpenLine(f, fcb))
}

// CheckBlock implements rule.BlockChecker. The Layer 0 scanner sets
// span.Closed for every BlockFencedCode span — true when it found a
// matching closing fence, false when the fence ran to end of file — which
// is exactly the AST path's hasClosingFence verdict, and span.Start is the
// opening fence line (fencepos.OpenLine). So the diagnostic is identical.
func (r *Rule) CheckBlock(span lint.BlockSpan, f *lint.File) []lint.Diagnostic {
	if span.Closed {
		return nil
	}
	return r.diag(f, span.Start)
}

// blockKinds is the static block-kind interest CheckBlock declares via
// rule.BlockChecker; package-level so BlockKinds returns it without
// allocating.
var blockKinds = []lint.BlockKind{lint.BlockFencedCode}

// BlockKinds implements rule.BlockChecker.
func (r *Rule) BlockKinds() []lint.BlockKind { return blockKinds }

// diag builds the single-element unclosed-fence diagnostic both paths emit.
func (r *Rule) diag(f *lint.File, line int) []lint.Diagnostic {
	return []lint.Diagnostic{{
		File:     f.Path,
		Line:     line,
		Column:   1,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Error,
		Message:  "unclosed fenced code block",
	}}
}

var (
	_ rule.NodeChecker  = (*Rule)(nil)
	_ rule.BlockChecker = (*Rule)(nil)
)

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

// enteringKinds is the static node-kind interest CheckNode declares
// via rule.KindScopedChecker; package-level so EnteringKinds returns
// it without allocating.
var enteringKinds = []ast.NodeKind{ast.KindFencedCodeBlock}

// EnteringKinds implements rule.KindScopedChecker: CheckNode only
// reacts to these node kinds, entering visits only.
func (r *Rule) EnteringKinds() []ast.NodeKind { return enteringKinds }

var _ rule.KindScopedChecker = (*Rule)(nil)
