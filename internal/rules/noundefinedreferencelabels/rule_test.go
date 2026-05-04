package noundefinedreferencelabels

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newFile(t *testing.T, src string) *lint.File {
	t.Helper()
	f, err := lint.NewFile("test.md", []byte(src))
	require.NoError(t, err)
	return f
}

func check(t *testing.T, src string) []lint.Diagnostic {
	t.Helper()
	return (&Rule{}).Check(newFile(t, src))
}

func checkWith(t *testing.T, src string, r *Rule) []lint.Diagnostic {
	t.Helper()
	return r.Check(newFile(t, src))
}

func TestRuleMetadata(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, "MDS054", r.ID())
	assert.Equal(t, "no-undefined-reference-labels", r.Name())
	assert.Equal(t, "link", r.Category())
	assert.True(t, r.EnabledByDefault())
}

// --- Full reference [text][label] ---

func TestFullRef_DefinedLabel_NoDiag(t *testing.T) {
	src := "See [example][site].\n\n[site]: https://example.com\n"
	assert.Empty(t, check(t, src))
}

func TestFullRef_UndefinedLabel_OneDiag(t *testing.T) {
	src := "See [example][broken].\n"
	diags := check(t, src)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, `"broken"`)
	assert.Equal(t, 1, diags[0].Line)
}

func TestFullRef_UndefinedLabel_Position(t *testing.T) {
	src := "# Heading\n\nSee [example][broken].\n"
	diags := check(t, src)
	require.Len(t, diags, 1)
	assert.Equal(t, 3, diags[0].Line)
	assert.Equal(t, 5, diags[0].Column) // '[' is column 5 on "See [example][broken]."
}

// --- Collapsed reference [label][] ---

func TestCollapsedRef_Defined_NoDiag(t *testing.T) {
	src := "See [site][].\n\n[site]: https://example.com\n"
	assert.Empty(t, check(t, src))
}

func TestCollapsedRef_Undefined_OneDiag(t *testing.T) {
	src := "See [broken][].\n"
	diags := check(t, src)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, `"broken"`)
}

// --- Shortcut reference [label] heuristic ---

func TestShortcutRef_Defined_NoDiag(t *testing.T) {
	src := "See [site].\n\n[site]: https://example.com\n"
	assert.Empty(t, check(t, src))
}

func TestShortcutRef_UndefinedLooksLikeRef_Flagged(t *testing.T) {
	// "plan128" has digits → looks like a reference target
	src := "See [plan128].\n"
	diags := check(t, src)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, `"plan128"`)
}

func TestShortcutRef_UndefinedProse_NotFlagged(t *testing.T) {
	// "just brackets" has spaces → heuristic skips it
	src := "Some [just brackets] in prose.\n"
	assert.Empty(t, check(t, src))
}

func TestShortcutRef_Always_FlagsProse(t *testing.T) {
	src := "Some [just brackets] in prose.\n"
	r := &Rule{Shortcut: shortcutAlways}
	diags := checkWith(t, src, r)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, `"just brackets"`)
}

func TestShortcutRef_CollapsedOnly_SkipsShortcut(t *testing.T) {
	src := "See [plan128].\n"
	r := &Rule{Shortcut: shortcutCollapsedOnly}
	assert.Empty(t, checkWith(t, src, r))
}

// --- CommonMark case-fold normalization ---

func TestFullRef_CaseFolded_NoDiag(t *testing.T) {
	// [Foo Bar][BAR] should match [bar]: url
	src := "See [Foo Bar][BAR].\n\n[bar]: https://example.com\n"
	assert.Empty(t, check(t, src))
}

func TestCollapsedRef_CaseFolded_NoDiag(t *testing.T) {
	src := "See [SITE][].\n\n[site]: https://example.com\n"
	assert.Empty(t, check(t, src))
}

// --- Placeholders ---

func TestPlaceholder_Label_NotFlagged(t *testing.T) {
	// {title} is a var-token placeholder; the label should be treated as opaque.
	src := "See [text][{title}].\n"
	r := &Rule{Placeholders: []string{"var-token"}}
	assert.Empty(t, checkWith(t, src, r))
}

// --- Code exclusions ---

func TestInCodeSpan_NotFlagged(t *testing.T) {
	src := "Use `` [broken][ref] `` for syntax.\n"
	assert.Empty(t, check(t, src))
}

func TestInFencedCode_NotFlagged(t *testing.T) {
	src := "```\n[broken][ref]\n```\n"
	assert.Empty(t, check(t, src))
}

func TestInIndentedCode_NotFlagged(t *testing.T) {
	src := "\n    [broken][ref]\n"
	assert.Empty(t, check(t, src))
}

// --- Reference definition line not flagged ---

func TestRefDefLine_NotFlagged(t *testing.T) {
	// [site]: ... should not be treated as a shortcut reference
	src := "[site]: https://example.com\n"
	assert.Empty(t, check(t, src))
}

// --- Images ---

func TestFullRefImage_Undefined_Flagged(t *testing.T) {
	src := "![alt][broken]\n"
	diags := check(t, src)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, `"broken"`)
}

func TestCollapsedRefImage_Undefined_Flagged(t *testing.T) {
	src := "![logo][]\n"
	diags := check(t, src)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, `"logo"`)
	assert.Equal(t, 1, diags[0].Column) // column points to '!'
}

func TestCollapsedRefImage_Defined_NoDiag(t *testing.T) {
	src := "![logo][]\n\n[logo]: https://example.com/img.png\n"
	assert.Empty(t, check(t, src))
}

func TestShortcutRefImage_Undefined_Flagged(t *testing.T) {
	// Image shortcuts bypass the heuristic since '!' makes intent unambiguous.
	src := "See ![logo] inline.\n"
	diags := check(t, src)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, `"logo"`)
	assert.Equal(t, 5, diags[0].Column) // column points to '!'
}

func TestShortcutRefImage_Defined_NoDiag(t *testing.T) {
	src := "See ![logo] inline.\n\n[logo]: https://example.com/img.png\n"
	assert.Empty(t, check(t, src))
}

func TestFullRefImage_Defined_NoDiag(t *testing.T) {
	src := "![alt][img]\n\n[img]: https://example.com/a.png\n"
	assert.Empty(t, check(t, src))
}

// --- Multiple diagnostics ---

func TestMultipleUndefined_MultiDiags(t *testing.T) {
	src := "See [a][x] and [b][y].\n"
	diags := check(t, src)
	assert.Len(t, diags, 2)
}

// --- ApplySettings ---

func TestApplySettings_ValidShortcut(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{"shortcut": "always"}))
	assert.Equal(t, "always", r.Shortcut)
}

func TestApplySettings_InvalidShortcut(t *testing.T) {
	r := &Rule{}
	assert.Error(t, r.ApplySettings(map[string]any{"shortcut": "bad"}))
}

func TestApplySettings_UnknownKey(t *testing.T) {
	r := &Rule{}
	assert.Error(t, r.ApplySettings(map[string]any{"unknown": true}))
}

func TestApplySettings_Placeholders(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{"placeholders": []any{"var-token"}}))
	assert.Equal(t, []string{"var-token"}, r.Placeholders)
}

func TestDefaultSettings(t *testing.T) {
	r := &Rule{}
	d := r.DefaultSettings()
	assert.Equal(t, shortcutHeuristic, d["shortcut"])
	assert.Equal(t, []string{}, d["placeholders"])
}

func TestSettingMergeMode(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, rule.MergeAppend, r.SettingMergeMode("placeholders"))
	assert.Equal(t, rule.MergeReplace, r.SettingMergeMode("shortcut"))
}

// --- Additional coverage tests ---

func TestApplySettings_ShortcutWrongType(t *testing.T) {
	r := &Rule{}
	assert.Error(t, r.ApplySettings(map[string]any{"shortcut": 42}))
}

func TestApplySettings_PlaceholdersWrongType(t *testing.T) {
	r := &Rule{}
	assert.Error(t, r.ApplySettings(map[string]any{"placeholders": 42}))
}

func TestApplySettings_PlaceholdersInvalidToken(t *testing.T) {
	r := &Rule{}
	assert.Error(t, r.ApplySettings(map[string]any{"placeholders": []any{"not-a-token"}}))
}

func TestApplySettings_PlaceholdersStringSlice(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{"placeholders": []string{"var-token"}}))
	assert.Equal(t, []string{"var-token"}, r.Placeholders)
}

func TestApplySettings_PlaceholdersNonStringItem(t *testing.T) {
	r := &Rule{}
	assert.Error(t, r.ApplySettings(map[string]any{"placeholders": []any{42}}))
}

func TestFullRef_FootnoteLike_NotFlagged(t *testing.T) {
	// [^note][label] looks like a full ref but the text starts with '^' — skip.
	src := "A [^note][ref] here.\n"
	assert.Empty(t, check(t, src))
}

func TestCollapsedRef_InCodeBlock_NotFlagged(t *testing.T) {
	src := "```\n[broken][]\n```\n"
	assert.Empty(t, check(t, src))
}

func TestCollapsedRef_FootnoteLike_NotFlagged(t *testing.T) {
	// [^note][] — text starts with '^', treated as footnote-like, not a collapsed ref.
	src := "A [^note][] here.\n"
	assert.Empty(t, check(t, src))
}

func TestCollapsedRef_Placeholder_NotFlagged(t *testing.T) {
	src := "See [{title}][].\n"
	r := &Rule{Placeholders: []string{"var-token"}}
	assert.Empty(t, checkWith(t, src, r))
}

func TestShortcutRef_LooksLikeRef_StartsWithDigit_NotFlagged(t *testing.T) {
	// Label starts with digit (e.g. [0-9]) — heuristic skips it.
	src := "Pattern [0-9] here.\n"
	assert.Empty(t, check(t, src))
}

func TestPIBlock_ContentNotFlagged(t *testing.T) {
	// Reference patterns inside a PI block should not produce diagnostics.
	src := "# Title\n\n<?note\n[broken][ref]\n[broken][]\n?>\n\nText after.\n"
	assert.Empty(t, check(t, src))
}

func TestCodeSpan_NoBacktickExtension_NotFlagged(t *testing.T) {
	// Code span content immediately adjacent to backtick (no padding spaces).
	src := "Use `[broken][ref]` here.\n"
	assert.Empty(t, check(t, src))
}

// --- Backslash escape ---

func TestFullRef_EscapedBracket_NotFlagged(t *testing.T) {
	// \[text][label] — the '[' is a CommonMark backslash escape, not a link.
	src := `\[text][label]` + "\n"
	assert.Empty(t, check(t, src))
}

func TestCollapsedRef_EscapedBracket_NotFlagged(t *testing.T) {
	src := `\[broken][]` + "\n"
	assert.Empty(t, check(t, src))
}

func TestShortcutRef_EscapedBracket_NotFlagged(t *testing.T) {
	src := `\[plan128]` + "\n"
	assert.Empty(t, check(t, src))
}

func TestFullRef_DoubleBackslashBracket_Flagged(t *testing.T) {
	// \\[text][label] — the first '\' escapes the second, so '[' is NOT escaped.
	src := `\\[text][broken]` + "\n"
	diags := check(t, src)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, `"broken"`)
}
