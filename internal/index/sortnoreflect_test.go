package index

import (
	"math/rand"
	"strconv"
	"testing"
)

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

// TestSortEdgesBySource_TiesPreserveInsertionOrder pins the function's
// own doc comment ("a stable, reviewable order"): edges that tie on
// every sort key (SourceFile, SourceLine, SourceCol) — for example two
// edges of different kinds recorded at one heading's declaration
// position — must come out in the order they went in, not whatever
// order an unstable sort's internal partitioning happens to leave
// them. A small tied slice doesn't reliably expose instability (an
// unstable sort can still leave a handful of equal elements
// untouched), so this test interleaves three tied groups across 300
// shuffled edges, large enough to reliably reorder under a plain
// (unstable) slices.SortFunc.
func TestSortEdgesBySource_TiesPreserveInsertionOrder(t *testing.T) {
	files := []string{"a.md", "b.md", "c.md"}
	const n = 300
	// TargetLabel carries each edge's original global index as a
	// string, so ties on (SourceFile, SourceLine, SourceCol) — the only
	// fields sortEdgesBySource compares — can still be traced back to
	// their input order after the sort and the shuffle.
	edges := make([]Edge, n)
	for i := range edges {
		edges[i] = Edge{
			SourceFile:  files[i%len(files)],
			SourceLine:  1,
			SourceCol:   1,
			TargetLabel: strconv.Itoa(i),
		}
	}
	rand.New(rand.NewSource(1)).Shuffle(len(edges), func(i, j int) {
		edges[i], edges[j] = edges[j], edges[i]
	})

	// wantOrder is the relative order each file's tied edges appear in
	// right before the sort — the input a stable sort must preserve.
	wantOrder := map[string][]string{}
	for _, e := range edges {
		wantOrder[e.SourceFile] = append(wantOrder[e.SourceFile], e.TargetLabel)
	}

	sortEdgesBySource(edges)

	gotOrder := map[string][]string{}
	for _, e := range edges {
		gotOrder[e.SourceFile] = append(gotOrder[e.SourceFile], e.TargetLabel)
	}
	for _, f := range files {
		want, got := wantOrder[f], gotOrder[f]
		if len(want) != len(got) {
			t.Fatalf("file %q: got %d tied edges, want %d", f, len(got), len(want))
		}
		for i := range want {
			if want[i] != got[i] {
				t.Fatalf("sortEdgesBySource reordered tied edges in %q at position %d: "+
					"got TargetLabel %q, want %q (pre-sort order not preserved)",
					f, i, got[i], want[i])
			}
		}
	}
}
