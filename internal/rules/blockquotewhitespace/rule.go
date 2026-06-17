// Package blockquotewhitespace implements MDS059, which flags two blockquote
// defects: more than one space after the > marker (MD027) and a blank line
// between two adjacent sibling blockquote nodes (MD028).
package blockquotewhitespace

import (
	"bytes"
	"regexp"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
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

// Check implements rule.Rule. MD027 (multiple spaces after the marker)
// reads only f.Lines and the code-line projection, both of which serve
// the nil-AST path unchanged. MD028 (blank line between adjacent sibling
// blockquotes) walks the AST on a parsed File and the Layer 0 BlockQuote
// spans on a parse-skipped File (f.AST nil); both resolve the same gaps,
// so the diagnostics are identical.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	if f == nil {
		return nil
	}
	var diags []lint.Diagnostic
	codeLines := lint.CollectCodeBlockLines(f)

	// MD027: flag blockquote lines where the last > in the marker prefix is
	// followed by two or more spaces. Only the leading prefix is scanned, so
	// a > that appears in the actual content of the blockquote is not flagged.
	for i, line := range f.Lines {
		// Candidate gate before the per-line set lookup: only lines whose
		// first non-blank byte is '>' carry a blockquote marker prefix.
		// Ordinary prose lines skip the map probe and the prefix scan.
		j := 0
		for j < len(line) && (line[j] == ' ' || line[j] == '\t') {
			j++
		}
		if j >= len(line) || line[j] != '>' {
			continue
		}
		lineNum := i + 1
		if _, ok := codeLines[lineNum]; ok {
			continue
		}
		if col, found := multiSpaceAfterMarker(line, j); found {
			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     lineNum,
				Column:   col + 1,
				RuleID:   r.ID(),
				RuleName: r.Name(),
				Severity: lint.Warning,
				Message:  "multiple spaces after blockquote marker",
			})
		}
	}

	// MD028: flag blank-line gaps between adjacent sibling blockquote nodes.
	if f.AST == nil {
		diags = append(diags, r.checkBlankBetweenLayer0(f)...)
	} else {
		diags = append(diags, r.checkBlankBetween(f)...)
	}
	return diags
}

// checkBlankBetweenLayer0 is the nil-AST counterpart of checkBlankBetween.
// The Layer 0 scanner emits one BlockQuote span per blockquote, splitting
// on the blank-line gap that ends a quote — exactly the sibling boundary
// the AST path keys on. Two consecutive BlockQuote spans whose gap is all
// blank lines are adjacent siblings separated only by blanks (MD028). The
// diagnostic lands on the first blank line of the gap (prev.End + 1),
// matching the AST path's scanLine+1, with the same column and message.
func (r *Rule) checkBlankBetweenLayer0(f *lint.File) []lint.Diagnostic {
	var diags []lint.Diagnostic
	var prevEnd int
	for _, span := range lint.Layer0(f).BlockSpans {
		if span.Kind != lint.BlockQuote {
			continue
		}
		if prevEnd > 0 && span.Start-prevEnd >= 2 && r.allBlankBetween(f, prevEnd, span.Start) {
			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     prevEnd + 1,
				Column:   1,
				RuleID:   r.ID(),
				RuleName: r.Name(),
				Severity: lint.Warning,
				Message:  "blank line between blockquotes",
			})
		}
		prevEnd = span.End
	}
	return diags
}

// allBlankBetween reports whether every line strictly between the 1-based
// lines lo and hi is blank.
func (r *Rule) allBlankBetween(f *lint.File, lo, hi int) bool {
	for ln := lo + 1; ln < hi; ln++ {
		if !isBlankLine(f, ln) {
			return false
		}
	}
	return true
}

// multiSpaceAfterMarker reports the 0-based index of the first '>' in
// the line's leading marker chain that is followed by two or more
// spaces. j is the index of the chain's first '>' (caller has skipped
// the leading blanks). It reproduces, without the two regex passes,
// `reMultiSpace.FindIndex(reBlockquotePrefix.Find(line))`: the scan
// stops where the marker-chain prefix ends, so a '>' inside content is
// never inspected, and only literal spaces (not tabs) directly after a
// '>' count toward the two-space defect.
func multiSpaceAfterMarker(line []byte, j int) (int, bool) {
	for j < len(line) && line[j] == '>' {
		marker := j
		j++
		spaces := 0
		for j < len(line) && line[j] == ' ' {
			j++
			spaces++
		}
		if spaces >= 2 {
			return marker, true
		}
		for j < len(line) && (line[j] == ' ' || line[j] == '\t') {
			j++
		}
	}
	return 0, false
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

// bqFixedSpace is the canonical single-space replacement for collapsed
// multi-space runs in blockquote prefixes; defined at package scope to avoid
// allocating []byte("> ") on every ReplaceAllLiteral call.
var bqFixedSpace = []byte("> ")

// Fix implements rule.FixableRule. Collapses multiple spaces after > to one
// space on every non-code-block blockquote line. MD028 violations are not
// auto-fixed because the intent (one quote vs two) is ambiguous.
func (r *Rule) Fix(f *lint.File) []byte {
	codeLines := lint.CollectCodeBlockLines(f)
	var buf bytes.Buffer
	buf.Grow(len(f.Source))
	for i, line := range f.Lines {
		if i > 0 {
			buf.WriteByte('\n')
		}
		lineNum := i + 1
		if _, ok := codeLines[lineNum]; ok {
			buf.Write(line)
			continue
		}
		prefix := reBlockquotePrefix.Find(line)
		if !reMultiSpace.Match(prefix) {
			buf.Write(line)
			continue
		}
		fixedPrefix := reMultiSpace.ReplaceAllLiteral(prefix, bqFixedSpace)
		content := line[len(prefix):]
		if len(content) == 0 {
			// No content after the marker chain: trim trailing space so we don't
			// introduce a trailing-whitespace violation that needs a second pass.
			buf.Write(bytes.TrimRight(fixedPrefix, " \t"))
		} else {
			buf.Write(fixedPrefix)
			buf.Write(content)
		}
	}
	return buf.Bytes()
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
