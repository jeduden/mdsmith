package refactor

import "testing"

// TestSortEditsByCharacterAsc_NoReflectSort pins the allocation cost
// of the edit-splice sort. It used sort.Slice / sort.SliceStable,
// which drive reflect.Swapper internally — the "reflect in hot paths"
// anti-pattern in docs/development/high-performance-go.md.
// slices.SortStableFunc on the concrete Edit type sorts with no
// reflection. This sort ran in cmd/mdsmith before ApplyEdits was
// extracted into this package.
func TestSortEditsByCharacterAsc_NoReflectSort(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	es := []Edit{
		{Range: Range{Start: Position{Character: 3}}},
		{Range: Range{Start: Position{Character: 9}}},
		{Range: Range{Start: Position{Character: 1}}},
	}
	sortEditsByCharacterAsc(es)
	if es[0].Range.Start.Character != 1 || es[2].Range.Start.Character != 9 {
		t.Fatalf("sortEditsByCharacterAsc did not sort ascending: %v", es)
	}

	const runs = 200
	allocs := testing.AllocsPerRun(runs, func() {
		sortEditsByCharacterAsc(es)
	})
	t.Logf("sortEditsByCharacterAsc allocs/op = %.0f", allocs)
	if allocs > 0 {
		t.Fatalf("sortEditsByCharacterAsc allocs/op = %.0f, want 0 (no reflection)", allocs)
	}
}
