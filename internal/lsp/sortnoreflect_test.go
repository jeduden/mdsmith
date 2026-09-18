package lsp

import "testing"

// TestSortTextEditsBottomUp_NoReflectSort pins the allocation cost of
// sortTextEditsBottomUp, which drove sort.SliceStable — reflect.Swapper
// internally — the "reflect in hot paths" anti-pattern in
// docs/development/high-performance-go.md, already fixed the same way
// elsewhere in this codebase (internal/backlinks.sortBacklinkRecords,
// internal/refactor.stableSortEdits). It runs once per LSP rename or
// willRenameFiles response, on the move/rename engine's new hot path.
func TestSortTextEditsBottomUp_NoReflectSort(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	edits := []textEdit{
		{Range: Range{Start: Position{Line: 1, Character: 0}}, NewText: "a"},
		{Range: Range{Start: Position{Line: 5, Character: 0}}, NewText: "b"},
		{Range: Range{Start: Position{Line: 2, Character: 3}}, NewText: "c"},
	}
	sortTextEditsBottomUp(edits)
	if edits[0].Range.Start.Line != 5 || edits[2].Range.Start.Line != 1 {
		t.Fatalf("sortTextEditsBottomUp did not sort bottom-up: %+v", edits)
	}

	const runs = 200
	allocs := testing.AllocsPerRun(runs, func() {
		sortTextEditsBottomUp(edits)
	})
	t.Logf("sortTextEditsBottomUp allocs/op = %.0f", allocs)
	if allocs > 0 {
		t.Fatalf("sortTextEditsBottomUp allocs/op = %.0f, want 0 (no reflection)", allocs)
	}
}
