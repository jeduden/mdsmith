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
