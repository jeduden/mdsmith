package noduplicateheadings

import (
	"strconv"
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rules/astutil"
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

// unsizedCheck reimplements Check's AST-walk path exactly, except its
// `seen` map is deliberately left unsized — the reference baseline
// TestCheck_SeenMapPresizedVsUnsized compares the real Check against.
func unsizedCheck(r *Rule, f *lint.File) []lint.Diagnostic {
	var diags []lint.Diagnostic
	seen := make(map[string]int) //nolint:gocritic // deliberately unsized: the comparison baseline
	for _, heading := range astutil.CollectHeadingNodes(f) {
		text := astutil.HeadingText(heading, f.Source)
		line := astutil.HeadingLine(heading, f)
		if d, ok := r.verdict(f, text, line, seen); ok {
			diags = append(diags, d)
		}
	}
	return diags
}

// TestCheck_SeenMapPresizedVsUnsized proves the real Check is actually
// pre-sizing its `seen` map by measuring it, in the same test run,
// against unsizedCheck — an identical AST-walk with a deliberately
// unsized map. A fixed allocs/op budget (the first version of this test
// used one) has a thin, Go-map-implementation-dependent margin between
// the pre-sized and unsized numbers — a toolchain bump changing map
// bucket internals could flip it either direction. Comparing the two
// implementations within the same run sidesteps that: whichever way a
// future Go version shifts the absolute counts, Check must still
// allocate strictly less than unsizedCheck on this input, since
// astutil.CollectHeadingNodes already knows the exact heading count
// before the map is built (docs/development/high-performance-go.md's
// "pre-size slices" pattern extended to a map's bucket array). Both
// sides parse a fresh *lint.File per iteration so the only variable
// between them is the map's initial capacity — reverting Check's
// presize fix collapses this test to Check == unsizedCheck, which
// fails the strict-less assertion.
func TestCheck_SeenMapPresizedVsUnsized(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	const n = 80
	src := []byte(manyHeadingsFixture(n))
	r := &Rule{}

	const runs = 50
	presized := testing.AllocsPerRun(runs, func() {
		f, err := lint.NewFile("presized.md", src)
		require.NoError(t, err)
		_ = r.Check(f)
	})
	unsized := testing.AllocsPerRun(runs, func() {
		f, err := lint.NewFile("unsized.md", src)
		require.NoError(t, err)
		_ = unsizedCheck(r, f)
	})

	t.Logf("Check allocs/op (%d headings): pre-sized = %.0f, unsized reference = %.0f",
		n, presized, unsized)
	require.Lessf(t, presized, unsized,
		"Check allocated %.0f, not fewer than the deliberately-unsized "+
			"reference's %.0f, on %d headings — Check's seen map may no "+
			"longer be pre-sized with len(headings)",
		presized, unsized, n)
}

// TestCheck_SeenMapAllocs logs Check's actual allocation count on a
// many-headings fixture for visibility in test output; informational
// only (not a hard gate — see TestCheck_SeenMapPresizedVsUnsized for
// the enforced regression net on this specific fix).
func TestCheck_SeenMapAllocs(t *testing.T) {
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
	t.Logf("MDS005 Check allocs/op (delta over parse, %d headings) = %.0f", n, delta)
}
