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

func TestFrontmatterWithBlankLineBetweenKeys_OK(t *testing.T) {
	// YAML 1.2 allows blank lines between mapping entries. A frontmatter block
	// like "layout: cover\n\nbackground: /bg.png" must not be dropped — the
	// blank line should not terminate the block early.
	src := "# Slide 1\n\n---\nlayout: image-left\n\nimage: ./bg.png\n---\n\n# Content\n"
	diags := check(t, src)
	assert.Empty(t, diags, "frontmatter with blank line between keys is valid: %s", messages(diags))
}

func TestFrontmatterWithBlankLine_MissingField_Flagged(t *testing.T) {
	// Blank line inside frontmatter does not swallow the layout key: if the
	// required image field is absent, MDS073 must still report it.
	src := "# Slide 1\n\n---\nlayout: image-left\n\n---\n\n# Content\n"
	diags := check(t, src)
	require.Len(t, diags, 1, "got: %s", messages(diags))
	assert.Contains(t, diags[0].Message, `"image" frontmatter field`)
}

func TestFrontmatterWithYAMLComment_OK(t *testing.T) {
	// YAML comment lines (# …) are valid inside a frontmatter block and must
	// not cause the block to be misidentified as plain prose.
	src := "# Slide 1\n\n---\n# set by theme\nlayout: center\n---\n\n# Content\n"
	diags := check(t, src)
	assert.Empty(t, diags, "frontmatter with YAML comment is valid: %s", messages(diags))
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

func TestHasFrontmatterAfter_IndentedLineBeforeKey_NotFrontmatter(t *testing.T) {
	// An indented line that appears BEFORE any key in the block that follows a
	// separator causes hasFrontmatterAfter to return false — the block is plain
	// content, not a frontmatter mapping.  The separator is treated as a
	// horizontal rule so neither slide gains a layout and no diagnostic fires.
	src := "# A\n\n---\n  nested: x\nlayout: center\n---\n\n# B\n"
	assert.Empty(t, check(t, src))
}

func TestHasFrontmatterAfter_IndentedLineAfterKey_OK(t *testing.T) {
	// An indented line that appears AFTER at least one key is a nested YAML
	// value; hasFrontmatterAfter continues past it and the block is parsed as
	// frontmatter normally.
	src := "# A\n\n---\nlayout: center\n  nested: x\n---\n\n# B\n"
	assert.Empty(t, check(t, src))
}

func TestHasFrontmatterAfter_ListEntryBeforeKey_NotFrontmatter(t *testing.T) {
	// A list entry (`- value`) before any key means the block is a YAML
	// sequence, not a mapping — hasFrontmatterAfter returns false and the
	// separator is treated as a horizontal rule.
	src := "# A\n\n---\n- list-item\nlayout: center\n---\n\n# B\n"
	assert.Empty(t, check(t, src))
}

func TestFrontmatterWithListEntry_AfterKey_OK(t *testing.T) {
	// A list entry that appears after a real key (e.g. a YAML multi-value
	// field) is accepted by hasFrontmatterAfter (sawKey=true) and skipped by
	// readFrontmatter — the layout key is still parsed correctly.
	src := "# A\n\n---\nlayout: center\n- list-item\n---\n\n# B\n"
	assert.Empty(t, check(t, src))
}

func TestLayoutCandidates_WithCustomLayouts_DidYouMean(t *testing.T) {
	// When custom layouts are declared, layoutCandidates merges them with the
	// builtins so a near-miss against a custom name is suggested.
	src := "---\nlayout: my-typo\n---\n\n# Slide\n"
	f := &lint.File{Path: "slides.md", Source: []byte(src), Lines: splitLines(src)}
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{"custom-layouts": []any{"my-type"}}))
	diags := r.Check(f)
	require.Len(t, diags, 1, "got: %s", messages(diags))
	assert.Contains(t, diags[0].Message, `did you mean "my-type"`)
}

func TestEditDistance_LongStrings(t *testing.T) {
	// Strings longer than 64 bytes are handled by the fast-path guard that
	// returns la+lb without running the DP table.
	long := "a string that is definitely longer than sixty-four characters total!"
	assert.Equal(t, len(long)+1, editDistance(long, "x"))
	assert.Equal(t, 1+len(long), editDistance("x", long))
}

// --- private helper unit tests ---

func TestSortedKeys(t *testing.T) {
	m := map[string]string{"b": "2", "a": "1", "c": "3"}
	assert.Equal(t, []string{"a", "b", "c"}, sortedKeys(m))
	assert.Empty(t, sortedKeys(nil))
}

func TestNearest(t *testing.T) {
	cands := []string{"center", "cover", "default"}
	assert.Equal(t, "center", nearest("centre", cands))
	assert.Equal(t, "", nearest("zzztotallydifferent", cands))
}

func TestSlideAnchor(t *testing.T) {
	r := &Rule{}
	// fmLine has layout key — returns its line
	s1 := &slide{startLine: 5, fmLine: map[string]int{"layout": 3}}
	assert.Equal(t, 3, r.slideAnchor(s1))
	// no fmLine — falls back to startLine
	s2 := &slide{startLine: 7}
	assert.Equal(t, 7, r.slideAnchor(s2))
	// fmLine present but no layout key — falls back to startLine
	s3 := &slide{startLine: 9, fmLine: map[string]int{"transition": 2}}
	assert.Equal(t, 9, r.slideAnchor(s3))
}

func TestIsCustomLayout(t *testing.T) {
	r := &Rule{CustomLayouts: []string{"my-layout", "other-layout"}}
	assert.True(t, r.isCustomLayout("my-layout"))
	assert.False(t, r.isCustomLayout("unknown"))
	assert.False(t, (&Rule{}).isCustomLayout("anything"))
}

func TestDiag(t *testing.T) {
	r := &Rule{}
	f := &lint.File{Path: "slides.md"}
	d := r.diag(f, 5, "test message")
	assert.Equal(t, "slides.md", d.File)
	assert.Equal(t, 5, d.Line)
	assert.Equal(t, 1, d.Column)
	assert.Equal(t, "MDS073", d.RuleID)
	assert.Equal(t, "slide-structure", d.RuleName)
	assert.Equal(t, "test message", d.Message)
}

func TestCheckUnknownLayout(t *testing.T) {
	r := &Rule{}
	f := &lint.File{Path: "test.md"}
	// no layout declared — no diagnostic, returns false
	unknown, diags := r.checkUnknownLayout(f, nil, "", false, 1)
	assert.False(t, unknown)
	assert.Empty(t, diags)
	// known builtin layout — no diagnostic, returns false
	unknown2, diags2 := r.checkUnknownLayout(f, nil, "center", true, 1)
	assert.False(t, unknown2)
	assert.Empty(t, diags2)
	// unknown layout — diagnostic emitted, returns true
	unknown3, diags3 := r.checkUnknownLayout(f, nil, "zzz-unknown-xyz", true, 1)
	assert.True(t, unknown3)
	require.Len(t, diags3, 1)
	assert.Contains(t, diags3[0].Message, "unknown Slidev layout")
}

func TestCheckMissingSlots(t *testing.T) {
	r := &Rule{}
	f := &lint.File{Path: "test.md"}
	// two-cols requires "right" — not present → diagnostic
	s := &slide{startLine: 1}
	diags := r.checkMissingSlots(s, f, nil, "two-cols", 1)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "::right::")
	// right slot present — no diagnostic
	s2 := &slide{startLine: 1, slots: []slotRef{{name: "right", line: 3}}}
	diags2 := r.checkMissingSlots(s2, f, nil, "two-cols", 1)
	assert.Empty(t, diags2)
	// layout with no required slots — no diagnostic
	diags3 := r.checkMissingSlots(s, f, nil, "default", 1)
	assert.Empty(t, diags3)
}

func TestCheckOrphanedSlots(t *testing.T) {
	r := &Rule{}
	f := &lint.File{Path: "test.md"}
	// slot not valid for layout — diagnostic anchored at slot line
	s := &slide{startLine: 1, slots: []slotRef{{name: "right", line: 5}}}
	diags := r.checkOrphanedSlots(s, f, nil, "default")
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "::right::")
	assert.Equal(t, 5, diags[0].Line)
	// valid slot for layout — no diagnostic
	s2 := &slide{startLine: 1, slots: []slotRef{{name: "right", line: 5}}}
	diags2 := r.checkOrphanedSlots(s2, f, nil, "two-cols")
	assert.Empty(t, diags2)
	// no slots — no diagnostic
	assert.Empty(t, r.checkOrphanedSlots(&slide{startLine: 1}, f, nil, "default"))
}

func TestCheckRequiredField(t *testing.T) {
	r := &Rule{}
	f := &lint.File{Path: "test.md"}
	// image layout requires "image" field — absent → diagnostic
	s := &slide{startLine: 1, fm: map[string]string{"layout": "image"}}
	diags := r.checkRequiredField(s, f, nil, "image", 1)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, `"image"`)
	// image field present — no diagnostic
	s2 := &slide{startLine: 1, fm: map[string]string{"layout": "image", "image": "./bg.png"}}
	diags2 := r.checkRequiredField(s2, f, nil, "image", 1)
	assert.Empty(t, diags2)
	// layout with no required field — no diagnostic
	s3 := &slide{startLine: 1, fm: map[string]string{"layout": "center"}}
	assert.Empty(t, r.checkRequiredField(s3, f, nil, "center", 1))
}

func TestCheckUnknownFMKeys(t *testing.T) {
	r := &Rule{}
	f := &lint.File{Path: "test.md"}
	// all known keys — no diagnostic
	s := &slide{
		startLine: 1,
		fm:        map[string]string{"layout": "center", "transition": "fade"},
		fmLine:    map[string]int{"layout": 2, "transition": 3},
	}
	assert.Empty(t, r.checkUnknownFMKeys(s, f, nil))
	// near-miss typo — diagnostic with did-you-mean
	s2 := &slide{
		startLine: 1,
		fm:        map[string]string{"transiton": "fade"},
		fmLine:    map[string]int{"transiton": 3},
	}
	diags := r.checkUnknownFMKeys(s2, f, nil)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "did you mean")
	// arbitrary data key far from any known key — no diagnostic
	s3 := &slide{
		startLine: 1,
		fm:        map[string]string{"myCustomData": "hello"},
		fmLine:    map[string]int{"myCustomData": 2},
	}
	assert.Empty(t, r.checkUnknownFMKeys(s3, f, nil))
}

func TestCheckSlide(t *testing.T) {
	r := &Rule{}
	f := &lint.File{Path: "test.md"}
	// slide with no fm — no diagnostics (defaults to "default" layout)
	s := &slide{startLine: 1}
	assert.Empty(t, r.checkSlide(s, f, nil))
	// valid builtin layout, no required field, no slots
	s2 := &slide{
		startLine: 1,
		fm:        map[string]string{"layout": "center"},
		fmLine:    map[string]int{"layout": 2},
	}
	assert.Empty(t, r.checkSlide(s2, f, nil))
	// unknown layout fires a diagnostic
	s3 := &slide{
		startLine: 1,
		fm:        map[string]string{"layout": "zzz-bogus-layout"},
		fmLine:    map[string]int{"layout": 2},
	}
	diags := r.checkSlide(s3, f, nil)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "unknown Slidev layout")
}
