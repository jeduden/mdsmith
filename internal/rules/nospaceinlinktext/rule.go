package nospaceinlinktext

import (
	"bytes"
	"fmt"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/yuin/goldmark/ast"
)

func init() {
	rule.Register(&Rule{CheckImages: true})
}

// Rule implements MDS049, flagging Markdown links and images whose visible
// text has leading or trailing whitespace inside the brackets.
type Rule struct {
	CheckImages bool
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS049" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "no-space-in-link-text" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "link" }

// EnabledByDefault implements rule.Defaultable. MDS049 is opt-in.
func (r *Rule) EnabledByDefault() bool { return false }

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(settings map[string]any) error {
	for k, v := range settings {
		switch k {
		case "check-images":
			b, ok := v.(bool)
			if !ok {
				return fmt.Errorf("no-space-in-link-text: check-images must be a bool, got %T", v)
			}
			r.CheckImages = b
		default:
			return fmt.Errorf("no-space-in-link-text: unknown setting %q", k)
		}
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"check-images": true,
	}
}

// minTextStart returns the minimum Segment.Start among all *ast.Text descendant
// nodes of n. Returns -1 if none are found.
func minTextStart(n ast.Node) int {
	minStart := -1
	_ = ast.Walk(n, func(child ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if child == n {
			return ast.WalkContinue, nil
		}
		t, ok := child.(*ast.Text)
		if !ok {
			return ast.WalkContinue, nil
		}
		if minStart == -1 || t.Segment.Start < minStart {
			minStart = t.Segment.Start
		}
		return ast.WalkContinue, nil
	})
	return minStart
}

// imageOpener reports whether source[i] == '[' is an image opener, i.e. it is
// immediately preceded by an unescaped '!'. On entry source[i] must be '['.
func imageOpener(source []byte, i int) bool {
	if i == 0 || source[i-1] != '!' {
		return false
	}
	bs := 0
	for j := i - 2; j >= 0 && source[j] == '\\'; j-- {
		bs++
	}
	return bs%2 == 0
}

// skipCodeSpanBackward scans backward past the code span whose closing backtick
// sequence ends at i. Returns the position just before the opening backtick
// sequence, or -1 if no matching opener is found.
func skipCodeSpanBackward(source []byte, i int) int {
	end := i
	for i > 0 && source[i-1] == '`' {
		i--
	}
	n := end - i + 1
	i--
	for i >= 0 {
		if source[i] != '`' {
			i--
			continue
		}
		j := i
		for j > 0 && source[j-1] == '`' {
			j--
		}
		if i-j+1 == n {
			return j - 1
		}
		i = j - 1
	}
	return -1
}

// findOpenBracket scans backward from from-1 to find the opening '[' for n.
// For links it skips image openers (![); for images it requires one.
// Code span contents are skipped so brackets inside backtick spans are ignored.
// Returns -1 if not found.
func findOpenBracket(source []byte, img bool, from int) int {
	for i := from - 1; i >= 0; i-- {
		if source[i] == '`' {
			if j := skipCodeSpanBackward(source, i); j >= 0 {
				i = j + 1 // loop will do i--
			}
			continue
		}
		if source[i] != '[' {
			continue
		}
		bs := 0
		for j := i - 1; j >= 0 && source[j] == '\\'; j-- {
			bs++
		}
		if bs%2 == 1 {
			continue // escaped [
		}
		if img != imageOpener(source, i) {
			continue // wrong opener type
		}
		return i
	}
	return -1
}

// skipCodeSpan advances past the backtick-delimited code span starting at i.
// Returns the index after the closing backtick sequence, or len(source) if no
// matching closer is found. On entry source[i] must be '`'.
func skipCodeSpan(source []byte, i int) int {
	n := 0
	for i+n < len(source) && source[i+n] == '`' {
		n++
	}
	i += n
	for i < len(source) {
		if source[i] != '`' {
			i++
			continue
		}
		j := i
		for j < len(source) && source[j] == '`' {
			j++
		}
		if j-i == n {
			return j
		}
		i = j
	}
	return i
}

// findCloseBracket scans forward from open+1 to find the matching ']'.
// Backslash-escaped bytes and code-span contents are skipped so structural
// brackets inside code spans or after '\' do not affect depth counting.
// Returns -1 if the bracket is unmatched.
func findCloseBracket(source []byte, open int) int {
	depth := 1
	i := open + 1
	for i < len(source) && depth > 0 {
		switch source[i] {
		case '`':
			if j := skipCodeSpan(source, i); j < len(source) {
				i = j
				continue
			}
			// Unmatched backtick sequence: treat as literal text so bracket
			// scanning continues and finds the structural ] normally.
		case '\\':
			if i+1 < len(source) {
				i += 2
				continue
			}
		case '[':
			depth++
		case ']':
			depth--
		}
		i++
	}
	if depth != 0 {
		return -1
	}
	return i - 1
}

// bracketSpan returns the byte range of the opening '[' and its matching ']'
// for a link or image node. Returns (-1, -1) if the span cannot be determined.
func bracketSpan(n ast.Node, source []byte) (open, close int) {
	minStart := minTextStart(n)
	if minStart == -1 {
		return -1, -1
	}
	img := isImage(n)
	openBracket := findOpenBracket(source, img, minStart)
	if openBracket == -1 {
		return -1, -1
	}
	closeBracket := findCloseBracket(source, openBracket)
	if closeBracket == -1 {
		return -1, -1
	}
	return openBracket, closeBracket
}

// isImage reports whether n is an *ast.Image.
func isImage(n ast.Node) bool {
	_, ok := n.(*ast.Image)
	return ok
}

type span struct {
	open  int
	close int
	img   bool
}

// collectSpans walks the AST and returns all bracket spans needing inspection.
func (r *Rule) collectSpans(f *lint.File) []span {
	var spans []span
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n.(type) {
		case *ast.Link, *ast.Image:
		default:
			return ast.WalkContinue, nil
		}
		img := isImage(n)
		if img && !r.CheckImages {
			return ast.WalkContinue, nil
		}
		open, close := bracketSpan(n, f.Source)
		if open == -1 {
			return ast.WalkContinue, nil
		}
		line := f.LineOfOffset(open)
		for _, gr := range f.GeneratedRanges {
			if gr.Contains(line) {
				return ast.WalkContinue, nil
			}
		}
		spans = append(spans, span{open: open, close: close, img: img})
		return ast.WalkContinue, nil
	})
	return spans
}

// Check implements rule.Rule. The per-link/image logic is pure and
// stateless, so it is expressed as CheckNode and the engine can fold
// this rule into one shared AST walk; a direct call still works via
// rule.WalkNodes.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	return rule.WalkNodes(r, f)
}

// CheckNode implements rule.NodeChecker.
func (r *Rule) CheckNode(n ast.Node, entering bool, f *lint.File) []lint.Diagnostic {
	if !entering {
		return nil
	}
	s, ok := r.spanForNode(n, f)
	if !ok {
		return nil
	}
	return diagsForSpan(s, f, r.ID(), r.Name())
}

// spanForNode returns the bracket span for a Link or Image node, or
// false if the node should be skipped (not a Link/Image, images
// disabled, undetectable bracket position, or located in a generated
// section).
func (r *Rule) spanForNode(n ast.Node, f *lint.File) (span, bool) {
	switch n.(type) {
	case *ast.Link, *ast.Image:
	default:
		return span{}, false
	}
	img := isImage(n)
	if img && !r.CheckImages {
		return span{}, false
	}
	open, close := bracketSpan(n, f.Source)
	if open == -1 {
		return span{}, false
	}
	line := f.LineOfOffset(open)
	for _, gr := range f.GeneratedRanges {
		if gr.Contains(line) {
			return span{}, false
		}
	}
	return span{open: open, close: close, img: img}, true
}

// diagsForSpan returns the leading/trailing-whitespace diagnostics
// for a single bracket span. Returns nil when neither end is flagged.
func diagsForSpan(s span, f *lint.File, ruleID, ruleName string) []lint.Diagnostic {
	inner := f.Source[s.open+1 : s.close]
	if s.img && len(bytes.TrimSpace(inner)) == 0 {
		return nil // whitespace-only image alt; leave to MDS032
	}
	if len(inner) == 0 {
		return nil
	}
	role := "link text"
	if s.img {
		role = "image alt text"
	}
	first := inner[0]
	last := inner[len(inner)-1]
	var diags []lint.Diagnostic
	if first == ' ' || first == '\t' {
		diags = append(diags, lint.Diagnostic{
			File:     f.Path,
			Line:     f.LineOfOffset(s.open + 1),
			Column:   f.ColumnOfOffset(s.open + 1),
			RuleID:   ruleID,
			RuleName: ruleName,
			Severity: lint.Warning,
			Message:  role + " has leading whitespace",
		})
	}
	if last == ' ' || last == '\t' {
		diags = append(diags, lint.Diagnostic{
			File:     f.Path,
			Line:     f.LineOfOffset(s.close - 1),
			Column:   f.ColumnOfOffset(s.close - 1),
			RuleID:   ruleID,
			RuleName: ruleName,
			Severity: lint.Warning,
			Message:  role + " has trailing whitespace",
		})
	}
	return diags
}

// Fix implements rule.FixableRule. Trims leading/trailing space/tab inside
// each bracket pair while leaving the surrounding markdown structure intact.
// Nested link/image brackets are fixed in a single pass.
func (r *Rule) Fix(f *lint.File) []byte {
	return fixSpans(f.Source, r.collectSpans(f), 0, len(f.Source))
}

// fixSpans builds the fixed output for source[from:to] by trimming each
// collected span whose opening bracket falls in [from, to). Nested spans are
// fixed recursively before the outer boundary is trimmed, so both outer and
// inner whitespace are removed in a single call.
func fixSpans(source []byte, spans []span, from, to int) []byte {
	var result []byte
	prev := from
	for i, s := range spans {
		if s.open < from || s.open >= to || s.open < prev {
			continue
		}
		result = append(result, source[prev:s.open+1]...) // up to and including [
		inner := fixSpans(source, spans[i+1:], s.open+1, s.close)
		trimmed := trimSpaceTab(inner)
		if s.img && len(bytes.TrimSpace(inner)) == 0 {
			result = append(result, inner...) // whitespace-only image alt; leave to MDS032
		} else {
			result = append(result, trimmed...)
		}
		prev = s.close
	}
	result = append(result, source[prev:to]...)
	return result
}

// trimSpaceTab trims leading and trailing space/tab bytes (not newlines).
func trimSpaceTab(b []byte) []byte {
	start := 0
	for start < len(b) && (b[start] == ' ' || b[start] == '\t') {
		start++
	}
	end := len(b)
	for end > start && (b[end-1] == ' ' || b[end-1] == '\t') {
		end--
	}
	return b[start:end]
}

var (
	_ rule.Configurable = (*Rule)(nil)
	_ rule.Defaultable  = (*Rule)(nil)
	_ rule.FixableRule  = (*Rule)(nil)
	_ rule.NodeChecker  = (*Rule)(nil)
)
