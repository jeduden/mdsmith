package engine

import (
	"strconv"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
)

// BenchmarkSortDiagnostics pins the CPU cost of sorting a
// representative-size diagnostic set from a large workspace run.
// sortDiagnostics runs once per Runner.Run/RunSource call over the
// full result set (internal/engine/runner.go's two call sites) — the
// one place in the codebase where the reflect-vs-slices.SortStableFunc
// fix from docs/development/high-performance-go.md's "reflect in hot
// paths" anti-pattern had not yet been applied, unlike the equivalent
// per-rule sortDiagnostics helpers in linkvalidity, catalog, astutil,
// duplicatedcontent, and markdownflavor.
func BenchmarkSortDiagnostics(b *testing.B) {
	const n = 2000
	base := make([]lint.Diagnostic, n)
	for i := range base {
		// Descending input order forces real comparison work instead
		// of a sort implementation detecting an already-sorted run.
		base[i] = lint.Diagnostic{
			File:    "file" + strconv.Itoa(n-i) + ".md",
			Line:    n - i,
			Column:  1,
			RuleID:  "MDS001",
			Message: "message",
		}
	}

	diags := make([]lint.Diagnostic, n)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		copy(diags, base)
		sortDiagnostics(diags)
	}
}
