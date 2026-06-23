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

	// Pre-seeded collision: "fix-1" already exists before the second "fix"
	// arrives. The inner loop must skip "fix-1" and land on "fix-2".
	slugs2 := map[string]struct{}{"fix": {}, "fix-1": {}}
	counts2 := map[string]int{}
	insertDisambiguated(slugs2, counts2, "fix")
	assert.Contains(t, slugs2, "fix-2", "inner loop skips pre-existing fix-1")
	assert.NotContains(t, slugs2, "fix-3", "stops at first free slot")
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
		// NOTE: CommonMark §4.2 allows trailing spaces after closing ##; the
		// implementation strips only \r\n before detecting closing hashes, so
		// spaces that precede ## prevent detection and leak into the text.
		// This is a known spec deviation: "## Title ##  \n" slugifies to
		// "title-" on mdsmith but "title" on GitHub. Tracked as a known gap.
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
	boldTxt := ast.NewTextSegment(goldmarktext.NewSegment(4, 8)) // src2[4:8] = "Bold"
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

	// Use stack-allocated backing arrays to match the production calling
	// pattern in collectSlugsAST (nil works too, but this is more faithful).
	var textBuf [256]byte
	var slugBuf [512]byte
	collectSlugsNode(doc, src, slugs, counts, textBuf[:0], slugBuf[:0])
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

	// No headings — collectSlugsAST must return nil (not an empty map) so
	// callers that nil-check the result (e.g. checkAST) work correctly.
	fEmpty, err := lint.NewFile("test.md", []byte("[link](#x).\n"))
	require.NoError(t, err)
	assert.Nil(t, collectSlugsAST(fEmpty), "no headings returns nil")
}

func TestCollectSlugsLayer0(t *testing.T) {
	f := lint.NewFileLines("test.md", []byte("# Gamma\n## Delta\n"))
	slugs := collectSlugsLayer0(f)
	assert.Contains(t, slugs, "gamma")
	assert.Contains(t, slugs, "delta")
	assert.Len(t, slugs, 2)

	// Setext heading — the BlockSetextHeading branch takes the first line of
	// the span as text and the second as the underline.
	fSetext := lint.NewFileLines("test.md", []byte("Setext Title\n============\n"))
	slugsSetext := collectSlugsLayer0(fSetext)
	assert.Contains(t, slugsSetext, "setext-title", "setext heading slug")

	// No headings — must return nil.
	fNone := lint.NewFileLines("test.md", []byte("plain paragraph\n"))
	assert.Nil(t, collectSlugsLayer0(fNone), "no headings returns nil")
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
