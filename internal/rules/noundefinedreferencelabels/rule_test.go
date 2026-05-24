package noundefinedreferencelabels

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"
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

// TestCollectCodeSpanRangesInto_NilNode pins the nil-node guard on
// the recursive helper. Production callers never feed nil, but the
// guard exists so a struct-literal *File with no AST stays safe.
func TestCollectCodeSpanRangesInto_NilNode(t *testing.T) {
	var out []byteRange
	collectCodeSpanRangesInto(nil, nil, &out)
	assert.Empty(t, out)
}

// TestCodeSpanTextBounds_NonTextChild pins the inline-code-span
// case where the child is not an *ast.Text (e.g., an emphasis or
// a hard-break node nested inside the span). The helper skips
// non-Text children and continues; the loop body's `continue`
// branch is otherwise unreachable from the rule's own AST.
func TestCodeSpanTextBounds_NonTextChild(t *testing.T) {
	f := newFile(t, "`code`\n")
	var span *ast.CodeSpan
	collectCodeSpansForTest(f.AST, &span)
	require.NotNil(t, span, "fixture must produce a code span")
	// Manually append a non-Text child so codeSpanTextBounds hits
	// its `continue` branch; goldmark's own parsed code spans
	// contain only *ast.Text children, which is why this branch
	// stays cold without a synthetic child.
	span.AppendChild(span, ast.NewEmphasis(1))
	first, last := codeSpanTextBounds(span)
	assert.GreaterOrEqual(t, first, 0)
	assert.GreaterOrEqual(t, last, first)
}

// collectCodeSpansForTest is a tiny test-only walker that returns
// the first *ast.CodeSpan it encounters.
func collectCodeSpansForTest(n ast.Node, out **ast.CodeSpan) {
	if *out != nil {
		return
	}
	if cs, ok := n.(*ast.CodeSpan); ok {
		*out = cs
		return
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		collectCodeSpansForTest(c, out)
		if *out != nil {
			return
		}
	}
}

// TestNextBracket_OrphanOpenBracket pins the "[ opened without
// matching ]" advance-and-keep-scanning branch. A line like
// `[unmatched ... [closed]` must skip the orphan `[` and still
// find the later `[closed]`.
func TestNextBracket_OrphanOpenBracket(t *testing.T) {
	src := []byte("[unmatched\n[closed]")
	// First call: scanner sees the orphan `[`, advances past it.
	// Eventually it returns the `[closed]` match.
	open, cs, ce, ca, ok := nextBracket(src, 0)
	require.True(t, ok)
	assert.Equal(t, "closed", string(src[cs:ce]))
	assert.Equal(t, byte('['), src[open])
	assert.Equal(t, ce+1, ca)
}

// TestScanFullRefs_FirstBracketNoAdjacentSecond pins the
// branch that skips `[text]` not immediately followed by another
// `[label]`. The previous regex form filtered these via the
// pattern itself; the byte scanner has to make the same decision
// explicitly.
func TestScanFullRefs_FirstBracketNoAdjacentSecond(t *testing.T) {
	f := newFile(t, "[label] not a full ref\n\n[label]: https://example.com/\n")
	r := &Rule{Shortcut: shortcutCollapsedOnly}
	diags := r.Check(f)
	assert.Empty(t, diags, "no full-ref pattern: must not flag the bare [label]")
}

// TestScanCollapsedRefs_EmptyLabel pins the byte scanner's
// "empty `[]` label" skip in the collapsed-ref scan. `[][]` would
// otherwise look like collapsed-ref territory but has no label
// to look up.
func TestScanCollapsedRefs_EmptyLabel(t *testing.T) {
	f := newFile(t, "[][] is empty\n")
	r := &Rule{}
	diags := r.Check(f)
	assert.Empty(t, diags)
}

// TestRefDefLineStarts_NoBracket pins the byte scanner's
// "line does not open with `[`" early return.
func TestRefDefLineStarts_NoBracket(t *testing.T) {
	src := []byte("just text, no bracket\n")
	assert.False(t, refDefLineStarts(src, 0, len(src)-1))
}

// TestRefDefLineStarts_NoClose pins the "`[` without matching
// `]`" early return.
func TestRefDefLineStarts_NoClose(t *testing.T) {
	src := []byte("[unclosed\n")
	assert.False(t, refDefLineStarts(src, 0, len(src)-1))
}

// TestRefDefLineStarts_NoColon pins the "`]` without trailing
// `:`" early return — exercises both the trailing-whitespace skip
// path and the missing-colon return.
func TestRefDefLineStarts_NoColon(t *testing.T) {
	src := []byte("[label]  no colon")
	assert.False(t, refDefLineStarts(src, 0, len(src)))
}

// TestScanFullRefs_SecondBracketUnclosed pins the `[text][...`
// case where the candidate full-ref opens its second bracket but
// the bracket never closes. The byte scanner advances past the
// first `[…]` (the next bracket call returns ok=false because no
// `]` is found on the same line) and resumes scanning.
func TestScanFullRefs_SecondBracketUnclosed(t *testing.T) {
	f := newFile(t, "# T\n\nSee [text][unclosed and end\n")
	r := &Rule{}
	diags := r.Check(f)
	assert.Empty(t, diags, "unclosed second bracket: must not flag a full-ref")
}

// TestCheck_NoBracketEarlyExit pins the bytes.ContainsRune
// fast path added by plan 195 task 7: a source with no `[`
// returns nil before allocating the defs slice or walking
// the AST. Removing the early-exit silently re-pays the
// alloc-budget delta this rule was grandfathered against.
func TestCheck_NoBracketEarlyExit(t *testing.T) {
	src := "# Title\n\nProse with no brackets at all.\n"
	require.Empty(t, check(t, src))
}
