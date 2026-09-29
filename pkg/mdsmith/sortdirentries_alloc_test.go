package mdsmith

import (
	"io/fs"
	"testing"
)

// sortFixtureDirEntries returns an unsorted []fs.DirEntry. Built once
// and reused for both the correctness check and the alloc-count loop
// below: the loop deliberately re-sorts the same (by then sorted)
// slice on every iteration, since sortDirEntries's allocation cost
// does not depend on the input's existing order and the test wants to
// measure the sort call itself, not fixture construction.
func sortFixtureDirEntries() []fs.DirEntry {
	names := []string{"zeta.md", "alpha.md", "mu.md", "beta.md", "gamma.md", "delta.md"}
	ents := make([]fs.DirEntry, len(names))
	for i, n := range names {
		ents[i] = memDirEntry{name: n}
	}
	return ents
}

// TestSortDirEntries_NoReflectSort pins the allocation cost of
// buildDirIndex's per-directory sort. sort.Slice drives
// reflect.Swapper internally (the "reflect in hot paths" anti-pattern
// in docs/development/high-performance-go.md); sortDirEntries must
// sort without it.
func TestSortDirEntries_NoReflectSort(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race; the race detector " +
			"adds allocation bookkeeping that perturbs the count")
	}

	ents := sortFixtureDirEntries()
	sortDirEntries(ents)
	if ents[0].Name() != "alpha.md" || ents[len(ents)-1].Name() != "zeta.md" {
		t.Fatalf("sortDirEntries did not sort: %v", ents)
	}

	const runs = 200
	allocs := testing.AllocsPerRun(runs, func() {
		sortDirEntries(ents)
	})
	t.Logf("sortDirEntries allocs/op = %.0f", allocs)
	if allocs > 0 {
		t.Fatalf("sortDirEntries allocs/op = %.0f, want 0 (no reflection)", allocs)
	}
}
