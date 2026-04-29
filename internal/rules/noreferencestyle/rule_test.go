package noreferencestyle

import (
	"strings"
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

func TestDefaultSettings(t *testing.T) {
	r := &Rule{}
	got := r.DefaultSettings()
	assert.Equal(t, map[string]any{"allow-footnotes": false}, got)
}

func TestFix_NoReferenceLinksReturnsCopy(t *testing.T) {
	src := "Just [inline](https://example.com) text.\n"
	got := (&Rule{}).Fix(f(t, src))
	assert.Equal(t, src, string(got))
}

func TestFix_PreservesUnusedDefinition(t *testing.T) {
	// [unused] has no link; only [used] is rewritten and its
	// definition removed. The unused def is left in place.
	src := "See [t][used].\n\n[used]: https://example.com\n[unused]: https://example.org\n"
	got := (&Rule{}).Fix(f(t, src))
	assert.Contains(t, string(got), "[unused]: https://example.org")
	assert.Contains(t, string(got), "[t](https://example.com)")
	assert.NotContains(t, string(got), "[used]: https://example.com")
}

func TestCheck_MultipleReferenceLinks(t *testing.T) {
	src := "See [a][s] and [b][s].\n\n[s]: https://example.com\n"
	diags := (&Rule{}).Check(f(t, src))
	require.Len(t, diags, 2)
	for _, d := range diags {
		assert.Equal(t, msgRefLink, d.Message)
	}
}

func TestCheck_MultipleUnusedDefinitions(t *testing.T) {
	src := "Plain prose.\n\n[a]: https://x.example\n[b]: https://y.example\n"
	diags := (&Rule{}).Check(f(t, src))
	require.Len(t, diags, 2)
	for _, d := range diags {
		assert.Contains(t, d.Message, "unused reference definition")
	}
}

func TestCheck_FootnoteDefinitionWithoutReference(t *testing.T) {
	// A bare definition with no `[^slug]` reference produces no
	// diagnostic — we only flag references.
	src := "Plain prose.\n\n[^orphan]: A floating note.\n"
	r := &Rule{AllowFootnotes: false}
	diags := r.Check(f(t, src))
	assert.Empty(t, diags)
}

func TestCheck_FootnoteAllowed_NoMatchingDefinition(t *testing.T) {
	// `allow-footnotes: true` but there is no def at all — the
	// "missing" diagnostic fires (distinct from "misplaced").
	src := "Some text.[^missing]\n"
	r := &Rule{AllowFootnotes: true}
	diags := r.Check(f(t, src))
	require.Len(t, diags, 1)
	assert.Equal(t, msgFootnoteMissing, diags[0].Message)
}

func TestCheck_AdjacentFootnoteRefs(t *testing.T) {
	// `[^a][^b]` — both refs must be detected; neither swallows the
	// `[` of the next.
	src := "See [^a][^b]\n"
	r := &Rule{AllowFootnotes: false}
	diags := r.Check(f(t, src))
	require.Len(t, diags, 2)
	for _, d := range diags {
		assert.Equal(t, msgFootnote, d.Message)
	}
}

func TestPathColumns(t *testing.T) {
	// Column numbers run on the source byte offset of `[`, not the
	// inner text node.
	src := "abc [example][site] xyz.\n\n[site]: https://example.com\n"
	diags := (&Rule{}).Check(f(t, src))
	require.Len(t, diags, 1)
	assert.Equal(t, 1, diags[0].Line)
	assert.Equal(t, 5, diags[0].Column)
}

func TestIsNumericSlug(t *testing.T) {
	assert.True(t, isNumericSlug("1"))
	assert.True(t, isNumericSlug("123"))
	assert.False(t, isNumericSlug(""))
	assert.False(t, isNumericSlug("a1"))
	assert.False(t, isNumericSlug("note"))
}

func TestApplySettings_NilOK(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(nil))
	assert.False(t, r.AllowFootnotes)
}

func TestCheck_EmptySource(t *testing.T) {
	diags := (&Rule{}).Check(f(t, ""))
	assert.Empty(t, diags)
}

func TestCheck_LinkWithEmphasizedText(t *testing.T) {
	// Link text is `*foo*` — the first text descendant lives inside
	// an Emphasis node, so nodePosition must walk into the link.
	src := "See [*foo*][s].\n\n[s]: https://example.com\n"
	diags := (&Rule{}).Check(f(t, src))
	require.Len(t, diags, 1)
	assert.Equal(t, msgRefLink, diags[0].Message)
	assert.Equal(t, 1, diags[0].Line)
	assert.Equal(t, 5, diags[0].Column)
}

func TestFix_MultipleLinksAndDefinitions(t *testing.T) {
	src := "[a][x] then [b][y].\n\n[x]: https://x.example\n[y]: https://y.example\n"
	got := (&Rule{}).Fix(f(t, src))
	assert.Contains(t, string(got), "[a](https://x.example)")
	assert.Contains(t, string(got), "[b](https://y.example)")
	assert.NotContains(t, string(got), "[x]:")
	assert.NotContains(t, string(got), "[y]:")
}

func TestFix_DefinitionWithTitle(t *testing.T) {
	src := "See [t][s].\n\n[s]: https://example.com \"My title\"\n"
	got := (&Rule{}).Fix(f(t, src))
	assert.Contains(t, string(got), `[t](https://example.com "My title")`)
}

func TestCheck_FootnoteRefInMultiLineParagraph(t *testing.T) {
	// The reference is on the first line of a multi-line paragraph;
	// the definition follows the paragraph. paragraphEndLine has to
	// scan past the second line.
	src := "Some prose with[^note]\nmore prose continues.\n[^note]: A note.\n"
	r := &Rule{AllowFootnotes: true}
	diags := r.Check(f(t, src))
	assert.Empty(t, diags)
}

func TestCheck_FootnoteWithUnrelatedDefinition(t *testing.T) {
	// Two definitions exist; one matches the slug, one does not.
	// The placement check iterates past the unrelated def.
	src := "Use [^note]\n\n[^other]: Other note.\n[^note]: A note.\n"
	r := &Rule{AllowFootnotes: true}
	diags := r.Check(f(t, src))
	require.Len(t, diags, 1)
	assert.Equal(t, msgFootnotePlace, diags[0].Message)
}

func TestCheck_FootnoteAcrossBlankLineFails(t *testing.T) {
	// allow-footnotes: true and a definition exists, but it sits two
	// blank lines after — placement message fires (not "missing").
	src := "Some text.[^note]\n\nMore prose.\n\n[^note]: A note.\n"
	r := &Rule{AllowFootnotes: true}
	diags := r.Check(f(t, src))
	require.Len(t, diags, 1)
	assert.Equal(t, msgFootnotePlace, diags[0].Message)
}

func TestCheck_FootnoteDefinitionInsideCodeBlock(t *testing.T) {
	// `[^1]: ...` inside a fenced block must not register as a real
	// definition. The reference outside the block still fires.
	src := "Use it.[^1]\n\n```text\n[^1]: not a real def\n```\n"
	r := &Rule{AllowFootnotes: false}
	diags := r.Check(f(t, src))
	require.Len(t, diags, 1)
	assert.Equal(t, msgFootnote, diags[0].Message)
	assert.Equal(t, 1, diags[0].Line)
}

func TestFix_EmphasizedLinkText(t *testing.T) {
	// Link text contains *foo* — the first text descendant is at the
	// `f` byte, several bytes inside the `[`. linkSourceSpan must
	// walk back past the `*` to reach the opening `[`.
	src := "See [*foo*][s].\n\n[s]: https://example.com\n"
	got := (&Rule{}).Fix(f(t, src))
	assert.Contains(t, string(got), "[*foo*](https://example.com)")
	assert.NotContains(t, string(got), "[s]:")
}

func TestFix_NestedBracketsInLinkText(t *testing.T) {
	// Goldmark accepts a full reference link whose text contains a
	// bare `[b]` pair: `[a [b] c][s]`. findClosingBracket must use
	// depth tracking to land on the outer `]`, not the inner one.
	src := "See [a [b] c][s].\n\n[s]: https://example.com\n"
	got := (&Rule{}).Fix(f(t, src))
	assert.Contains(t, string(got),
		"[a [b] c](https://example.com)")
	assert.NotContains(t, string(got), "[s]: https://example.com")
}

func TestFix_LinkTextWithEscapedBracket(t *testing.T) {
	// Link text contains an escaped bracket. The Fix must walk past
	// the escape rather than treat `]` after `\` as the closer.
	src := "See [a\\]b][s].\n\n[s]: https://example.com\n"
	got := (&Rule{}).Fix(f(t, src))
	assert.Contains(t, string(got), "[a\\]b](https://example.com)")
	assert.NotContains(t, string(got), "[s]:")
}

func TestCheck_MidLineFootnoteLikeNotDefinition(t *testing.T) {
	// A `[^slug]:` that appears mid-line (not at the start of a line
	// with ≤3 spaces of indent) must be treated as a footnote
	// reference, not a definition. The rule should fire.
	src := "Some text.[^note]: more text.\n"
	r := &Rule{AllowFootnotes: false}
	diags := r.Check(f(t, src))
	require.Len(t, diags, 1)
	assert.Equal(t, msgFootnote, diags[0].Message)
}

func TestFix_EmptyLinkTextSkipsRewrite(t *testing.T) {
	// A reference link with no text content (e.g. `[][id]`) has no
	// text descendant, so linkSourceSpan can't recover the span. Fix
	// must skip the rewrite (and leave the definition in place)
	// rather than computing a negative start offset and panicking.
	src := "See [][site].\n\n[site]: https://example.com\n"
	got := (&Rule{}).Fix(f(t, src))
	assert.Equal(t, src, string(got))
}

func TestFix_NoReferenceLinksSkipsDefinitionScan(t *testing.T) {
	// When no reference-style link exists, collectDefinitionCuts must
	// short-circuit before re-parsing for definitions. The output is
	// a byte-identical copy of the source.
	src := "Just [inline](https://example.com) text and more prose.\n"
	got := (&Rule{}).Fix(f(t, src))
	assert.Equal(t, src, string(got))
}

func TestCheck_ReferenceStyleImage(t *testing.T) {
	// Reference-style images (![alt][id]) must be flagged with
	// msgRefImage, distinct from msgRefLink.
	src := "![logo][img]\n\n[img]: https://example.com/logo.png\n"
	diags := (&Rule{}).Check(f(t, src))
	require.Len(t, diags, 1)
	assert.Equal(t, msgRefImage, diags[0].Message)
	assert.Equal(t, 1, diags[0].Line)
}

func TestCheck_ReferenceImageCountsAsHasRef(t *testing.T) {
	// A reference-style image counts as "hasRef" so the unused-def
	// diagnostic is suppressed (the image already flags the issue).
	src := "![logo][img]\n\n[img]: https://example.com/logo.png\n"
	diags := (&Rule{}).Check(f(t, src))
	for _, d := range diags {
		assert.NotEqual(t, "unused reference definition", d.Message)
	}
}

func TestFix_ReferenceStyleImage(t *testing.T) {
	// Fix rewrites reference-style images to inline form `![alt](url)`
	// and removes the definition.
	src := "![logo][img]\n\n[img]: https://example.com/logo.png\n"
	got := (&Rule{}).Fix(f(t, src))
	assert.Contains(t, string(got), "![logo](https://example.com/logo.png)")
	assert.NotContains(t, string(got), "[img]:")
}

func TestCheck_DefinitionInCodeBlockNotFlagged(t *testing.T) {
	// A line matching `[label]: url` inside a fenced code block must
	// not be reported as an unused reference definition.
	src := "Plain prose.\n\n```text\n[ref]: https://example.com\n```\n"
	diags := (&Rule{}).Check(f(t, src))
	assert.Empty(t, diags)
}

func TestFix_DefinitionInCodeBlockNotRemoved(t *testing.T) {
	// Fix must not remove definition-like lines from code blocks, even
	// when a real reference for the same label exists in the document.
	src := "See [ref][ref].\n\n[ref]: https://example.com\n\n```text\n[ref]: https://example.com\n```\n"
	got := (&Rule{}).Fix(f(t, src))
	// Reference link is rewritten to inline form.
	assert.Contains(t, string(got), "See [ref](https://example.com).")
	// Real definition is removed; code block content must be preserved.
	assert.Contains(t, string(got), "```text\n[ref]: https://example.com\n```")
}

func TestRegistration(t *testing.T) {
	// init() registered an instance; verify it's the *Rule type and
	// configurable.
	r := &Rule{}
	_, ok := any(r).(rule.Configurable)
	assert.True(t, ok)
	_, ok = any(r).(rule.FixableRule)
	assert.True(t, ok)
	_, ok = any(r).(rule.Defaultable)
	assert.True(t, ok)
}
