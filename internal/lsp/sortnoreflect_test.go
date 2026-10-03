package lsp

import "testing"

// TestSortItems_NoReflectSort pins the allocation cost of sortItems,
// which drove sort.Slice — reflect.Swapper internally — the "reflect in
// hot paths" anti-pattern in docs/development/high-performance-go.md.
// It runs on every textDocument/completion request.
func TestSortItems_NoReflectSort(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	items := []completionItem{
		{Label: "zeta", SortText: "3"},
		{Label: "alpha", SortText: "1"},
		{Label: "beta", SortText: "2"},
	}
	buf := make([]completionItem, len(items))
	copy(buf, items)
	sortItems(buf)
	if buf[0].Label != "alpha" || buf[2].Label != "zeta" {
		t.Fatalf("sortItems did not sort: %+v", buf)
	}

	// Refill buf from the unsorted items on every run, so each run
	// moves elements instead of re-sorting an already-sorted slice.
	const runs = 200
	allocs := testing.AllocsPerRun(runs, func() {
		copy(buf, items)
		sortItems(buf)
	})
	t.Logf("sortItems allocs/op = %.0f", allocs)
	if allocs > 0 {
		t.Fatalf("sortItems allocs/op = %.0f, want 0 (no reflection)", allocs)
	}
}

// TestSortSymbolInformation_NoReflectSort pins the allocation cost of
// sortSymbolInformation, which drove sort.Slice — reflect.Swapper
// internally. It runs on every workspace/symbol request.
func TestSortSymbolInformation_NoReflectSort(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	out := []symbolInformation{
		{Name: "z", ContainerName: "b.md"},
		{Name: "a", ContainerName: "a.md"},
		{Name: "b", ContainerName: "a.md"},
	}
	buf := make([]symbolInformation, len(out))
	copy(buf, out)
	sortSymbolInformation(buf)
	if buf[0].ContainerName != "a.md" || buf[0].Name != "a" || buf[2].ContainerName != "b.md" {
		t.Fatalf("sortSymbolInformation did not sort: %+v", buf)
	}

	const runs = 200
	allocs := testing.AllocsPerRun(runs, func() {
		copy(buf, out)
		sortSymbolInformation(buf)
	})
	t.Logf("sortSymbolInformation allocs/op = %.0f", allocs)
	if allocs > 0 {
		t.Fatalf("sortSymbolInformation allocs/op = %.0f, want 0 (no reflection)", allocs)
	}
}

// TestSortLocations_NoReflectSort pins the allocation cost of
// sortLocations, which replaced sort.Slice at 4 separate call sites
// (sort.Slice drives reflect.Swapper internally). It runs on every
// textDocument/references and workspace-symbol-by-kind LSP request.
func TestSortLocations_NoReflectSort(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	locs := []location{
		{URI: "file:///z.md", Range: Range{Start: Position{Line: 1}}},
		{URI: "file:///a.md", Range: Range{Start: Position{Line: 5}}},
		{URI: "file:///a.md", Range: Range{Start: Position{Line: 2}}},
	}
	buf := make([]location, len(locs))
	copy(buf, locs)
	sortLocations(buf)
	if buf[0].URI != "file:///a.md" || buf[0].Range.Start.Line != 2 || buf[2].URI != "file:///z.md" {
		t.Fatalf("sortLocations did not sort: %+v", buf)
	}

	const runs = 200
	allocs := testing.AllocsPerRun(runs, func() {
		copy(buf, locs)
		sortLocations(buf)
	})
	t.Logf("sortLocations allocs/op = %.0f", allocs)
	if allocs > 0 {
		t.Fatalf("sortLocations allocs/op = %.0f, want 0 (no reflection)", allocs)
	}
}

// TestSortTextEditsBottomUp_NoReflectSort pins the allocation cost of
// sortTextEditsBottomUp, which drove sort.SliceStable — reflect.Swapper
// internally — the "reflect in hot paths" anti-pattern in
// docs/development/high-performance-go.md, already fixed the same way
// elsewhere in this codebase (internal/backlinks.sortBacklinkRecords,
// internal/refactor.stableSortEdits). It runs once per LSP link-reference
// rename response; a willRenameFiles response sorts through
// dropConflictingTextEdits instead.
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
