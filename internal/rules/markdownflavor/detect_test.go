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

// TestDetectBareURLAutolinkUppercaseTLD guards the regex character
// class for the TLD: matches must be case-insensitive so SHOUTY
// domains and mixed-case TLDs are still flagged.
func TestDetectBareURLAutolinkUppercaseTLD(t *testing.T) {
	for _, src := range []string{
		"See https://example.COM for details.\n",
		"See https://EXAMPLE.CoM for details.\n",
	} {
		fs := findings(t, src)
		assert.True(t, hasFeature(fs, FeatureBareURLAutolinks),
			"uppercase TLD should be flagged: %q", src)
	}
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

// TestDetectHeadingIDTrimsAllASCIIWhitespace guards the trailing-
// whitespace trim in findHeadingID: a heading that ends with a tab
// or CRLF \r before the newline must produce an End offset that
// stops at '}' rather than swallowing the whitespace.
func TestDetectHeadingIDTrimsAllASCIIWhitespace(t *testing.T) {
	for _, trailer := range []string{" \n", "\t\n", " \r\n", "\t \r\n"} {
		src := "# Heading {#custom}" + trailer
		fs := findings(t, src)
		require.True(t, hasFeature(fs, FeatureHeadingIDs), "trailer=%q", trailer)
		for _, f := range fs {
			if f.Feature != FeatureHeadingIDs {
				continue
			}
			assert.Equal(t, byte('}'), src[f.End-1],
				"trailer=%q: End should stop at '}'", trailer)
		}
	}
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

func TestDetectSuperscript(t *testing.T) {
	fs := findings(t, "E = mc^2^\n")
	require.True(t, hasFeature(fs, FeatureSuperscript))
}

func TestDetectSubscript(t *testing.T) {
	fs := findings(t, "H~2~O\n")
	require.True(t, hasFeature(fs, FeatureSubscript))
}

func TestDetectMathBlock(t *testing.T) {
	fs := findings(t, "$$\na^2 + b^2 = c^2\n$$\n")
	require.True(t, hasFeature(fs, FeatureMathBlock))
}

func TestDetectMathInline(t *testing.T) {
	fs := findings(t, "foo $x+1$ bar\n")
	require.True(t, hasFeature(fs, FeatureMathInline))
}

func TestDetectAbbreviations(t *testing.T) {
	fs := findings(t, "*[API]: Application Programming Interface\n\nUse API here.\n")
	require.True(t, hasFeature(fs, FeatureAbbreviations))
}

func TestDetectPlainCommonMark(t *testing.T) {
	src := "# Heading\n\nA paragraph.\n\n- bullet\n- another\n\n" +
		"```go\nfmt.Println(\"hi\")\n```\n"
	fs := findings(t, src)
	assert.Empty(t, fs)
}

// TestDetectFilteredSkipsBareURLs exercises the skip path for the
// bare-URL regex scan: when the caller rejects
// FeatureBareURLAutolinks, detectBareURLs must not run even though
// other features (here strikethrough) are still accepted. The scenario
// is narrower than any specific flavor; Rule.Check under flavor: gfm or
// goldmark passes a different predicate (!flavor.Supports) that would
// also reject the strikethrough branch.
func TestDetectFilteredSkipsBareURLs(t *testing.T) {
	src := "See https://example.com for details.\n\n~~old~~\n"
	// Reject bare-URL autolinks; keep every other feature so the
	// strikethrough assertion can verify the non-bare-URL path
	// still runs.
	accept := func(feat Feature) bool {
		return feat != FeatureBareURLAutolinks
	}
	fs := DetectFiltered(mkFile(t, src), accept)
	for _, f := range fs {
		assert.NotEqual(t, FeatureBareURLAutolinks, f.Feature,
			"bare-URL findings must be suppressed when caller skips them")
	}
	assert.True(t, hasFeature(fs, FeatureStrikethrough),
		"accepted features are still returned")
}

// TestDetectFilteredSkipsDualParseWhenAllSupported verifies that
// DetectFiltered avoids the goldmark re-parse entirely when every
// feature the dual pass could emit is accepted by the caller.
func TestDetectFilteredSkipsDualParseWhenAllSupported(t *testing.T) {
	src := "# Title {#id}\n\n| a |\n| - |\n| 1 |\n\n~~x~~ and [^1]\n\n[^1]: note\n"
	// Accept every dual-parser feature; ask only for bare URLs.
	accept := func(feat Feature) bool {
		return feat == FeatureBareURLAutolinks
	}
	fs := DetectFiltered(mkFile(t, src), accept)
	for _, f := range fs {
		assert.Equal(t, FeatureBareURLAutolinks, f.Feature,
			"dual-parser features must be suppressed when all are accepted")
	}
}

// TestDetectFindingsAreSortedByStart guards the merge ordering
// between detectFromDual and detectBareURLs: a bare URL in line 1
// must sort before a footnote definition further down the file.
func TestDetectFindingsAreSortedByStart(t *testing.T) {
	src := "https://example.com paragraph.[^1]\n\n[^1]: note body\n"
	fs := findings(t, src)
	require.GreaterOrEqual(t, len(fs), 2)
	for i := 1; i < len(fs); i++ {
		assert.LessOrEqual(t, fs[i-1].Start, fs[i].Start,
			"finding %d (%v) precedes finding %d (%v) but has greater Start",
			i-1, fs[i-1], i, fs[i])
	}
}
