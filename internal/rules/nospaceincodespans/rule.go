// Package nospaceincodespans implements MDS049, which flags inline code spans
// with leading or trailing whitespace inside the backticks. The CommonMark
// "balanced single-space trim" case (` x ` with exactly one space on each
// side) is exempt; every other leading or trailing whitespace pattern emits
// a diagnostic.
//
// Goldmark applies CommonMark's trim-one-space rule before exposing the code
// span body via its text-child segments. To inspect the RAW bytes between
// the delimiters (which is what this rule needs), the implementation backs
// up from the first segment start and forward from the last segment stop
// until it finds the enclosing backtick delimiters, then reads the slice of
// source between those delimiters.
package nospaceincodespans

import (
	"bytes"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/yuin/goldmark/ast"
)

func init() {
	rule.Register(&Rule{})
}

// Rule implements MDS049.
type Rule struct{}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS049" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "no-space-in-code-spans" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "whitespace" }

// EnabledByDefault implements rule.Defaultable. MDS049 is opt-in.
func (r *Rule) EnabledByDefault() bool { return false }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	var diags []lint.Diagnostic

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		cs, ok := n.(*ast.CodeSpan)
		if !ok {
			return ast.WalkContinue, nil
		}

		rawStart, rawEnd := rawBodyOffsets(cs, f.Source)
		if rawStart < 0 || rawEnd <= rawStart {
			return ast.WalkContinue, nil
		}
		body := f.Source[rawStart:rawEnd]

		// CommonMark single-space-trim exception:
		// exactly one ASCII space on each side, with non-whitespace content.
		if isBalancedSingleSpace(body) {
			return ast.WalkContinue, nil
		}

		// Report position at the opening backtick of the span.
		offset := rawStart
		for offset > 0 && f.Source[offset-1] == '`' {
			offset--
		}

		if isWhitespace(body[0]) {
			line, col := lineColOfOffset(f.Source, offset)
			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     line,
				Column:   col,
				RuleID:   r.ID(),
				RuleName: r.Name(),
				Severity: lint.Warning,
				Message:  "code span has leading whitespace",
			})
		}

		if isWhitespace(body[len(body)-1]) {
			line, col := lineColOfOffset(f.Source, offset)
			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     line,
				Column:   col,
				RuleID:   r.ID(),
				RuleName: r.Name(),
				Severity: lint.Warning,
				Message:  "code span has trailing whitespace",
			})
		}

		return ast.WalkContinue, nil
	})

	return diags
}

// spanReplacement records a single code-span body that needs whitespace trimmed.
type spanReplacement struct {
	start   int // byte offset of raw body start in source
	end     int // byte offset of raw body end in source (exclusive)
	trimmed []byte
}

// collectReplacements walks the AST and returns all code-span bodies that
// should have leading/trailing whitespace removed.
func collectReplacements(f *lint.File) []spanReplacement {
	var out []spanReplacement
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		cs, ok := n.(*ast.CodeSpan)
		if !ok {
			return ast.WalkContinue, nil
		}
		rawStart, rawEnd := rawBodyOffsets(cs, f.Source)
		if rawStart < 0 || rawEnd <= rawStart {
			return ast.WalkContinue, nil
		}
		body := f.Source[rawStart:rawEnd]
		if isBalancedSingleSpace(body) {
			return ast.WalkContinue, nil
		}
		trimmed := bytes.TrimFunc(body, func(r rune) bool {
			return r == ' ' || r == '\t' || r == '\n' || r == '\r'
		})
		if len(trimmed) == 0 || bytes.Equal(trimmed, body) {
			return ast.WalkContinue, nil
		}
		out = append(out, spanReplacement{start: rawStart, end: rawEnd, trimmed: trimmed})
		return ast.WalkContinue, nil
	})
	return out
}

// Fix implements rule.FixableRule. It trims leading and trailing whitespace
// from each code span's body while preserving the delimiter count.
// When the trimmed body would be empty, the span is left unchanged.
func (r *Rule) Fix(f *lint.File) []byte {
	replacements := collectReplacements(f)
	if len(replacements) == 0 {
		result := make([]byte, len(f.Source))
		copy(result, f.Source)
		return result
	}
	var result []byte
	prev := 0
	for _, rep := range replacements {
		result = append(result, f.Source[prev:rep.start]...)
		result = append(result, rep.trimmed...)
		prev = rep.end
	}
	result = append(result, f.Source[prev:]...)
	return result
}

// rawBodyOffsets returns the start (inclusive) and end (exclusive) byte
// offsets of the raw code span body in source — i.e. the bytes between
// the opening and closing backtick delimiter runs.
//
// Goldmark strips one ASCII space from each side of a code span body when
// both sides have a space (CommonMark trim). We recover the original bytes
// by backing up from the segment start until we reach the delimiter backtick,
// and advancing from the segment stop until we reach the closing backtick.
func rawBodyOffsets(cs *ast.CodeSpan, source []byte) (start, end int) {
	// Collect the first and last segment positions from text children.
	segStart := -1
	segEnd := -1
	for c := cs.FirstChild(); c != nil; c = c.NextSibling() {
		t, ok := c.(*ast.Text)
		if !ok {
			continue
		}
		if segStart < 0 || t.Segment.Start < segStart {
			segStart = t.Segment.Start
		}
		if t.Segment.Stop > segEnd {
			segEnd = t.Segment.Stop
		}
	}
	if segStart < 0 {
		return -1, -1
	}

	// Back up from segStart until we find the opening backtick.
	rawStart := segStart
	for rawStart > 0 && source[rawStart-1] != '`' {
		rawStart--
	}

	// Advance from segEnd until we find the closing backtick.
	rawEnd := segEnd
	for rawEnd < len(source) && source[rawEnd] != '`' {
		rawEnd++
	}

	return rawStart, rawEnd
}

// isBalancedSingleSpace returns true when body is the CommonMark
// single-space-trim case: exactly one ASCII space on each side with
// non-whitespace content between them (so the renderer strips both).
func isBalancedSingleSpace(body []byte) bool {
	if len(body) < 2 {
		return false
	}
	if body[0] != ' ' || body[len(body)-1] != ' ' {
		return false
	}
	// The inner bytes must not start or end with whitespace.
	// (A body like "   " would have body[1]==' ' so it is not the trim case.)
	if len(body) < 3 {
		// Only two bytes, both spaces — e.g. "  ". Not the trim case.
		return false
	}
	inner := body[1 : len(body)-1]
	return !isWhitespace(inner[0]) && !isWhitespace(inner[len(inner)-1])
}

// isWhitespace returns true for ASCII whitespace bytes.
func isWhitespace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

// lineColOfOffset converts a byte offset to 1-based line and column numbers.
func lineColOfOffset(source []byte, offset int) (line, col int) {
	line = 1
	lineStart := 0
	for i := 0; i < offset && i < len(source); i++ {
		if source[i] == '\n' {
			line++
			lineStart = i + 1
		}
	}
	col = offset - lineStart + 1
	return
}

var (
	_ rule.Defaultable = (*Rule)(nil)
)
