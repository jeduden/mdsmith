package build

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// manyOutputEntries builds n cache entries, each with a few output
// paths, in reverse output-set-key order — the worst case for a stable
// sort, and representative of a real .mdsmith/build-cache.json with
// many declared build targets.
func manyOutputEntries(n int) []CacheEntry {
	entries := make([]CacheEntry, n)
	for i := 0; i < n; i++ {
		// Descending numeric suffix so the natural key order is the
		// reverse of slice order.
		suffix := n - i
		entries[i] = CacheEntry{
			Outputs: []OutputHash{
				{Path: fmt.Sprintf("dist/out-%03d.txt", suffix), Hash: "h1"},
				{Path: fmt.Sprintf("dist/out-%03d.json", suffix), Hash: "h2"},
			},
			ActionID: fmt.Sprintf("sha256-%d", suffix),
		}
	}
	return entries
}

// TestSortEntriesByOutputKey_SortsByKey pins the correctness of the
// decorate-sort-undecorate refactor: the result must still be ordered
// by outputSetKey, identical to the sort.SliceStable comparator it
// replaced.
func TestSortEntriesByOutputKey_SortsByKey(t *testing.T) {
	entries := manyOutputEntries(5)
	sortEntriesByOutputKey(entries)
	for i := 1; i < len(entries); i++ {
		prevKey := outputSetKey(entries[i-1].outputPaths())
		key := outputSetKey(entries[i].outputPaths())
		require.LessOrEqualf(t, prevKey, key,
			"entries not sorted by output-set key at index %d: %q > %q",
			i, prevKey, key)
	}
}

// TestSortEntriesByOutputKey_AllocBudget pins the allocation cost of
// sortEntriesByOutputKey on a cache with many entries. The replaced
// sort.SliceStable comparator called outputSetKey — which itself
// allocates a sorted copy of the path slice and builds a joined string
// — on BOTH sides of every comparison during the sort, redoing that
// work on every one of the O(n log n) comparisons instead of once per
// entry. It also drove reflect.Swapper. This runs on every `mdsmith
// build` invocation that persists the cache.
func TestSortEntriesByOutputKey_AllocBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	const n = 60
	base := manyOutputEntries(n)

	const runs = 30
	allocs := testing.AllocsPerRun(runs, func() {
		entries := append([]CacheEntry(nil), base...)
		sortEntriesByOutputKey(entries)
	})

	const allocBudget = 260
	t.Logf("sortEntriesByOutputKey allocs/op (%d entries) = %.0f (budget = %d)",
		n, allocs, allocBudget)
	require.LessOrEqualf(t, allocs, float64(allocBudget),
		"sortEntriesByOutputKey allocs/op = %.0f exceeds budget %d: "+
			"outputSetKey must be computed once per entry, not twice "+
			"per comparison, and the sort must not use reflect",
		allocs, allocBudget)
}
