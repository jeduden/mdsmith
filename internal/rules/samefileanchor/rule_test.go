package samefileanchor_test

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rules/samefileanchor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func check(t *testing.T, src string) []lint.Diagnostic {
	t.Helper()
	r := &samefileanchor.Rule{}
	f, err := lint.NewFile("test.md", []byte(src))
	require.NoError(t, err)
	return r.Check(f)
}

func checkLines(t *testing.T, src string) []lint.Diagnostic {
	t.Helper()
	r := &samefileanchor.Rule{}
	f := lint.NewFileLines("test.md", []byte(src))
	return r.Check(f)
}

// TestRule_ID verifies the rule metadata.
func TestRule_ID(t *testing.T) {
	r := &samefileanchor.Rule{}
	assert.Equal(t, "MDS070", r.ID())
	assert.Equal(t, "same-file-anchor", r.Name())
	assert.Equal(t, "link", r.Category())
}

// TestRule_NoFragment verifies that links without fragments are ignored.
func TestRule_NoFragment(t *testing.T) {
	src := "# Heading\n\nSee [other](other.md).\n"
	assert.Empty(t, check(t, src))
	assert.Empty(t, checkLines(t, src))
}

// TestRule_ResolvedFragment verifies a valid same-file anchor is accepted.
func TestRule_ResolvedFragment(t *testing.T) {
	src := "# My Heading\n\nSee [link](#my-heading).\n"
	assert.Empty(t, check(t, src))
	assert.Empty(t, checkLines(t, src))
}

// TestRule_UnresolvedFragment verifies that an unresolved anchor is flagged.
func TestRule_UnresolvedFragment(t *testing.T) {
	src := "# My Heading\n\nSee [link](#nonexistent).\n"
	diags := check(t, src)
	require.Len(t, diags, 1)
	assert.Equal(t, "MDS070", diags[0].RuleID)
	assert.Contains(t, diags[0].Message, "#nonexistent")

	// nil-AST path should produce the same result.
	diags2 := checkLines(t, src)
	require.Len(t, diags2, 1)
	assert.Equal(t, "MDS070", diags2[0].RuleID)
	assert.Contains(t, diags2[0].Message, "#nonexistent")
}

// TestRule_SlugWithSpaces verifies that GitHub-style slug computation handles spaces.
func TestRule_SlugWithSpaces(t *testing.T) {
	src := "# Hello World\n\nSee [link](#hello-world).\n"
	assert.Empty(t, check(t, src))
	assert.Empty(t, checkLines(t, src))
}

// TestRule_SlugUppercase verifies that slugs are lowercased.
func TestRule_SlugUppercase(t *testing.T) {
	src := "# UPPER CASE\n\nSee [link](#upper-case).\n"
	assert.Empty(t, check(t, src))
	assert.Empty(t, checkLines(t, src))
}

// TestRule_SlugPunctuation verifies that punctuation is stripped in slugs.
func TestRule_SlugPunctuation(t *testing.T) {
	src := "# Hello, World!\n\nSee [link](#hello-world).\n"
	assert.Empty(t, check(t, src))
	assert.Empty(t, checkLines(t, src))
}

// TestRule_MultipleHeadings verifies that multiple headings are all indexed.
func TestRule_MultipleHeadings(t *testing.T) {
	src := "# First Heading\n\n## Second Heading\n\n" +
		"See [a](#first-heading) and [b](#second-heading).\n"
	assert.Empty(t, check(t, src))
	assert.Empty(t, checkLines(t, src))
}

// TestRule_MultipleUnresolved verifies that multiple unresolved fragments are all reported.
func TestRule_MultipleUnresolved(t *testing.T) {
	src := "# Only Heading\n\nSee [a](#missing-one) and [b](#missing-two).\n"
	diags := check(t, src)
	assert.Len(t, diags, 2)

	diags2 := checkLines(t, src)
	assert.Len(t, diags2, 2)
}

// TestRule_EmptyFile handles the nil file guard.
func TestRule_EmptyFile(t *testing.T) {
	r := &samefileanchor.Rule{}
	assert.Empty(t, r.Check(nil))
}

// TestRule_CodeBlockIgnored verifies anchors inside code blocks are not flagged.
func TestRule_CodeBlockIgnored(t *testing.T) {
	// A link inside a fenced code block is not a live link.
	src := "# Heading\n\n```\n[bad](#nope)\n```\n"
	assert.Empty(t, check(t, src))
	assert.Empty(t, checkLines(t, src))
}

// TestRule_ImageFragment verifies that image destinations with fragments are also checked.
func TestRule_ImageFragment(t *testing.T) {
	src := "# Section\n\n![img](#missing-section)\n"
	diags := check(t, src)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "#missing-section")

	diags2 := checkLines(t, src)
	require.Len(t, diags2, 1)
	assert.Contains(t, diags2[0].Message, "#missing-section")
}

// TestRule_ExternalLinkIgnored verifies that external links (non-same-file) are ignored.
func TestRule_ExternalLinkIgnored(t *testing.T) {
	src := "# Heading\n\nSee [link](https://example.com#section).\n"
	assert.Empty(t, check(t, src))
	assert.Empty(t, checkLines(t, src))
}

// TestRule_RelativeLinkWithFragment verifies that relative links with fragments
// (i.e. links to another file#anchor) are not treated as same-file.
func TestRule_RelativeLinkWithFragment(t *testing.T) {
	// "other.md#section" is NOT a same-file fragment — only "#fragment" is.
	src := "# Heading\n\nSee [link](other.md#section).\n"
	assert.Empty(t, check(t, src))
	assert.Empty(t, checkLines(t, src))
}

// TestRule_SetextHeading verifies that setext headings are indexed.
func TestRule_SetextHeading(t *testing.T) {
	src := "Setext Heading\n==============\n\nSee [link](#setext-heading).\n"
	assert.Empty(t, check(t, src))
	assert.Empty(t, checkLines(t, src))
}

// TestRule_EmptyFragment verifies that a bare # (empty fragment) is not flagged
// since GitHub also allows it as a top-of-page anchor.
func TestRule_EmptyFragment(t *testing.T) {
	src := "# Heading\n\nSee [link](#).\n"
	assert.Empty(t, check(t, src))
	assert.Empty(t, checkLines(t, src))
}

// TestRule_EnabledByDefault verifies the rule is on by default.
func TestRule_EnabledByDefault(t *testing.T) {
	r := &samefileanchor.Rule{}
	assert.True(t, r.EnabledByDefault())
}

// TestRule_NoHashInSource verifies that a file with no '#' at all returns
// immediately via the pre-filter, reporting zero diagnostics.
func TestRule_NoHashInSource(t *testing.T) {
	src := "Plain text with no hash characters at all.\n"
	assert.Empty(t, check(t, src))
	assert.Empty(t, checkLines(t, src))
}

// TestRule_NoHeadings verifies that a file that has '#' (in a fragment link)
// but no headings reports every same-file fragment link as unresolved.
func TestRule_NoHeadings(t *testing.T) {
	src := "See [link](#something).\n"
	diags := check(t, src)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "#something")

	diags2 := checkLines(t, src)
	require.Len(t, diags2, 1)
	assert.Contains(t, diags2[0].Message, "#something")
}

// TestRule_PunctuationOnlyHeading verifies that a heading whose text reduces to
// an empty slug (e.g. "# !!!") is correctly skipped — no slug is added,
// so any fragment link targeting it is reported unresolved.
func TestRule_PunctuationOnlyHeading(t *testing.T) {
	src := "# !!!\n\nSee [link](#something).\n"
	// The heading "!!!" produces an empty slug and is skipped; "#something" is unresolved.
	diags := check(t, src)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "#something")

	diags2 := checkLines(t, src)
	require.Len(t, diags2, 1)
	assert.Contains(t, diags2[0].Message, "#something")
}

// TestRule_ATXClosingMarkers verifies that ## Heading ## (closing ## markers) is
// correctly slugified — the trailing ## and any space before it are stripped.
func TestRule_ATXClosingMarkers(t *testing.T) {
	src := "## My Section ##\n\nSee [link](#my-section).\n"
	assert.Empty(t, check(t, src))
	assert.Empty(t, checkLines(t, src))
}

// TestRule_UnicodeHeading verifies that non-ASCII letters are kept and lowercased
// in slugs (e.g. accented characters).
func TestRule_UnicodeHeading(t *testing.T) {
	// 'é' (U+00E9) is a non-ASCII letter; slug keeps it lowercased.
	src := "# Café Style\n\nSee [link](#café-style).\n"
	assert.Empty(t, check(t, src))
	assert.Empty(t, checkLines(t, src))
}

// TestRule_NonLetterUnicodeDropped verifies that non-letter, non-number Unicode
// characters are dropped from the slug.
func TestRule_NonLetterUnicodeDropped(t *testing.T) {
	// '→' (U+2192) is not a letter/number and is not a space, so it is
	// dropped. "Test→Value" slugifies to "testvalue" (no hyphen inserted).
	src := "# Test→Value\n\nSee [link](#testvalue).\n"
	assert.Empty(t, check(t, src))
	assert.Empty(t, checkLines(t, src))
}

// TestRule_InlineNodeLineRecursive verifies that line numbers are found for links
// whose text children are nested inside inline markup (strong, em).
func TestRule_InlineNodeLineRecursive(t *testing.T) {
	// [**bold**](#missing) — the link's direct child is *ast.Strong, not *ast.Text.
	// inlineNodeLine must recurse into the strong to find the text node's offset.
	src := "# Section\n\n[**bold**](#missing).\n"
	diags := check(t, src)
	require.Len(t, diags, 1)
	assert.Equal(t, "MDS070", diags[0].RuleID)
	assert.Equal(t, 3, diags[0].Line)
}

// TestRule_ImageNoAltFallbackLine verifies that the line-number fallback (line 1)
// fires when an image node has no text children (empty alt text).
func TestRule_ImageNoAltFallbackLine(t *testing.T) {
	// ![]( #missing) — empty alt; the *ast.Image has no text children, so
	// inlineNodeLine falls back to 1.
	src := "# Section\n\n![](#missing).\n"
	diags := check(t, src)
	require.Len(t, diags, 1)
	assert.Equal(t, "MDS070", diags[0].RuleID)
	assert.Equal(t, 1, diags[0].Line)
}

// TestRule_ATXHeadingLeadingSpaces verifies that ATX headings with up to 3
// leading spaces are parsed correctly (the leading-space strip path).
func TestRule_ATXHeadingLeadingSpaces(t *testing.T) {
	src := "   # Indented Heading\n\nSee [link](#indented-heading).\n"
	assert.Empty(t, check(t, src))
	assert.Empty(t, checkLines(t, src))
}

// TestRule_EmphasisInHeading verifies that text inside inline emphasis is included
// in the slug so that #bold-section resolves for a heading like # **Bold** Section.
func TestRule_EmphasisInHeading(t *testing.T) {
	src := "# **Bold** Section\n\nSee [link](#bold-section).\n"
	assert.Empty(t, check(t, src))
	assert.Empty(t, checkLines(t, src))
}

// TestRule_CodeSpanInHeading verifies that text inside an inline code span is
// included in the slug, matching GitHub's rendering of # Hello `world`.
func TestRule_CodeSpanInHeading(t *testing.T) {
	src := "# Hello `world`\n\nSee [link](#hello-world).\n"
	assert.Empty(t, check(t, src))
	assert.Empty(t, checkLines(t, src))
}

// TestRule_DuplicateHeadings verifies that duplicate headings produce
// disambiguated anchors (#installation and #installation-1), matching GitHub.
func TestRule_DuplicateHeadings(t *testing.T) {
	src := "## Installation\n\n## Installation\n\n" +
		"See [first](#installation) and [second](#installation-1).\n"
	assert.Empty(t, check(t, src))
	assert.Empty(t, checkLines(t, src))
}

// TestRule_DuplicateHeadingsUnresolved verifies that a link to #installation-2
// when only two Installation headings exist is flagged as broken.
func TestRule_DuplicateHeadingsUnresolved(t *testing.T) {
	src := "## Installation\n\n## Installation\n\nSee [link](#installation-2).\n"
	diags := check(t, src)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "#installation-2")
}

// TestRule_TabInHeading verifies that a tab character in a heading is converted
// to a hyphen in the slug, matching GitHub's tab→space→hyphen algorithm.
func TestRule_TabInHeading(t *testing.T) {
	src := "# foo\tbar\n\nSee [link](#foo-bar).\n"
	assert.Empty(t, check(t, src))
	assert.Empty(t, checkLines(t, src))
}

// TestRule_InlineNodeLineOnLine1 verifies that a link with nested inline markup
// (e.g. [**a**](#missing)) on line 1 reports a diagnostic at line 1, not at a
// later sibling's line — the old ln>1 sentinel would discard valid line-1 results.
func TestRule_InlineNodeLineOnLine1(t *testing.T) {
	src := "[**a** text](#missing).\n"
	diags := check(t, src)
	require.Len(t, diags, 1)
	assert.Equal(t, 1, diags[0].Line)
}
