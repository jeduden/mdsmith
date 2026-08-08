package markdownflavor

import (
	"testing"

	"github.com/jeduden/mdsmith/pkg/markdown"
	"github.com/jeduden/mdsmith/pkg/markdown/flavor"
	"github.com/stretchr/testify/require"
)

// TestSortFindingsByStart_NoReflectSort and
// TestSortEditsByStart_NoReflectSort pin the allocation cost of the
// two sort helpers extracted from Check and fixByteRangeFeatures.
// sort.SliceStable drove reflect.Swapper internally (the "reflect in
// hot paths" anti-pattern in docs/development/high-performance-go.md);
// slices.SortStableFunc sorts the concrete values directly with no
// reflection.
func TestSortFindingsByStart_NoReflectSort(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	base := []flavor.Finding{
		{Feature: flavor.FeatureBareURLAutolinks, Start: 30},
		{Feature: flavor.FeatureGitHubAlerts, Start: 10},
		{Feature: flavor.FeatureBareURLAutolinks, Start: 20},
	}

	const runs = 200
	allocs := testing.AllocsPerRun(runs, func() {
		f := make([]flavor.Finding, len(base))
		copy(f, base)
		sortFindingsByStart(f)
	})
	t.Logf("sortFindingsByStart (3 findings, incl. slice copy) allocs/op = %.0f", allocs)
	require.LessOrEqualf(t, allocs, float64(1),
		"sortFindingsByStart allocs/op = %.0f, want <= 1 (the make() copy only, no reflect sort)", allocs)

	got := make([]flavor.Finding, len(base))
	copy(got, base)
	sortFindingsByStart(got)
	require.Equal(t, []int{10, 20, 30}, []int{got[0].Start, got[1].Start, got[2].Start})
}

func TestSortEditsByStart_NoReflectSort(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	base := []markdown.Edit{
		{Start: 30, End: 32},
		{Start: 10, End: 12},
		{Start: 20, End: 22},
	}

	const runs = 200
	allocs := testing.AllocsPerRun(runs, func() {
		e := make([]markdown.Edit, len(base))
		copy(e, base)
		sortEditsByStart(e)
	})
	t.Logf("sortEditsByStart (3 edits, incl. slice copy) allocs/op = %.0f", allocs)
	require.LessOrEqualf(t, allocs, float64(1),
		"sortEditsByStart allocs/op = %.0f, want <= 1 (the make() copy only, no reflect sort)", allocs)

	got := make([]markdown.Edit, len(base))
	copy(got, base)
	sortEditsByStart(got)
	require.Equal(t, []int{10, 20, 30}, []int{got[0].Start, got[1].Start, got[2].Start})
}
