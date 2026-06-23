package samefileanchor

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
	goldmarktext "github.com/jeduden/mdsmith/pkg/goldmark/text"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSameFileFragment(t *testing.T) {
	assert.Nil(t, sameFileFragment(nil), "nil input returns nil")
	assert.Nil(t, sameFileFragment([]byte{}), "empty input returns nil")
	assert.Nil(t, sameFileFragment([]byte("other.md#section")), "path+fragment returns nil")
	assert.Nil(t, sameFileFragment([]byte("https://example.com#foo")), "absolute URL returns nil")
	assert.Equal(t, []byte(""), sameFileFragment([]byte("#")), "bare # returns empty fragment")
	assert.Equal(t, []byte("section"), sameFileFragment([]byte("#section")), "same-file fragment")
	assert.Equal(t, []byte("my-heading"), sameFileFragment([]byte("#my-heading")), "hyphenated fragment")
}

func TestAppendHeadingTextRaw(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"plain text", "Hello World", "Hello World"},
		{"link text only", "[click here](url)", "click here"},
		{"image alt text stripped", "![logo](logo.png)", "logo"},
		{"image prefix before text", "![alt](src) after", "alt after"},
		// appendHeadingTextRaw preserves inner bracket chars (depth-tracked) in output.
		{"nested brackets", "[outer [inner] rest](url)", "outer [inner] rest"},
		{"paren URL with nested parens", "[text](url(inner))", "text"},
		{"ref link label discarded", "[text][ref]", "text"},
		{"no markup passthrough", "no links", "no links"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := appendHeadingTextRaw(nil, []byte(tc.input))
			assert.Equal(t, tc.want, string(got))
		})
	}
}

func TestSkipLinkDest(t *testing.T) {
	// Normal inline link destination.
	text := []byte("(url) trailing")
	assert.Equal(t, 5, skipLinkDest(text, 0), "advances past (url)")

	// Not a '(' at position — stays put.
	assert.Equal(t, 5, skipLinkDest(text, 5), "non-( position unchanged")

	// Nested parentheses in URL (Wikipedia-style).
	nested := []byte("(outer(inner)) rest")
	assert.Equal(t, 14, skipLinkDest(nested, 0), "nested parens consumed")

	// Out-of-bounds i — returns i.
	assert.Equal(t, 100, skipLinkDest([]byte("abc"), 100), "out-of-bounds returns i")

	// Empty content between parens.
	assert.Equal(t, 2, skipLinkDest([]byte("()"), 0), "empty dest")
}

func TestSkipRefLabel(t *testing.T) {
	text := []byte("[ref] trailing")
	assert.Equal(t, 5, skipRefLabel(text, 0), "advances past [ref]")

	// Not a '[' at position — stays put.
	assert.Equal(t, 5, skipRefLabel(text, 5), "non-[ position unchanged")

	// Unterminated label — advances to end.
	unterminated := []byte("[noclose")
	assert.Equal(t, len(unterminated), skipRefLabel(unterminated, 0), "unterminated label reaches end")

	// Out-of-bounds i — returns i.
	assert.Equal(t, 100, skipRefLabel([]byte("abc"), 100), "out-of-bounds returns i")

	// Empty label.
	assert.Equal(t, 2, skipRefLabel([]byte("[]"), 0), "empty label")
}

func TestInsertDisambiguated(t *testing.T) {
	slugs := make(map[string]struct{})
	counts := make(map[string]int)

	insertDisambiguated(slugs, counts, "intro")
	assert.Contains(t, slugs, "intro", "first insertion uses bare slug")

	insertDisambiguated(slugs, counts, "intro")
	assert.Contains(t, slugs, "intro-1", "second insertion gets -1 suffix")

	insertDisambiguated(slugs, counts, "intro")
	assert.Contains(t, slugs, "intro-2", "third insertion gets -2 suffix")

	// A different slug does not inherit the intro counter.
	insertDisambiguated(slugs, counts, "other")
	assert.Contains(t, slugs, "other", "different slug inserted without suffix")
	assert.NotContains(t, slugs, "other-1", "different slug not disambiguated")
}

func TestAtxHeadingText(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"basic h1", "# Hello\n", "Hello"},
		{"h2", "## World\n", "World"},
		{"closing markers stripped", "## Section ##\n", "Section"},
		{"leading spaces (3)", "   # Indented\n", "Indented"},
		{"tab after marker", "#\tTabbed\n", "Tabbed"},
		{"CRLF ending", "# Heading\r\n", "Heading"},
		{"empty body", "#\n", ""},
		// TrimRight strips only \r\n; trailing spaces before ## block detection.
		{"closing hashes with trailing space", "## Title ##  \n", "Title ##"},
		{"no space after marker", "#NoSpace\n", "NoSpace"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := atxHeadingText([]byte(tc.input))
			assert.Equal(t, tc.want, string(got))
		})
	}
}

func TestAppendSlug(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"lowercase passthrough", "hello", "hello"},
		{"uppercase lowercased", "HELLO", "hello"},
		{"space to hyphen", "hello world", "hello-world"},
		{"tab to hyphen", "hello\tworld", "hello-world"},
		{"punctuation dropped", "hello, world!", "hello-world"},
		{"digits kept", "test123", "test123"},
		{"hyphen kept", "already-hyphenated", "already-hyphenated"},
		{"empty input", "", ""},
		{"unicode letter kept", "café", "café"},
		{"unicode non-letter dropped", "test→value", "testvalue"},
		{"unicode uppercase lowercased", "ÜBER", "über"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := appendSlug(nil, []byte(tc.input))
			assert.Equal(t, tc.want, string(got))
		})
	}
}

func TestAppendHeadingText(t *testing.T) {
	// "# Hello World\n"
	//   0123456789012 3
	src := []byte("# Hello World\n")
	h := ast.NewHeading(1)
	txt := ast.NewTextSegment(goldmarktext.NewSegment(2, 13)) // "Hello World"
	h.AppendChild(h, txt)

	got := appendHeadingText(h, src, nil)
	assert.Equal(t, "Hello World", string(got))

	// Nested markup: strong node wrapping a text node.
	// "# **Bold** Section\n"
	//   0123456789012345678
	src2 := []byte("# **Bold** Section\n")
	h2 := ast.NewHeading(1)
	em := ast.NewEmphasis(2)
	boldTxt := ast.NewTextSegment(goldmarktext.NewSegment(4, 8)) // "Bold" (4..7 inclusive)
	em.AppendChild(em, boldTxt)
	restTxt := ast.NewTextSegment(goldmarktext.NewSegment(10, 18)) // " Section"
	h2.AppendChild(h2, em)
	h2.AppendChild(h2, restTxt)

	got2 := appendHeadingText(h2, src2, nil)
	assert.Equal(t, "Bold Section", string(got2))

	// No children — empty result.
	empty := ast.NewHeading(1)
	got3 := appendHeadingText(empty, src, nil)
	assert.Empty(t, got3)
}

func TestCollectSlugsNode(t *testing.T) {
	// "# First\n## Second\n"
	//   01234567890123456789
	src := []byte("# First\n## Second\n")
	slugs := make(map[string]struct{})
	counts := make(map[string]int)

	h1 := ast.NewHeading(1)
	t1 := ast.NewTextSegment(goldmarktext.NewSegment(2, 7)) // "First"
	h1.AppendChild(h1, t1)

	h2 := ast.NewHeading(2)
	t2 := ast.NewTextSegment(goldmarktext.NewSegment(11, 17)) // "Second"
	h2.AppendChild(h2, t2)

	doc := ast.NewDocument()
	doc.AppendChild(doc, h1)
	doc.AppendChild(doc, h2)

	collectSlugsNode(doc, src, slugs, counts, nil, nil)
	assert.Contains(t, slugs, "first")
	assert.Contains(t, slugs, "second")
	assert.Len(t, slugs, 2)
}

func TestCollectSlugsAST(t *testing.T) {
	f, err := lint.NewFile("test.md", []byte("# Alpha\n## Beta\n"))
	require.NoError(t, err)
	require.NotNil(t, f.AST, "test requires a parsed AST")

	slugs := collectSlugsAST(f)
	assert.Contains(t, slugs, "alpha")
	assert.Contains(t, slugs, "beta")
	assert.Len(t, slugs, 2)
}

func TestCollectSlugsLayer0(t *testing.T) {
	f := lint.NewFileLines("test.md", []byte("# Gamma\n## Delta\n"))
	slugs := collectSlugsLayer0(f)
	assert.Contains(t, slugs, "gamma")
	assert.Contains(t, slugs, "delta")
	assert.Len(t, slugs, 2)
}

func TestCollectSlugs(t *testing.T) {
	src := []byte("# One\n## Two\n")

	// AST path (lint.NewFile parses a goldmark AST).
	fAST, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	require.NotNil(t, fAST.AST, "test requires a parsed AST")
	slugsAST := collectSlugs(fAST)
	assert.Contains(t, slugsAST, "one")
	assert.Contains(t, slugsAST, "two")

	// nil-AST (Layer0) path.
	fLines := lint.NewFileLines("test.md", src)
	assert.Nil(t, fLines.AST, "test requires nil AST")
	slugsL0 := collectSlugs(fLines)
	assert.Contains(t, slugsL0, "one")
	assert.Contains(t, slugsL0, "two")
}
