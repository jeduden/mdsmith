package astutil

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/require"
)

// headingSortFixture has several out-of-source-order headings once
// nested sections are accounted for, so buildSectionHeadings's sort
// actually reorders elements.
const headingSortFixture = "# Title\n\n" +
	"## A\n\nprose\n\n" +
	"### A1\n\nprose\n\n" +
	"## B\n\nprose\n\n" +
	"### B1\n\nprose\n\n" +
	"## C\n\nprose\n"

// TestBuildSectionHeadings_NoReflectSort pins buildSectionHeadings's
// allocation cost. sort.Slice drove reflect.Swapper internally (the
// "reflect in hot paths" anti-pattern in
// docs/development/high-performance-go.md); slices.SortFunc sorts
// concrete SectionHeading values with no reflection.
func TestBuildSectionHeadings_NoReflectSort(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	f, err := lint.NewFile("headings.md", []byte(headingSortFixture))
	require.NoError(t, err)

	got := buildSectionHeadings(f).([]SectionHeading)
	require.Greater(t, len(got), 1, "fixture must produce multiple headings to exercise the sort")

	const runs = 200
	allocsWithWalk := testing.AllocsPerRun(runs, func() {
		buildSectionHeadings(f)
	})
	t.Logf("buildSectionHeadings allocs/op = %.0f", allocsWithWalk)

	headings := append([]SectionHeading(nil), got...)
	sortAllocs := testing.AllocsPerRun(runs, func() {
		sortSectionHeadings(headings)
	})
	t.Logf("sortSectionHeadings allocs/op = %.0f", sortAllocs)
	require.LessOrEqualf(t, sortAllocs, float64(0),
		"sortSectionHeadings allocs/op = %.0f, want 0 (no reflection)", sortAllocs)
}
