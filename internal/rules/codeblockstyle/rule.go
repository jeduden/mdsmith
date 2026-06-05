// Package codeblockstyle implements MDS065 code-block-style, which
// enforces a single code-block delimiter across a file: fenced
// (```) or indented (four-space). Mirrors markdownlint MD046 with
// the same `consistent | fenced | indented` setting; the default
// is `fenced` because indented code blocks lose the language tag.
//
// Autofix converts indented blocks to fenced. The converted block
// carries the `text` language tag so the result satisfies MDS011
// (fenced-code-language); users are expected to refine the tag.
// The reverse direction (fenced→indented) is not auto-applied,
// because it would drop an existing language tag.
package codeblockstyle

import (
	"fmt"
	"strings"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/fencepos"
	"github.com/yuin/goldmark/ast"
)

func init() {
	rule.Register(&Rule{Style: "fenced"})
}

// Rule checks that code blocks use a consistent style.
type Rule struct {
	Style string // "consistent", "fenced", or "indented"
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS065" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "code-block-style" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "code" }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	if f == nil || f.AST == nil {
		return nil
	}
	blocks := collectBlocks(f)
	want := r.effectiveStyle(blocks)
	if want == "" {
		return nil
	}
	var diags []lint.Diagnostic
	for _, b := range blocks {
		if b.style == want {
			continue
		}
		diags = append(diags, lint.Diagnostic{
			File:     f.Path,
			Line:     b.line,
			Column:   1,
			RuleID:   r.ID(),
			RuleName: r.Name(),
			Severity: lint.Warning,
			Message:  "code block should use " + want + " style",
		})
	}
	return diags
}

// Fix implements rule.FixableRule. Only the indented→fenced direction
// is applied; the reverse loses the language tag and so is left alone.
// Nested indented blocks (inside a list item or blockquote) are also
// skipped — emitting unindented fences would break the parent
// container, and computing the correct outer prefix is left to the
// author.
func (r *Rule) Fix(f *lint.File) []byte {
	if f == nil {
		return nil
	}
	if f.AST == nil {
		return f.Source
	}
	blocks := collectBlocks(f)
	want := r.effectiveStyle(blocks)
	if want != "fenced" {
		return f.Source
	}

	type indentedRange struct {
		firstLine int    // 1-based, inclusive
		lastLine  int    // 1-based, inclusive (last content line)
		fence     string // backtick fence sized to clear any in-content run
	}
	var ranges []indentedRange
	for _, b := range blocks {
		if b.style != "indented" {
			continue
		}
		if !b.topLevel {
			continue
		}
		ranges = append(ranges, indentedRange{
			firstLine: b.line,
			lastLine:  b.lastLine,
			fence:     fenceFor(f.Lines, b.line, b.lastLine),
		})
	}
	if len(ranges) == 0 {
		return f.Source
	}

	// ranges are appended in source order by collectBlocks (goldmark
	// walks the AST top-to-bottom), so the rewrite needs only a single
	// pass over f.Lines with an advancing range pointer — O(lines +
	// blocks) instead of O(lines × blocks).
	out := make([]string, 0, len(f.Lines)+2*len(ranges))
	ri := 0
	for i, raw := range f.Lines {
		lineNum := i + 1
		for ri < len(ranges) && ranges[ri].lastLine < lineNum {
			ri++
		}
		inRange := ri < len(ranges) &&
			lineNum >= ranges[ri].firstLine &&
			lineNum <= ranges[ri].lastLine
		if !inRange {
			out = append(out, string(raw))
			continue
		}
		if lineNum == ranges[ri].firstLine {
			out = append(out, ranges[ri].fence+"text")
		}
		out = append(out, stripIndent(raw))
		if lineNum == ranges[ri].lastLine {
			out = append(out, ranges[ri].fence)
		}
	}
	return []byte(strings.Join(out, "\n"))
}

// fenceFor returns a backtick fence at least three long and strictly
// longer than any run of backticks appearing at the start of an
// indent-stripped content line in [first, last]. A content line like
// "```" would close a 3-backtick fence, so the chosen fence must be
// at least one longer.
func fenceFor(lines [][]byte, first, last int) string {
	maxRun := 0
	for ln := first; ln <= last; ln++ {
		stripped := stripIndent(lines[ln-1])
		n := 0
		for n < len(stripped) && stripped[n] == '`' {
			n++
		}
		if n > maxRun {
			maxRun = n
		}
	}
	fenceLen := 3
	if maxRun >= fenceLen {
		fenceLen = maxRun + 1
	}
	return strings.Repeat("`", fenceLen)
}

// blockInfo records one code block's style ("fenced" or "indented"),
// its opening source line (1-based), the last-line anchor used by
// indented→fenced fixes, and whether the block is a direct child of
// the document (top-level). For fenced blocks lastLine is the closing
// fence line; for indented blocks it is the last content line — the
// field is only consumed by the indented→fenced fix path.
type blockInfo struct {
	style    string
	line     int
	lastLine int
	topLevel bool
}

func collectBlocks(f *lint.File) []blockInfo {
	var blocks []blockInfo
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch cb := n.(type) {
		case *ast.FencedCodeBlock:
			line := fencepos.OpenLine(f, cb)
			if skipBlock(f, line) {
				return ast.WalkContinue, nil
			}
			last := fencepos.CloseLine(f, cb)
			blocks = append(blocks, blockInfo{
				style: "fenced", line: line, lastLine: last,
				topLevel: isTopLevel(cb),
			})
		case *ast.CodeBlock:
			segs := cb.Lines()
			if segs.Len() == 0 {
				return ast.WalkContinue, nil
			}
			first := f.LineOfOffset(segs.At(0).Start)
			if skipBlock(f, first) {
				return ast.WalkContinue, nil
			}
			last := f.LineOfOffset(segs.At(segs.Len() - 1).Start)
			blocks = append(blocks, blockInfo{
				style: "indented", line: first, lastLine: last,
				topLevel: isTopLevel(cb),
			})
		}
		return ast.WalkContinue, nil
	})
	return blocks
}

// isTopLevel reports whether n's parent is the document root. Code
// blocks inside list items or blockquotes return false.
func isTopLevel(n ast.Node) bool {
	parent := n.Parent()
	if parent == nil {
		return false
	}
	_, ok := parent.(*ast.Document)
	return ok
}

func skipBlock(f *lint.File, line int) bool {
	for _, gr := range f.GeneratedRanges {
		if gr.Contains(line) {
			return true
		}
	}
	return false
}

// effectiveStyle returns the style the file should use, given the rule
// configuration and the set of blocks. Returns "" when there is no
// applicable target (no blocks under "consistent").
func (r *Rule) effectiveStyle(blocks []blockInfo) string {
	switch r.Style {
	case "fenced", "indented":
		return r.Style
	case "consistent":
		if len(blocks) == 0 {
			return ""
		}
		return blocks[0].style
	}
	return ""
}

// stripIndent removes up to four leading spaces (or one leading tab)
// from a single source line. Blank lines pass through unchanged.
func stripIndent(line []byte) string {
	if len(line) == 0 {
		return ""
	}
	if line[0] == '\t' {
		return string(line[1:])
	}
	n := 0
	for n < 4 && n < len(line) && line[n] == ' ' {
		n++
	}
	return string(line[n:])
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(settings map[string]any) error {
	for k, v := range settings {
		switch k {
		case "style":
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("code-block-style: style must be a string, got %T", v)
			}
			if s != "consistent" && s != "fenced" && s != "indented" {
				return fmt.Errorf("code-block-style: invalid style %q (valid: consistent, fenced, indented)", s)
			}
			r.Style = s
		default:
			return fmt.Errorf("code-block-style: unknown setting %q", k)
		}
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"style": "fenced",
	}
}

var _ rule.FixableRule = (*Rule)(nil)
var _ rule.Configurable = (*Rule)(nil)

// FixTitle implements rule.QuickFixTitler.
func (r *Rule) FixTitle() string { return "Convert to configured code-block style" }
