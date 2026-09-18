package noduplicateheadings

import (
	"strconv"
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/require"
)

// manyHeadingsFixture builds a document with n unique, non-duplicate
// headings — the shape of a large real document (a long reference page
// with one heading per entry), which is exactly the input size where an
// unsized `seen` map pays for several incremental bucket-array
// reallocations as it grows past Go's map load factor, one Check call
// at a time across every such file in a workspace scan.
func manyHeadingsFixture(n int) string {
	var b strings.Builder
	b.WriteString("# Title\n\n")
	for i := 0; i < n; i++ {
		b.WriteString("## Heading " + strconv.Itoa(i) + "\n\n")
		b.WriteString("A short paragraph under this heading.\n\n")
	}
	return b.String()
}

// TestCheck_SeenMapAllocBudget pins the allocation cost of Check's
// `seen` map on a document with many unique headings. The map is sized
// with make(map[string]int) with no capacity hint even though
// astutil.CollectHeadingNodes already returns a slice of known length
// on the line above it — docs/development/high-performance-go.md's
// "Pre-size slices" pattern extends to a map's initial bucket array the
// same way. Verified red (higher allocs/op) against an unsized map
// before the make(map[string]int, len(headings)) fix landed.
func TestCheck_SeenMapAllocBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	const n = 80
	src := []byte(manyHeadingsFixture(n))
	r := &Rule{}

	warm, err := lint.NewFile("warm.md", src)
	require.NoError(t, err)
	diags := r.Check(warm)
	require.Empty(t, diags, "fixture must have no duplicate headings")

	const runs = 50
	parse := testing.AllocsPerRun(runs, func() {
		_, err := lint.NewFile("parse.md", src)
		require.NoError(t, err)
	})
	full := testing.AllocsPerRun(runs, func() {
		f, err := lint.NewFile("check.md", src)
		require.NoError(t, err)
		_ = r.Check(f)
	})
	delta := full - parse
	if delta < 0 {
		delta = 0
	}

	// Measured 91 allocs/op after the fix (was 97 before); budgeted with
	// headroom above the measured value so an unrelated 1-2 alloc drift
	// elsewhere (a Go point release, a dependency bump) doesn't flip this
	// gate, while a regression back to an unsized map (97) still fails it.
	const allocBudget = 94
	t.Logf("MDS005 Check allocs/op (delta over parse, %d headings) = %.0f (budget = %d)",
		n, delta, allocBudget)
	require.LessOrEqualf(t, delta, float64(allocBudget),
		"MDS005 Check allocs/op = %.0f exceeds budget %d: the seen map "+
			"must be pre-sized with len(headings), see rule.go's Check",
		delta, allocBudget)
}
