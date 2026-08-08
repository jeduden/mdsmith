package catalog

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// sortEntriesFixture returns fresh, out-of-order entries each call so
// AllocsPerRun measures a real (non-degenerate) sort every iteration.
func sortEntriesFixture() []fileEntry {
	return []fileEntry{
		{fields: map[string]any{"filename": "c.md", "title": "Charlie"}, matchPath: "c.md"},
		{fields: map[string]any{"filename": "a.md", "title": "Alpha"}, matchPath: "a.md"},
		{fields: map[string]any{"filename": "b.md", "title": "Bravo"}, matchPath: "b.md"},
	}
}

func TestSortEntries_Correctness(t *testing.T) {
	entries := sortEntriesFixture()
	sortEntries(entries, "title", false, false)
	require.Equal(t, []string{"a.md", "b.md", "c.md"}, []string{
		entries[0].matchPath, entries[1].matchPath, entries[2].matchPath,
	})
}

// TestSortSortables_NoReflectSort pins sortSortables's allocation
// cost. sort.SliceStable drove reflect.Swapper internally (the
// "reflect in hot paths" anti-pattern in
// docs/development/high-performance-go.md); slices.SortStableFunc
// sorts the concrete sortable values with no reflection.
func TestSortSortables_NoReflectSort(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	base := []sortable{
		{entry: fileEntry{matchPath: "c.md"}, sortKey: "charlie", tiebreakerKey: "c.md"},
		{entry: fileEntry{matchPath: "a.md"}, sortKey: "alpha", tiebreakerKey: "a.md"},
		{entry: fileEntry{matchPath: "b.md"}, sortKey: "bravo", tiebreakerKey: "b.md"},
	}

	const runs = 200
	allocs := testing.AllocsPerRun(runs, func() {
		s := make([]sortable, len(base))
		copy(s, base)
		sortSortables(s, "title", false, false)
	})
	t.Logf("sortSortables (3 entries, incl. slice copy) allocs/op = %.0f", allocs)
	require.LessOrEqualf(t, allocs, float64(1),
		"sortSortables allocs/op = %.0f, want <= 1 (the make() copy only, no reflect sort)", allocs)
}
