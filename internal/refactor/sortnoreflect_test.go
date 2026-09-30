package refactor

import "testing"

// TestSortEditsByRange_NoReflectSort pins the order and the allocation
// cost of the edit-splice sort. It used sort.SliceStable, which drives
// reflect.Swapper internally — the "reflect in hot paths" anti-pattern
// in docs/development/high-performance-go.md. slices.SortStableFunc on
// the concrete Edit type sorts with no reflection. This sort ran in
// cmd/mdsmith before ApplyEdits was moved into this package.
func TestSortEditsByRange_NoReflectSort(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	es := []Edit{
		{Range: Range{Start: Position{Character: 3}, End: Position{Character: 5}}},
		{Range: Range{Start: Position{Character: 9}, End: Position{Character: 9}}},
		{Range: Range{Start: Position{Character: 1}, End: Position{Character: 2}}},
		{Range: Range{Start: Position{Character: 3}, End: Position{Character: 3}}},
	}
	sortEditsByRange(es)
	got := make([][2]int, len(es))
	for i, e := range es {
		got[i] = [2]int{e.Range.Start.Character, e.Range.End.Character}
	}
	want := [][2]int{{1, 2}, {3, 3}, {3, 5}, {9, 9}}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sortEditsByRange order = %v, want %v (by start, then end)", got, want)
		}
	}

	const runs = 200
	allocs := testing.AllocsPerRun(runs, func() {
		sortEditsByRange(es)
	})
	t.Logf("sortEditsByRange allocs/op = %.0f", allocs)
	if allocs > 0 {
		t.Fatalf("sortEditsByRange allocs/op = %.0f, want 0 (no reflection)", allocs)
	}
}
