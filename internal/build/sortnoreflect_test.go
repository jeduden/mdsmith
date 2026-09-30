package build

import (
	"fmt"
	"strconv"
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
	copyOnly := testing.AllocsPerRun(runs, func() {
		_ = append([]CacheEntry(nil), base...)
	})
	full := testing.AllocsPerRun(runs, func() {
		entries := append([]CacheEntry(nil), base...)
		sortEntriesByOutputKey(entries)
	})
	delta := full - copyOnly
	if delta < 0 {
		delta = 0
	}

	// Measured 242. The budget leaves headroom because the regression it
	// guards against is far larger: computing outputSetKey on both sides
	// of every comparison again measured ~5914.
	const allocBudget = 300
	t.Logf("sortEntriesByOutputKey allocs/op (%d entries, delta over the test's own "+
		"input copy) = %.0f (budget = %d)", n, delta, allocBudget)
	require.LessOrEqualf(t, delta, float64(allocBudget),
		"sortEntriesByOutputKey allocs/op = %.0f exceeds budget %d: "+
			"outputSetKey must be computed once per entry, not twice "+
			"per comparison, and the sort must not use reflect",
		delta, allocBudget)
}

// TestSortEntriesByOutputKey_SingleOrEmptyAllocatesNothing pins the
// early return for fewer than 2 entries — a cache with 0 or 1 build
// targets is already sorted, and Save calls this on every persisted
// cache regardless of size.
func TestSortEntriesByOutputKey_SingleOrEmptyAllocatesNothing(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	for _, entries := range [][]CacheEntry{nil, manyOutputEntries(1)} {
		allocs := testing.AllocsPerRun(200, func() {
			sortEntriesByOutputKey(entries)
		})
		t.Logf("sortEntriesByOutputKey allocs/op (%d entries) = %.0f", len(entries), allocs)
		if allocs > 0 {
			t.Fatalf("sortEntriesByOutputKey allocs/op = %.0f for %d entr(y/ies), want 0",
				allocs, len(entries))
		}
	}
}

// TestSortEntriesByOutputKey_StableForEqualKeys pins that entries with
// the same output set keep their input order, as the sort.SliceStable
// call this replaced did. Put replaces by output set, so Save never
// writes duplicates itself, but a hand-edited or older cache file can
// hold them, and Save must write those back in a fixed order. Thirty
// entries over three output sets put the sort past the size where an
// unstable sort still behaves stably.
func TestSortEntriesByOutputKey_StableForEqualKeys(t *testing.T) {
	sets := []string{"c.txt", "a.txt", "b.txt"}
	entries := make([]CacheEntry, 30)
	for i := range entries {
		entries[i] = CacheEntry{
			Outputs:  []OutputHash{{Path: sets[i%len(sets)]}},
			ActionID: strconv.Itoa(i),
		}
	}
	sortEntriesByOutputKey(entries)

	prev := map[string]int{}
	for i, e := range entries {
		if i > 0 {
			require.LessOrEqual(t, entries[i-1].Outputs[0].Path, e.Outputs[0].Path,
				"not sorted by output set at index %d", i)
		}
		id, err := strconv.Atoi(e.ActionID)
		require.NoError(t, err)
		path := e.Outputs[0].Path
		if last, ok := prev[path]; ok {
			require.Greater(t, id, last, "entries for %s lost their input order", path)
		}
		prev[path] = id
	}
}

// TestOutputSetKey_LengthFramedSortedFormat pins outputSetKey's bytes:
// each path as "<byte length>:<path>|", in sorted order, without
// reordering the caller's slice. The length framing keeps two output
// sets apart even when their joined paths read the same.
func TestOutputSetKey_LengthFramedSortedFormat(t *testing.T) {
	paths := []string{"dist/bb.md", "a|1:x"}
	require.Equal(t, "5:a|1:x|10:dist/bb.md|", outputSetKey(paths))
	require.Equal(t, []string{"dist/bb.md", "a|1:x"}, paths, "input slice was reordered")
	require.NotEqual(t, outputSetKey([]string{"a|1:b"}), outputSetKey([]string{"a", "b"}))
	require.Empty(t, outputSetKey(nil))
}

// TestOutputSetKey_AllocsDoNotScaleWithPaths pins the strconv key
// builder. fmt.Fprintf boxes each path into an interface, one
// allocation per path (74 for 64 paths); strconv.Itoa and
// strings.Builder allocate only the sorted copy and the builder's
// growth steps (10 for 64 paths).
func TestOutputSetKey_AllocsDoNotScaleWithPaths(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	paths := make([]string, 64)
	for i := range paths {
		paths[i] = fmt.Sprintf("dist/out-%03d.txt", len(paths)-i)
	}
	const allocBudget = 16
	allocs := testing.AllocsPerRun(200, func() {
		_ = outputSetKey(paths)
	})
	t.Logf("outputSetKey allocs/op (%d paths) = %.0f (budget = %d)", len(paths), allocs, allocBudget)
	require.LessOrEqualf(t, allocs, float64(allocBudget),
		"outputSetKey allocs/op = %.0f for %d paths: one allocation per path means "+
			"it formats with fmt again", allocs, len(paths))
}
