package flavor

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestBareURLFindingsInTree_NoSchemeSkipsRegex pins the allocation
// cost of BareURLFindingsInTree on prose with no "://" anywhere.
// docs/development/high-performance-go.md: "Gate expensive analyzers
// behind a cheap pre-check... byte-needles gate regex paths." Every
// Text node in every document previously ran bareURLPattern's
// multi-alternative regex unconditionally whenever
// FeatureBareURLAutolinks is accepted (the GFM/goldmark/Pandoc
// default), even though the overwhelming majority of prose has no
// URL at all.
func TestBareURLFindingsInTree_NoSchemeSkipsRegex(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	src := []byte("A short paragraph with several words, some emphasis, " +
		"and a mention of example.com without a scheme, repeated across " +
		"a few more words so the text node is a realistic size.")
	doc := mkDoc(t, string(src))

	const runs = 200
	allocs := testing.AllocsPerRun(runs, func() {
		got := BareURLFindingsInTree(doc.Body, doc.AST, 0)
		if got != nil {
			t.Fatalf("expected no bare URLs, got %v", got)
		}
	})
	t.Logf("BareURLFindingsInTree (no scheme) allocs/op = %.0f", allocs)
	require.LessOrEqualf(t, allocs, float64(0),
		"BareURLFindingsInTree allocs/op = %.0f on scheme-free prose, want 0 (gate should skip the regex)", allocs)
}

func TestBareURLFindingsInTree_StillFindsRealURLs(t *testing.T) {
	doc := mkDoc(t, "See https://example.com/path for details.\n")
	got := BareURLFindingsInTree(doc.Body, doc.AST, 0)
	require.Len(t, got, 1)
	require.Equal(t, FeatureBareURLAutolinks, got[0].Feature)
}

// TestSortFindingsByStart_NoReflectSort pins the allocation cost of
// Detect's finding sort. sort.SliceStable drove reflect.Swapper
// internally (the "reflect in hot paths" anti-pattern in
// docs/development/high-performance-go.md); slices.SortStableFunc
// sorts the concrete Finding values with no reflection.
func TestSortFindingsByStart_NoReflectSort(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	base := []Finding{
		{Feature: FeatureBareURLAutolinks, Start: 30},
		{Feature: FeatureGitHubAlerts, Start: 10},
		{Feature: FeatureBareURLAutolinks, Start: 20},
	}

	const runs = 200
	allocs := testing.AllocsPerRun(runs, func() {
		f := make([]Finding, len(base))
		copy(f, base)
		sortFindingsByStart(f)
	})
	t.Logf("sortFindingsByStart (3 findings, incl. slice copy) allocs/op = %.0f", allocs)
	require.LessOrEqualf(t, allocs, float64(1),
		"sortFindingsByStart allocs/op = %.0f, want <= 1 (the make() copy only, no reflect sort)", allocs)
}
