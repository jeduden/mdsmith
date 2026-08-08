package linkvalidity

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/require"
)

// sortAllocBudget pins the allocation cost of the diagnostic sort in
// Check. sort.SliceStable used reflect.Swapper internally (~3 allocs
// on a handful of elements per docs/development/high-performance-go.md's
// "reflect in hot paths" anti-pattern); slices.SortStableFunc sorts
// concrete lint.Diagnostic values with no reflection and 0 allocs.
const sortAllocBudget = 0

// sortFixture is a source with multiple MDS062 findings (reversed
// links and empty links/images) so the sort actually reorders more
// than one element.
const sortFixture = "# Title\n\n" +
	"(one)[a.md] and (two)[b.md] and (three)[c.md]\n\n" +
	"[](empty.md) and [empty]()\n"

func TestDiagnosticSort_NoAllocs(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	r := &Rule{}
	f, err := lint.NewFile("sort.md", []byte(sortFixture))
	require.NoError(t, err)
	diags := r.checkEmpty(f)
	diags = append(diags, r.checkReversed(f)...)
	require.Greaterf(t, len(diags), 1, "fixture must produce multiple diagnostics to exercise the sort")

	const runs = 200
	allocs := testing.AllocsPerRun(runs, func() {
		sortDiagnostics(diags)
	})
	t.Logf("sortDiagnostics allocs/op = %.0f (budget = %d)", allocs, sortAllocBudget)
	require.LessOrEqualf(t, allocs, float64(sortAllocBudget),
		"sortDiagnostics allocs/op = %.0f, budget = %d", allocs, sortAllocBudget)
}
