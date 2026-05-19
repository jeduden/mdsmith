package fencedcodestyle

import (
	"fmt"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/fencepos"
	"github.com/yuin/goldmark/ast"
)

func init() {
	rule.Register(&Rule{Style: "backtick"})
}

// Rule checks that fenced code blocks use a consistent fence style.
// Default style is "backtick". Set Style to "tilde" for tilde fences.
type Rule struct {
	Style string // "backtick" or "tilde"
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS010" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "fenced-code-style" }

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

	openStart, _ := fencepos.OpenLineRange(f.Source, fcb)
	if openStart >= len(f.Source) {
		return nil
	}

	fenceChar := fencepos.CharAt(f.Source, openStart)
	if fenceChar == 0 {
		return nil
	}

	if fenceChar != r.wantChar() {
		return []lint.Diagnostic{{
			File:     f.Path,
			Line:     f.LineOfOffset(openStart),
			Column:   1,
			RuleID:   r.ID(),
			RuleName: r.Name(),
			Severity: lint.Warning,
			Message:  "fenced code block should use " + r.Style + " style",
		}}
	}
	return nil
}

// Fix implements rule.FixableRule.
func (r *Rule) Fix(f *lint.File) []byte {
	type fenceRange struct {
		openStart, openEnd   int
		closeStart, closeEnd int
	}
	var ranges []fenceRange

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		fcb, ok := n.(*ast.FencedCodeBlock)
		if !ok {
			return ast.WalkContinue, nil
		}

		openStart, openEnd := fencepos.OpenLineRange(f.Source, fcb)
		if openStart >= len(f.Source) {
			return ast.WalkContinue, nil
		}

		fenceChar := fencepos.CharAt(f.Source, openStart)
		if fenceChar == 0 {
			return ast.WalkContinue, nil
		}

		wantChar := r.wantChar()
		if fenceChar != wantChar {
			closeStart, closeEnd := fencepos.CloseLineRange(f.Source, fcb, openEnd)
			ranges = append(ranges, fenceRange{
				openStart: openStart, openEnd: openEnd,
				closeStart: closeStart, closeEnd: closeEnd,
			})
		}

		return ast.WalkContinue, nil
	})

	if len(ranges) == 0 {
		return f.Source
	}

	wantChar := r.wantChar()
	result := make([]byte, 0, len(f.Source))
	prev := 0
	for _, fr := range ranges {
		result = append(result, f.Source[prev:fr.openStart]...)
		result = append(result, replaceFenceChars(f.Source[fr.openStart:fr.openEnd], wantChar)...)
		result = append(result, f.Source[fr.openEnd:fr.closeStart]...)
		result = append(result, replaceFenceChars(f.Source[fr.closeStart:fr.closeEnd], wantChar)...)
		prev = fr.closeEnd
	}
	result = append(result, f.Source[prev:]...)
	return result
}

func (r *Rule) wantChar() byte {
	if r.Style == "tilde" {
		return '~'
	}
	return '`'
}

// replaceFenceChars replaces backtick or tilde chars in a fence line with the target char,
// preserving count, leading spaces, and any info string.
func replaceFenceChars(line []byte, targetChar byte) []byte {
	result := make([]byte, len(line))
	copy(result, line)
	i := 0
	// Skip leading spaces
	for i < len(result) && result[i] == ' ' {
		i++
	}
	// Replace fence characters
	for i < len(result) && (result[i] == '`' || result[i] == '~') {
		result[i] = targetChar
		i++
	}
	return result
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(settings map[string]any) error {
	for k, v := range settings {
		switch k {
		case "style":
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("fenced-code-style: style must be a string, got %T", v)
			}
			if s != "backtick" && s != "tilde" {
				return fmt.Errorf("fenced-code-style: invalid style %q (valid: backtick, tilde)", s)
			}
			r.Style = s
		default:
			return fmt.Errorf("fenced-code-style: unknown setting %q", k)
		}
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"style": "backtick",
	}
}

var _ rule.Configurable = (*Rule)(nil)
var _ rule.NodeChecker = (*Rule)(nil)
