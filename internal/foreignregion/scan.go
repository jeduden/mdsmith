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
	RuleID   = "MDS073"
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
	var ranges []lint.LineRange
	var diags []lint.Diagnostic
	for _, reg := range regions {
		rs, ds := scanOne(f, reg)
		ranges = append(ranges, rs...)
		diags = append(diags, ds...)
	}
	return ranges, diags
}

// scanOne walks f.Lines once for a single marker pair, matching a line
// against a marker by trimmed-line equality so leading indentation does
// not defeat the match while incidental in-prose mentions do not.
func scanOne(f *lint.File, reg config.ForeignRegion) ([]lint.LineRange, []lint.Diagnostic) {
	start := strings.TrimSpace(reg.Start)
	end := strings.TrimSpace(reg.End)
	var ranges []lint.LineRange
	var diags []lint.Diagnostic
	openLine := 0 // 1-based line of the current unclosed start; 0 when none
	for i, line := range f.Lines {
		lineNum := i + 1
		trimmed := strings.TrimSpace(string(line))
		switch trimmed {
		case start:
			if openLine != 0 {
				diags = append(diags, diag(lineNum, fmt.Sprintf(
					"duplicate foreign-region start marker %q before %q closed the open region",
					start, end)))
				continue
			}
			openLine = lineNum
		case end:
			if openLine == 0 {
				diags = append(diags, diag(lineNum, fmt.Sprintf(
					"foreign-region end marker %q without a matching start marker %q",
					end, start)))
				continue
			}
			ranges = append(ranges, lint.LineRange{From: openLine, To: lineNum})
			openLine = 0
		}
	}
	if openLine != 0 {
		diags = append(diags, diag(openLine, fmt.Sprintf(
			"foreign-region start marker %q has no matching end marker %q",
			start, end)))
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
