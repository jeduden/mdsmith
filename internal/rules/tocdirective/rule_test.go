package tocdirective

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestID(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, "MDS035", r.ID())
}

func TestName(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, "toc-directive", r.Name())
}

func TestCategory(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, "meta", r.Category())
}

func TestEnabledByDefault_OptIn(t *testing.T) {
	var r rule.Defaultable = &Rule{}
	assert.False(t, r.EnabledByDefault(),
		"MDS035 must be opt-in (disabled by default)")
}

func TestCheck_BracketedTOC(t *testing.T) {
	src := []byte("# Title\n\n[TOC]\n\nContent.\n")
	f, err := lint.NewFile("t.md", src)
	require.NoError(t, err)

	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, "MDS035", diags[0].RuleID)
	assert.Equal(t, lint.Warning, diags[0].Severity)
	assert.Equal(t, 3, diags[0].Line)
	assert.Contains(t, diags[0].Message, "`[TOC]`")
	assert.Contains(t, diags[0].Message, "<?toc?>")
	assert.Contains(t, diags[0].Message, "MDS038")
}

func TestCheck_GitLabTOC(t *testing.T) {
	src := []byte("# Title\n\n[[_TOC_]]\n")
	f, err := lint.NewFile("t.md", src)
	require.NoError(t, err)

	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "`[[_TOC_]]`")
	assert.Equal(t, 3, diags[0].Line)
}

func TestCheck_MarkdownItTOC(t *testing.T) {
	src := []byte("# Title\n\n[[toc]]\n")
	f, err := lint.NewFile("t.md", src)
	require.NoError(t, err)

	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "`[[toc]]`")
	assert.Equal(t, 3, diags[0].Line)
}

func TestCheck_VitepressDollarTOC(t *testing.T) {
	src := []byte("# Title\n\n${toc}\n")
	f, err := lint.NewFile("t.md", src)
	require.NoError(t, err)

	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "`${toc}`")
	assert.Equal(t, 3, diags[0].Line)
}

func TestCheck_TOCInsideFencedCodeBlock_NoDiagnostic(t *testing.T) {
	src := []byte("# Title\n\n```\n[TOC]\n[[_TOC_]]\n[[toc]]\n${toc}\n```\n")
	f, err := lint.NewFile("t.md", src)
	require.NoError(t, err)

	diags := (&Rule{}).Check(f)
	assert.Empty(t, diags)
}

func TestCheck_TOCInsideIndentedCodeBlock_NoDiagnostic(t *testing.T) {
	src := []byte("# Title\n\n    [TOC]\n")
	f, err := lint.NewFile("t.md", src)
	require.NoError(t, err)

	diags := (&Rule{}).Check(f)
	assert.Empty(t, diags)
}

func TestCheck_TOCInsideInlineCodeSpan_NoDiagnostic(t *testing.T) {
	src := []byte("# Title\n\nUse `[TOC]` for TOC.\n")
	f, err := lint.NewFile("t.md", src)
	require.NoError(t, err)

	diags := (&Rule{}).Check(f)
	assert.Empty(t, diags)
}

func TestCheck_TOCWithLinkRefDefinition_NoDiagnostic(t *testing.T) {
	// [TOC] resolves to https://example.com via the link reference
	// definition, so it is a legitimate link and must not be flagged.
	src := []byte("# Title\n\n[TOC]: https://example.com\n\n[TOC]\n")
	f, err := lint.NewFile("t.md", src)
	require.NoError(t, err)

	diags := (&Rule{}).Check(f)
	assert.Empty(t, diags)
}

func TestCheck_TOCWithLowercaseLinkRefDefinition_NoDiagnostic(t *testing.T) {
	// Label match is case-insensitive per CommonMark: `[TOC]` matches
	// `[toc]: ...`.
	src := []byte("# Title\n\n[toc]: https://example.com\n\n[TOC]\n")
	f, err := lint.NewFile("t.md", src)
	require.NoError(t, err)

	diags := (&Rule{}).Check(f)
	assert.Empty(t, diags)
}

func TestCheck_OtherVariantsWithTOCLinkRef_StillFlagged(t *testing.T) {
	// The link-ref suppression applies only to the `[TOC]` pattern; the
	// other three tokens cannot be valid CommonMark references.
	src := []byte("# Title\n\n[TOC]: https://example.com\n\n[[_TOC_]]\n\n[[toc]]\n\n${toc}\n")
	f, err := lint.NewFile("t.md", src)
	require.NoError(t, err)

	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 3)
	assert.Contains(t, diags[0].Message, "`[[_TOC_]]`")
	assert.Contains(t, diags[1].Message, "`[[toc]]`")
	assert.Contains(t, diags[2].Message, "`${toc}`")
}

func TestCheck_TOCWithTrailingWhitespace(t *testing.T) {
	src := []byte("# Title\n\n[TOC]   \n")
	f, err := lint.NewFile("t.md", src)
	require.NoError(t, err)

	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, 3, diags[0].Line)
}

func TestCheck_TOCNotOnOwnLine_NoDiagnostic(t *testing.T) {
	// `[TOC]` embedded in prose should not be flagged; the rule targets
	// the directive-on-its-own-line usage.
	src := []byte("# Title\n\nSee the [TOC] above for details.\n")
	f, err := lint.NewFile("t.md", src)
	require.NoError(t, err)

	diags := (&Rule{}).Check(f)
	assert.Empty(t, diags)
}

func TestCheck_NoTOCTokens_NoDiagnostic(t *testing.T) {
	src := []byte("# Title\n\nJust some paragraph text.\n")
	f, err := lint.NewFile("t.md", src)
	require.NoError(t, err)

	diags := (&Rule{}).Check(f)
	assert.Empty(t, diags)
}

func TestCheck_EmptyFile(t *testing.T) {
	f, err := lint.NewFile("t.md", nil)
	require.NoError(t, err)
	assert.Empty(t, (&Rule{}).Check(f))
}

func TestRule_IsFixable(t *testing.T) {
	var r any = &Rule{}
	_, ok := r.(rule.FixableRule)
	assert.True(t, ok, "MDS035 must implement FixableRule")
}

func TestFix_BracketedTOC(t *testing.T) {
	src := []byte("# Title\n\n[TOC]\n\nContent.\n")
	f, err := lint.NewFile("t.md", src)
	require.NoError(t, err)
	fixed := string((&Rule{}).Fix(f))
	assert.Contains(t, fixed, "<?toc?>")
	assert.Contains(t, fixed, "<?/toc?>")
	assert.NotContains(t, fixed, "[TOC]")
}

func TestFix_GitLabTOC(t *testing.T) {
	src := []byte("# Title\n\n[[_TOC_]]\n\nContent.\n")
	f, err := lint.NewFile("t.md", src)
	require.NoError(t, err)
	fixed := string((&Rule{}).Fix(f))
	assert.Contains(t, fixed, "<?toc?>")
	assert.NotContains(t, fixed, "[[_TOC_]]")
}

func TestFix_TOCLinkRef_Untouched(t *testing.T) {
	// [TOC] with a link reference definition must not be replaced.
	src := []byte("# Title\n\n[TOC]\n\n[TOC]: https://example.com\n")
	f, err := lint.NewFile("t.md", src)
	require.NoError(t, err)
	fixed := string((&Rule{}).Fix(f))
	assert.Contains(t, fixed, "[TOC]")
	assert.NotContains(t, fixed, "<?toc?>")
}

func TestFix_InsertsBlankLines(t *testing.T) {
	src := []byte("Paragraph.\n[TOC]\nMore text.\n")
	f, err := lint.NewFile("t.md", src)
	require.NoError(t, err)
	fixed := string((&Rule{}).Fix(f))
	assert.Contains(t, fixed, "\n\n<?toc?>\n<?/toc?>\n\n")
}

func TestCheck_NilFile(t *testing.T) {
	assert.Nil(t, (&Rule{}).Check(nil))
}

func TestFix_NilFile(t *testing.T) {
	assert.Nil(t, (&Rule{}).Fix(nil))
}

func TestFix_CodeBlocks_Untouched(t *testing.T) {
	// [TOC] inside fenced or indented code blocks must not be replaced.
	src := []byte("# Title\n\nFenced:\n\n```\n[TOC]\n[[_TOC_]]\n```\n\nIndented:\n\n    [TOC]\n    [[toc]]\n\nEnd.\n")
	f, err := lint.NewFile("t.md", src)
	require.NoError(t, err)
	fixed := string((&Rule{}).Fix(f))
	// Code block content should be unchanged.
	assert.Contains(t, fixed, "```\n[TOC]\n[[_TOC_]]\n```")
	assert.Contains(t, fixed, "    [TOC]\n    [[toc]]")
	// But not replaced with <?toc?>.
	assert.NotContains(t, fixed, "<?toc?>")
}
