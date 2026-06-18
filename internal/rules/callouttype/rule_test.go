package callouttype

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/lint"
)

func newFile(t *testing.T, src string) *lint.File {
	t.Helper()
	f, err := lint.NewFile("test.md", []byte(src))
	require.NoError(t, err)
	return f
}

func TestRuleMetadata(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, "MDS067", r.ID())
	assert.Equal(t, "callout-type", r.Name())
	assert.Equal(t, "structural", r.Category())
	assert.False(t, r.EnabledByDefault())
}

func TestCheck_BaseTypeAllowed(t *testing.T) {
	r := &Rule{}
	for _, ty := range []string{
		"note", "abstract", "summary", "tldr",
		"info", "todo",
		"tip", "hint", "important",
		"success", "check", "done",
		"question", "help", "faq",
		"warning", "caution", "attention",
		"failure", "fail", "missing",
		"danger", "error",
		"bug", "example",
		"quote", "cite",
	} {
		src := "> [!" + ty + "]\n> body\n"
		f := newFile(t, src)
		diags := r.Check(f)
		assert.Emptyf(t, diags, "type %q should be allowed", ty)
	}
}

func TestCheck_CaseInsensitive(t *testing.T) {
	f := newFile(t, "> [!NOTE]\n> body\n")
	diags := (&Rule{}).Check(f)
	assert.Empty(t, diags)
}

func TestCheck_UnknownType(t *testing.T) {
	f := newFile(t, "> [!REVIEW]\n> body\n")
	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, "MDS067", diags[0].RuleID)
	assert.Contains(t, diags[0].Message, "REVIEW")
	assert.Contains(t, diags[0].Message, "note")
	assert.Contains(t, diags[0].Message, "allow-unknown")
	// The wording must call out base-type vs alias so a user
	// reading the message does not assume "summary" or "tldr" are
	// rejected just because they aren't in the printed list.
	assert.Contains(t, diags[0].Message, "valid base types")
	assert.Contains(t, diags[0].Message, "aliases")
}

func TestCheck_AllowList(t *testing.T) {
	f := newFile(t, "> [!custom]\n> body\n")
	r := &Rule{Allow: []string{"custom"}}
	diags := r.Check(f)
	assert.Empty(t, diags)
}

func TestCheck_AllowUnknownDisablesValidation(t *testing.T) {
	f := newFile(t, "> [!anything]\n> body\n")
	r := &Rule{AllowUnknown: true}
	diags := r.Check(f)
	assert.Empty(t, diags)
}

func TestCheck_PlainBlockquoteIgnored(t *testing.T) {
	f := newFile(t, "> just a quote\n> no callout marker here\n")
	diags := (&Rule{}).Check(f)
	assert.Empty(t, diags)
}

func TestCheck_ReportsLineAndColumn(t *testing.T) {
	f := newFile(t, "# Heading\n\n> [!REVIEW]\n> body\n")
	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, 3, diags[0].Line)
	// The "[" sits after "> " on the line, so col 3.
	assert.Equal(t, 3, diags[0].Column)
}

func TestCheck_NestedBlockquote(t *testing.T) {
	// A blockquote whose first paragraph does not start with `[!type]`
	// is not a callout — even if a deeper child contains the token.
	f := newFile(t, "> not a callout\n>\n> > [!REVIEW]\n> > body\n")
	diags := (&Rule{}).Check(f)
	// The inner blockquote is itself walked; if its first paragraph
	// holds the unknown token it still triggers.
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "REVIEW")
}

func TestApplySettings_AllowList(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{"allow": []any{"custom", "other"}}))
	assert.Equal(t, []string{"custom", "other"}, r.Allow)
}

func TestApplySettings_AllowUnknown(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{"allow-unknown": true}))
	assert.True(t, r.AllowUnknown)
}

func TestApplySettings_BadTypes(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"allow": "string-not-list"})
	require.Error(t, err)

	err = r.ApplySettings(map[string]any{"allow-unknown": "yes"})
	require.Error(t, err)

	err = r.ApplySettings(map[string]any{"unknown-key": true})
	require.Error(t, err)
}

func TestDefaultSettings(t *testing.T) {
	ds := (&Rule{}).DefaultSettings()
	assert.Equal(t, []string{}, ds["allow"])
	assert.Equal(t, false, ds["allow-unknown"])
}

func TestSettingMergeMode(t *testing.T) {
	r := &Rule{}
	assert.NotEqual(t, r.SettingMergeMode("allow"), r.SettingMergeMode("allow-unknown"))
}

func TestCheck_NilFileAndNilASTReturnNil(t *testing.T) {
	// lint.File explicitly supports the struct-literal construction
	// path where AST is never populated. Rule.Check walks f.AST, so
	// it must short-circuit instead of panicking on a nil tree —
	// and gracefully accept a nil *lint.File the same way.
	r := &Rule{}
	assert.NotPanics(t, func() { assert.Nil(t, r.Check(nil)) })
	assert.NotPanics(t, func() { assert.Nil(t, r.Check(&lint.File{})) })
}

func TestCheck_BlockquoteWithoutParagraph(t *testing.T) {
	// A blockquote whose first child is a list, code block, or another
	// blockquote (not a paragraph) cannot carry a callout marker. The
	// rule must skip it without panicking on the type assertion.
	f := newFile(t, "> - item 1\n> - item 2\n")
	diags := (&Rule{}).Check(f)
	assert.Empty(t, diags)
}

func TestUnknownTypeDiag_AllowListExtras(t *testing.T) {
	// When the rule has user-configured types in Allow, the diagnostic
	// message lists them after the built-in vocabulary so the user
	// sees both sets when they triage a violation.
	f := newFile(t, "> [!REVIEW]\n> body\n")
	r := &Rule{Allow: []string{"decision", "custom"}}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "(plus custom, decision)")
}

func TestCheck_AllowSetCachedPerFileAfterApplySettings(t *testing.T) {
	// After ApplySettings registers a custom type, Check must accept it.
	// Calling Check twice on the same file exercises the per-file memo cache
	// (f.Memo) that buildAllowSet stores its result in.
	f := newFile(t, "> [!custom]\n> body\n")
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{"allow": []any{"custom"}}))
	assert.Empty(t, r.Check(f))
	assert.Empty(t, r.Check(f))
}

// TestCalloutTokenFromLine_OOBLine covers the idx-out-of-range guard in
// calloutTokenFromLine: lineNum 0 (idx == -1) and lineNum > lines must
// return ok==false without panicking.
func TestCalloutTokenFromLine_OOBLine(t *testing.T) {
	f := lint.NewFileLines("f.md", []byte("> [!NOTE]\n> body\n"))
	_, _, _, ok := calloutTokenFromLine(f, 0, 1)
	assert.False(t, ok, "lineNum 0 (idx -1) must return false")
	_, _, _, ok = calloutTokenFromLine(f, 99, 1)
	assert.False(t, ok, "lineNum past EOF must return false")
}

// TestCalloutTokenFromLine_DepthExceedsMarkers covers the early-return in
// calloutTokenFromLine when depth exceeds the actual number of blockquote
// markers on the line (quoteMarkerLen returns 0 before depth is reached).
func TestCalloutTokenFromLine_DepthExceedsMarkers(t *testing.T) {
	// Line has only one level of `>` but depth=2 is requested.
	f := lint.NewFileLines("f.md", []byte("> [!NOTE]\n> body\n"))
	_, _, _, ok := calloutTokenFromLine(f, 1, 2)
	assert.False(t, ok, "depth greater than actual marker count must return false")
}

// TestQuoteMarkerLen_NoMarker covers quoteMarkerLen returning 0 when the
// line has no '>' (e.g. all spaces, or non-quote content).
func TestQuoteMarkerLen_NoMarker(t *testing.T) {
	assert.Equal(t, 0, quoteMarkerLen([]byte("   ")), "spaces-only line has no marker")
	assert.Equal(t, 0, quoteMarkerLen([]byte("text")), "plain text has no marker")
	assert.Equal(t, 2, quoteMarkerLen([]byte("> ")), "single marker with space")
	assert.Equal(t, 1, quoteMarkerLen([]byte(">")), "marker at EOL, no space")
}

// TestCheck_NilASTMatchesAST pins the nil-AST path: Check on a parse-
// skipped File (f.AST nil) must produce byte-identical diagnostics to the
// AST path for callout blockquotes, including the column of the `[!`
// token and quotes that are not callouts.
func TestCheck_NilASTMatchesAST(t *testing.T) {
	srcs := []string{
		"# Bad Callout\n\n> [!REVIEW]\n> Unknown type.\n",
		"> [!note]\n> A valid callout.\n",
		"> [!REVIEW] with trailing text\n> body\n",
		">[!REVIEW]\n> no space after marker\n",
		"> just a normal quote\n> second line\n",
		"> [!info]\n\n> [!BOGUS]\n",
		"- a list\n- of items\n\n> [!WAT]\n> body\n",
		"text\n\n> [!UNKNOWN]\n",
		"> > [!REVIEW]\n> > doubly nested\n",
		"> outer\n> > [!BOGUS]\n> > body\n",
		// Lazy continuation: non-blank line without `>` must not reset
		// depth — the next `>` line is a continuation paragraph, not a
		// new callout opener.
		"> [!note]\nlazy continuation line\n> following para\n",
		// Depth decrease: after a depth-2 line, a depth-1 line ratchets
		// prevDepth down without emitting a spurious diagnostic.
		"> > [!REVIEW]\n> outer continues\n",
		// Code fence at the top level: `>` lines inside the fence must
		// not produce diagnostics (exercises the inCode branch).
		"```\n> [!REVIEW]\n```\n",
	}
	for _, src := range srcs {
		b := []byte(src)
		astFile, err := lint.NewFile("f.md", b)
		require.NoError(t, err)
		astDiags := (&Rule{}).Check(astFile)
		l0Diags := (&Rule{}).Check(lint.NewFileLines("f.md", b))
		assert.Equal(t, astDiags, l0Diags,
			"nil-AST diagnostics must match AST for %q", src)
	}
}

// TestBuildAllowSet_SetType verifies that buildAllowSet returns
// map[string]struct{} rather than map[string]bool. Per the high-performance
// Go guidelines: "map[K]struct{} for sets — zero-byte value type."
func TestBuildAllowSet_SetType(t *testing.T) {
	r := &Rule{}
	got := reflect.TypeOf(r.buildAllowSet()).String()
	want := reflect.TypeOf(map[string]struct{}{}).String()
	if got != want {
		t.Fatalf("buildAllowSet returns %s; want %s (guideline: use map[K]struct{} for sets)", got, want)
	}
}
