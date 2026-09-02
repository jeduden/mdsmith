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
