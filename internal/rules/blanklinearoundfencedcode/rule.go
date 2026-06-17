package blanklinearoundfencedcode

import (
	"bytes"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/fencepos"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
)

// newlineSep is the bytes.Join separator; a package-level var avoids
// a heap allocation for []byte("\n") on every Fix call.
var newlineSep = []byte("\n")

func init() {
	rule.Register(&Rule{})
}

// Rule checks that fenced code blocks have blank lines before and after.
type Rule struct{}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS015" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "blank-line-around-fenced-code" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "code" }

// Check implements rule.Rule. The per-block logic depends only on the
// fenced block's open and close line and the blank lines around them —
// never the inline tree — so the rule is a rule.BlockChecker: on a
// parsed File it folds into the engine's shared AST walk
// (rule.WalkNodes), and on a parse-skipped File (f.AST nil) it reads the
// Layer 0 block scan instead (rule.WalkBlocks). Both resolve the same
// open/close lines, so the diagnostics are identical.
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

	openStart, openEnd := fencepos.OpenLineRange(f.Source, fcb)
	closeStart, _ := fencepos.CloseLineRange(f.Source, fcb, openEnd)

	return r.checkBlanks(f, f.LineOfOffset(openStart), f.LineOfOffset(closeStart))
}

// CheckBlock implements rule.BlockChecker. A BlockFencedCode span's Start
// is the opening fence line and End the closing fence line — the same
// open/close lines fencepos resolves on the AST path — so the blank-line
// verdict is byte-identical.
func (r *Rule) CheckBlock(span lint.BlockSpan, f *lint.File) []lint.Diagnostic {
	return r.checkBlanks(f, span.Start, span.End)
}

// blockKinds is the static block-kind interest CheckBlock declares via
// rule.BlockChecker; package-level so BlockKinds returns it without
// allocating.
var blockKinds = []lint.BlockKind{lint.BlockFencedCode}

// BlockKinds implements rule.BlockChecker.
func (r *Rule) BlockKinds() []lint.BlockKind { return blockKinds }

var _ rule.BlockChecker = (*Rule)(nil)

// checkBlanks is the shared per-block check both the AST (CheckNode) and
// Layer 0 (CheckBlock) paths drive: openLine is the 1-based opening fence
// line, closeLine the closing fence line.
func (r *Rule) checkBlanks(f *lint.File, openLine, closeLine int) []lint.Diagnostic {
	var diags []lint.Diagnostic

	// Check blank line before opening fence
	if openLine > 1 {
		prevLineIdx := openLine - 2 // 0-based index of the line before
		if prevLineIdx >= 0 && prevLineIdx < len(f.Lines) {
			if !isBlank(f.Lines[prevLineIdx]) {
				diags = append(diags, lint.Diagnostic{
					File:     f.Path,
					Line:     openLine,
					Column:   1,
					RuleID:   r.ID(),
					RuleName: r.Name(),
					Severity: lint.Warning,
					Message:  "fenced code block should be preceded by a blank line",
				})
			}
		}
	}

	// Check blank line after closing fence
	closeLineIdx := closeLine - 1 // 0-based index of closing fence line
	nextLineIdx := closeLineIdx + 1
	if nextLineIdx < len(f.Lines) {
		if !isBlank(f.Lines[nextLineIdx]) {
			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     closeLine,
				Column:   1,
				RuleID:   r.ID(),
				RuleName: r.Name(),
				Severity: lint.Warning,
				Message:  "fenced code block should be followed by a blank line",
			})
		}
	}
	return diags
}

var _ rule.NodeChecker = (*Rule)(nil)

// Fix implements rule.FixableRule.
func (r *Rule) Fix(f *lint.File) []byte {
	insertBeforeLine, insertAfterLine := collectFenceBlankLineInsertions(f)

	if len(insertBeforeLine) == 0 && len(insertAfterLine) == 0 {
		return f.Source
	}

	result := make([][]byte, 0, len(f.Lines)+len(insertBeforeLine)+len(insertAfterLine))
	for i, line := range f.Lines {
		lineNum := i + 1
		if _, ok := insertBeforeLine[lineNum]; ok {
			result = append(result, nil)
		}
		result = append(result, line)
		if _, ok := insertAfterLine[lineNum]; ok {
			result = append(result, nil)
		}
	}

	return bytes.Join(result, newlineSep)
}

// collectFenceBlankLineInsertions walks the AST and returns sets of 1-based line
// numbers that need a blank line inserted before or after them.
func collectFenceBlankLineInsertions(f *lint.File) (beforeSet, afterSet map[int]struct{}) {
	beforeSet = make(map[int]struct{})
	afterSet = make(map[int]struct{})

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		fcb, ok := n.(*ast.FencedCodeBlock)
		if !ok {
			return ast.WalkContinue, nil
		}

		openStart, openEnd := fencepos.OpenLineRange(f.Source, fcb)
		closeStart, _ := fencepos.CloseLineRange(f.Source, fcb, openEnd)

		openLine := f.LineOfOffset(openStart)
		closeLine := f.LineOfOffset(closeStart)

		if needsBlankBefore(f, openLine) {
			beforeSet[openLine] = struct{}{}
		}
		if needsBlankAfter(f, closeLine) {
			afterSet[closeLine] = struct{}{}
		}

		return ast.WalkContinue, nil
	})

	return beforeSet, afterSet
}

// needsBlankBefore returns true if the line before the given 1-based line
// exists and is non-blank.
func needsBlankBefore(f *lint.File, line int) bool {
	if line <= 1 {
		return false
	}
	prevIdx := line - 2
	return prevIdx >= 0 && prevIdx < len(f.Lines) && !isBlank(f.Lines[prevIdx])
}

// needsBlankAfter returns true if the line after the given 1-based line
// exists and is non-blank.
func needsBlankAfter(f *lint.File, line int) bool {
	nextIdx := line // 0-based index of the next line
	return nextIdx < len(f.Lines) && !isBlank(f.Lines[nextIdx])
}

func isBlank(line []byte) bool {
	return len(bytes.TrimSpace(line)) == 0
}

// FixTitle implements rule.QuickFixTitler.
func (r *Rule) FixTitle() string { return "Add blank lines around code fence" }

// enteringKinds is the static node-kind interest CheckNode declares
// via rule.KindScopedChecker; package-level so EnteringKinds returns
// it without allocating.
var enteringKinds = []ast.NodeKind{ast.KindFencedCodeBlock}

// EnteringKinds implements rule.KindScopedChecker: CheckNode only
// reacts to these node kinds, entering visits only.
func (r *Rule) EnteringKinds() []ast.NodeKind { return enteringKinds }

var _ rule.KindScopedChecker = (*Rule)(nil)
