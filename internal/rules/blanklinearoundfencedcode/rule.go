package blanklinearoundfencedcode

import (
	"bytes"
	"strings"

	"github.com/jeduden/tidymark/internal/lint"
	"github.com/jeduden/tidymark/internal/rule"
	"github.com/jeduden/tidymark/internal/rules/fencedcodestyle"
	"github.com/yuin/goldmark/ast"
)

func init() {
	rule.Register(&Rule{})
}

// Rule checks that fenced code blocks have blank lines before and after.
type Rule struct{}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "TM015" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "blank-line-around-fenced-code" }

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

		openStart, openEnd := fencedcodestyle.FenceOpenLineRange(f.Source, fcb)
		closeStart, _ := fencedcodestyle.FenceCloseLineRange(f.Source, fcb, openEnd)

		openLine := f.LineOfOffset(openStart)
		closeLine := f.LineOfOffset(closeStart)

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

		return ast.WalkContinue, nil
	})

	return diags
}

// Fix implements rule.FixableRule.
func (r *Rule) Fix(f *lint.File) []byte {
	insertBeforeLine, insertAfterLine := collectFenceBlankLineInsertions(f)

	if len(insertBeforeLine) == 0 && len(insertAfterLine) == 0 {
		return f.Source
	}

	var result []string
	for i, line := range f.Lines {
		lineNum := i + 1
		if insertBeforeLine[lineNum] {
			result = append(result, "")
		}
		result = append(result, string(line))
		if insertAfterLine[lineNum] {
			result = append(result, "")
		}
	}

	return []byte(strings.Join(result, "\n"))
}

// collectFenceBlankLineInsertions walks the AST and returns sets of 1-based line
// numbers that need a blank line inserted before or after them.
func collectFenceBlankLineInsertions(f *lint.File) (beforeSet, afterSet map[int]bool) {
	beforeSet = make(map[int]bool)
	afterSet = make(map[int]bool)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		fcb, ok := n.(*ast.FencedCodeBlock)
		if !ok {
			return ast.WalkContinue, nil
		}

		openStart, openEnd := fencedcodestyle.FenceOpenLineRange(f.Source, fcb)
		closeStart, _ := fencedcodestyle.FenceCloseLineRange(f.Source, fcb, openEnd)

		openLine := f.LineOfOffset(openStart)
		closeLine := f.LineOfOffset(closeStart)

		if needsBlankBefore(f, openLine) {
			beforeSet[openLine] = true
		}
		if needsBlankAfter(f, closeLine) {
			afterSet[closeLine] = true
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
