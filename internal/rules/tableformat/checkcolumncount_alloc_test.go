package tableformat

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/require"
)

// TestCheckColumnCountCompliant_NoAlloc pins that checkColumnCount
// returns nil, with zero allocations, when every row's cell count
// matches the header. The old make([]lint.Diagnostic, 0,
// len(t.rows)-1) allocated a slice on every call and then discarded
// it via `if len(diags) == 0 { return nil }` on this common
// compliant-table path — see docs/development/high-performance-go.md's
// "Return nil, not []T{}" pattern.
func TestCheckColumnCountCompliant_NoAlloc(t *testing.T) {
	f, err := lint.NewFile("t.md", []byte("| a | b |\n|---|---|\n| 1 | 2 |\n"))
	require.NoError(t, err)
	tables := findStructureTables(f.Lines, structureSkipFunc(f))
	require.Len(t, tables, 1)
	tbl := tables[0]

	if got := checkColumnCount(f, tbl, "MDS025", "table-format"); got != nil {
		t.Fatalf("checkColumnCount on a compliant table = %#v, want nil", got)
	}

	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	allocs := testing.AllocsPerRun(200, func() {
		_ = checkColumnCount(f, tbl, "MDS025", "table-format")
	})
	t.Logf("checkColumnCount(compliant) allocs/op = %.0f", allocs)
	if allocs > 0 {
		t.Fatalf("checkColumnCount(compliant) allocs/op = %.0f, want 0: allocate diags "+
			"lazily via append instead of an eager make() that gets discarded", allocs)
	}
}

// allocBudgetCheckColumnCountManyMismatches is the per-call ceiling
// for checkColumnCount on a table with 6 column-count mismatches: 1
// alloc for the presized diags slice plus 1 alloc per
// fmt.Sprintf-built diagnostic message. A bare `var diags
// []lint.Diagnostic` growing via unpresized append instead pays
// growslice's doubling reallocations (measured 4 extra, for 10
// total) on top of that — see
// docs/development/high-performance-go.md "Pre-size slices."
const allocBudgetCheckColumnCountManyMismatches = 7

// checkColumnCountManyMismatchesFixture has a two-cell header and six
// one-cell body rows, so every body row mismatches the header's cell
// count and checkColumnCount must grow diags past the point a small
// unpresized append would need to reallocate.
const checkColumnCountManyMismatchesFixture = "| a | b |\n|---|---|\n" +
	"| 1 |\n| 1 |\n| 1 |\n| 1 |\n| 1 |\n| 1 |\n"

func TestCheckColumnCountManyMismatches_PresizedAllocBudget(t *testing.T) {
	f, err := lint.NewFile("t.md", []byte(checkColumnCountManyMismatchesFixture))
	require.NoError(t, err)
	tables := findStructureTables(f.Lines, structureSkipFunc(f))
	require.Len(t, tables, 1)
	tbl := tables[0]

	got := checkColumnCount(f, tbl, "MDS025", "table-format")
	require.Lenf(t, got, 6, "fixture must produce one diagnostic per mismatched body row")

	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	allocs := testing.AllocsPerRun(200, func() {
		_ = checkColumnCount(f, tbl, "MDS025", "table-format")
	})
	t.Logf("checkColumnCount(6 mismatches) allocs/op = %.0f (budget = %d)",
		allocs, allocBudgetCheckColumnCountManyMismatches)
	if allocs > float64(allocBudgetCheckColumnCountManyMismatches) {
		t.Fatalf("checkColumnCount(6 mismatches) allocs/op = %.0f, budget = %d: "+
			"diags must be presized with make([]lint.Diagnostic, 0, len(t.rows)-1) "+
			"on the first mismatch instead of growing via bare append",
			allocs, allocBudgetCheckColumnCountManyMismatches)
	}
}
