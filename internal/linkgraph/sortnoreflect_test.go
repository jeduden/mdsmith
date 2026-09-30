package linkgraph

import (
	"math/rand"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSortByDepthThenName_NoReflectSort pins the allocation cost of
// sortByDepthThenName, which drove sort.Slice — reflect.Swapper
// internally — the "reflect in hot paths" anti-pattern in
// docs/development/high-performance-go.md. It runs once per colliding-
// basename bucket on every WikilinkIndex (re)build. The sort compares
// the paths in place, so no bucket size allocates.
func TestSortByDepthThenName_NoReflectSort(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	unsorted := []string{
		"docs/deep/nested/z.md",
		"a.md",
		"docs/b.md",
	}
	buf := make([]string, len(unsorted))
	copy(buf, unsorted)
	sortByDepthThenName(buf)
	if buf[0] != "a.md" || buf[len(buf)-1] != "docs/deep/nested/z.md" {
		t.Fatalf("sortByDepthThenName did not sort by depth then name: %v", buf)
	}

	for _, n := range []int{0, 1, len(unsorted)} {
		src := unsorted[:n]
		dst := buf[:n]
		// Refill dst from the unsorted input on every run, so each run
		// moves elements instead of re-sorting an already-sorted slice.
		allocs := testing.AllocsPerRun(200, func() {
			copy(dst, src)
			sortByDepthThenName(dst)
		})
		t.Logf("sortByDepthThenName allocs/op (%d paths) = %.0f", n, allocs)
		if allocs > 0 {
			t.Fatalf("sortByDepthThenName allocs/op = %.0f for %d path(s), want 0 (no reflection)",
				allocs, n)
		}
	}
}

// TestSortByDepthThenName_DeterministicAcrossInputOrders pins the
// bucket order WikilinkIndex.Resolve relies on: it returns the first
// path of a basename bucket, so the shallowest path wins and a name
// tie at one depth goes to the lexically smaller path. The walk fills
// buckets in fs.WalkDir order; whatever order they arrive in, the
// sorted bucket must be the same. Twenty paths put the sort past
// pdqsort's 12-element insertion-sort cutoff.
func TestSortByDepthThenName_DeterministicAcrossInputOrders(t *testing.T) {
	want := make([]string, 0, 20)
	for _, dir := range []string{"", "a/", "b/", "a/x/", "b/y/z/"} {
		for i := range 4 {
			want = append(want, dir+"n"+strconv.Itoa(i)+"/README.md")
		}
	}
	for seed := int64(1); seed <= 5; seed++ {
		got := append([]string(nil), want...)
		rand.New(rand.NewSource(seed)).Shuffle(len(got), func(i, j int) {
			got[i], got[j] = got[j], got[i]
		})
		sortByDepthThenName(got)
		require.Equal(t, want, got, "seed %d", seed)
	}
}
