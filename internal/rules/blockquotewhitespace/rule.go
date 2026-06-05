// Package blockquotewhitespace implements MDS059, which flags two blockquote
// defects: more than one space after the > marker (MD027) and a blank line
// between two adjacent sibling blockquote nodes (MD028).
package blockquotewhitespace

import (
	"bytes"
	"regexp"
	"strings"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/yuin/goldmark/ast"
)

func init() {
	rule.Register(&Rule{})
}

// Rule flags two blockquote whitespace defects.
// MD027: more than one space after the > marker; autofix collapses to one.
// MD028: blank line(s) between two adjacent sibling blockquotes; flag only.
type Rule struct{}

var (
	// reBlockquotePrefix extracts the leading chain of > markers and their
	// trailing whitespace from the start of a line. The leading [ \t]* allows
	// any amount of indent because inside a list item the raw source line can
	// have more than 3 spaces of indent (relative to the container); each > may
	// be followed by a space or tab ([ \t]*). Only this prefix is checked for
	// MD027, so a > inside the blockquote's content (e.g. `> text >  more`) is
	// never treated as a marker.
	reBlockquotePrefix = regexp.MustCompile(`^[ \t]*(?:>[ \t]*)*`)
	// reMultiSpace matches a > followed by two or more spaces.
	reMultiSpace = regexp.MustCompile(`> {2,}`)
)

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS059" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "blockquote-whitespace" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "whitespace" }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	var diags []lint.Diagnostic
	codeLines := lint.CollectCodeBlockLines(f)

	// MD027: flag blockquote lines where the last > in the marker prefix is
	// followed by two or more spaces. Only the leading prefix is scanned, so
	// a > that appears in the actual content of the blockquote is not flagged.
	for i, line := range f.Lines {
		lineNum := i + 1
		if _, ok := codeLines[lineNum]; ok {
			continue
		}
		prefix := reBlockquotePrefix.Find(line)
		if loc := reMultiSpace.FindIndex(prefix); loc != nil {
			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     lineNum,
				Column:   loc[0] + 1,
				RuleID:   r.ID(),
				RuleName: r.Name(),
				Severity: lint.Warning,
				Message:  "multiple spaces after blockquote marker",
			})
		}
	}

	// MD028: flag blank-line gaps between adjacent sibling blockquote nodes.
	diags = append(diags, r.checkBlankBetween(f)...)
	return diags
}

func (r *Rule) checkBlankBetween(f *lint.File) []lint.Diagnostic {
	var diags []lint.Diagnostic
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		bq, ok := n.(*ast.Blockquote)
		if !ok {
			return ast.WalkContinue, nil
		}
		nextBq, ok := bq.NextSibling().(*ast.Blockquote)
		if !ok {
			return ast.WalkContinue, nil
		}
		first := nodeFirstLine(f, nextBq)
		if first == 0 {
			// nextBq is an empty blockquote with no source segments; derive its
			// line via a source scan rather than silently skipping the violation.
			first = emptyBQLine(f, nextBq)
		}
		if first == 0 {
			return ast.WalkContinue, nil
		}
		// Scan backwards through blank lines immediately before nextBq.
		// Adjacent sibling blockquotes separated only by blank lines trigger
		// MD028. A non-blank line in the gap means no all-blank gap exists.
		// This approach works even when the first blockquote is empty (no
		// source segments), which would cause nodeLastLine to return 0.
		scanLine := first - 1
		for scanLine > 0 && isBlankLine(f, scanLine) {
			scanLine--
		}
		if scanLine <= 0 || scanLine >= first-1 {
			return ast.WalkContinue, nil
		}
		diags = append(diags, lint.Diagnostic{
			File:     f.Path,
			Line:     scanLine + 1,
			Column:   1,
			RuleID:   r.ID(),
			RuleName: r.Name(),
			Severity: lint.Warning,
			Message:  "blank line between blockquotes",
		})
		return ast.WalkContinue, nil
	})
	return diags
}

// Fix implements rule.FixableRule. Collapses multiple spaces after > to one
// space on every non-code-block blockquote line. MD028 violations are not
// auto-fixed because the intent (one quote vs two) is ambiguous.
func (r *Rule) Fix(f *lint.File) []byte {
	codeLines := lint.CollectCodeBlockLines(f)
	lines := make([]string, len(f.Lines))
	for i, line := range f.Lines {
		lineNum := i + 1
		if _, ok := codeLines[lineNum]; ok {
			lines[i] = string(line)
			continue
		}
		prefix := reBlockquotePrefix.Find(line)
		if !reMultiSpace.Match(prefix) {
			lines[i] = string(line)
			continue
		}
		fixedPrefix := reMultiSpace.ReplaceAll(prefix, []byte("> "))
		content := line[len(prefix):]
		if len(content) == 0 {
			// No content after the marker chain: trim trailing space so we don't
			// introduce a trailing-whitespace violation that needs a second pass.
			lines[i] = strings.TrimRight(string(fixedPrefix), " \t")
		} else {
			lines[i] = string(fixedPrefix) + string(content)
		}
	}
	return []byte(strings.Join(lines, "\n"))
}

// emptyBQLine finds the source line of n, an empty AST blockquote node with
// no source segments. It walks back through preceding siblings to find one with
// a known position, then steps forward one blank-gap per empty preceding
// sibling. Falls back to the parent's position (or line 1) when no positioned
// sibling exists.
func emptyBQLine(f *lint.File, n ast.Node) int {
	steps := 0
	for cur := n.PreviousSibling(); cur != nil; cur = cur.PreviousSibling() {
		if first := nodeFirstLine(f, cur); first > 0 {
			line := first
			for i := 0; i <= steps; i++ {
				line = firstBQLineAfterGap(f, line)
				if line == 0 {
					return 0
				}
			}
			return line
		}
		steps++
	}
	// No positioned sibling found; start from the parent's first line.
	base := 1
	if p := n.Parent(); p != nil {
		if pFirst := nodeFirstLine(f, p); pFirst > 0 {
			base = pFirst
		}
	}
	line := firstBQLineFrom(f, base)
	for i := 0; i < steps; i++ {
		line = firstBQLineAfterGap(f, line)
		if line == 0 {
			return 0
		}
	}
	return line
}

// firstBQLineFrom scans forward from fromLine and returns the first 1-based
// line number whose leading prefix contains a > marker.
func firstBQLineFrom(f *lint.File, fromLine int) int {
	for i := fromLine; i <= len(f.Lines); i++ {
		if hasBQMarker(f.Lines[i-1]) {
			return i
		}
	}
	return 0
}

// firstBQLineAfterGap scans from fromLine past non-blank lines, then past
// blank lines, and returns the line number of the next blockquote marker line.
// Returns 0 if no such line exists or the content after the gap is not a
// blockquote.
func firstBQLineAfterGap(f *lint.File, fromLine int) int {
	scan := fromLine
	for scan <= len(f.Lines) && !isBlankLine(f, scan) {
		scan++
	}
	if scan > len(f.Lines) {
		return 0
	}
	for scan <= len(f.Lines) && isBlankLine(f, scan) {
		scan++
	}
	if scan > len(f.Lines) || !hasBQMarker(f.Lines[scan-1]) {
		return 0
	}
	return scan
}

func hasBQMarker(line []byte) bool {
	return bytes.ContainsRune(reBlockquotePrefix.Find(line), '>')
}

// nodeFirstLine returns the 1-based source line of n's first content byte.
// For container nodes (e.g. Blockquote) it recurses into children.
func nodeFirstLine(f *lint.File, n ast.Node) int {
	if n.Lines().Len() > 0 {
		return f.LineOfOffset(n.Lines().At(0).Start)
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if line := nodeFirstLine(f, c); line > 0 {
			return line
		}
	}
	return 0
}

func isBlankLine(f *lint.File, lineNum int) bool {
	idx := lineNum - 1
	if idx < 0 || idx >= len(f.Lines) {
		return false
	}
	return len(bytes.TrimSpace(f.Lines[idx])) == 0
}

var _ rule.FixableRule = (*Rule)(nil)

// FixTitle implements rule.QuickFixTitler.
func (r *Rule) FixTitle() string { return "Fix blockquote spacing" }
