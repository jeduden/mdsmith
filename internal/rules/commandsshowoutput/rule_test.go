package commandsshowoutput

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheck_SingleCommandNoOutput_Flagged(t *testing.T) {
	src := []byte("```sh\n$ ls\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	d := diags[0]
	assert.Equal(t, "MDS066", d.RuleID)
	assert.Equal(t, 1, d.Line)
	assert.Equal(t, 1, d.Column)
	assert.Equal(t, "commands shown with $ prefix but no output", d.Message)
}

func TestCheck_MultipleCommandsNoOutput_Flagged(t *testing.T) {
	src := []byte("```sh\n$ ls\n$ pwd\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, 1, diags[0].Line)
}

func TestCheck_CommandWithOutput_NotFlagged(t *testing.T) {
	src := []byte("```sh\n$ ls\nfoo bar\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	assert.Empty(t, diags)
}

func TestCheck_NoCommands_NotFlagged(t *testing.T) {
	src := []byte("```sh\nls\npwd\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	assert.Empty(t, diags)
}

func TestCheck_BlankLinesBetweenCommands_Flagged(t *testing.T) {
	src := []byte("```sh\n$ ls\n\n$ pwd\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 1)
}

func TestCheck_EmptyBlock_NotFlagged(t *testing.T) {
	src := []byte("```sh\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	assert.Empty(t, diags)
}

func TestCheck_IndentedCodeBlock_NotChecked(t *testing.T) {
	// MDS066 only inspects fenced code blocks.
	src := []byte("Prose.\n\n    $ ls\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	assert.Empty(t, diags)
}

func TestCheck_DollarWithoutSpace_NotFlagged(t *testing.T) {
	// "$cmd" (no space after $) is not a recognized prompt.
	src := []byte("```sh\n$cmd\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	assert.Empty(t, diags)
}

func TestCheck_BareDollar_NotFlagged(t *testing.T) {
	src := []byte("```sh\n$\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	assert.Empty(t, diags)
}

func TestCheck_MultipleBlocks_OnlyOffenderFlagged(t *testing.T) {
	src := []byte("```sh\n$ ls\n```\n\nText.\n\n```sh\n$ ls\nfoo\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, 1, diags[0].Line)
}

func TestFix_StripsDollarSpace(t *testing.T) {
	src := []byte("```sh\n$ ls\n$ pwd\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	got := r.Fix(f)
	want := "```sh\nls\npwd\n```\n"
	assert.Equal(t, want, string(got))
}

func TestFix_PreservesBlankLines(t *testing.T) {
	src := []byte("```sh\n$ ls\n\n$ pwd\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	got := r.Fix(f)
	want := "```sh\nls\n\npwd\n```\n"
	assert.Equal(t, want, string(got))
}

func TestFix_LeavesUnaffectedBlocksAlone(t *testing.T) {
	src := []byte("```sh\n$ ls\nfoo\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	got := r.Fix(f)
	assert.Equal(t, string(src), string(got))
}

func TestFix_MultipleBlocks_FixOnlyOffender(t *testing.T) {
	src := []byte("```sh\n$ ls\n```\n\ntext\n\n```sh\n$ ls\nfoo\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	got := r.Fix(f)
	want := "```sh\nls\n```\n\ntext\n\n```sh\n$ ls\nfoo\n```\n"
	assert.Equal(t, want, string(got))
}

func TestCheck_ID(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, "MDS066", r.ID())
}

func TestCheck_Name(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, "commands-show-output", r.Name())
}

func TestCheck_Category(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, "code", r.Category())
}

func TestCheck_SkipsGeneratedRange(t *testing.T) {
	src := []byte("```sh\n$ ls\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	f.GeneratedRanges = []lint.LineRange{{From: 1, To: 3}}
	r := &Rule{}
	diags := r.Check(f)
	assert.Empty(t, diags)
}

// --- Nested fenced blocks (inside list item) ---

// TestCheck_NestedFencedBlockInList_DetectedWithLeadingIndent verifies
// MDS066 sees `$ ls` as a prompt even when the fenced block is nested
// in a list item, where the content lines carry the list-item indent.
func TestCheck_NestedFencedBlockInList_DetectedWithLeadingIndent(t *testing.T) {
	src := []byte("- Item:\n\n  ```sh\n  $ ls\n  $ pwd\n  ```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, "commands shown with $ prefix but no output", diags[0].Message)
}

// TestFix_NestedFencedBlockInList_PreservesListIndent verifies that the
// fix preserves the list-item indent when stripping the prompt.
func TestFix_NestedFencedBlockInList_PreservesListIndent(t *testing.T) {
	src := []byte("- Item:\n\n  ```sh\n  $ ls\n  $ pwd\n  ```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	got := r.Fix(f)
	want := "- Item:\n\n  ```sh\n  ls\n  pwd\n  ```\n"
	assert.Equal(t, want, string(got))
}

// --- splitLeadingWhitespace helper ---

func TestSplitLeadingWhitespace(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		leading string
		rest    string
	}{
		{"empty", "", "", ""},
		{"no leading", "code", "", "code"},
		{"spaces", "  code", "  ", "code"},
		{"tab", "\tcode", "\t", "code"},
		{"mixed", " \t code", " \t ", "code"},
		{"all whitespace", "   ", "   ", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			leading, rest := splitLeadingWhitespace([]byte(tc.in))
			assert.Equal(t, tc.leading, string(leading))
			assert.Equal(t, tc.rest, string(rest))
		})
	}
}

// --- Defensive paths ---

func TestFix_NilFile_ReturnsSource(t *testing.T) {
	r := &Rule{}
	// f.Source is nil for a zero-valued File.
	assert.Nil(t, r.Fix(&lint.File{}))
}

func TestFix_NoOffendingBlocks_ReturnsSourceUnchanged(t *testing.T) {
	src := []byte("```sh\n$ ls\nfoo\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	got := r.Fix(f)
	assert.Equal(t, string(src), string(got))
}

func TestStripPrompt_NoPrompt_Unchanged(t *testing.T) {
	assert.Equal(t, "not a prompt", stripPrompt([]byte("not a prompt")))
}

func TestStripPrompt_Indented_PreservesLeadingWhitespace(t *testing.T) {
	assert.Equal(t, "  ls", stripPrompt([]byte("  $ ls")))
}

func TestStripPrompt_BlankLine_Unchanged(t *testing.T) {
	assert.Equal(t, "   ", stripPrompt([]byte("   ")))
}

// TestFix_SkipsGeneratedRange covers the inGeneratedRange guard in
// the Fix walker (Check's path is already covered by
// TestCheck_SkipsGeneratedRange).
func TestFix_SkipsGeneratedRange(t *testing.T) {
	src := []byte("```sh\n$ ls\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	f.GeneratedRanges = []lint.LineRange{{From: 1, To: 3}}
	r := &Rule{}
	got := r.Fix(f)
	assert.Equal(t, string(src), string(got))
}
