package refactor

import "testing"

// TestStableSortEdits_NoReflectSort pins the allocation cost of
// stableSortEdits, which drove sort.SliceStable — reflect.Swapper
// internally — the "reflect in hot paths" anti-pattern in
// docs/development/high-performance-go.md, already fixed the same way
// elsewhere in this codebase (internal/backlinks.sortBacklinkRecords,
// internal/fix.sortDiagnostics). It runs once per Move/Heading call, on
// the move engine's new hot path.
func TestStableSortEdits_NoReflectSort(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	changes := map[string][]Edit{
		"doc.md": {
			{Range: Range{Start: Position{Line: 1, Character: 0}}, NewText: "a"},
			{Range: Range{Start: Position{Line: 5, Character: 0}}, NewText: "b"},
			{Range: Range{Start: Position{Line: 2, Character: 3}}, NewText: "c"},
		},
	}
	stableSortEdits(changes)
	edits := changes["doc.md"]
	if edits[0].Range.Start.Line != 5 || edits[2].Range.Start.Line != 1 {
		t.Fatalf("stableSortEdits did not sort bottom-up: %+v", edits)
	}

	const runs = 200
	allocs := testing.AllocsPerRun(runs, func() {
		stableSortEdits(changes)
	})
	t.Logf("stableSortEdits allocs/op = %.0f", allocs)
	if allocs > 0 {
		t.Fatalf("stableSortEdits allocs/op = %.0f, want 0 (no reflection)", allocs)
	}
}
