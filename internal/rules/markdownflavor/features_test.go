package markdownflavor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFlavor(t *testing.T) {
	tests := []struct {
		in   string
		want Flavor
		ok   bool
	}{
		{"commonmark", FlavorCommonMark, true},
		{"gfm", FlavorGFM, true},
		{"goldmark", FlavorGoldmark, true},
		{"any", FlavorAny, true},
		{"pandoc", FlavorPandoc, true},
		{"phpextra", FlavorPHPExtra, true},
		{"multimarkdown", FlavorMultiMarkdown, true},
		{"myst", FlavorMyST, true},
		{"GFM", 0, false},
		{"", 0, false},
		{"markdown", 0, false},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got, ok := ParseFlavor(tc.in)
			assert.Equal(t, tc.ok, ok)
			if tc.ok {
				assert.Equal(t, tc.want, got)
			}
		})
	}
}

func TestFlavorString(t *testing.T) {
	assert.Equal(t, "commonmark", FlavorCommonMark.String())
	assert.Equal(t, "gfm", FlavorGFM.String())
	assert.Equal(t, "goldmark", FlavorGoldmark.String())
	assert.Equal(t, "any", FlavorAny.String())
	assert.Equal(t, "pandoc", FlavorPandoc.String())
	assert.Equal(t, "phpextra", FlavorPHPExtra.String())
	assert.Equal(t, "multimarkdown", FlavorMultiMarkdown.String())
	assert.Equal(t, "myst", FlavorMyST.String())
}

// assertSupports checks every feature in the supported set is
// accepted by flavor and every feature not in that set is rejected.
func assertSupports(t *testing.T, f Flavor, supported ...Feature) {
	t.Helper()
	want := map[Feature]bool{}
	for _, feat := range supported {
		want[feat] = true
	}
	for _, feat := range AllFeatures() {
		got := f.Supports(feat)
		assert.Equal(t, want[feat], got,
			"flavor %s feature %s: want=%v got=%v",
			f.String(), feat.Name(), want[feat], got)
	}
}

func TestFlavorStringUnknownIsEmpty(t *testing.T) {
	var zero Flavor
	assert.Equal(t, "", zero.String())
	assert.Equal(t, "", Flavor(999).String())
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
		FeatureBareURLAutolinks)
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
	// Ensure AllFeatures enumerates exactly the 12 features we track.
	require.Len(t, AllFeatures(), 12)
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
}
