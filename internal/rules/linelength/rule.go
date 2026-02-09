package linelength

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/jeduden/tidymark/internal/lint"
	"github.com/jeduden/tidymark/internal/rule"
	"github.com/yuin/goldmark/ast"
)

func init() {
	rule.Register(&Rule{Max: 80, Strict: false})
}

// Rule checks that no line exceeds the configured maximum length.
// When Strict is false (the default), lines inside fenced or indented code
// blocks and lines whose only non-whitespace content is a URL are excluded.
type Rule struct {
	Max    int
	Strict bool
}

func (r *Rule) ID() string   { return "TM001" }
func (r *Rule) Name() string { return "line-length" }

// urlOnlyRe matches a line whose trimmed content is a single URL.
var urlOnlyRe = regexp.MustCompile(`^https?://\S+$`)

func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	max := r.Max
	if max <= 0 {
		max = 80
	}

	// Build the set of 1-based line numbers inside code blocks.
	codeLines := map[int]bool{}
	if !r.Strict {
		codeLines = collectCodeBlockLines(f)
	}

	var diags []lint.Diagnostic
	for i, line := range f.Lines {
		lineNum := i + 1 // 1-based

		if len(line) <= max {
			continue
		}

		// When not strict, skip code-block lines.
		if !r.Strict && codeLines[lineNum] {
			continue
		}

		// When not strict, skip URL-only lines.
		if !r.Strict && urlOnlyRe.MatchString(strings.TrimSpace(string(line))) {
			continue
		}

		diags = append(diags, lint.Diagnostic{
			File:     f.Path,
			Line:     lineNum,
			Column:   max + 1,
			RuleID:   r.ID(),
			RuleName: r.Name(),
			Severity: lint.Warning,
			Message:  fmt.Sprintf("line too long (%d > %d)", len(line), max),
		})
	}

	return diags
}

// collectCodeBlockLines walks the AST and returns a set of 1-based line
// numbers that belong to fenced code blocks (including fence lines) or
// indented code blocks.
func collectCodeBlockLines(f *lint.File) map[int]bool {
	lines := map[int]bool{}

	ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		switch cb := n.(type) {
		case *ast.FencedCodeBlock:
			addFencedCodeBlockLines(f, cb, lines)
		case *ast.CodeBlock:
			addBlockLines(f, cb, lines)
		}

		return ast.WalkContinue, nil
	})

	return lines
}

// addFencedCodeBlockLines marks the opening fence line, all content lines,
// and the closing fence line.
func addFencedCodeBlockLines(f *lint.File, fcb *ast.FencedCodeBlock, set map[int]bool) {
	// Determine the opening fence line by looking at the node's info or
	// the first content line. The opening fence is always the line before
	// the first content line (or, when there are no content lines, we find
	// it via the Info segment).
	openLine := findFencedOpenLine(f, fcb)
	if openLine > 0 {
		set[openLine] = true
	}

	// Content lines from the code block's segments.
	segs := fcb.Lines()
	lastContentLine := 0
	for i := 0; i < segs.Len(); i++ {
		seg := segs.At(i)
		ln := f.LineOfOffset(seg.Start)
		set[ln] = true
		if ln > lastContentLine {
			lastContentLine = ln
		}
	}

	// Closing fence line is the line after the last content line.
	// If there are no content lines, the closing fence is the line after
	// the opening fence.
	closeLine := 0
	if lastContentLine > 0 {
		closeLine = lastContentLine + 1
	} else if openLine > 0 {
		closeLine = openLine + 1
	}
	if closeLine > 0 && closeLine <= len(f.Lines) {
		set[closeLine] = true
	}
}

// findFencedOpenLine returns the 1-based line number of the opening fence.
func findFencedOpenLine(f *lint.File, fcb *ast.FencedCodeBlock) int {
	// If the code block has an info string, walk backwards from it to find
	// the start of the line.
	if fcb.Info != nil {
		return f.LineOfOffset(fcb.Info.Segment.Start)
	}
	// If there are content lines, the opening fence is on the previous line.
	if fcb.Lines().Len() > 0 {
		firstContentLine := f.LineOfOffset(fcb.Lines().At(0).Start)
		if firstContentLine > 1 {
			return firstContentLine - 1
		}
		return 1
	}
	// Empty fenced code block with no info: scan from the node's text position.
	// Fall back to using previous sibling or document start.
	return 0
}

// addBlockLines marks all content lines of an indented code block.
func addBlockLines(f *lint.File, cb *ast.CodeBlock, set map[int]bool) {
	segs := cb.Lines()
	for i := 0; i < segs.Len(); i++ {
		seg := segs.At(i)
		ln := f.LineOfOffset(seg.Start)
		set[ln] = true
	}
}
