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

// Rule checks that inline code spans do not have leading or trailing
// whitespace inside the backtick delimiters.
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

		body, bodyStart, _ := codeSpanSegment(cs, f.Source)
		if bodyStart < 0 {
			return ast.WalkContinue, nil
		}

		if isWhitespace(body[0]) {
			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     f.LineOfOffset(bodyStart),
				Column:   f.ColumnOfOffset(bodyStart),
				RuleID:   r.ID(),
				RuleName: r.Name(),
				Severity: lint.Warning,
				Message:  "code span has leading whitespace",
			})
		}

		if isWhitespace(body[len(body)-1]) {
			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     f.LineOfOffset(bodyStart),
				Column:   f.ColumnOfOffset(bodyStart),
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

type fixRange struct {
	rawStart int
	rawEnd   int
	trimmed  []byte
}

// Fix implements rule.FixableRule. It trims leading and trailing
// whitespace from the raw bytes between backtick delimiters. When
// trimming produces an empty span, no fix is applied.
func (r *Rule) Fix(f *lint.File) []byte {
	fixes := collectFixes(f)
	if len(fixes) == 0 {
		return f.Source
	}

	result := make([]byte, 0, len(f.Source))
	prev := 0
	for _, fr := range fixes {
		result = append(result, f.Source[prev:fr.rawStart]...)
		result = append(result, fr.trimmed...)
		prev = fr.rawEnd
	}
	result = append(result, f.Source[prev:]...)
	return result
}

func collectFixes(f *lint.File) []fixRange {
	var fixes []fixRange

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		cs, ok := n.(*ast.CodeSpan)
		if !ok {
			return ast.WalkContinue, nil
		}

		body, _, _ := codeSpanSegment(cs, f.Source)
		if body == nil {
			return ast.WalkContinue, nil
		}

		if !isWhitespace(body[0]) && !isWhitespace(body[len(body)-1]) {
			return ast.WalkContinue, nil
		}

		trimmed := bytes.TrimFunc(body, func(r rune) bool {
			return isWhitespace(byte(r))
		})
		if len(trimmed) == 0 {
			return ast.WalkContinue, nil
		}

		rawStart, rawEnd := findRawBodyRange(cs, f.Source)
		if rawStart < 0 {
			return ast.WalkContinue, nil
		}

		fixes = append(fixes, fixRange{
			rawStart: rawStart,
			rawEnd:   rawEnd,
			trimmed:  trimmed,
		})

		return ast.WalkContinue, nil
	})

	return fixes
}

func isWhitespace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

func codeSpanSegment(cs *ast.CodeSpan, src []byte) (body []byte, start, end int) {
	first := -1
	last := -1
	for c := cs.FirstChild(); c != nil; c = c.NextSibling() {
		t, ok := c.(*ast.Text)
		if !ok {
			continue
		}
		if first == -1 || t.Segment.Start < first {
			first = t.Segment.Start
		}
		if t.Segment.Stop > last {
			last = t.Segment.Stop
		}
	}
	if first < 0 || last < 0 || last <= first {
		return nil, -1, -1
	}
	return src[first:last], first, last
}

func findRawBodyRange(cs *ast.CodeSpan, src []byte) (start, end int) {
	_, segStart, segEnd := codeSpanSegment(cs, src)
	if segStart < 0 {
		return -1, -1
	}

	bodyStart, delimLen := findBodyStart(src, segStart)
	if delimLen == 0 {
		return -1, -1
	}

	bodyEnd := findBodyEnd(src, segEnd, delimLen)
	if bodyEnd < 0 {
		return -1, -1
	}

	return bodyStart, bodyEnd
}

func findBodyStart(src []byte, segStart int) (bodyStart, delimLen int) {
	pos := segStart - 1
	for pos >= 0 && isWhitespace(src[pos]) {
		pos--
	}
	if pos < 0 || src[pos] != '`' {
		return -1, 0
	}
	backtickEnd := pos + 1
	for pos >= 0 && src[pos] == '`' {
		pos--
	}
	n := backtickEnd - (pos + 1)
	return backtickEnd, n
}

func findBodyEnd(src []byte, segEnd, delimLen int) int {
	pos := segEnd
	for pos < len(src) && isWhitespace(src[pos]) {
		pos++
	}
	if pos+delimLen > len(src) {
		return -1
	}
	for i := 0; i < delimLen; i++ {
		if src[pos+i] != '`' {
			return -1
		}
	}
	return pos
}

var _ rule.Defaultable = (*Rule)(nil)
