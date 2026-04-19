package markdownflavor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/lint"
)

func mkFile(t *testing.T, src string) *lint.File {
	t.Helper()
	f, err := lint.NewFile("test.md", []byte(src))
	require.NoError(t, err)
	return f
}

func findings(t *testing.T, src string) []Finding {
	t.Helper()
	return Detect(mkFile(t, src))
}

func hasFeature(fs []Finding, feat Feature) bool {
	for _, f := range fs {
		if f.Feature == feat {
			return true
		}
	}
	return false
}

func TestDetectTable(t *testing.T) {
	fs := findings(t, "| a | b |\n| - | - |\n| 1 | 2 |\n")
	require.True(t, hasFeature(fs, FeatureTables))
	for _, f := range fs {
		if f.Feature == FeatureTables {
			assert.Equal(t, 1, f.Line)
			assert.Equal(t, 1, f.Column)
			return
		}
	}
}

func TestDetectStrikethrough(t *testing.T) {
	fs := findings(t, "hello ~~world~~\n")
	require.True(t, hasFeature(fs, FeatureStrikethrough))
}

func TestDetectTaskList(t *testing.T) {
	fs := findings(t, "- [ ] todo\n- [x] done\n")
	require.True(t, hasFeature(fs, FeatureTaskLists))
}

func TestDetectFootnote(t *testing.T) {
	fs := findings(t, "A paragraph.[^1]\n\n[^1]: footnote body\n")
	require.True(t, hasFeature(fs, FeatureFootnotes))
}

func TestDetectDefinitionList(t *testing.T) {
	fs := findings(t, "term\n:   definition\n")
	require.True(t, hasFeature(fs, FeatureDefinitionLists))
}

func TestDetectBareURLAutolink(t *testing.T) {
	fs := findings(t, "See https://example.com for details.\n")
	require.True(t, hasFeature(fs, FeatureBareURLAutolinks))
}

func TestDetectIgnoresBracketedAutolink(t *testing.T) {
	fs := findings(t, "See <https://example.com> for details.\n")
	assert.False(t, hasFeature(fs, FeatureBareURLAutolinks),
		"<url> bracketed autolinks are CommonMark; must not be flagged as bare-URL autolinks")
}

func TestDetectIgnoresURLInsideLink(t *testing.T) {
	fs := findings(t, "See [here](https://example.com).\n")
	assert.False(t, hasFeature(fs, FeatureBareURLAutolinks),
		"URLs inside Markdown link destinations are not bare")
}

func TestDetectIgnoresURLInCodeSpan(t *testing.T) {
	fs := findings(t, "See `https://example.com` for details.\n")
	assert.False(t, hasFeature(fs, FeatureBareURLAutolinks),
		"URLs inside inline code must not be flagged")
}

func TestDetectIgnoresURLInFencedCode(t *testing.T) {
	src := "```\nhttps://example.com\n```\n"
	fs := findings(t, src)
	assert.False(t, hasFeature(fs, FeatureBareURLAutolinks),
		"URLs inside fenced code blocks must not be flagged")
}

func TestDetectHeadingID(t *testing.T) {
	fs := findings(t, "# Heading {#custom}\n")
	require.True(t, hasFeature(fs, FeatureHeadingIDs))
}

func TestDetectMultipleFeatures(t *testing.T) {
	src := "# Title {#top}\n\n- [ ] task\n\n| a | b |\n| - | - |\n| 1 | 2 |\n\n" +
		"~~old~~ https://example.com\n"
	fs := findings(t, src)
	assert.True(t, hasFeature(fs, FeatureHeadingIDs))
	assert.True(t, hasFeature(fs, FeatureTaskLists))
	assert.True(t, hasFeature(fs, FeatureTables))
	assert.True(t, hasFeature(fs, FeatureStrikethrough))
	assert.True(t, hasFeature(fs, FeatureBareURLAutolinks))
}

func TestDetectEmptyDocument(t *testing.T) {
	fs := findings(t, "\n")
	assert.Empty(t, fs)
}

func TestDetectPlainCommonMark(t *testing.T) {
	src := "# Heading\n\nA paragraph.\n\n- bullet\n- another\n\n" +
		"```go\nfmt.Println(\"hi\")\n```\n"
	fs := findings(t, src)
	assert.Empty(t, fs)
}
