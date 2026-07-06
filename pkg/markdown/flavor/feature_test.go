package flavor

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// assertSupports checks every feature in the supported set is
// accepted by fl and every feature not in that set is rejected.
func assertSupports(t *testing.T, fl Flavor, supported ...Feature) {
	t.Helper()
	want := map[Feature]bool{}
	for _, feat := range supported {
		want[feat] = true
	}
	for _, feat := range AllFeatures() {
		got := Supports(fl, feat)
		assert.Equal(t, want[feat], got,
			"flavor %s feature %s: want=%v got=%v",
			fl.String(), feat.Name(), want[feat], got)
	}
}

func TestFeatureNameUnknownIsEmpty(t *testing.T) {
	assert.Equal(t, "", Feature(999).Name())
}

func TestFeatureSupportCommonMark(t *testing.T) {
	assertSupports(t, FlavorCommonMark)
}

func TestFeatureSupportGFM(t *testing.T) {
	assertSupports(t, FlavorGFM,
		FeatureTables, FeatureTaskLists, FeatureStrikethrough,
		FeatureBareURLAutolinks, FeatureGitHubAlerts)
}

func TestFeatureSupportGoldmark(t *testing.T) {
	assertSupports(t, FlavorGoldmark,
		FeatureTables, FeatureTaskLists, FeatureStrikethrough,
		FeatureBareURLAutolinks, FeatureHeadingIDs)
}

func TestFeatureSupportAny(t *testing.T) {
	assertSupports(t, FlavorAny, AllFeatures()...)
}

func TestFeatureSupportPandoc(t *testing.T) {
	assertSupports(t, FlavorPandoc,
		FeatureTables, FeatureTaskLists, FeatureStrikethrough,
		FeatureBareURLAutolinks, FeatureFootnotes, FeatureDefinitionLists,
		FeatureHeadingIDs, FeatureSuperscript, FeatureSubscript,
		FeatureMathBlock, FeatureMathInline)
}

func TestFeatureSupportPHPExtra(t *testing.T) {
	assertSupports(t, FlavorPHPExtra,
		FeatureTables, FeatureFootnotes, FeatureDefinitionLists,
		FeatureHeadingIDs, FeatureAbbreviations)
}

func TestFeatureSupportMultiMarkdown(t *testing.T) {
	assertSupports(t, FlavorMultiMarkdown,
		FeatureTables, FeatureFootnotes, FeatureDefinitionLists,
		FeatureHeadingIDs, FeatureAbbreviations,
		FeatureMathBlock, FeatureMathInline)
}

func TestFeatureSupportMyST(t *testing.T) {
	assertSupports(t, FlavorMyST,
		FeatureTables, FeatureStrikethrough, FeatureFootnotes,
		FeatureDefinitionLists, FeatureHeadingIDs,
		FeatureMathBlock, FeatureMathInline)
}

func TestAllFeaturesComplete(t *testing.T) {
	// Ensure AllFeatures enumerates exactly the 13 features we track.
	require.Len(t, AllFeatures(), 13)
}

// TestSupportTable_IsArrayNotMap pins the (Flavor, Feature) support
// table as a flat array rather than a map[Flavor]map[Feature]bool.
// Both types are small dense int enums, so a 2D array removes two
// map-indirection lookups per Supports call per
// docs/development/high-performance-go.md "Sorted slice + binary
// search beats a map for n < ~100" / small fixed-set guidance.
func TestSupportTable_IsArrayNotMap(t *testing.T) {
	typ := reflect.TypeOf(support)
	assert.Equal(t, reflect.Array, typ.Kind(),
		"support table should be a flat array, not a nested map")
}

// TestSupports_OutOfRangeReturnsFalse pins Supports' documented
// contract for an unrecognised Flavor or Feature: false, not a panic
// — the same behavior the map-based implementation gave for a missing
// key. pkg/markdown is a public package (see
// docs/development/markdown-library.md), so an out-of-range caller
// value must degrade gracefully rather than index out of bounds.
func TestSupports_OutOfRangeReturnsFalse(t *testing.T) {
	assert.False(t, Supports(Flavor(999), FeatureTables))
	assert.False(t, Supports(FlavorGFM, Feature(999)))
	assert.False(t, Supports(Flavor(-1), FeatureTables))
	assert.False(t, Supports(FlavorGFM, Feature(-1)))
}

func TestFeatureName(t *testing.T) {
	assert.Equal(t, "tables", FeatureTables.Name())
	assert.Equal(t, "task lists", FeatureTaskLists.Name())
	assert.Equal(t, "strikethrough", FeatureStrikethrough.Name())
	assert.Equal(t, "bare-URL autolinks", FeatureBareURLAutolinks.Name())
	assert.Equal(t, "footnotes", FeatureFootnotes.Name())
	assert.Equal(t, "definition lists", FeatureDefinitionLists.Name())
	assert.Equal(t, "heading IDs", FeatureHeadingIDs.Name())
	assert.Equal(t, "superscript", FeatureSuperscript.Name())
	assert.Equal(t, "subscript", FeatureSubscript.Name())
	assert.Equal(t, "math blocks", FeatureMathBlock.Name())
	assert.Equal(t, "inline math", FeatureMathInline.Name())
	assert.Equal(t, "abbreviations", FeatureAbbreviations.Name())
	assert.Equal(t, "github alerts", FeatureGitHubAlerts.Name())
}
