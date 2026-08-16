// Package foreignregion locates the line spans a foreign generator owns
// inside a Markdown file — the bytes bounded by a declared marker pair
// such as APM's `<!-- apm:start -->` / `<!-- apm:end -->`. The fix
// pipeline feeds the returned spans into lint.File.GeneratedRanges so
// style rules skip diagnostics there and fixers never rewrite the bytes,
// exactly as it already does for `<?include?>` / `<?catalog?>` bodies.
// Whole-file rules (MDS022, MDS028) read the raw source, so they keep
// counting the protected bytes.
package foreignregion

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
)

// RuleID and RuleName identify the malformed-region diagnostic. It is
// not a registered rule.Rule: the spans are computed in the fix/check
// pipeline (like generated-section ranges), so the malformed check
// rides along there rather than through the per-rule engine.
const (
	RuleID   = "MDS074"
	RuleName = "foreign-region"
)

// Scan locates every matched marker pair in f for each declared region
// and returns their inclusive line ranges (start marker line through end
// marker line, in f.Lines coordinates) plus a diagnostic for each
// malformed region — a start with no matching end, an end with no
// matching start, or a second start opening before the first closes.
// Both slices are nil when regions is empty.
func Scan(f *lint.File, regions []config.ForeignRegion) ([]lint.LineRange, []lint.Diagnostic) {
	if len(regions) == 0 || f == nil {
		return nil, nil
	}
	states := make([]regionScanState, len(regions))
	for i, reg := range regions {
		states[i] = regionScanState{
			start: []byte(strings.TrimSpace(reg.Start)),
			end:   []byte(strings.TrimSpace(reg.End)),
		}
	}

	var ranges []lint.LineRange
	var diags []lint.Diagnostic
	for i, line := range f.Lines {
		lineNum := i + 1
		trimmed := bytes.TrimSpace(line)
		for s := range states {
			rs, ds := states[s].step(trimmed, lineNum)
			ranges = append(ranges, rs...)
			diags = append(diags, ds...)
		}
	}
	for s := range states {
		if ds := states[s].unclosed(); ds != nil {
			diags = append(diags, ds...)
		}
	}
	return ranges, diags
}

// regionScanState tracks one declared region's marker pair across a
// single walk of f.Lines. A single pass evaluates every region's
// state against the line's shared trim, instead of Scan walking
// f.Lines once per region and re-converting every line to a string
// each time — the redundant re-scanning docs/development/
// high-performance-go.md calls out.
type regionScanState struct {
	start, end []byte
	openLine   int // 1-based line of the current unclosed start; 0 when none
}

// step evaluates one already-trimmed line against this region's
// markers and returns any newly closed range or malformed-region
// diagnostic.
func (st *regionScanState) step(trimmed []byte, lineNum int) ([]lint.LineRange, []lint.Diagnostic) {
	switch {
	case bytes.Equal(trimmed, st.start):
		if st.openLine != 0 {
			return nil, []lint.Diagnostic{diag(lineNum, fmt.Sprintf(
				"duplicate foreign-region start marker %q before %q closed the open region",
				st.start, st.end))}
		}
		st.openLine = lineNum
	case bytes.Equal(trimmed, st.end):
		if st.openLine == 0 {
			return nil, []lint.Diagnostic{diag(lineNum, fmt.Sprintf(
				"foreign-region end marker %q without a matching start marker %q",
				st.end, st.start))}
		}
		r := lint.LineRange{From: st.openLine, To: lineNum}
		st.openLine = 0
		return []lint.LineRange{r}, nil
	}
	return nil, nil
}

// unclosed reports the missing-end diagnostic for a region whose
// start marker was never closed, once the whole file has been walked.
func (st *regionScanState) unclosed() []lint.Diagnostic {
	if st.openLine == 0 {
		return nil
	}
	return []lint.Diagnostic{diag(st.openLine, fmt.Sprintf(
		"foreign-region start marker %q has no matching end marker %q",
		st.start, st.end))}
}

// Apply appends the foreign-region spans for path to f.GeneratedRanges —
// the same exclusion set that keeps style rules and fixers out of
// `<?include?>` / `<?catalog?>` bodies — and returns the malformed-region
// diagnostics with File and the file's front-matter line offset already
// applied, ready to append to the file's diagnostic set after the rule
// check has run. It is the single-scan entry point for the pooled check
// and fix paths, where f is local to one goroutine. Returns nil when no
// marker pairs apply to path.
func Apply(f *lint.File, cfg *config.Config, path string) []lint.Diagnostic {
	ranges, diags := resolve(f, cfg, path)
	f.GeneratedRanges = append(f.GeneratedRanges, ranges...)
	return diags
}

// AppendRanges appends the foreign-region spans for path to
// f.GeneratedRanges and discards the malformed diagnostics. The
// RunSource (LSP) path uses it to populate the exclusion set once,
// before f is published to the parse cache, so the diagnostic pass can
// stay read-only on the shared *File (see Diagnostics).
func AppendRanges(f *lint.File, cfg *config.Config, path string) {
	ranges, _ := resolve(f, cfg, path)
	f.GeneratedRanges = append(f.GeneratedRanges, ranges...)
}

// Diagnostics returns the malformed-region diagnostics for path without
// mutating f. The RunSource path calls it during the check pass, whose
// contract forbids writing to the cached *File; AppendRanges has already
// set the ranges, so re-scanning here only rebuilds the diagnostics.
func Diagnostics(f *lint.File, cfg *config.Config, path string) []lint.Diagnostic {
	_, diags := resolve(f, cfg, path)
	return diags
}

// resolve scans f for the marker pairs that apply to path and returns
// their ranges plus the malformed diagnostics, with each diagnostic's
// File set and Line shifted into display coordinates by f.LineOffset.
func resolve(f *lint.File, cfg *config.Config, path string) ([]lint.LineRange, []lint.Diagnostic) {
	regions := config.EffectiveForeignRegions(cfg, path)
	if len(regions) == 0 {
		return nil, nil
	}
	ranges, diags := Scan(f, regions)
	for i := range diags {
		diags[i].File = path
		diags[i].Line += f.LineOffset
	}
	return ranges, diags
}

// diag builds one malformed-region diagnostic at the given 1-based line
// (in f.Lines coordinates; the caller adds any front-matter offset).
func diag(line int, msg string) lint.Diagnostic {
	return lint.Diagnostic{
		RuleID:   RuleID,
		RuleName: RuleName,
		Severity: lint.Warning,
		Message:  msg,
		Line:     line,
		Column:   1,
	}
}
