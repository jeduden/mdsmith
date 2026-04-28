package noreferencestyle

import (
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newFile(t *testing.T, src string) *lint.File {
	t.Helper()
	f, err := lint.NewFile("test.md", []byte(src))
	require.NoError(t, err)
	return f
}

func TestRuleMetadata(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, "MDS043", r.ID())
	assert.Equal(t, "no-reference-style", r.Name())
	assert.Equal(t, "link", r.Category())
	assert.False(t, r.EnabledByDefault())
}

func TestCheck_InlineLink_NoDiagnostic(t *testing.T) {
	f := newFile(t, "See [example](https://example.com).\n")
	diags := (&Rule{}).Check(f)
	assert.Empty(t, diags)
}

func TestCheck_FullReferenceLink(t *testing.T) {
	src := "See [example][site].\n\n[site]: https://example.com\n"
	diags := (&Rule{}).Check(f(t, src))
	require.Len(t, diags, 1)
	assert.Equal(t, msgRefLink, diags[0].Message)
	assert.Equal(t, 1, diags[0].Line)
}

func TestCheck_CollapsedReference(t *testing.T) {
	src := "See [example][].\n\n[example]: https://example.com\n"
	diags := (&Rule{}).Check(f(t, src))
	require.Len(t, diags, 1)
	assert.Equal(t, msgRefLink, diags[0].Message)
}

func TestCheck_ShortcutReference(t *testing.T) {
	src := "See [example].\n\n[example]: https://example.com\n"
	diags := (&Rule{}).Check(f(t, src))
	require.Len(t, diags, 1)
	assert.Equal(t, msgRefLink, diags[0].Message)
}

func TestCheck_UnusedReferenceDefinition(t *testing.T) {
	src := "Plain text.\n\n[unused]: https://example.com\n"
	diags := (&Rule{}).Check(f(t, src))
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "unused reference definition")
	assert.Contains(t, diags[0].Message, "[unused]")
}

func TestCheck_UnusedDefSilencedWhenLinkPresent(t *testing.T) {
	// Link uses [used] — definition for [unused] is dead code, but
	// we leave it alone because the link diagnostic already covers
	// the reference-style issue.
	src := "[a][used] and stuff.\n\n[used]: https://example.com\n[unused]: https://example.com\n"
	diags := (&Rule{}).Check(f(t, src))
	require.Len(t, diags, 1)
	assert.Equal(t, msgRefLink, diags[0].Message)
}

func TestCheck_FootnoteRefDisabled(t *testing.T) {
	src := "Some text.[^1]\n\n[^1]: A note.\n"
	diags := (&Rule{}).Check(f(t, src))
	require.Len(t, diags, 1)
	assert.Equal(t, msgFootnote, diags[0].Message)
}

func TestCheck_FootnoteAllowed_NumericRejected(t *testing.T) {
	src := "Some text.[^1]\n\n[^1]: A note.\n"
	r := &Rule{AllowFootnotes: true}
	diags := r.Check(f(t, src))
	require.Len(t, diags, 1)
	assert.Equal(t, msgFootnoteNum, diags[0].Message)
}

func TestCheck_FootnoteAllowed_SlugDefinedAdjacent(t *testing.T) {
	src := "Some text.[^note]\n[^note]: A note.\n"
	r := &Rule{AllowFootnotes: true}
	diags := r.Check(f(t, src))
	assert.Empty(t, diags)
}

func TestCheck_FootnoteAllowed_SlugDefinitionFar(t *testing.T) {
	src := "Some text.[^note]\n\nMore prose.\n\n[^note]: A note.\n"
	r := &Rule{AllowFootnotes: true}
	diags := r.Check(f(t, src))
	require.Len(t, diags, 1)
	assert.Equal(t, msgFootnotePlace, diags[0].Message)
}

func TestCheck_FootnoteRefInsideCodeSpan(t *testing.T) {
	src := "Use the `[^1]` token.\n"
	r := &Rule{AllowFootnotes: false}
	diags := r.Check(f(t, src))
	assert.Empty(t, diags)
}

func TestCheck_FootnoteRefInsideCodeBlock(t *testing.T) {
	src := "Example:\n\n```text\n[^1]\n```\n"
	r := &Rule{AllowFootnotes: false}
	diags := r.Check(f(t, src))
	assert.Empty(t, diags)
}

func TestFix_RewriteFullReference(t *testing.T) {
	src := "See [example][site].\n\n[site]: https://example.com\n"
	got := (&Rule{}).Fix(f(t, src))
	want := "See [example](https://example.com).\n"
	assert.Equal(t, want, string(got))
}

func TestFix_RewriteCollapsedReference(t *testing.T) {
	src := "See [example][].\n\n[example]: https://example.com\n"
	got := (&Rule{}).Fix(f(t, src))
	want := "See [example](https://example.com).\n"
	assert.Equal(t, want, string(got))
}

func TestFix_RewriteShortcutReference(t *testing.T) {
	src := "See [example].\n\n[example]: https://example.com\n"
	got := (&Rule{}).Fix(f(t, src))
	want := "See [example](https://example.com).\n"
	assert.Equal(t, want, string(got))
}

func TestFix_PreservesTitle(t *testing.T) {
	src := "See [example][site].\n\n[site]: https://example.com \"Example\"\n"
	got := (&Rule{}).Fix(f(t, src))
	assert.True(t, strings.HasPrefix(string(got), "See [example](https://example.com \"Example\")"),
		"got=%q", string(got))
}

func TestApplySettings(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{"allow-footnotes": true}))
	assert.True(t, r.AllowFootnotes)

	require.NoError(t, r.ApplySettings(map[string]any{"allow-footnotes": false}))
	assert.False(t, r.AllowFootnotes)

	err := r.ApplySettings(map[string]any{"allow-footnotes": "yes"})
	assert.Error(t, err)

	err = r.ApplySettings(map[string]any{"unknown": true})
	assert.Error(t, err)
}

// f is a shorter alias for newFile for the assertion-heavy tests.
func f(t *testing.T, src string) *lint.File {
	return newFile(t, src)
}
