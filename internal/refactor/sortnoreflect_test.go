package refactor

import (
	"math/rand"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
// reordering same-position edits when applying them). At five edits
// the stability half is illustrative only: slices.SortFunc falls back
// to insertion sort — itself stable — below 13 elements, so this
// fixture passes under an unstable sort too.
// TestStableSortEdits_TiesPreserveInputOrder below is the one that
// fails if stableSortEdits drops SortStableFunc.
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

// TestStableSortEdits_TiesPreserveInputOrder pins stableSortEdits'
// stability at a size an unstable sort actually disturbs: 300 edits
// shuffled across three tied start positions reorder under
// slices.SortFunc (pdqsort partitions them), while SortStableFunc must
// keep each position's edits in their pre-sort order. NewText carries
// each edit's original index so the tied edits stay traceable.
func TestStableSortEdits_TiesPreserveInputOrder(t *testing.T) {
	positions := []Position{
		{Line: 4, Character: 2},
		{Line: 9, Character: 0},
		{Line: 4, Character: 7},
	}
	const n = 300
	edits := make([]Edit, n)
	for i := range edits {
		edits[i] = Edit{
			Range:   Range{Start: positions[i%len(positions)]},
			NewText: strconv.Itoa(i),
		}
	}
	rand.New(rand.NewSource(1)).Shuffle(n, func(i, j int) {
		edits[i], edits[j] = edits[j], edits[i]
	})

	// want is each tied position's edit order right before the sort —
	// the input a stable sort must preserve.
	want := map[Position][]string{}
	for _, e := range edits {
		want[e.Range.Start] = append(want[e.Range.Start], e.NewText)
	}

	changes := map[string][]Edit{"doc.md": edits}
	stableSortEdits(changes)
	sorted := changes["doc.md"]
	require.Len(t, sorted, n)

	got := map[Position][]string{}
	for i, e := range sorted {
		if i > 0 {
			require.LessOrEqual(t,
				ComparePositionsBottomUp(sorted[i-1].Range.Start, e.Range.Start), 0,
				"edits not in bottom-up order at index %d", i)
		}
		got[e.Range.Start] = append(got[e.Range.Start], e.NewText)
	}
	assert.Equal(t, want, got, "tied edits lost their pre-sort relative order")
}

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
