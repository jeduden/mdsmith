package index

import (
	"math/rand"
	"reflect"
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

// TestSortEdgesBySource_DeterministicAcrossInputOrders pins the
// function's doc comment ("a stable, reviewable order"): its callers
// collect edges by ranging over the index's file map, so the input
// order varies from call to call, and the output must not. Edges that
// tie on (SourceFile, SourceLine, SourceCol) — for example two edges
// of different kinds recorded at one position — must still land in
// one fixed order, whatever order they arrive in. A small tied slice
// doesn't reliably expose an order-dependent result (pdqsort falls
// back to insertion sort below 13 elements), so this test interleaves
// three tied groups across 300 edges and sorts two different shuffles
// of them.
func TestSortEdgesBySource_DeterministicAcrossInputOrders(t *testing.T) {
	files := []string{"a.md", "b.md", "c.md"}
	const n = 300
	// Every edge ties with its group on the three source keys; Kind
	// and TargetLabel (the edge's index as a string) keep each edge
	// distinct so a reordering is visible.
	base := make([]Edge, n)
	for i := range base {
		base[i] = Edge{
			SourceFile:  files[i%len(files)],
			SourceLine:  1,
			SourceCol:   1,
			Kind:        EdgeKind(i % 2),
			TargetLabel: strconv.Itoa(i),
		}
	}

	sortShuffled := func(seed int64) []Edge {
		edges := append([]Edge(nil), base...)
		rand.New(rand.NewSource(seed)).Shuffle(len(edges), func(i, j int) {
			edges[i], edges[j] = edges[j], edges[i]
		})
		sortEdgesBySource(edges)
		return edges
	}

	first := sortShuffled(1)
	second := sortShuffled(2)
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("sortEdgesBySource output depends on input order at position %d: "+
				"%+v after one shuffle, %+v after another", i, first[i], second[i])
		}
	}
}

// TestCompareEdgesBySource_EveryFieldBreaksTies pins the comparator's
// completeness: two edges that differ in any one Edge field must not
// compare equal, or an unstable sort could order them differently
// from call to call. It walks Edge's fields by reflection, so a field
// added later without a matching tie-break fails here.
func TestCompareEdgesBySource_EveryFieldBreaksTies(t *testing.T) {
	typ := reflect.TypeOf(Edge{})
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		t.Run(field.Name, func(t *testing.T) {
			var a, b Edge
			v := reflect.ValueOf(&b).Elem().Field(i)
			switch v.Kind() {
			case reflect.String:
				v.SetString("x")
			case reflect.Int:
				v.SetInt(1)
			case reflect.Bool:
				v.SetBool(true)
			default:
				t.Fatalf("Edge.%s has kind %s; extend this test and compareEdgesBySource",
					field.Name, v.Kind())
			}
			if compareEdgesBySource(a, b) >= 0 || compareEdgesBySource(b, a) <= 0 {
				t.Fatalf("compareEdgesBySource does not order edges that differ only in %s",
					field.Name)
			}
		})
	}
}

// TestCompareEdgesBySource_EqualEdgesCompareEqual pins the other half
// of the total order: an edge compared with an identical copy returns
// 0, so the comparator is reflexive and SortFunc treats duplicates as
// ties rather than as strictly ordered.
func TestCompareEdgesBySource_EqualEdgesCompareEqual(t *testing.T) {
	for _, unresolved := range []bool{false, true} {
		e := Edge{
			SourceFile:   "a.md",
			SourceLine:   3,
			SourceCol:    5,
			Kind:         EdgeKind(1),
			TargetFile:   "b.md",
			TargetAnchor: "intro",
			TargetLabel:  "ref",
			Unresolved:   unresolved,
		}
		if got := compareEdgesBySource(e, e); got != 0 {
			t.Fatalf("compareEdgesBySource(e, e) = %d for %+v, want 0", got, e)
		}
	}
}
