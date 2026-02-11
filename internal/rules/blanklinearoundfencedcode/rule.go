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
	// Collect fence line numbers (1-based) that need blank lines
	type blankNeeded struct {
		beforeLine int // 1-based line that needs a blank line before it
		afterLine  int // 1-based line that needs a blank line after it
	}
	var needs []blankNeeded

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

		need := blankNeeded{}

		// Check blank line before opening fence
		if openLine > 1 {
			prevLineIdx := openLine - 2
			if prevLineIdx >= 0 && prevLineIdx < len(f.Lines) {
				if !isBlank(f.Lines[prevLineIdx]) {
					need.beforeLine = openLine
				}
			}
		}

		// Check blank line after closing fence
		closeLineIdx := closeLine - 1
		nextLineIdx := closeLineIdx + 1
		if nextLineIdx < len(f.Lines) {
			if !isBlank(f.Lines[nextLineIdx]) {
				need.afterLine = closeLine
			}
		}

		if need.beforeLine > 0 || need.afterLine > 0 {
			needs = append(needs, need)
		}

		return ast.WalkContinue, nil
	})

	if len(needs) == 0 {
		return f.Source
	}

	// Build sets of lines that need blank lines before/after them
	insertBeforeLine := make(map[int]bool) // 1-based line numbers
	insertAfterLine := make(map[int]bool)
	for _, n := range needs {
		if n.beforeLine > 0 {
			insertBeforeLine[n.beforeLine] = true
		}
		if n.afterLine > 0 {
			insertAfterLine[n.afterLine] = true
		}
	}

	var result []string
	for i, line := range f.Lines {
		lineNum := i + 1 // 1-based
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

func isBlank(line []byte) bool {
	return len(bytes.TrimSpace(line)) == 0
}
