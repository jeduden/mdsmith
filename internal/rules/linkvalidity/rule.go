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
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
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
// Direct recursion (entering visits only) replaces ast.Walk: the rule
// only reacts to two node types, and the closure-driven double visit
// per node was measurable on every file.
//
// On the parse-skipped path (f.AST nil) the document tree is unavailable,
// so each inline-bearing Layer 0 block span is parsed in isolation and the
// same node recursion runs over it, with span-local segment offsets mapped
// back to the document via the span's start offset. Re-using goldmark's
// parser per span reproduces the link/image nodes byte-identically.
func (r *Rule) checkEmpty(f *lint.File) []lint.Diagnostic {
	var diags []lint.Diagnostic
	if f.AST != nil {
		r.checkEmptyNode(f.AST, f, 0, &diags)
		return diags
	}
	// On the parse-skipped path the document tree is unavailable, so the
	// same per-node check runs over the shared run-grouped inline parse,
	// with run-local segment offsets mapped to the document via base.
	lint.WalkInlineNodes(f, func(n ast.Node, base int) {
		r.checkEmptyNodeOne(n, f, base, &diags)
	})
	return diags
}

// checkEmptyNode recursively applies checkEmptyNodeOne over n's subtree on
// the AST path, where offsets are document-absolute (base zero).
func (r *Rule) checkEmptyNode(n ast.Node, f *lint.File, base int, diags *[]lint.Diagnostic) {
	if n == nil {
		return
	}
	r.checkEmptyNodeOne(n, f, base, diags)
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		r.checkEmptyNode(c, f, base, diags)
	}
}

// checkEmptyNodeOne flags a single Link or Image node with an empty/`#`
// destination, or (links only) empty visible text. base maps the node's
// segment offsets to the document: zero on the AST path, the run's start
// offset on the inline-walk path (where WalkInlineNodes already visits
// every node, so no extra recursion is needed here).
func (r *Rule) checkEmptyNodeOne(n ast.Node, f *lint.File, base int, diags *[]lint.Diagnostic) {
	switch node := n.(type) {
	case *ast.Image:
		if emptyDestination(node.Destination) {
			*diags = append(*diags, r.diag(f, nodeLine(node, f, base), "empty image destination"))
		}
	case *ast.Link:
		switch {
		case emptyDestination(node.Destination):
			*diags = append(*diags, r.diag(f, nodeLine(node, f, base), "empty link destination"))
		case !hasVisibleContent(node, f.Source, base):
			*diags = append(*diags, r.diag(f, nodeLine(node, f, base), "empty link text"))
		}
	}
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
	return len(t) == 0 || (len(t) == 1 && t[0] == '#')
}

// hasVisibleContent reports whether the link renders any visible
// content: an image, code span, autolink, raw HTML, or non-whitespace
// text anywhere in its subtree. base maps span-local Text segment offsets
// to the document on the per-block path (zero on the AST path).
func hasVisibleContent(link *ast.Link, source []byte, base int) bool {
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
			if len(bytes.TrimSpace(source[base+t.Segment.Start:base+t.Segment.Stop])) > 0 {
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
// and falls back to the nearest block ancestor's first line. base maps
// span-local offsets to the document on the per-block path (zero on the
// AST path).
func nodeLine(n ast.Node, f *lint.File, base int) int {
	if ln := firstTextLine(n, f, base); ln > 0 {
		return ln
	}
	for p := n.Parent(); p != nil; p = p.Parent() {
		if isInlineNode(p) {
			continue
		}
		lines := p.Lines()
		if lines != nil && lines.Len() > 0 {
			return f.LineOfOffset(base + lines.At(0).Start)
		}
	}
	return 1
}

func firstTextLine(n ast.Node, f *lint.File, base int) int {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			return f.LineOfOffset(base + t.Segment.Start)
		}
		if ln := firstTextLine(c, f, base); ln > 0 {
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

// reversedNeedle is the two-byte sequence every reversedRe match must
// contain (nothing may sit between `)` and `[`). Lines without it —
// nearly every line — skip the skip-predicate, the code-span masking,
// and the regex entirely. Masking only blanks bytes, so it can never
// introduce the needle on a line that lacked it.
var reversedNeedle = []byte(")[")

func (r *Rule) checkReversed(f *lint.File) []lint.Diagnostic {
	skip := r.skipPredicate(f)
	csRanges := f.CodeSpanContentRanges()
	var diags []lint.Diagnostic
	for i, line := range f.Lines {
		if !bytes.Contains(line, reversedNeedle) {
			continue
		}
		ln := i + 1
		if skip(ln) {
			continue
		}
		masked := lint.MaskRanges(line, f.LineStartOffset(i), csRanges)
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
	csRanges := f.CodeSpanContentRanges()
	out := make([][]byte, len(f.Lines))
	for i, line := range f.Lines {
		if !bytes.Contains(line, reversedNeedle) || skip(i+1) {
			out[i] = line
			continue
		}
		masked := lint.MaskRanges(line, f.LineStartOffset(i), csRanges)
		matches := reversedInLine(line, masked)
		if len(matches) == 0 {
			out[i] = line
			continue
		}
		b := make([]byte, 0, len(line))
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
		if lint.InCodeOrPI(codeLines, piLines, ln) {
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

var _ rule.FixableRule = (*Rule)(nil)

// FixTitle implements rule.QuickFixTitler.
func (r *Rule) FixTitle() string { return "Fix reversed link syntax" }
