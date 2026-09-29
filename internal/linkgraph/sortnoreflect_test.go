package linkgraph

import "testing"

// TestSortByDepthThenName_NoReflectSort pins the allocation cost of
// sortByDepthThenName, which drove sort.Slice — reflect.Swapper
// internally — the "reflect in hot paths" anti-pattern in
// docs/development/high-performance-go.md. It runs once per colliding-
// basename bucket on every WikilinkIndex (re)build.
//
// The budget is 1, not 0: sortByDepthThenName decorates each path with
// its precomputed depth (one []keyedPath allocation) so the comparator
// doesn't re-run strings.Count on both operands on every one of the
// O(n log n) comparisons — trading one allocation for O(n) instead of
// O(n log n) strings.Count calls, the same "memoize per-input
// computations" trade-off internal/build/cache.go's
// sortEntriesByOutputKey makes for its own comparator.
func TestSortByDepthThenName_NoReflectSort(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	paths := []string{
		"docs/deep/nested/z.md",
		"a.md",
		"docs/b.md",
	}
	sortByDepthThenName(paths)
	if paths[0] != "a.md" || paths[len(paths)-1] != "docs/deep/nested/z.md" {
		t.Fatalf("sortByDepthThenName did not sort by depth then name: %v", paths)
	}

	const runs = 200
	const allocBudget = 1
	allocs := testing.AllocsPerRun(runs, func() {
		sortByDepthThenName(paths)
	})
	t.Logf("sortByDepthThenName allocs/op = %.0f (budget = %d)", allocs, allocBudget)
	if allocs > allocBudget {
		t.Fatalf("sortByDepthThenName allocs/op = %.0f, want <= %d (no reflection, "+
			"one decorate allocation)", allocs, allocBudget)
	}
}

// TestSortByDepthThenName_SingleOrEmptyAllocatesNothing pins the early
// return for fewer than 2 paths — the common case, since most basenames
// are unique in a real workspace. See sortByDepthThenName's comment for
// why that guard exists (test determinism, not a measured performance
// win — the property already held without it).
func TestSortByDepthThenName_SingleOrEmptyAllocatesNothing(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	for _, paths := range [][]string{nil, {"a.md"}} {
		allocs := testing.AllocsPerRun(200, func() {
			sortByDepthThenName(paths)
		})
		t.Logf("sortByDepthThenName allocs/op (%d paths) = %.0f", len(paths), allocs)
		if allocs > 0 {
			t.Fatalf("sortByDepthThenName allocs/op = %.0f for %d path(s), want 0",
				allocs, len(paths))
		}
	}
}
