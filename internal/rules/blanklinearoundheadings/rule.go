package blanklinearoundheadings

import (
	"bytes"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/astutil"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
)

func init() {
	rule.Register(&Rule{})
}

// newlineSep is the bytes.Join separator; a package-level var avoids
// a heap allocation for []byte("\n") on every Fix call.
var newlineSep = []byte("\n")

// Rule checks that headings have blank lines before and after them.
type Rule struct{}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS013" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "blank-line-around-headings" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "heading" }

// Check implements rule.Rule. The per-heading logic is pure and
// stateless, so it is expressed as CheckNode and the engine can fold
// this rule into one shared AST walk; a direct call still works via
// rule.WalkNodes.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	return rule.WalkNodes(r, f)
}

// CheckNode implements rule.NodeChecker. Code-line lookup runs lazily
// via lint.CollectCodeBlockLines (cached on f); the rule cannot
// precompute it once per Check because the engine multiplexes
// CheckNode calls, but the cache makes it a single walk per file
// across all callers.
func (r *Rule) CheckNode(n ast.Node, entering bool, f *lint.File) []lint.Diagnostic {
	if !entering {
		return nil
	}
	heading, ok := n.(*ast.Heading)
	if !ok {
		return nil
	}

	line := astutil.HeadingLine(heading, f)

	// Skip headings whose lines overlap with code block regions.
	codeLines := lint.CollectCodeBlockLines(f)
	if _, ok := codeLines[line]; ok {
		return nil
	}
	lastLine := headingLastLine(heading, f)

	var diags []lint.Diagnostic

	// Check blank line before (not needed for line 1)
	if line > 1 {
		prevLineIdx := line - 2 // 0-based index
		if prevLineIdx >= 0 && prevLineIdx < len(f.Lines) {
			if len(bytes.TrimSpace(f.Lines[prevLineIdx])) != 0 {
				diags = append(diags, lint.Diagnostic{
					File:     f.Path,
					Line:     line,
					Column:   1,
					RuleID:   r.ID(),
					RuleName: r.Name(),
					Severity: lint.Warning,
					Message:  "heading should have a blank line before",
				})
			}
		}
	}

	// Check blank line after (not needed for last line)
	nextLineIdx := lastLine // 0-based index of line after heading
	if nextLineIdx < len(f.Lines) {
		if len(bytes.TrimSpace(f.Lines[nextLineIdx])) != 0 {
			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     line,
				Column:   1,
				RuleID:   r.ID(),
				RuleName: r.Name(),
				Severity: lint.Warning,
				Message:  "heading should have a blank line after",
			})
		}
	}
	return diags
}

// Fix implements rule.FixableRule.
func (r *Rule) Fix(f *lint.File) []byte {
	insertBefore, insertAfter := collectHeadingBlankLineInsertions(f)

	if len(insertBefore) == 0 && len(insertAfter) == 0 {
		return f.Source
	}

	// Pre-size to avoid growth allocations; worst case each line gets a
	// blank before and after. Work directly with []byte to avoid the
	// O(n) string([]byte) conversion that the previous string-join path
	// paid for every line in the document.
	result := make([][]byte, 0, len(f.Lines)+len(insertBefore)+len(insertAfter))
	for i, line := range f.Lines {
		lineNum := i + 1
		if _, ok := insertBefore[lineNum]; ok {
			// Avoid inserting a blank line if one was just inserted
			// after the previous line (prevents double blank lines).
			if _, ok2 := insertAfter[lineNum-1]; !ok2 {
				result = append(result, nil)
			}
		}
		result = append(result, line)
		if _, ok := insertAfter[lineNum]; ok {
			result = append(result, nil)
		}
	}

	return bytes.Join(result, newlineSep)
}

// collectHeadingBlankLineInsertions walks the AST and returns sets of 1-based
// line numbers that need a blank line inserted before or after them. Insertion
// decisions are made directly inside the walk to avoid an intermediate slice
// and its growth allocations.
func collectHeadingBlankLineInsertions(f *lint.File) (insertBefore, insertAfter map[int]struct{}) {
	insertBefore = make(map[int]struct{})
	insertAfter = make(map[int]struct{})
	codeLines := lint.CollectCodeBlockLines(f)
	lines := f.Lines

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		heading, ok := n.(*ast.Heading)
		if !ok {
			return ast.WalkContinue, nil
		}
		line := astutil.HeadingLine(heading, f)
		if _, ok := codeLines[line]; ok {
			return ast.WalkContinue, nil
		}
		lastLine := headingLastLine(heading, f)
		if line > 1 && isNonBlankLine(lines, line-2) {
			insertBefore[line] = struct{}{}
		}
		if isNonBlankLine(lines, lastLine) {
			insertAfter[lastLine] = struct{}{}
		}
		return ast.WalkContinue, nil
	})

	return insertBefore, insertAfter
}

// isNonBlankLine returns true if the 0-based index is within bounds and the
// line is non-blank.
func isNonBlankLine(lines [][]byte, idx int) bool {
	if idx < 0 || idx >= len(lines) {
		return false
	}
	return len(bytes.TrimSpace(lines[idx])) != 0
}

func headingLastLine(heading *ast.Heading, f *lint.File) int {
	lines := heading.Lines()
	if lines.Len() > 0 {
		// Setext headings: the underline is on the line after the text
		lastSeg := lines.At(lines.Len() - 1)
		textLine := f.LineOfOffset(lastSeg.Start)
		// Check if next line is an underline (setext)
		if isSetextHeading(heading, f.Source) {
			return textLine + 1
		}
		return textLine
	}
	// ATX heading is a single line
	return astutil.HeadingLine(heading, f)
}

func isSetextHeading(heading *ast.Heading, source []byte) bool {
	lines := heading.Lines()
	if lines.Len() == 0 {
		return false
	}
	seg := lines.At(0)
	lineStart := seg.Start
	for lineStart > 0 && source[lineStart-1] != '\n' {
		lineStart--
	}
	if lineStart < len(source) && source[lineStart] == '#' {
		return false
	}
	return true
}

var _ rule.FixableRule = (*Rule)(nil)
var _ rule.NodeChecker = (*Rule)(nil)

// FixTitle implements rule.QuickFixTitler.
func (r *Rule) FixTitle() string { return "Add blank lines around heading" }
