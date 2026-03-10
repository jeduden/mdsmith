package unclosedcodeblock

import (
	"bytes"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/fencedcodestyle"
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

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	var diags []lint.Diagnostic

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		fcb, ok := n.(*ast.FencedCodeBlock)
		if !ok {
			return ast.WalkContinue, nil
		}

		if !hasClosingFence(f, fcb) {
			openLine := fencedcodestyle.FenceOpenLine(f, fcb)
			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     openLine,
				Column:   1,
				RuleID:   r.ID(),
				RuleName: r.Name(),
				Severity: lint.Error,
				Message:  "unclosed fenced code block",
			})
		}

		return ast.WalkContinue, nil
	})

	return diags
}

// hasClosingFence checks whether a fenced code block has a proper closing
// fence line after its content.
func hasClosingFence(f *lint.File, fcb *ast.FencedCodeBlock) bool {
	openStart, openEnd := fencedcodestyle.FenceOpenLineRange(f.Source, fcb)
	if openStart >= len(f.Source) {
		return true
	}

	fenceChar := fenceCharAt(f.Source, openStart)
	if fenceChar == 0 {
		return true
	}

	closeStart, closeEnd := fencedcodestyle.FenceCloseLineRange(f.Source, fcb, openEnd)

	// No closing line exists (at or past EOF).
	if closeStart >= len(f.Source) {
		return false
	}

	// The closing line must contain the fence character.
	if closeStart == closeEnd {
		return false
	}
	closingLine := bytes.TrimLeft(f.Source[closeStart:closeEnd], " ")
	minFence := []byte{fenceChar, fenceChar, fenceChar}
	return bytes.HasPrefix(closingLine, minFence)
}

// fenceCharAt returns the fence character at the given position, skipping
// leading spaces.
func fenceCharAt(src []byte, pos int) byte {
	for pos < len(src) && src[pos] == ' ' {
		pos++
	}
	if pos < len(src) && (src[pos] == '`' || src[pos] == '~') {
		return src[pos]
	}
	return 0
}
