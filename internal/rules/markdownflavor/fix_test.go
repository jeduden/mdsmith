package markdownflavor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/rules/markdownflavor/ext"
)

// fixWith parses src into a *lint.File, applies the configured rule's
// Fix, and returns the result as a string for compact assertion.
func fixWith(t *testing.T, flavor, src string) string {
	t.Helper()
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{"flavor": flavor}))
	return string(r.Fix(mkFile(t, src)))
}

// --- heading IDs --------------------------------------------------------

func TestRuleFixHeadingIDRemovesAttributeBlock(t *testing.T) {
	got := fixWith(t, "commonmark", "# Heading {#top}\n\nBody text.\n")
	assert.Equal(t, "# Heading\n\nBody text.\n", got)
}

func TestRuleFixHeadingIDPreservesTrailingNewline(t *testing.T) {
	// A heading at end-of-file with no trailing newline must stay that way.
	got := fixWith(t, "commonmark", "# Heading {#top}")
	assert.Equal(t, "# Heading", got)
}

func TestRuleFixHeadingIDMultiple(t *testing.T) {
	src := "# A {#a}\n\n## B {#b}\n"
	got := fixWith(t, "commonmark", src)
	assert.Equal(t, "# A\n\n## B\n", got)
}

func TestRuleFixHeadingIDGoldmarkAccepts(t *testing.T) {
	// goldmark supports heading IDs, so Fix must not strip them.
	src := "# Heading {#top}\n"
	got := fixWith(t, "goldmark", src)
	assert.Equal(t, src, got)
}

// --- strikethrough ------------------------------------------------------

func TestRuleFixStrikethroughRemovesMarkers(t *testing.T) {
	got := fixWith(t, "commonmark", "Text ~~crossed out~~ here.\n")
	assert.Equal(t, "Text crossed out here.\n", got)
}

func TestRuleFixStrikethroughGFMAccepts(t *testing.T) {
	src := "Text ~~crossed out~~ here.\n"
	got := fixWith(t, "gfm", src)
	assert.Equal(t, src, got)
}

// --- task lists ---------------------------------------------------------

func TestRuleFixTaskListRemovesMarker(t *testing.T) {
	src := "- [x] done\n- [ ] todo\n"
	got := fixWith(t, "commonmark", src)
	assert.Equal(t, "- done\n- todo\n", got)
}

func TestRuleFixTaskListPreservesBullet(t *testing.T) {
	// The plan calls out `*` and `+` bullets explicitly.
	src := "* [X] one\n+ [ ] two\n"
	got := fixWith(t, "commonmark", src)
	assert.Equal(t, "* one\n+ two\n", got)
}

func TestRuleFixTaskListGFMAccepts(t *testing.T) {
	src := "- [x] done\n"
	got := fixWith(t, "gfm", src)
	assert.Equal(t, src, got)
}

// --- superscript --------------------------------------------------------

func TestRuleFixSuperscriptRemovesCarets(t *testing.T) {
	got := fixWith(t, "commonmark", "E = mc^2^ is famous.\n")
	assert.Equal(t, "E = mc2 is famous.\n", got)
}

// --- subscript ----------------------------------------------------------

func TestRuleFixSubscriptRemovesTildes(t *testing.T) {
	got := fixWith(t, "commonmark", "H~2~O is water.\n")
	assert.Equal(t, "H2O is water.\n", got)
}

// --- bare URLs ----------------------------------------------------------

func TestRuleFixBareURLWrapsInAngleBrackets(t *testing.T) {
	got := fixWith(t, "commonmark",
		"Visit https://example.com for details.\n")
	assert.Equal(t,
		"Visit <https://example.com> for details.\n", got)
}

func TestRuleFixBareURLGFMAccepts(t *testing.T) {
	src := "Visit https://example.com for details.\n"
	got := fixWith(t, "gfm", src)
	assert.Equal(t, src, got)
}

// --- combined -----------------------------------------------------------

// TestRuleFixMultipleFeaturesOnOneLine covers the ascending single-
// pass build in applyEdits: two byte-range edits on the same line
// must compose into a contiguous output without one corrupting the
// other's spans.
func TestRuleFixMultipleFeaturesOnOneLine(t *testing.T) {
	got := fixWith(t, "commonmark",
		"# Heading {#top}\n\nText ~~old~~ at https://example.com.\n")
	assert.Equal(t,
		"# Heading\n\nText old at <https://example.com>.\n", got)
}

// TestRuleFixAlertAndByteRangeFeatureCompose verifies that Fix() runs
// alerts stripping AND byte-range fixes in the same call. Before the
// pipeline composition fix, alerts caused an early return that left
// strikethrough / bare URLs / heading IDs unfixed in alert-bearing
// docs; the next fixer pass would catch them, but it required two
// passes for what should be one.
func TestRuleFixAlertAndByteRangeFeatureCompose(t *testing.T) {
	src := "> [!NOTE]\n> Visit https://example.com for ~~old~~ details.\n"
	got := fixWith(t, "commonmark", src)
	assert.Equal(t,
		"> Visit <https://example.com> for old details.\n", got)
}

// TestRuleFixStrikethroughWithNestedInlineSkips guards the robustness
// of delimiterPairEdits: a wrapper containing nested inline markup
// (emphasis, link, code span) cannot be safely unwrapped without
// tracking each nested marker's own span, so the fix declines and the
// diagnostic remains for the user.
func TestRuleFixStrikethroughWithNestedInlineSkips(t *testing.T) {
	src := "Text ~~*bold*~~ here.\n"
	got := fixWith(t, "commonmark", src)
	assert.Equal(t, src, got)
}

// TestRuleFixStrikethroughWithMixedChildrenSkips exercises the
// sibling loop in delimiterPairEdits: a wrapper whose first child is
// Text but whose later children include nested inline markup must
// still skip the fix.
func TestRuleFixStrikethroughWithMixedChildrenSkips(t *testing.T) {
	src := "Text ~~start *mid* end~~ here.\n"
	got := fixWith(t, "commonmark", src)
	assert.Equal(t, src, got)
}

// TestRuleFixFlavorAnyIsNoop covers the early-out paths in Fix and
// fixByteRangeFeatures when the flavor accepts every tracked feature.
func TestRuleFixFlavorAnyIsNoop(t *testing.T) {
	src := "# Head {#top}\n\nText ~~strike~~ and https://example.com\n"
	got := fixWith(t, "any", src)
	assert.Equal(t, src, got)
}

// TestRuleDualNodeEditsSupportedFeaturesReturnNil exercises the
// "feature is supported" branches in dualNodeEdits for super/sub.
// Pandoc supports every dual-parser feature so needsAnyDualFix is
// false in real Fix calls; we invoke dualNodeEdits directly with a
// constructed AST so the supports-branch returns are reached.
func TestRuleDualNodeEditsSupportedFeaturesReturnNil(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{"flavor": "pandoc"}))
	assert.Nil(t, r.dualNodeEdits(nil, &ext.SuperscriptNode{}))
	assert.Nil(t, r.dualNodeEdits(nil, &ext.SubscriptNode{}))
}

// TestApplyEditsHandlesAdjacentEdits guards the single-pass build in
// applyEdits: adjacent edits (e.g. an opening and closing delimiter
// of a strikethrough) must compose into a contiguous output without
// dropping or duplicating bytes between them.
func TestApplyEditsHandlesAdjacentEdits(t *testing.T) {
	src := []byte("ab~~xy~~cd")
	edits := []edit{
		{start: 2, end: 4},                     // opening "~~"
		{start: 6, end: 8},                     // closing "~~"
		{start: 0, end: 0, repl: []byte("> ")}, // pure insertion at start
	}
	got := applyEdits(src, edits)
	assert.Equal(t, "> abxycd", string(got))
}
