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
	sortItems(items)
	if items[0].Label != "alpha" || items[2].Label != "zeta" {
		t.Fatalf("sortItems did not sort: %+v", items)
	}

	const runs = 200
	allocs := testing.AllocsPerRun(runs, func() {
		sortItems(items)
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
	sortSymbolInformation(out)
	if out[0].ContainerName != "a.md" || out[0].Name != "a" || out[2].ContainerName != "b.md" {
		t.Fatalf("sortSymbolInformation did not sort: %+v", out)
	}

	const runs = 200
	allocs := testing.AllocsPerRun(runs, func() {
		sortSymbolInformation(out)
	})
	t.Logf("sortSymbolInformation allocs/op = %.0f", allocs)
	if allocs > 0 {
		t.Fatalf("sortSymbolInformation allocs/op = %.0f, want 0 (no reflection)", allocs)
	}
}

// TestSortLocationsByURIThenLine_NoReflectSort pins the allocation cost
// of sortLocationsByURIThenLine, which drove sort.Slice (4 separate call
// sites) — reflect.Swapper internally. It runs on every
// textDocument/references and workspace-symbol-by-kind LSP request.
func TestSortLocationsByURIThenLine_NoReflectSort(t *testing.T) {
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
	sortLocationsByURIThenLine(locs)
	if locs[0].URI != "file:///a.md" || locs[0].Range.Start.Line != 2 || locs[2].URI != "file:///z.md" {
		t.Fatalf("sortLocationsByURIThenLine did not sort: %+v", locs)
	}

	const runs = 200
	allocs := testing.AllocsPerRun(runs, func() {
		sortLocationsByURIThenLine(locs)
	})
	t.Logf("sortLocationsByURIThenLine allocs/op = %.0f", allocs)
	if allocs > 0 {
		t.Fatalf("sortLocationsByURIThenLine allocs/op = %.0f, want 0 (no reflection)", allocs)
	}
}

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
