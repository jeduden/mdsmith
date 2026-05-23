package codeblockstyle

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheck_FencedStyle_FencedBlock_NoViolation(t *testing.T) {
	src := []byte("# Hello\n\n```go\nfmt.Println()\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: "fenced"}
	diags := r.Check(f)
	assert.Empty(t, diags)
}

func TestCheck_FencedStyle_IndentedBlock_Flagged(t *testing.T) {
	src := []byte("Some prose.\n\n    code line\n    another line\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: "fenced"}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	d := diags[0]
	assert.Equal(t, "MDS065", d.RuleID)
	assert.Equal(t, 3, d.Line)
	assert.Equal(t, 1, d.Column)
	assert.Equal(t, "code block should use fenced style", d.Message)
}

func TestCheck_IndentedStyle_IndentedBlock_NoViolation(t *testing.T) {
	src := []byte("Some prose.\n\n    code line\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: "indented"}
	diags := r.Check(f)
	assert.Empty(t, diags)
}

func TestCheck_IndentedStyle_FencedBlock_Flagged(t *testing.T) {
	src := []byte("```go\nfmt.Println()\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: "indented"}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, 1, diags[0].Line)
	assert.Equal(t, "code block should use indented style", diags[0].Message)
}

func TestCheck_Consistent_FirstFenced_SubsequentIndented_Flagged(t *testing.T) {
	src := []byte("```go\ncode\n```\n\nText.\n\n    indented\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: "consistent"}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, 7, diags[0].Line)
	assert.Equal(t, "code block should use fenced style", diags[0].Message)
}

func TestCheck_Consistent_FirstIndented_SubsequentFenced_Flagged(t *testing.T) {
	src := []byte("    indented\n\nText.\n\n```\nfenced\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: "consistent"}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, 5, diags[0].Line)
	assert.Equal(t, "code block should use indented style", diags[0].Message)
}

func TestCheck_Consistent_AllSameStyle_NoViolation(t *testing.T) {
	src := []byte("```\na\n```\n\ntext\n\n```\nb\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: "consistent"}
	diags := r.Check(f)
	assert.Empty(t, diags)
}

func TestFix_IndentedToFenced(t *testing.T) {
	src := []byte("Some prose.\n\n    code line\n    another line\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: "fenced"}
	got := r.Fix(f)
	want := "Some prose.\n\n```text\ncode line\nanother line\n```\n"
	assert.Equal(t, want, string(got))
}

func TestFix_IndentedToFenced_TrailingText(t *testing.T) {
	src := []byte("Prose.\n\n    code\n\nMore prose.\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: "fenced"}
	got := r.Fix(f)
	want := "Prose.\n\n```text\ncode\n```\n\nMore prose.\n"
	assert.Equal(t, want, string(got))
}

func TestFix_IndentedToFenced_TabIndent(t *testing.T) {
	src := []byte("Prose.\n\n\tcode\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: "fenced"}
	got := r.Fix(f)
	want := "Prose.\n\n```text\ncode\n```\n"
	assert.Equal(t, want, string(got))
}

// TestFix_IndentedToFenced_TripleBacktickContent_UsesLongerFence verifies
// that when the indented block contains a line consisting of ``` (or
// longer), the converted fence is one backtick longer so the embedded
// triple-backtick does not prematurely close the new block.
func TestFix_IndentedToFenced_TripleBacktickContent_UsesLongerFence(t *testing.T) {
	src := []byte("Prose.\n\n    foo\n    ```\n    bar\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: "fenced"}
	got := r.Fix(f)
	want := "Prose.\n\n````text\nfoo\n```\nbar\n````\n"
	assert.Equal(t, want, string(got))
}

// TestFix_IndentedToFenced_FourBacktickContent_UsesFiveBacktickFence
// verifies the fence length scales with the longest backtick run in
// the content, not just past the default 3.
func TestFix_IndentedToFenced_FourBacktickContent_UsesFiveBacktickFence(t *testing.T) {
	src := []byte("Prose.\n\n    foo\n    ````\n    bar\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: "fenced"}
	got := r.Fix(f)
	want := "Prose.\n\n`````text\nfoo\n````\nbar\n`````\n"
	assert.Equal(t, want, string(got))
}

func TestFix_FencedStyle_FencedBlock_Unchanged(t *testing.T) {
	src := []byte("```go\ncode\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: "fenced"}
	got := r.Fix(f)
	assert.Equal(t, string(src), string(got))
}

func TestFix_IndentedStyle_FencedBlock_Unchanged(t *testing.T) {
	// Reverse conversion is lossy (loses language tag); no autofix.
	src := []byte("```go\ncode\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: "indented"}
	got := r.Fix(f)
	assert.Equal(t, string(src), string(got))
}

func TestFix_Consistent_FirstFenced_LaterIndentedConverted(t *testing.T) {
	src := []byte("```go\nfirst\n```\n\ntext\n\n    second\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: "consistent"}
	got := r.Fix(f)
	want := "```go\nfirst\n```\n\ntext\n\n```text\nsecond\n```\n"
	assert.Equal(t, want, string(got))
}

func TestFix_Consistent_FirstIndented_LaterFencedUnchanged(t *testing.T) {
	src := []byte("    first\n\ntext\n\n```\nsecond\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: "consistent"}
	got := r.Fix(f)
	// No autofix: first block is indented, can't safely convert fenced to indented.
	assert.Equal(t, string(src), string(got))
}

func TestCheck_ID(t *testing.T) {
	r := &Rule{Style: "fenced"}
	assert.Equal(t, "MDS065", r.ID())
}

func TestCheck_Name(t *testing.T) {
	r := &Rule{Style: "fenced"}
	assert.Equal(t, "code-block-style", r.Name())
}

func TestCheck_Category(t *testing.T) {
	r := &Rule{Style: "fenced"}
	assert.Equal(t, "code", r.Category())
}

// --- Configurable tests ---

func TestApplySettings_ValidStyle(t *testing.T) {
	r := &Rule{Style: "fenced"}
	require.NoError(t, r.ApplySettings(map[string]any{"style": "indented"}))
	assert.Equal(t, "indented", r.Style)

	require.NoError(t, r.ApplySettings(map[string]any{"style": "consistent"}))
	assert.Equal(t, "consistent", r.Style)
}

func TestApplySettings_InvalidStyle(t *testing.T) {
	r := &Rule{Style: "fenced"}
	require.Error(t, r.ApplySettings(map[string]any{"style": "invalid"}))
}

func TestApplySettings_InvalidStyleType(t *testing.T) {
	r := &Rule{Style: "fenced"}
	require.Error(t, r.ApplySettings(map[string]any{"style": 42}))
}

func TestApplySettings_UnknownKey(t *testing.T) {
	r := &Rule{Style: "fenced"}
	require.Error(t, r.ApplySettings(map[string]any{"unknown": true}))
}

func TestDefaultSettings(t *testing.T) {
	r := &Rule{}
	ds := r.DefaultSettings()
	assert.Equal(t, "fenced", ds["style"])
}

// --- Directive body skip ---

func TestCheck_SkipsGeneratedRange(t *testing.T) {
	src := []byte("```\ncode\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	// Mark the fenced block lines as generated.
	f.GeneratedRanges = []lint.LineRange{{From: 1, To: 3}}
	r := &Rule{Style: "indented"}
	diags := r.Check(f)
	assert.Empty(t, diags)
}

// --- Nested-block handling ---

func TestCheck_NestedIndentedInList_StillFlagged(t *testing.T) {
	// Indented code block inside a list item is nested. Check still
	// emits the diagnostic — the user should know about the violation.
	src := []byte("- Item:\n\n      code line\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: "fenced"}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, "code block should use fenced style", diags[0].Message)
}

func TestFix_NestedIndentedInList_LeftAlone(t *testing.T) {
	// Autofix is skipped for nested blocks — emitting unindented
	// fences would break the list structure.
	src := []byte("- Item:\n\n      code line\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: "fenced"}
	got := r.Fix(f)
	assert.Equal(t, string(src), string(got))
}

func TestFix_NestedIndentedInBlockquote_LeftAlone(t *testing.T) {
	src := []byte("> Quote:\n>\n>     code\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: "fenced"}
	got := r.Fix(f)
	assert.Equal(t, string(src), string(got))
}

func TestFix_MixedTopLevelAndNested_OnlyTopLevelFixed(t *testing.T) {
	src := []byte("    top level code\n\n- Item:\n\n      nested code\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: "fenced"}
	got := r.Fix(f)
	want := "```text\ntop level code\n```\n\n- Item:\n\n      nested code\n"
	assert.Equal(t, want, string(got))
}

// --- effectiveStyle edge cases ---

func TestEffectiveStyle_UnknownStyle_ReturnsEmpty(t *testing.T) {
	r := &Rule{Style: "invalid"}
	assert.Equal(t, "", r.effectiveStyle(nil))
}

func TestEffectiveStyle_ConsistentNoBlocks_ReturnsEmpty(t *testing.T) {
	r := &Rule{Style: "consistent"}
	assert.Equal(t, "", r.effectiveStyle(nil))
}

func TestCheck_NoBlocks_NoDiagnostics(t *testing.T) {
	src := []byte("# Just prose.\n\nNo code here.\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: "fenced"}
	assert.Empty(t, r.Check(f))
}

func TestCheck_NilFile_NoDiagnostics(t *testing.T) {
	r := &Rule{Style: "fenced"}
	assert.Nil(t, r.Check(nil))
	assert.Nil(t, r.Check(&lint.File{}))
}

func TestFix_NilFile_ReturnsNil(t *testing.T) {
	r := &Rule{Style: "fenced"}
	assert.Nil(t, r.Fix(nil))
}

func TestFix_FileWithNilAST_ReturnsSource(t *testing.T) {
	r := &Rule{Style: "fenced"}
	// Zero-valued File has AST == nil and Source == nil; Fix must not
	// dereference f when f is non-nil but f.AST is nil.
	assert.Nil(t, r.Fix(&lint.File{}))
}

func TestFix_NoIndentedBlocks_ReturnsSourceUnchanged(t *testing.T) {
	src := []byte("```go\ncode\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: "fenced"}
	got := r.Fix(f)
	assert.Equal(t, string(src), string(got))
}

func TestStripIndent_EmptyLine(t *testing.T) {
	assert.Equal(t, "", stripIndent(nil))
	assert.Equal(t, "", stripIndent([]byte("")))
}

func TestStripIndent_PartialIndent(t *testing.T) {
	// Less than 4 spaces — strip what's there.
	assert.Equal(t, "code", stripIndent([]byte("  code")))
}

// --- isTopLevel helper ---

func TestIsTopLevel_NilParent(t *testing.T) {
	// A node without a parent is not top-level.
	src := []byte("```\ncode\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	// f.AST is the Document; its parent is nil.
	assert.False(t, isTopLevel(f.AST))
}

// TestCheck_Consistent_NoBlocks_NoDiagnostics drives effectiveStyle
// to return "" (consistent with no blocks) and Check to early-return.
func TestCheck_Consistent_NoBlocks_NoDiagnostics(t *testing.T) {
	src := []byte("Just prose.\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: "consistent"}
	assert.Empty(t, r.Check(f))
}

// TestCheck_IndentedBlockInGeneratedRange_Skipped covers the indented-
// branch skipBlock guard in collectBlocks.
func TestCheck_IndentedBlockInGeneratedRange_Skipped(t *testing.T) {
	src := []byte("Prose.\n\n    code\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	f.GeneratedRanges = []lint.LineRange{{From: 3, To: 3}}
	r := &Rule{Style: "fenced"}
	assert.Empty(t, r.Check(f))
}
