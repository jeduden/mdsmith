package backlinks

import "testing"

// TestSortBacklinkRecords_NoReflectSort pins the allocation cost of
// the backlink-record sort. It used sort.SliceStable, which drives
// reflect.Swapper internally — the "reflect in hot paths"
// anti-pattern in docs/development/high-performance-go.md.
// slices.SortStableFunc on the concrete Record type sorts with no
// reflection. This sort ran in cmd/mdsmith before the algorithm was
// extracted into this package.
func TestSortBacklinkRecords_NoReflectSort(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	records := []Record{
		{Source: "z.md", Line: 1},
		{Source: "a.md", Line: 5},
		{Source: "a.md", Line: 2},
	}
	sortBacklinkRecords(records)
	if records[0].Source != "a.md" || records[0].Line != 2 || records[2].Source != "z.md" {
		t.Fatalf("sortBacklinkRecords did not sort: %v", records)
	}

	const runs = 200
	allocs := testing.AllocsPerRun(runs, func() {
		sortBacklinkRecords(records)
	})
	t.Logf("sortBacklinkRecords allocs/op = %.0f", allocs)
	if allocs > 0 {
		t.Fatalf("sortBacklinkRecords allocs/op = %.0f, want 0 (no reflection)", allocs)
	}
}
