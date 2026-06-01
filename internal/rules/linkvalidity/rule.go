// Package linkvalidity implements MDS062, which flags links that
// silently do not work: the reversed (text)[url] form (markdownlint
// MD011) and links or images whose destination is empty/`#` or whose
// visible text is empty (markdownlint MD042). The reversed form is the
// only autofixable defect — an empty target has no safe replacement.
package linkvalidity

import (
	"bytes"
	"regexp"
	"sort"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/yuin/goldmark/ast"
)

func init() {
	rule.Register(&Rule{})
}

// Rule flags reversed-syntax links and empty links/images. It is
// default-enabled: both shapes are correctness defects, not style
// choices.
type Rule struct{}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS062" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "link-validity" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "link" }

// reversedRe matches the literal (text)[url] shape. goldmark never
// parses this as a link, so it survives as plain text and a regex over
// the source line is the only way to see it. Guards in reversedInLine
// reject the false positives RE2's lack of look-around cannot.
var reversedRe = regexp.MustCompile(`\(([^)]+)\)\[([^\]]+)\]`)

const reversedMessage = "reversed link: use [text](url) instead of (text)[url]"

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	diags := r.checkEmpty(f)
	diags = append(diags, r.checkReversed(f)...)
	sort.SliceStable(diags, func(i, j int) bool {
		if diags[i].Line != diags[j].Line {
			return diags[i].Line < diags[j].Line
		}
		return diags[i].Column < diags[j].Column
	})
	return diags
}

// checkEmpty walks real link/image nodes and flags an empty or `#`-only
// destination, or (links only) empty visible text. Empty image alt text
// with a valid destination is MDS032's concern, not this rule's.
func (r *Rule) checkEmpty(f *lint.File) []lint.Diagnostic {
	var diags []lint.Diagnostic
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch node := n.(type) {
		case *ast.Image:
			if emptyDestination(node.Destination) {
				diags = append(diags, r.diag(f, nodeLine(node, f), "empty image destination"))
			}
		case *ast.Link:
			switch {
			case emptyDestination(node.Destination):
				diags = append(diags, r.diag(f, nodeLine(node, f), "empty link destination"))
			case !hasVisibleContent(node, f.Source):
				diags = append(diags, r.diag(f, nodeLine(node, f), "empty link text"))
			}
		}
		return ast.WalkContinue, nil
	})
	return diags
}

func (r *Rule) diag(f *lint.File, line int, msg string) lint.Diagnostic {
	return lint.Diagnostic{
		File:     f.Path,
		Line:     line,
		Column:   1,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Warning,
		Message:  msg,
	}
}

// emptyDestination reports whether dest is missing for link purposes.
// A whitespace-only destination and a bare `#` both render as a link
// that goes nowhere; markdownlint MD042 treats `#` the same way.
func emptyDestination(dest []byte) bool {
	t := bytes.TrimSpace(dest)
	return len(t) == 0 || string(t) == "#"
}

// hasVisibleContent reports whether the link renders any visible
// content: an image, code span, autolink, raw HTML, or non-whitespace
// text anywhere in its subtree.
func hasVisibleContent(link *ast.Link, source []byte) bool {
	found := false
	_ = ast.Walk(link, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch t := n.(type) {
		case *ast.Image, *ast.AutoLink, *ast.CodeSpan, *ast.RawHTML:
			found = true
			return ast.WalkStop, nil
		case *ast.Text:
			if len(bytes.TrimSpace(t.Segment.Value(source))) > 0 {
				found = true
				return ast.WalkStop, nil
			}
		}
		return ast.WalkContinue, nil
	})
	return found
}

// nodeLine returns the 1-based source line of an inline node. Inline
// nodes carry no position, so it uses the first descendant text segment
// and falls back to the nearest block ancestor's first line.
func nodeLine(n ast.Node, f *lint.File) int {
	if ln := firstTextLine(n, f); ln > 0 {
		return ln
	}
	for p := n.Parent(); p != nil; p = p.Parent() {
		if isInlineNode(p) {
			continue
		}
		lines := p.Lines()
		if lines != nil && lines.Len() > 0 {
			return f.LineOfOffset(lines.At(0).Start)
		}
	}
	return 1
}

func firstTextLine(n ast.Node, f *lint.File) int {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			return f.LineOfOffset(t.Segment.Start)
		}
		if ln := firstTextLine(c, f); ln > 0 {
			return ln
		}
	}
	return 0
}

// isInlineNode reports whether n is an inline node, whose Lines() would
// panic. nodeLine skips these while walking ancestors for a block with a
// source position.
func isInlineNode(n ast.Node) bool {
	switch n.(type) {
	case *ast.Text, *ast.CodeSpan, *ast.Emphasis,
		*ast.Link, *ast.Image, *ast.AutoLink, *ast.RawHTML:
		return true
	}
	return false
}

// --- reversed-link scan (MD011) ---

type revMatch struct {
	col0     int // 0-based byte index of '(' within the line
	matchEnd int // exclusive byte index just past ']' within the line
	text     []byte
	url      []byte
}

func (r *Rule) checkReversed(f *lint.File) []lint.Diagnostic {
	skip := r.skipPredicate(f)
	csRanges := collectCodeSpanRanges(f)
	lineStarts := computeLineStarts(f.Source)
	var diags []lint.Diagnostic
	for i, line := range f.Lines {
		ln := i + 1
		if skip(ln) {
			continue
		}
		masked := maskLine(line, lineStarts[i], csRanges)
		for _, mm := range reversedInLine(line, masked) {
			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     ln,
				Column:   mm.col0 + 1,
				RuleID:   r.ID(),
				RuleName: r.Name(),
				Severity: lint.Warning,
				Message:  reversedMessage,
			})
		}
	}
	return diags
}

// Fix implements rule.FixableRule. It rewrites every reversed (text)[url]
// to [text](url); empty links/images have no safe target and are left
// untouched.
func (r *Rule) Fix(f *lint.File) []byte {
	skip := r.skipPredicate(f)
	csRanges := collectCodeSpanRanges(f)
	lineStarts := computeLineStarts(f.Source)
	out := make([][]byte, len(f.Lines))
	for i, line := range f.Lines {
		if skip(i + 1) {
			out[i] = line
			continue
		}
		masked := maskLine(line, lineStarts[i], csRanges)
		matches := reversedInLine(line, masked)
		if len(matches) == 0 {
			out[i] = line
			continue
		}
		var b []byte
		prev := 0
		for _, mm := range matches {
			b = append(b, line[prev:mm.col0]...)
			b = append(b, '[')
			b = append(b, mm.text...)
			b = append(b, ']', '(')
			b = append(b, mm.url...)
			b = append(b, ')')
			prev = mm.matchEnd
		}
		b = append(b, line[prev:]...)
		out[i] = b
	}
	return bytes.Join(out, []byte("\n"))
}

// reversedInLine returns the reversed-link matches on one line. Detection
// runs on masked (code-span bytes blanked) while text, url, and the
// boundary guards read orig at the same offsets — the mask preserves
// length and position.
func reversedInLine(orig, masked []byte) []revMatch {
	// Reversed links always start with '('; skip regex on lines without it.
	// bytes.IndexByte takes a bare byte — zero allocation vs []byte{'('}.
	if bytes.IndexByte(masked, '(') < 0 {
		return nil
	}
	idx := reversedRe.FindAllSubmatchIndex(masked, -1)
	if idx == nil {
		return nil
	}
	var out []revMatch
	for _, m := range idx {
		s, e := m[0], m[1]
		if s > 0 && orig[s-1] == '\\' {
			continue // escaped '(' — literal text
		}
		if s > 0 && orig[s-1] == ']' {
			continue // ](text)[ref] — '(...)' is a real link destination
		}
		if e < len(orig) && orig[e] == '(' {
			continue // [text](url) — a normal link, not reversed
		}
		out = append(out, revMatch{
			col0:     s,
			matchEnd: e,
			text:     orig[m[2]:m[3]],
			url:      orig[m[4]:m[5]],
		})
	}
	return out
}

// skipPredicate returns a test for whether a 1-based line must not be
// scanned for the reversed pattern: it falls inside a fenced/indented
// code block, a processing-instruction marker, or an include/catalog
// generated body. The code-block and PI line sets are already built and
// cached on f, so they are consulted directly; generated ranges are few
// per file, so they are tested by containment rather than expanded into
// a per-line map (which would be O(section lines) on every call).
func (r *Rule) skipPredicate(f *lint.File) func(int) bool {
	codeLines := lint.CollectCodeBlockLines(f)
	piLines := lint.CollectPIBlockLines(f)
	ranges := f.GeneratedRanges
	return func(ln int) bool {
		if codeLines[ln] || piLines[ln] {
			return true
		}
		for _, gr := range ranges {
			if gr.Contains(ln) {
				return true
			}
		}
		return false
	}
}

type byteRange struct{ start, end int } // absolute, half-open

// collectCodeSpanRanges returns the source byte ranges of every code
// span's content so reversedInLine can blank them before matching.
func collectCodeSpanRanges(f *lint.File) []byteRange {
	var ranges []byteRange
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		cs, ok := n.(*ast.CodeSpan)
		if !ok {
			return ast.WalkContinue, nil
		}
		first, last := -1, -1
		for c := cs.FirstChild(); c != nil; c = c.NextSibling() {
			if t, ok := c.(*ast.Text); ok {
				if first == -1 || t.Segment.Start < first {
					first = t.Segment.Start
				}
				if t.Segment.Stop > last {
					last = t.Segment.Stop
				}
			}
		}
		if first >= 0 && last > first {
			ranges = append(ranges, byteRange{first, last})
		}
		return ast.WalkContinue, nil
	})
	return ranges
}

// maskLine returns line with any bytes inside a code-span range replaced
// by spaces. The original slice is returned unchanged when no range
// overlaps so the common path allocates nothing.
func maskLine(line []byte, lineStart int, ranges []byteRange) []byte {
	lineEnd := lineStart + len(line)
	var out []byte
	for _, rg := range ranges {
		if rg.end <= lineStart || rg.start >= lineEnd {
			continue
		}
		if out == nil {
			out = make([]byte, len(line))
			copy(out, line)
		}
		from := rg.start - lineStart
		to := rg.end - lineStart
		if from < 0 {
			from = 0
		}
		if to > len(out) {
			to = len(out)
		}
		for k := from; k < to; k++ {
			out[k] = ' '
		}
	}
	if out == nil {
		return line
	}
	return out
}

// computeLineStarts returns s where s[i] is the 0-based offset in src
// of the first byte of line i+1; len(s) equals bytes.Split(src,"\n")
// length. The initial cap of `bytes.Count(src, "\n") + 1` lets the
// loop append into the right-sized backing without geometric grows,
// which the engine-bench profile flagged as ~8 grow-allocs per call
// for the cap-0 starting slice.
func computeLineStarts(src []byte) []int {
	starts := make([]int, 1, bytes.Count(src, newline)+1)
	for i, b := range src {
		if b == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}

var newline = []byte{'\n'}

var _ rule.FixableRule = (*Rule)(nil)
