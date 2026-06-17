package fencedcodestyle

import (
	"fmt"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/fencepos"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
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

// Check implements rule.Rule. The per-block verdict depends only on the
// fence character of a fenced block's opening line, never the inline
// tree, so the rule is a rule.BlockChecker: on a parsed File it folds
// into the engine's shared AST walk (rule.WalkNodes); on a parse-skipped
// File (f.AST nil) it reads the Layer 0 block scan (rule.WalkBlocks).
// Both resolve the same per-block verdict, so the diagnostics match.
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

	openStart, _ := fencepos.OpenLineRange(f.Source, fcb)
	if openStart >= len(f.Source) {
		return nil
	}
	return r.verdict(f, fencepos.CharAt(f.Source, openStart), f.LineOfOffset(openStart))
}

// CheckBlock implements rule.BlockChecker. A BlockFencedCode span's Start
// is the opening fence line (the line LineOfOffset(openStart) yields on
// the AST path), and the fence character is the first backtick or tilde
// after any leading spaces — exactly what fencepos.CharAt reads — so the
// verdict is byte-identical.
func (r *Rule) CheckBlock(span lint.BlockSpan, f *lint.File) []lint.Diagnostic {
	return r.verdict(f, fenceCharOfLine(f.Lines[span.Start-1]), span.Start)
}

// blockKinds is the static block-kind interest CheckBlock declares via
// rule.BlockChecker; package-level so BlockKinds returns it without
// allocating.
var blockKinds = []lint.BlockKind{lint.BlockFencedCode}

// BlockKinds implements rule.BlockChecker.
func (r *Rule) BlockKinds() []lint.BlockKind { return blockKinds }

var _ rule.BlockChecker = (*Rule)(nil)

// verdict is the shared per-block check both paths drive: fenceChar is
// the opening fence character (0 when none could be read) and line its
// 1-based opening line.
func (r *Rule) verdict(f *lint.File, fenceChar byte, line int) []lint.Diagnostic {
	if fenceChar == 0 || fenceChar == r.wantChar() {
		return nil
	}
	return []lint.Diagnostic{{
		File:     f.Path,
		Line:     line,
		Column:   1,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Warning,
		Message:  "fenced code block should use " + r.Style + " style",
	}}
}

// fenceCharOfLine returns the opening fence character of a line the
// Layer 0 scanner classified BlockFencedCode: the first backtick or
// tilde after any leading spaces, mirroring fencepos.CharAt (which skips
// all leading spaces from the line start), or 0 if neither is present.
func fenceCharOfLine(line []byte) byte {
	i := 0
	for i < len(line) && line[i] == ' ' {
		i++
	}
	if i < len(line) && (line[i] == '`' || line[i] == '~') {
		return line[i]
	}
	return 0
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

// FixTitle implements rule.QuickFixTitler.
func (r *Rule) FixTitle() string { return "Convert to configured code-fence style" }

// enteringKinds is the static node-kind interest CheckNode declares
// via rule.KindScopedChecker; package-level so EnteringKinds returns
// it without allocating.
var enteringKinds = []ast.NodeKind{ast.KindFencedCodeBlock}

// EnteringKinds implements rule.KindScopedChecker: CheckNode only
// reacts to these node kinds, entering visits only.
func (r *Rule) EnteringKinds() []ast.NodeKind { return enteringKinds }

var _ rule.KindScopedChecker = (*Rule)(nil)
