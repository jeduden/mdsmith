package index

import "testing"

// TestSortEdgesBySource_NoReflectSort pins the allocation cost of
// sortEdgesBySource, which drove sort.Slice — reflect.Swapper
// internally — the "reflect in hot paths" anti-pattern in
// docs/development/high-performance-go.md, already fixed the same way
// elsewhere in this codebase (internal/backlinks.sortBacklinkRecords,
// internal/fix.sortDiagnostics). It runs on every BacklinksFor,
// IncomingPathEdges, and IncomingWikilinkEdges call, which the move
// engine calls once per renamed file.
func TestSortEdgesBySource_NoReflectSort(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	edges := []Edge{
		{SourceFile: "z.md", SourceLine: 1, SourceCol: 1},
		{SourceFile: "a.md", SourceLine: 5, SourceCol: 1},
		{SourceFile: "a.md", SourceLine: 2, SourceCol: 3},
	}
	sortEdgesBySource(edges)
	if edges[0].SourceFile != "a.md" || edges[0].SourceLine != 2 || edges[2].SourceFile != "z.md" {
		t.Fatalf("sortEdgesBySource did not sort: %v", edges)
	}

	const runs = 200
	allocs := testing.AllocsPerRun(runs, func() {
		sortEdgesBySource(edges)
	})
	t.Logf("sortEdgesBySource allocs/op = %.0f", allocs)
	if allocs > 0 {
		t.Fatalf("sortEdgesBySource allocs/op = %.0f, want 0 (no reflection)", allocs)
	}
}
