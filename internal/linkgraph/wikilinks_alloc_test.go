package linkgraph

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// noWikilinksFixture has a fenced code block and a link so
// CollectCodeBlockLines/CollectPIBlockLines have real work to skip,
// but no "[[" anywhere — the common case for the vast majority of
// Markdown files, since wikilinks are an Obsidian-specific extension.
const noWikilinksFixture = "# Title\n\n" +
	"See [other](other.md) for details.\n\n" +
	"```go\nfunc f() int { return 0 }\n```\n\n" +
	"More prose after the code block.\n"

// TestExtractWikiLinks_NoMarkerSkipsCollection pins the allocation
// cost of ExtractWikiLinks on a file with no "[[" anywhere.
// docs/development/high-performance-go.md: "Gate expensive analyzers
// behind a cheap pre-check... byte-needles gate regex paths." Before
// the "[[" gate, every call unconditionally built the code-block,
// PI-block, and code-span line sets and ran the wikilink regex over
// the whole source, even though MDS027 (the only caller) runs on
// every default-enabled-rule Check and wikilinks are rare.
func TestExtractWikiLinks_NoMarkerSkipsCollection(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	const runs = 100
	allocs := testing.AllocsPerRun(runs, func() {
		f := newFile(t, noWikilinksFixture)
		got := ExtractWikiLinks(f)
		if got != nil {
			t.Fatalf("expected no wikilinks, got %v", got)
		}
	})
	t.Logf("ExtractWikiLinks (no marker, incl. NewFile parse) allocs/op = %.0f", allocs)
	// The bulk of this is lint.NewFile's own parse cost; the gate's
	// contribution is proven by TestExtractWikiLinks_GateSkipsCodeBlockCollection
	// below, which isolates the pre-existing-File case.
	require.Greater(t, allocs, float64(0))
}

// TestExtractWikiLinks_GateSkipsCodeBlockCollection isolates the
// gate's own saving: on a warm *lint.File (parse cost already paid),
// ExtractWikiLinks with no "[[" must not build the code-block, PI-block,
// or code-span line sets — those memoize on f itself, so triggering
// them here would show up as allocations on this call.
func TestExtractWikiLinks_GateSkipsCodeBlockCollection(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	f := newFile(t, noWikilinksFixture)
	const runs = 100
	allocs := testing.AllocsPerRun(runs, func() {
		got := ExtractWikiLinks(f)
		if got != nil {
			t.Fatalf("expected no wikilinks, got %v", got)
		}
	})
	t.Logf("ExtractWikiLinks (no marker, warm File) allocs/op = %.0f", allocs)
	require.LessOrEqualf(t, allocs, float64(0),
		"ExtractWikiLinks allocs/op = %.0f on a no-wikilink file, want 0 (gate should skip all collection work)", allocs)
}

func TestExtractWikiLinks_StillFindsRealWikilinks(t *testing.T) {
	f := newFile(t, "# Title\n\nSee [[Other Page]] for details.\n")
	got := ExtractWikiLinks(f)
	require.Len(t, got, 1)
	require.Equal(t, "Other Page", got[0].Target)
}
