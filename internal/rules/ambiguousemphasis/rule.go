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
	"bytes"
	"cmp"
	"fmt"
	"slices"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/settings"
)

// starNeedle and underscoreNeedle are single-byte needles for
// bytes.Count, held at package scope so scanLine never allocates a
// slice literal per call (docs/development/high-performance-go.md
// "Compile regexes at package scope" applies equally to byte
// needles).
var (
	starNeedle       = []byte{'*'}
	underscoreNeedle = []byte{'_'}
)

func init() {
	rule.Register(&Rule{})
}

// Rule flags ambiguous emphasis sequences. Each detector is gated by
// its own setting. The long-run and adjacent-same-delim detectors
// dedupe per (char, length) shape per line so symmetric openers and
// closers collapse into one report; the escaped-in-run detector emits
// one diagnostic per matching escape so each ambiguous source position
// is reported.
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
func (r *Rule) Category() string { return "prose" }

// EnabledByDefault implements rule.Defaultable.
func (r *Rule) EnabledByDefault() bool { return false }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	if !r.active() {
		return nil
	}

	skip := lint.CollectCodeBlockLines(f)
	codeSpanRanges := f.CodeSpanContentRanges()

	// Unlike most per-Check diagnostic slices, this one is not pre-sized:
	// len(f.Lines)/4+1 guesses at a violation rate real documents rarely
	// hit (MDS047 fires on ambiguous emphasis, not on every paragraph),
	// so the guess cost an allocation on the common zero-diagnostic
	// path. A nil start also means Check returns nil rather than a
	// non-nil empty slice when nothing is found, per project convention
	// (docs/development/index.md "Return nil, not []T{}").
	var diags []lint.Diagnostic
	for i, line := range f.Lines {
		lineNum := i + 1
		if _, ok := skip[lineNum]; ok {
			continue
		}
		masked := lint.MaskRanges(line, f.LineStartOffset(i), codeSpanRanges)
		diags = append(diags, r.checkLine(f, lineNum, masked)...)
	}

	// slices.SortStableFunc sorts the concrete lint.Diagnostic values
	// directly, unlike sort.SliceStable, which drives reflect.Swapper
	// under the hood — see docs/development/high-performance-go.md's
	// "reflect in hot paths" anti-pattern.
	slices.SortStableFunc(diags, func(a, b lint.Diagnostic) int {
		if a.Line != b.Line {
			return cmp.Compare(a.Line, b.Line)
		}
		return cmp.Compare(a.Column, b.Column)
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
// positions. A cheap byte count gates the walk: a line with no '*' or
// '_' at all can hold no run or escape, so it returns immediately
// without touching the per-byte loop
// (docs/development/high-performance-go.md "Gate expensive analyzers
// behind a cheap pre-check"). When delimiters are present, the count
// also bounds runs' capacity — no run can exceed the total delimiter
// byte count — so the common multi-emphasis line (`**bold** *and*
// _more_`) fills runs via one pre-sized slice instead of growing it
// through repeated append reallocations. Measured against a single
// bytes.IndexAny presence check (one pass, no capacity hint): that
// alternative costs fewer scanned bytes on a delimiter-free line but
// nearly doubles allocs/op on delimiter-bearing lines (21 vs 11 on
// this package's alloc-budget fixture) by forcing runs to grow via
// repeated append — allocation count, not scan count, is the metric
// CLAUDE.md's ceiling and the CI gate enforce, so the two-count form
// wins here.
func scanLine(line []byte) ([]emphRun, []escape) {
	numDelims := bytes.Count(line, starNeedle) + bytes.Count(line, underscoreNeedle)
	if numDelims == 0 {
		return nil, nil
	}

	runs := make([]emphRun, 0, numDelims)
	var escapes []escape

	// cur/open track the in-progress run by value instead of *emphRun:
	// taking the address of a composite literal reassigned across loop
	// iterations forces the compiler to heap-allocate a fresh emphRun
	// per delimiter run (docs/development/high-performance-go.md
	// "Fixed-size arrays beat slices" / avoid unnecessary pointers). A
	// value plus a bool keeps the in-progress run on the stack; only
	// the copy appended to runs is ever observed outside this loop.
	var cur emphRun
	open := false
	closeRun := func() {
		if open {
			runs = append(runs, cur)
			open = false
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
			if open && cur.char == b {
				cur.end = i + 1
			} else {
				closeRun()
				cur = emphRun{char: b, start: i, end: i + 1}
				open = true
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
	if len(runs) == 0 {
		return nil
	}
	type key struct {
		char   byte
		length int
	}
	seen := map[key]struct{}{}
	var diags []lint.Diagnostic
	for _, run := range runs {
		length := run.length()
		if length <= r.MaxRun {
			continue
		}
		k := key{char: run.char, length: length}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
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
	// Without an escape there is nothing to match against a run end, so
	// skip building runEnds — most emphasis-bearing lines have runs but
	// no escapes, and make(map[K]V, len(runs)) with a nonzero hint
	// allocates real bucket storage immediately, unlike a zero-hint map
	// literal (docs/development/high-performance-go.md "Gate expensive
	// analyzers behind a cheap pre-check").
	if len(escapes) == 0 || len(runs) == 0 {
		return nil
	}
	type runEndKey struct {
		char byte
		end  int
	}
	runEnds := make(map[runEndKey]struct{}, len(runs))
	for _, run := range runs {
		runEnds[runEndKey{char: run.char, end: run.end}] = struct{}{}
	}

	var diags []lint.Diagnostic
	for _, e := range escapes {
		if _, ok := runEnds[runEndKey{char: e.char, end: e.pos}]; !ok {
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

// minAdjacentRunsForAmbiguity is the minimum number of same-shape
// delimiter runs separated by non-whitespace before the pattern is
// considered ambiguous.
const minAdjacentRunsForAmbiguity = 3

// adjacentSameDelimDiags emits one diagnostic per unique (char,
// length) where three runs of that shape appear on the line with at
// least one non-whitespace byte between consecutive occurrences.
func (r *Rule) adjacentSameDelimDiags(f *lint.File, lineNum int, line []byte, runs []emphRun) []lint.Diagnostic {
	// Fewer than three runs can never satisfy the three-in-a-row
	// pattern, so skip building the tracking maps entirely — most
	// emphasis-bearing lines (one bold, one italic span) fall well
	// short of this.
	if len(runs) < minAdjacentRunsForAmbiguity {
		return nil
	}
	type key struct {
		char   byte
		length int
	}
	type adjacentRunState struct {
		first emphRun
		last  emphRun
		count int
	}

	emitted := map[key]struct{}{}
	states := make(map[key]adjacentRunState)
	var diags []lint.Diagnostic

	for _, curr := range runs {
		k := key{char: curr.char, length: curr.length()}
		if _, ok := emitted[k]; ok {
			continue
		}

		state, ok := states[k]
		if !ok {
			states[k] = adjacentRunState{
				first: curr,
				last:  curr,
				count: 1,
			}
			continue
		}

		if gapNonEmptyAllNonWhitespace(line[state.last.end:curr.start]) {
			state.last = curr
			state.count++
			states[k] = state
			if state.count >= minAdjacentRunsForAmbiguity {
				emitted[k] = struct{}{}
				diags = append(diags, lint.Diagnostic{
					File:     f.Path,
					Line:     lineNum,
					Column:   state.first.start + 1,
					RuleID:   r.ID(),
					RuleName: r.Name(),
					Severity: lint.Warning,
					Message:  "adjacent same-delimiter emphasis is ambiguous",
				})
			}
			continue
		}

		states[k] = adjacentRunState{
			first: curr,
			last:  curr,
			count: 1,
		}
	}
	return diags
}

// gapNonEmptyAllNonWhitespace reports whether b is non-empty and
// contains no ASCII space or tab. Gaps are computed within a single
// line (newlines never appear), so space and tab cover the relevant
// CommonMark whitespace cases. The adjacent-same-delim detector
// treats a gap that contains a space or tab as a clean separation:
// CommonMark's flanking rules then resolve the runs unambiguously,
// so the rule stays silent.
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
