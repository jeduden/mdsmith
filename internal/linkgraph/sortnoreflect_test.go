package linkgraph

import "testing"

// TestSortByDepthThenName_NoReflectSort pins the allocation cost of
// sortByDepthThenName, which drove sort.Slice — reflect.Swapper
// internally — the "reflect in hot paths" anti-pattern in
// docs/development/high-performance-go.md. It runs once per colliding-
// basename bucket on every WikilinkIndex (re)build.
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
	allocs := testing.AllocsPerRun(runs, func() {
		sortByDepthThenName(paths)
	})
	t.Logf("sortByDepthThenName allocs/op = %.0f", allocs)
	if allocs > 0 {
		t.Fatalf("sortByDepthThenName allocs/op = %.0f, want 0 (no reflection)", allocs)
	}
}
