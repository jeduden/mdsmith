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
		if codeLines[lineNum] {
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
		if codeLines[lineNum] {
			lines[i] = string(line)
			continue
		}
		prefix := reBlockquotePrefix.Find(line)
		if !reMultiSpace.Match(prefix) {
			lines[i] = string(line)
			continue
		}
		fixedPrefix := reMultiSpace.ReplaceAll(prefix, []byte("> "))
		lines[i] = string(fixedPrefix) + string(line[len(prefix):])
	}
	return []byte(strings.Join(lines, "\n"))
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
