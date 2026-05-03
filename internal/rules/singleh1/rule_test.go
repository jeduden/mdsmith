package singleh1

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	goldast "github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

func newFile(t *testing.T, src string) *lint.File {
	t.Helper()
	f, err := lint.NewFile("test.md", []byte(src))
	require.NoError(t, err)
	return f
}

func newFileStrip(t *testing.T, src string) *lint.File {
	t.Helper()
	f, err := lint.NewFileFromSource("test.md", []byte(src), true)
	require.NoError(t, err)
	return f
}

// --- Check tests ---

func TestCheck_OneH1_NoViolation(t *testing.T) {
	f := newFile(t, "# Title\n\n## Section\n\n### Sub\n")
	diags := (&Rule{}).Check(f)
	assert.Empty(t, diags)
}

func TestCheck_NoHeadings_NoViolation(t *testing.T) {
	f := newFile(t, "Just text.\n")
	diags := (&Rule{}).Check(f)
	assert.Empty(t, diags)
}

func TestCheck_ZeroH1_NoViolation(t *testing.T) {
	f := newFile(t, "## Section\n\n### Sub\n")
	diags := (&Rule{}).Check(f)
	assert.Empty(t, diags)
}

func TestCheck_TwoH1s_SecondFlagged(t *testing.T) {
	f := newFile(t, "# First\n\n## Section\n\n# Second\n")
	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, "extra H1 heading; only one H1 is allowed per file", diags[0].Message)
	assert.Equal(t, 5, diags[0].Line)
}

func TestCheck_ThreeH1s_SecondAndThirdFlagged(t *testing.T) {
	f := newFile(t, "# First\n\n# Second\n\n# Third\n")
	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 2)
	assert.Equal(t, 3, diags[0].Line)
	assert.Equal(t, 5, diags[1].Line)
}

func TestCheck_SetextH1_Second_Flagged(t *testing.T) {
	src := "# First\n\nSecond\n======\n"
	f := newFile(t, src)
	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, "extra H1 heading; only one H1 is allowed per file", diags[0].Message)
	assert.Equal(t, 3, diags[0].Line)
}

func TestCheck_FrontMatterTitle_Conflict(t *testing.T) {
	src := "---\ntitle: Foo\n---\n\n# Title\n"
	f := newFileStrip(t, src)
	diags := (&Rule{FrontMatterTitle: "title"}).Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, "h1 heading conflicts with front-matter title", diags[0].Message)
}

func TestCheck_FrontMatterTitle_Empty_Field_NoConflict(t *testing.T) {
	// front-matter-title: "" disables the check
	src := "---\ntitle: Foo\n---\n\n# Title\n"
	f := newFileStrip(t, src)
	diags := (&Rule{FrontMatterTitle: ""}).Check(f)
	assert.Empty(t, diags)
}

func TestCheck_FrontMatterTitle_FieldAbsent_NoConflict(t *testing.T) {
	// front matter has no 'title' field
	src := "---\nauthor: Alice\n---\n\n# Title\n"
	f := newFileStrip(t, src)
	diags := (&Rule{FrontMatterTitle: "title"}).Check(f)
	assert.Empty(t, diags)
}

func TestCheck_FrontMatterTitle_NoFrontMatter_NoConflict(t *testing.T) {
	f := newFile(t, "# Title\n")
	diags := (&Rule{FrontMatterTitle: "title"}).Check(f)
	assert.Empty(t, diags)
}

func TestCheck_RuleID(t *testing.T) {
	f := newFile(t, "# First\n\n# Second\n")
	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, "MDS051", diags[0].RuleID)
	assert.Equal(t, "single-h1", diags[0].RuleName)
}

// --- Fix tests ---

func TestFix_TwoH1s_DemotesSecond(t *testing.T) {
	src := "# First\n\n# Second\n"
	f := newFile(t, src)
	got := (&Rule{}).Fix(f)
	assert.Equal(t, "# First\n\n## Second\n", string(got))
}

func TestFix_ThreeH1s_DemotesSecondAndThird(t *testing.T) {
	src := "# First\n\n# Second\n\n# Third\n"
	f := newFile(t, src)
	got := (&Rule{}).Fix(f)
	assert.Equal(t, "# First\n\n## Second\n\n## Third\n", string(got))
}

func TestFix_SetextH1_DemotesToSetextH2(t *testing.T) {
	src := "# First\n\nSecond\n======\n"
	f := newFile(t, src)
	got := (&Rule{}).Fix(f)
	assert.Equal(t, "# First\n\nSecond\n------\n", string(got))
}

func TestFix_FrontMatterConflict_NoFix(t *testing.T) {
	src := "---\ntitle: Foo\n---\n\n# Title\n"
	f := newFileStrip(t, src)
	got := (&Rule{FrontMatterTitle: "title"}).Fix(f)
	// Source unchanged (Fix returns original source when only FM conflict present)
	assert.Equal(t, string(f.Source), string(got))
}

func TestFix_OneH1_NoChange(t *testing.T) {
	src := "# Title\n\n## Section\n"
	f := newFile(t, src)
	got := (&Rule{}).Fix(f)
	assert.Equal(t, src, string(got))
}

// --- Configurable ---

func TestApplySettings_FrontMatterTitle(t *testing.T) {
	r := &Rule{FrontMatterTitle: "title"}
	require.NoError(t, r.ApplySettings(map[string]any{"front-matter-title": ""}))
	assert.Equal(t, "", r.FrontMatterTitle)
}

func TestApplySettings_UnknownKey(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"bogus": "x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown setting")
}

func TestDefaultSettings(t *testing.T) {
	r := &Rule{FrontMatterTitle: "title"}
	ds := r.DefaultSettings()
	assert.Equal(t, "title", ds["front-matter-title"])
}

func TestEnabledByDefault(t *testing.T) {
	assert.False(t, (&Rule{}).EnabledByDefault())
}

func TestCategory(t *testing.T) {
	assert.Equal(t, "heading", (&Rule{}).Category())
}

// TestCheck_FrontMatterTitle_MultipleH1s exercises the branch where
// hasFMTitle=true AND there are 2+ in-document H1s: the first H1 gets
// the front-matter conflict message and subsequent ones get the extra-H1 message.
func TestCheck_FrontMatterTitle_MultipleH1s(t *testing.T) {
	src := "---\ntitle: Foo\n---\n\n# First\n\n# Second\n"
	f := newFileStrip(t, src)
	diags := (&Rule{FrontMatterTitle: "title"}).Check(f)
	require.Len(t, diags, 2)
	assert.Equal(t, "h1 heading conflicts with front-matter title", diags[0].Message)
	assert.Equal(t, "extra H1 heading; only one H1 is allowed per file", diags[1].Message)
}

// TestFix_FrontMatterConflict_ExtraH1s exercises the branch where
// hasFMTitle=true AND there are 2+ H1s: Fix should demote the second H1
// (which is a pure extra) but leave the first (front-matter conflict) alone.
func TestFix_FrontMatterConflict_ExtraH1s(t *testing.T) {
	src := "---\ntitle: Foo\n---\n\n# First\n\n# Second\n"
	f := newFileStrip(t, src)
	got := (&Rule{FrontMatterTitle: "title"}).Fix(f)
	// First H1 is untouched; second H1 is demoted to H2.
	// f.Source starts with a blank line (the blank line between --- and # First
	// after front-matter stripping), so the fixed output does too.
	assert.Equal(t, "\n# First\n\n## Second\n", string(got))
}

// TestCheck_HeadingNotOnFirstLine exercises headingLineStart's offset-rewind
// loop, which runs for headings after line 1.
func TestCheck_HeadingNotOnFirstLine(t *testing.T) {
	// H1s on lines 3 and 5 exercise the rewind loop; line 1 takes the trivial path.
	src := "# First\n\n# Second\n\n# Third\n"
	f := newFile(t, src)
	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 2)
	assert.Equal(t, 3, diags[0].Line)
	assert.Equal(t, 5, diags[1].Line)
}

// TestCheck_HeadingWithEmphasis exercises the headingLineStart walk path for
// headings whose inline content includes non-Text nodes (e.g. Emphasis).
func TestCheck_HeadingWithEmphasis(t *testing.T) {
	src := "# *First*\n\n# *Second*\n"
	f := newFile(t, src)
	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, 3, diags[0].Line)
}

// TestFix_HeadingWithEmphasis ensures Fix demotes an emphasized H1.
func TestFix_HeadingWithEmphasis(t *testing.T) {
	src := "# First\n\n# *Second*\n"
	f := newFile(t, src)
	got := (&Rule{}).Fix(f)
	assert.Equal(t, "# First\n\n## *Second*\n", string(got))
}

// TestApplySettings_NonStringValue exercises the type-assertion failure path.
func TestApplySettings_NonStringValue(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"front-matter-title": 42})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a string")
}

// TestFrontMatterHasTitle_BadYAML exercises the yaml.Unmarshal error path.
func TestFrontMatterHasTitle_BadYAML(t *testing.T) {
	// Construct a file whose FrontMatter contains invalid YAML.
	f, err := lint.NewFileFromSource("test.md", []byte("---\n: bad: yaml: [\n---\n\n# Title\n"), true)
	require.NoError(t, err)
	r := &Rule{FrontMatterTitle: "title"}
	// Should not panic; bad YAML means title is absent → no conflict.
	assert.False(t, r.frontMatterHasTitle(f))
}

// TestFrontMatterHasTitle_NonStringValue exercises the path where the
// configured field is present but its YAML value is not a string.
func TestFrontMatterHasTitle_NonStringValue(t *testing.T) {
	src := "---\ntitle: 42\n---\n\n# Title\n"
	f := newFileStrip(t, src)
	r := &Rule{FrontMatterTitle: "title"}
	assert.False(t, r.frontMatterHasTitle(f))
}

// TestExtractYAMLBody strips delimiters correctly.
func TestExtractYAMLBody(t *testing.T) {
	got := extractYAMLBody([]byte("---\ntitle: Foo\n---\n"))
	assert.Equal(t, "title: Foo\n", string(got))
}

// TestFrontMatterHasTitle_SourceFallback covers the path where f.FrontMatter is
// empty (lint.NewFile called) but the source itself contains front matter.
func TestFrontMatterHasTitle_SourceFallback(t *testing.T) {
	// newFile does not strip FM, so f.FrontMatter is nil; rule extracts from source.
	f := newFile(t, "---\ntitle: From Source\n---\n\n# Title\n")
	r := &Rule{FrontMatterTitle: "title"}
	assert.True(t, r.frontMatterHasTitle(f))
}

// TestFrontMatterHasTitle_YAMLAliasRejected checks that alias-bearing YAML is
// rejected (treated as no title) rather than expanded.
func TestFrontMatterHasTitle_YAMLAliasRejected(t *testing.T) {
	f := newFileStrip(t, "---\nbase: &a Foo\ntitle: *a\n---\n\n# Title\n")
	r := &Rule{FrontMatterTitle: "title"}
	assert.False(t, r.frontMatterHasTitle(f))
}

// TestFrontMatterHasTitle_EmptyBody covers the extractYAMLBody→empty path.
func TestFrontMatterHasTitle_EmptyBody(t *testing.T) {
	// Front matter with just delimiters and no YAML fields.
	f := newFileStrip(t, "---\n---\n\n# Title\n")
	r := &Rule{FrontMatterTitle: "title"}
	assert.False(t, r.frontMatterHasTitle(f))
}

// TestCheck_IndentedATXHeading exercises the leading-space handling in
// isATXHeadingAt and buildDemoteReplacement for CommonMark-allowed indented ATX.
func TestCheck_IndentedATXHeading(t *testing.T) {
	// CommonMark allows 1-3 spaces before the # marker.
	src := "# First\n\n  # Second\n"
	f := newFile(t, src)
	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, "extra H1 heading; only one H1 is allowed per file", diags[0].Message)
}

// TestFix_IndentedATXHeading verifies Fix demotes a space-indented H1 correctly.
func TestFix_IndentedATXHeading(t *testing.T) {
	src := "# First\n\n  # Second\n"
	f := newFile(t, src)
	got := (&Rule{}).Fix(f)
	assert.Equal(t, "# First\n\n  ## Second\n", string(got))
}

// TestHeadingLineStart_NoLines_WithTextChild covers headingLineStart's fallback
// to child text segments when Lines().Len() == 0 (e.g. certain ATX headings).
func TestHeadingLineStart_NoLines_WithTextChild(t *testing.T) {
	// Build a synthetic heading with no Lines() but one Text child.
	source := []byte("# Title\n")
	h := goldast.NewHeading(1)
	seg := text.NewSegment(2, 7) // points at "Title"
	child := goldast.NewText()
	child.Segment = seg
	h.AppendChild(h, child)
	// Lines().Len() == 0, so headingLineStart descends to find offset 2,
	// then rewinds to the '#' at offset 0.
	got := headingLineStart(h, source)
	assert.Equal(t, 0, got)
}

// TestHeadingLineStart_NoLines_NestedTextChild covers the walk-continue path
// when Lines().Len() == 0 and the text segment is nested inside an inline node
// (e.g. Emphasis contains Text).
func TestHeadingLineStart_NoLines_NestedTextChild(t *testing.T) {
	// source: "# *Title*\n" — emphasis wraps the text
	source := []byte("# *Title*\n")
	h := goldast.NewHeading(1)
	em := goldast.NewEmphasis(1)
	seg := text.NewSegment(3, 8) // points at "Title" inside *...*
	inner := goldast.NewText()
	inner.Segment = seg
	em.AppendChild(em, inner)
	h.AppendChild(h, em)
	// Walk skips the Emphasis node (not Text) then finds the inner Text.
	got := headingLineStart(h, source)
	assert.Equal(t, 0, got)
}

// TestHeadingLineStart_NoLines_NoChildren covers the -1 sentinel path when
// Lines().Len() == 0 and there are no child text nodes.
func TestHeadingLineStart_NoLines_NoChildren(t *testing.T) {
	source := []byte("# Title\n")
	h := goldast.NewHeading(1)
	assert.Equal(t, -1, headingLineStart(h, source))
}

// TestBuildDemoteReplacement_ATX_NoLines_NoChildren verifies that
// buildDemoteReplacement returns false when headingLineStart returns -1.
func TestBuildDemoteReplacement_ATX_NoLines_NoChildren(t *testing.T) {
	source := []byte("# First\n\n# Second\n")
	h := goldast.NewHeading(1)
	_, ok := buildDemoteReplacement(h, source)
	assert.False(t, ok)
}

// TestBuildDemoteReplacement_Setext_NoLines_NoChildren verifies that the
// setext path in buildDemoteReplacement returns false when headingLineStart
// returns -1.  We construct an H1 with no Lines() and no children so that
// isATXHeading returns false (start<0) and the setext branch also exits early.
func TestBuildDemoteReplacement_Setext_NoLines_NoChildren(t *testing.T) {
	source := []byte("Title\n=====\n")
	// headingLineStart returns -1, so buildDemoteReplacement returns false immediately.
	h := goldast.NewHeading(1)
	_, ok := buildDemoteReplacement(h, source)
	assert.False(t, ok)
}
