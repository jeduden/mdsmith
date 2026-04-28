// Package ambiguousemphasis implements MDS047, which flags emphasis
// runs whose meaning a human cannot predict at a glance. The rule
// scans raw source bytes for three shapes that CommonMark resolves
// deterministically but rarely match author intent:
//
//   - delimiter runs longer than max-run
//   - backslash-escaped delimiters adjacent to a run (`*\*`, `_\_`)
//   - the same delimiter appearing three times on a line with
//     non-whitespace between the occurrences (`*a*b*c`, `__a__b__`)
//
// The rule is disabled by default. Activation flips at least one of
// the three knobs to a non-zero / true value.
package ambiguousemphasis

import (
	"fmt"
	"sort"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/settings"
	"github.com/yuin/goldmark/ast"
)

func init() {
	rule.Register(&Rule{})
}

// Rule flags ambiguous emphasis sequences. Each detector is gated by
// its own setting and contributes at most one diagnostic per
// (char, length) shape per line, so symmetric openers and closers
// collapse into a single report.
type Rule struct {
	MaxRun                  int
	ForbidEscapedInRun      bool
	ForbidAdjacentSameDelim bool
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS047" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "ambiguous-emphasis" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "meta" }

// EnabledByDefault implements rule.Defaultable.
func (r *Rule) EnabledByDefault() bool { return false }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	if !r.active() {
		return nil
	}

	skip := lint.CollectCodeBlockLines(f)
	codeSpanRanges := collectCodeSpanRanges(f)

	var diags []lint.Diagnostic
	for i, line := range f.Lines {
		lineNum := i + 1
		if skip[lineNum] {
			continue
		}
		masked := maskCodeSpans(line, lineNum, codeSpanRanges, f)
		diags = append(diags, r.checkLine(f, lineNum, masked)...)
	}

	sort.SliceStable(diags, func(i, j int) bool {
		if diags[i].Line != diags[j].Line {
			return diags[i].Line < diags[j].Line
		}
		return diags[i].Column < diags[j].Column
	})
	return diags
}

func (r *Rule) active() bool {
	return r.MaxRun > 0 || r.ForbidEscapedInRun || r.ForbidAdjacentSameDelim
}

// run is a contiguous block of one delimiter character.
type emphRun struct {
	char  byte
	start int // 0-based byte offset within the line
	end   int // exclusive
}

func (e emphRun) length() int { return e.end - e.start }

// escape records a backslash-escaped delimiter at pos (the '\' byte).
type escape struct {
	char byte
	pos  int
}

// scanLine walks line bytes tracking backslash-escape state and
// returns the unescaped delimiter runs and escaped-delimiter
// positions.
func scanLine(line []byte) ([]emphRun, []escape) {
	var runs []emphRun
	var escapes []escape

	var cur *emphRun
	closeRun := func() {
		if cur != nil {
			runs = append(runs, *cur)
			cur = nil
		}
	}

	escaped := false
	for i := 0; i < len(line); i++ {
		b := line[i]
		if escaped {
			if b == '*' || b == '_' {
				escapes = append(escapes, escape{char: b, pos: i - 1})
			}
			escaped = false
			closeRun()
			continue
		}
		switch b {
		case '\\':
			escaped = true
			closeRun()
		case '*', '_':
			if cur != nil && cur.char == b {
				cur.end = i + 1
			} else {
				closeRun()
				cur = &emphRun{char: b, start: i, end: i + 1}
			}
		default:
			closeRun()
		}
	}
	closeRun()
	return runs, escapes
}

func (r *Rule) checkLine(f *lint.File, lineNum int, line []byte) []lint.Diagnostic {
	runs, escapes := scanLine(line)

	var diags []lint.Diagnostic
	if r.MaxRun > 0 {
		diags = append(diags, r.longRunDiags(f, lineNum, runs)...)
	}
	if r.ForbidEscapedInRun {
		diags = append(diags, r.escapedInRunDiags(f, lineNum, runs, escapes)...)
	}
	if r.ForbidAdjacentSameDelim {
		diags = append(diags, r.adjacentSameDelimDiags(f, lineNum, line, runs)...)
	}
	return diags
}

// longRunDiags emits one diagnostic per unique (char, length) of any
// run that exceeds MaxRun, anchored at the first occurrence on the
// line.
func (r *Rule) longRunDiags(f *lint.File, lineNum int, runs []emphRun) []lint.Diagnostic {
	type key struct {
		char   byte
		length int
	}
	seen := map[key]bool{}
	var diags []lint.Diagnostic
	for _, run := range runs {
		length := run.length()
		if length <= r.MaxRun {
			continue
		}
		k := key{char: run.char, length: length}
		if seen[k] {
			continue
		}
		seen[k] = true
		diags = append(diags, lint.Diagnostic{
			File:     f.Path,
			Line:     lineNum,
			Column:   run.start + 1,
			RuleID:   r.ID(),
			RuleName: r.Name(),
			Severity: lint.Warning,
			Message: fmt.Sprintf(
				"emphasis run of %d delimiters; max is %d",
				length, r.MaxRun,
			),
		})
	}
	return diags
}

// escapedInRunDiags emits one diagnostic for each escaped delimiter
// that sits immediately after an unescaped run of the same character.
func (r *Rule) escapedInRunDiags(f *lint.File, lineNum int, runs []emphRun, escapes []escape) []lint.Diagnostic {
	var diags []lint.Diagnostic
	for _, e := range escapes {
		matched := false
		for _, run := range runs {
			if run.char == e.char && run.end == e.pos {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		diags = append(diags, lint.Diagnostic{
			File:     f.Path,
			Line:     lineNum,
			Column:   e.pos + 1,
			RuleID:   r.ID(),
			RuleName: r.Name(),
			Severity: lint.Warning,
			Message:  "escaped delimiter inside emphasis run",
		})
	}
	return diags
}

// adjacentSameDelimDiags emits one diagnostic per unique (char,
// length) where three runs of that shape appear on the line with at
// least one non-whitespace byte between consecutive occurrences.
func (r *Rule) adjacentSameDelimDiags(f *lint.File, lineNum int, line []byte, runs []emphRun) []lint.Diagnostic {
	type key struct {
		char   byte
		length int
	}
	emitted := map[key]bool{}
	var diags []lint.Diagnostic
	for i, r1 := range runs {
		k := key{char: r1.char, length: r1.length()}
		if emitted[k] {
			continue
		}
		var r2 emphRun
		found2 := false
		for j := i + 1; j < len(runs); j++ {
			rj := runs[j]
			if rj.char != k.char || rj.length() != k.length {
				continue
			}
			if !gapNonEmptyAllNonWhitespace(line[r1.end:rj.start]) {
				continue
			}
			r2 = rj
			found2 = true
			startK := j + 1
			for m := startK; m < len(runs); m++ {
				rm := runs[m]
				if rm.char != k.char || rm.length() != k.length {
					continue
				}
				if !gapNonEmptyAllNonWhitespace(line[r2.end:rm.start]) {
					continue
				}
				emitted[k] = true
				diags = append(diags, lint.Diagnostic{
					File:     f.Path,
					Line:     lineNum,
					Column:   r1.start + 1,
					RuleID:   r.ID(),
					RuleName: r.Name(),
					Severity: lint.Warning,
					Message:  "adjacent same-delimiter emphasis is ambiguous",
				})
				break
			}
			if emitted[k] {
				break
			}
		}
		_ = found2
	}
	return diags
}

// gapNonEmptyAllNonWhitespace reports whether b is non-empty and
// contains no whitespace. The adjacent-same-delim detector treats a
// gap that contains a space as a clean separation: CommonMark's
// flanking rules then resolve the runs unambiguously, so the rule
// stays silent.
func gapNonEmptyAllNonWhitespace(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	for _, c := range b {
		if c == ' ' || c == '\t' {
			return false
		}
	}
	return true
}

// codeSpanRange records the inclusive byte range of one CodeSpan's
// delimited content within f.Source.
type codeSpanRange struct {
	start int // absolute byte offset in source, inclusive
	end   int // absolute byte offset in source, exclusive
}

// collectCodeSpanRanges walks the AST and returns the byte ranges of
// every code span's text content. CodeSpan nodes do not expose a
// segment directly, so the range spans from the first text-child
// segment start to the last text-child segment end.
func collectCodeSpanRanges(f *lint.File) []codeSpanRange {
	var ranges []codeSpanRange
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		cs, ok := n.(*ast.CodeSpan)
		if !ok {
			return ast.WalkContinue, nil
		}
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
		if first >= 0 && last > first {
			ranges = append(ranges, codeSpanRange{start: first, end: last})
		}
		return ast.WalkContinue, nil
	})
	return ranges
}

// maskCodeSpans returns a copy of line with bytes that lie inside a
// code span replaced by a space. Spaces never participate in emphasis
// detection, so the mask removes the bytes from delimiter accounting
// without disturbing column positions.
func maskCodeSpans(line []byte, lineNum int, ranges []codeSpanRange, f *lint.File) []byte {
	if len(ranges) == 0 {
		return line
	}
	lineStart := lineStartOffset(f, lineNum)
	lineEnd := lineStart + len(line)

	var out []byte
	for _, r := range ranges {
		if r.end <= lineStart || r.start >= lineEnd {
			continue
		}
		if out == nil {
			out = make([]byte, len(line))
			copy(out, line)
		}
		from := r.start - lineStart
		to := r.end - lineStart
		if from < 0 {
			from = 0
		}
		if to > len(out) {
			to = len(out)
		}
		for i := from; i < to; i++ {
			out[i] = ' '
		}
	}
	if out == nil {
		return line
	}
	return out
}

// lineStartOffset returns the 0-based byte offset in f.Source of the
// first character of the given 1-based line.
func lineStartOffset(f *lint.File, lineNum int) int {
	if lineNum <= 1 {
		return 0
	}
	count := 1
	for i := 0; i < len(f.Source); i++ {
		if f.Source[i] == '\n' {
			count++
			if count == lineNum {
				return i + 1
			}
		}
	}
	return len(f.Source)
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(s map[string]any) error {
	for k, v := range s {
		switch k {
		case "max-run":
			n, ok := settings.ToInt(v)
			if !ok {
				return fmt.Errorf("ambiguous-emphasis: max-run must be an integer, got %T", v)
			}
			if n < 0 {
				return fmt.Errorf("ambiguous-emphasis: max-run must be non-negative, got %d", n)
			}
			r.MaxRun = n
		case "forbid-escaped-in-run":
			b, ok := v.(bool)
			if !ok {
				return fmt.Errorf("ambiguous-emphasis: forbid-escaped-in-run must be a bool, got %T", v)
			}
			r.ForbidEscapedInRun = b
		case "forbid-adjacent-same-delim":
			b, ok := v.(bool)
			if !ok {
				return fmt.Errorf("ambiguous-emphasis: forbid-adjacent-same-delim must be a bool, got %T", v)
			}
			r.ForbidAdjacentSameDelim = b
		default:
			return fmt.Errorf("ambiguous-emphasis: unknown setting %q", k)
		}
	}
	return nil
}

// DefaultSettings implements rule.Configurable. The defaults make the
// rule a no-op so it can ship registered but disabled; profile
// activation supplies the active values.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"max-run":                    0,
		"forbid-escaped-in-run":      false,
		"forbid-adjacent-same-delim": false,
	}
}

var (
	_ rule.Configurable = (*Rule)(nil)
	_ rule.Defaultable  = (*Rule)(nil)
)
