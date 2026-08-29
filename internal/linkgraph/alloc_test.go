package linkgraph

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// linkDoc builds a document with n inline links whose visible label is
// `label`. Every destination is local so ParseTarget accepts it and the
// walk reaches the Link-append path for each one.
func linkDoc(n int, label string) string {
	var b strings.Builder
	b.WriteString("# Document\n\n")
	for i := 0; i < n; i++ {
		b.WriteString("See [")
		b.WriteString(label)
		b.WriteString("](guide.md#intro) here.\n\n")
	}
	return b.String()
}

// TestExtractLinks_TextNotMaterialisedDuringWalk pins the guideline's
// "the cheapest call is the one you never make"
// (docs/development/high-performance-go.md#skip-work-you-dont-need).
//
// No rule on the `check` path reads a link's visible text — only the
// `backlinks` CLI does — yet ExtractLinks ran mdtext.ExtractPlainText
// for every link, allocating a bytes.Buffer plus its String copy each
// time.
//
// The assertion is scale-invariant rather than a magic total: if the
// text is built during the walk, a longer label (and one carrying
// emphasis, which forces extra buffer growth) costs strictly more
// allocations than a short one. If the text is resolved on demand,
// label size cannot move the number at all. That keeps this test
// measuring only this fix, independent of the other allocations in
// the link pipeline.
func TestExtractLinks_TextNotMaterialisedDuringWalk(t *testing.T) {
	const links = 200

	short := newFile(t, linkDoc(links, "a"))
	long := newFile(t, linkDoc(links,
		"a *considerably* longer link label with **emphasis** inside it"))

	require.Len(t, ExtractLinks(short), links)
	require.Len(t, ExtractLinks(long), links)

	shortAllocs := testing.AllocsPerRun(50, func() { _ = ExtractLinks(short) })
	longAllocs := testing.AllocsPerRun(50, func() { _ = ExtractLinks(long) })

	assert.Equal(t, shortAllocs, longAllocs,
		"ExtractLinks allocations scale with link-label size: the visible "+
			"text is still being materialised during the walk")
}

// TestLinkText_ResolvesOnDemand is the behavioural half of the fix:
// dropping the eager field must not lose the text `backlinks` prints,
// including the emphasis flattening the old linkText helper did.
func TestLinkText_ResolvesOnDemand(t *testing.T) {
	f := newFile(t, "# Doc\n\nSee [the *guide* text](guide.md#intro).\n")

	got := ExtractLinks(f)
	require.Len(t, got, 1)

	assert.Equal(t, "the guide text", got[0].Text(f.Source))
}

// TestImageText_ResolvesOnDemand covers the ExtractImages write site,
// which carried the same eager field.
func TestImageText_ResolvesOnDemand(t *testing.T) {
	f := newFile(t, "# Doc\n\n![a *diagram* alt](img.png)\n")

	got := ExtractImages(f)
	require.Len(t, got, 1)

	assert.Equal(t, "a diagram alt", got[0].Text(f.Source))
}

// TestLinkText_ZeroValueIsEmpty covers a Link built by a caller (or a
// test) that never went through the walk, so it carries no node.
func TestLinkText_ZeroValueIsEmpty(t *testing.T) {
	assert.Empty(t, Link{}.Text([]byte("# Doc\n")))
}
