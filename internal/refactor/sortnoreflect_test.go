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

// TestStableSortEdits_TieBreakAndStability pins the comparator's tie
// break on Character within a shared Line, and its stability across
// edits that share both Line and Character (two edits on the same
// line and column, e.g. an insertion and an adjacent rewrite, must
// keep their original relative order — callers rely on that to avoid
// reordering same-position edits when applying them). slices.SortFunc
// (unlike SortStableFunc) would not guarantee this, so this test would
// have caught the wrong choice between the two.
func TestStableSortEdits_TieBreakAndStability(t *testing.T) {
	changes := map[string][]Edit{
		"doc.md": {
			{Range: Range{Start: Position{Line: 5, Character: 2}}, NewText: "tie-1"},
			{Range: Range{Start: Position{Line: 2, Character: 0}}, NewText: "last"},
			{Range: Range{Start: Position{Line: 5, Character: 4}}, NewText: "before-ties"},
			{Range: Range{Start: Position{Line: 7, Character: 0}}, NewText: "first"},
			{Range: Range{Start: Position{Line: 5, Character: 2}}, NewText: "tie-2"},
		},
	}
	stableSortEdits(changes)
	edits := changes["doc.md"]

	got := make([]string, len(edits))
	for i, e := range edits {
		got[i] = e.NewText
	}
	want := []string{"first", "before-ties", "tie-1", "tie-2", "last"}
	if len(got) != len(want) {
		t.Fatalf("stableSortEdits order = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("stableSortEdits order = %v, want %v (tie-break on Character or "+
				"stability among same-position edits is broken)", got, want)
		}
	}
}
