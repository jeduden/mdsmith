package slidevstructure

import (
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// check runs the rule on raw deck source (no front-matter stripping,
// so line numbers are absolute 1-based over the whole string).
func check(t *testing.T, src string) []lint.Diagnostic {
	t.Helper()
	f := &lint.File{Path: "slides.md", Source: []byte(src)}
	f.Lines = splitLines(src)
	return (&Rule{}).Check(f)
}

func splitLines(s string) [][]byte {
	parts := strings.Split(s, "\n")
	out := make([][]byte, len(parts))
	for i, p := range parts {
		out[i] = []byte(p)
	}
	return out
}

func messages(diags []lint.Diagnostic) string {
	var b strings.Builder
	for _, d := range diags {
		b.WriteString(d.Message)
		b.WriteByte('\n')
	}
	return b.String()
}

func TestNoMarkers_ReturnsNil(t *testing.T) {
	// An ordinary Markdown file (no --- fence, no ::slot::) must be a
	// clean early return — this is the allocation-budget hot path.
	src := "# Title\n\nA paragraph.\n\n## Section\n\nMore text.\n"
	diags := check(t, src)
	assert.Nil(t, diags, "ordinary Markdown must produce no diagnostics")
}

func TestMissingRightSlot_TwoCols(t *testing.T) {
	src := "# Intro\n\n---\nlayout: two-cols\n---\n\n# Only left\n\nBullets.\n"
	diags := check(t, src)
	require.Len(t, diags, 1, "got: %s", messages(diags))
	assert.Contains(t, diags[0].Message, "requires a ::right:: slot")
	assert.Equal(t, "MDS073", diags[0].RuleID)
}

func TestTwoCols_WithRightSlot_OK(t *testing.T) {
	src := "# Intro\n\n---\nlayout: two-cols\n---\n\n# Left\n\n::right::\n\n# Right\n"
	diags := check(t, src)
	assert.Empty(t, diags, "a two-cols slide with ::right:: is valid: %s", messages(diags))
}

func TestTwoColsHeader_RequiresLeftAndRight(t *testing.T) {
	src := "---\nlayout: two-cols-header\n---\n\n# Header\n\n::left::\n\nleft only\n"
	diags := check(t, src)
	require.Len(t, diags, 1, "got: %s", messages(diags))
	assert.Contains(t, diags[0].Message, "::right:: slot")
}

func TestOrphanedSlot_ContentVanishes(t *testing.T) {
	src := "---\nlayout: default\n---\n\n# Reused slide\n\n::right::\n\nvanishes\n"
	diags := check(t, src)
	require.Len(t, diags, 1, "got: %s", messages(diags))
	assert.Contains(t, diags[0].Message, "no matching slot in layout \"default\"")
	assert.Equal(t, 7, diags[0].Line, "diagnostic should anchor on the ::right:: line")
}

func TestUnknownLayout_DidYouMean(t *testing.T) {
	src := "---\nlayout: centre\n---\n\n# Typo'd layout\n"
	diags := check(t, src)
	require.Len(t, diags, 1, "got: %s", messages(diags))
	assert.Contains(t, diags[0].Message, "unknown Slidev layout \"centre\"")
	assert.Contains(t, diags[0].Message, "did you mean \"center\"")
	assert.Equal(t, 2, diags[0].Line, "anchor on the layout: line")
}

func TestUnknownFrontmatterKey_DidYouMean(t *testing.T) {
	src := "---\nlayout: center\ntransiton: fade\n---\n\n# Slide\n"
	diags := check(t, src)
	require.Len(t, diags, 1, "got: %s", messages(diags))
	assert.Contains(t, diags[0].Message, "unknown Slidev frontmatter key \"transiton\"")
	assert.Contains(t, diags[0].Message, "did you mean \"transition\"")
	assert.Equal(t, 3, diags[0].Line)
}

func TestArbitraryDataKey_NotFlagged(t *testing.T) {
	// Slidev allows arbitrary pass-through data keys. A key that is not
	// near any real key must not be flagged — only typos are.
	src := "---\nlayout: center\nmyCustomData: hello\n---\n\n# Slide\n"
	diags := check(t, src)
	assert.Empty(t, diags, "arbitrary data key must pass: %s", messages(diags))
}

func TestMissingImageField(t *testing.T) {
	src := "---\nlayout: image-left\n---\n\n# No image\n"
	diags := check(t, src)
	require.Len(t, diags, 1, "got: %s", messages(diags))
	assert.Contains(t, diags[0].Message, "layout \"image-left\" requires the \"image\" frontmatter field")
}

func TestImageLeft_WithImage_OK(t *testing.T) {
	src := "---\nlayout: image-left\nimage: ./a.png\n---\n\n# Has image\n"
	diags := check(t, src)
	assert.Empty(t, diags, "image-left with image: is valid: %s", messages(diags))
}

func TestCustomLayout_NotFlaggedWhenDeclared(t *testing.T) {
	src := "---\nlayout: my-theme-cover\n---\n\n# Themed\n"
	f := &lint.File{Path: "slides.md", Source: []byte(src), Lines: splitLines(src)}
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{"custom-layouts": []any{"my-theme-cover"}}))
	assert.Empty(t, r.Check(f), "declared custom layout must not be flagged")

	// Without the declaration it is flagged as unknown.
	assert.NotEmpty(t, (&Rule{}).Check(f), "undeclared custom layout should be unknown")
}

func TestMultipleSlides_IndependentFrontmatter(t *testing.T) {
	// Two separate silent failures on two slides — proves per-slide parsing.
	src := "# Intro\n\n---\nlayout: two-cols\n---\n\n# Missing right\n\n---\nlayout: centre\n---\n\n# Typo\n"
	diags := check(t, src)
	require.Len(t, diags, 2, "got: %s", messages(diags))
	assert.Contains(t, diags[0].Message, "::right:: slot")
	assert.Contains(t, diags[1].Message, "unknown Slidev layout \"centre\"")
}

func TestHeadmatterLayout_ReadFromFrontMatter_NoFalsePositive(t *testing.T) {
	// The engine strips the deck headmatter into f.FrontMatter. A first
	// slide whose layout lives there, with a matching ::right::, must
	// NOT be flagged as an orphaned slot.
	body := "# Left\n\n::right::\n\n# Right\n"
	f := &lint.File{
		Path:        "slides.md",
		Source:      []byte(body),
		Lines:       splitLines(body),
		FrontMatter: []byte("---\nlayout: two-cols\n---\n"),
	}
	assert.Empty(t, (&Rule{}).Check(f),
		"headmatter layout two-cols + ::right:: is valid: %s", messages((&Rule{}).Check(f)))
}

func TestHeadmatterLayout_MissingSlot_Flagged(t *testing.T) {
	body := "# Left only\n\nBody.\n"
	f := &lint.File{
		Path:        "slides.md",
		Source:      []byte(body),
		Lines:       splitLines(body),
		FrontMatter: []byte("---\nlayout: two-cols\n---\n"),
	}
	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 1, "got: %s", messages(diags))
	assert.Contains(t, diags[0].Message, "requires a ::right:: slot")
}

func TestCodeBlock_SeparatorsAndSlotsIgnored(t *testing.T) {
	// A slide that shows a YAML frontmatter example and a ::right::
	// inside a fenced code block must not be parsed as real slide
	// structure — no false positives.
	src := "# Demo\n\n```md\n---\nlayout: two-cols\n---\n\n::right::\n```\n\nProse.\n"
	diags := check(t, src)
	assert.Empty(t, diags, "code-block content must be literal: %s", messages(diags))
}

func TestApplySettings_RejectsUnknownAndBadType(t *testing.T) {
	assert.Error(t, (&Rule{}).ApplySettings(map[string]any{"nope": 1}))
	assert.Error(t, (&Rule{}).ApplySettings(map[string]any{"custom-layouts": 42}))
}

func TestEnabledByDefault_False(t *testing.T) {
	assert.False(t, (&Rule{}).EnabledByDefault(), "MDS073 is opt-in")
}

// --- coverage for trivial methods and edge-case parsing branches ---

func TestTrivialAndDefaults(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, "MDS073", r.ID())
	assert.Equal(t, "slide-structure", r.Name())
	assert.Equal(t, "structural", r.Category())
	assert.False(t, r.EnabledByDefault())
	ds := r.DefaultSettings()
	assert.Contains(t, ds, "custom-layouts")

	// Check's guard clauses: nil file and a file with no lines.
	assert.Nil(t, r.Check(nil))
	assert.Nil(t, r.Check(&lint.File{Path: "empty.md"}))
}

func TestParseSlides_LeadingHeadmatterFenceInLines(t *testing.T) {
	// f.Lines itself starts with the headmatter fence (front matter not
	// stripped): parseSlides must read it as slide 0's frontmatter.
	src := "---\nlayout: two-cols\n---\n\n# Left\n\n::right::\n\n# Right\n"
	assert.Empty(t, check(t, src), "leading-fence headmatter with ::right:: is valid")
}

func TestParseSlides_PlainSeparatorNoFrontmatter(t *testing.T) {
	// A `---` padded by blank lines is a plain separator, not frontmatter.
	src := "# A\n\nBody.\n\n---\n\n# B\n\nBody two.\n"
	assert.Empty(t, check(t, src))
}

func TestParseSlides_CodeFenceWithRealSeparatorPresent(t *testing.T) {
	// A real separator makes the rule parse; a `---` inside a later code
	// block must be skipped by parseSlides' fence tracking (not counted
	// as a slide boundary or frontmatter).
	src := "# A\n\n---\nlayout: center\n---\n\n# Real\n\n```md\n---\nnot a separator\n---\n```\n\nProse.\n"
	assert.Empty(t, check(t, src))
}

func TestHasFrontmatterAfter_ProseAfterSeparator(t *testing.T) {
	// `---` immediately followed by a prose line (no key, no blank) is a
	// plain separator, not a frontmatter block.
	src := "# A\n\nBody.\n\n---\nJust prose, not a key\n\n# B\n"
	assert.Empty(t, check(t, src))
}

func TestHasFrontmatterAfter_KeysToEndOfFile(t *testing.T) {
	// A separator followed by key lines that run to EOF with no closing
	// fence is not a valid frontmatter block.
	src := "# A\n\nBody.\n\n---\nlayout: center\nzoom: 2"
	assert.Empty(t, check(t, src))
}

func TestReadFrontmatter_SkipsNonKeyLine(t *testing.T) {
	// A line without a colon inside the frontmatter block is skipped.
	src := "---\nlayout: center\nnocolonhere\n---\n\n# Slide\n"
	assert.Empty(t, check(t, src))
}

func TestParseFrontMatterBytes_SkipsFenceIndentedAndNonKey(t *testing.T) {
	// Stripped headmatter (f.FrontMatter) with fences, an indented nested
	// key, and a non-key line — only the top-level layout key is read.
	body := "# Left only\n\nBody.\n"
	f := &lint.File{
		Path:        "slides.md",
		Source:      []byte(body),
		Lines:       splitLines(body),
		FrontMatter: []byte("---\nlayout: two-cols\n  nested: x\nnocolon\n---\n"),
	}
	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 1, "got: %s", messages(diags))
	assert.Contains(t, diags[0].Message, "::right:: slot")
}

func TestEmptyLayoutValue_FlaggedAndDidYouMeanOnEmpty(t *testing.T) {
	// `layout:` with an empty value: unknown layout, and nearest("") must
	// not panic (exercises editDistance's empty-string branch).
	src := "---\nlayout:\n---\n\n# Slide\n"
	diags := check(t, src)
	require.Len(t, diags, 1, "got: %s", messages(diags))
	assert.Contains(t, diags[0].Message, "unknown Slidev layout")
}

func TestEditDistance_EmptyStrings(t *testing.T) {
	assert.Equal(t, 3, editDistance("", "abc"))
	assert.Equal(t, 3, editDistance("abc", ""))
	assert.Equal(t, 0, editDistance("", ""))
}

func TestDottedPassThroughKey_DoesNotBlockFrontmatterParsing(t *testing.T) {
	// A mid-deck frontmatter block that contains a dotted pass-through data
	// key (valid in Slidev, not a pure YAML identifier) must not cause the
	// entire block to be dropped. The layout key in the same block must still
	// be parsed and checked.
	src := "# Slide 1\n\n---\nlayout: image-left\nv1.data: foo\n---\n\n# Slide 2\n"
	diags := check(t, src)
	require.Len(t, diags, 1, "got: %s", messages(diags))
	assert.Contains(t, diags[0].Message, `layout "image-left" requires the "image" frontmatter field`)
}
