package mdtext_test

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/mdtext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// parseDoc parses markdown and returns the document root node.
func parseDoc(t *testing.T, src string) (ast.Node, []byte) {
	t.Helper()
	source := []byte(src)
	reader := text.NewReader(source)
	doc := goldmark.DefaultParser().Parse(reader)
	return doc, source
}

// parseParagraph parses markdown and returns the first Paragraph node.
func parseParagraph(t *testing.T, src string) (ast.Node, []byte) {
	t.Helper()
	source := []byte(src)
	reader := text.NewReader(source)
	doc := goldmark.DefaultParser().Parse(reader)
	var para ast.Node
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			if _, ok := n.(*ast.Paragraph); ok {
				para = n
				return ast.WalkStop, nil
			}
		}
		return ast.WalkContinue, nil
	})
	require.NotNil(t, para, "no paragraph found")
	return para, source
}

func TestExtractPlainText_PlainParagraph(t *testing.T) {
	para, src := parseParagraph(t, "Hello world.\n")
	assert.Equal(t, "Hello world.", mdtext.ExtractPlainText(para, src))
}

func TestExtractPlainText_Link(t *testing.T) {
	para, src := parseParagraph(t, "Click [here](https://example.com) now.\n")
	assert.Equal(t, "Click here now.", mdtext.ExtractPlainText(para, src))
}

func TestExtractPlainText_Emphasis(t *testing.T) {
	para, src := parseParagraph(t, "This is *important* text.\n")
	assert.Equal(t, "This is important text.", mdtext.ExtractPlainText(para, src))
}

func TestExtractPlainText_Strong(t *testing.T) {
	para, src := parseParagraph(t, "This is **bold** text.\n")
	assert.Equal(t, "This is bold text.", mdtext.ExtractPlainText(para, src))
}

func TestExtractPlainText_CodeSpan(t *testing.T) {
	para, src := parseParagraph(t, "Use `fmt.Println` to print.\n")
	assert.Equal(t, "Use fmt.Println to print.", mdtext.ExtractPlainText(para, src))
}

func TestExtractPlainText_Image(t *testing.T) {
	para, src := parseParagraph(t, "See ![alt text](image.png) here.\n")
	assert.Equal(t, "See alt text here.", mdtext.ExtractPlainText(para, src))
}

func TestExtractPlainText_NestedMarkup(t *testing.T) {
	para, src := parseParagraph(
		t,
		"Click [**bold link**](https://example.com) now.\n",
	)
	assert.Equal(t, "Click bold link now.", mdtext.ExtractPlainText(para, src))
}

func TestExtractPlainText_SoftLineBreak(t *testing.T) {
	para, src := parseParagraph(t, "Hello\nworld.\n")
	assert.Equal(t, "Hello world.", mdtext.ExtractPlainText(para, src))
}

// TestExtractPlainText_AstString covers heading/paragraph children
// emitted as *ast.String — the goldmark typographer extension and
// other inline transformers replace text segments with String nodes
// whose value lives in the node rather than the source buffer.
// Without explicit handling, ExtractPlainText drops them, which
// silently breaks anchor slugs and link-text extraction for any
// heading or link processed by such an extension.
func TestExtractPlainText_AstString(t *testing.T) {
	para := ast.NewParagraph()
	para.AppendChild(para, ast.NewString([]byte("hello world")))
	assert.Equal(t, "hello world", mdtext.ExtractPlainText(para, nil))
}

// --- CountWords tests ---

func TestCountWords_Simple(t *testing.T) {
	assert.Equal(t, 2, mdtext.CountWords("hello world"))
}

func TestCountWords_Empty(t *testing.T) {
	assert.Equal(t, 0, mdtext.CountWords(""))
}

func TestCountWords_MultipleSpaces(t *testing.T) {
	assert.Equal(t, 2, mdtext.CountWords("  hello   world  "))
}

// --- CountSentences tests ---

func TestCountSentences_OneSentence(t *testing.T) {
	assert.Equal(t, 1, mdtext.CountSentences("Hello world."))
}

func TestCountSentences_TwoSentences(t *testing.T) {
	assert.Equal(t, 2, mdtext.CountSentences("Hello world. How are you?"))
}

func TestCountSentences_NoTerminator(t *testing.T) {
	assert.Equal(t, 1, mdtext.CountSentences("Hello world"))
}

func TestCountSentences_Empty(t *testing.T) {
	assert.Equal(t, 0, mdtext.CountSentences(""))
}

func TestCountSentences_Exclamation(t *testing.T) {
	assert.Equal(t, 2, mdtext.CountSentences("Wow! Amazing!"))
}

func TestCountSentences_AbbreviationNotCounted(t *testing.T) {
	assert.Equal(t, 2, mdtext.CountSentences("Use e.g. this one."))
}

// --- SplitSentences tests ---

func TestSplitSentences_Simple(t *testing.T) {
	got := mdtext.SplitSentences("Hello world. How are you?")
	require.Len(t, got, 2)
	assert.Equal(t, "Hello world.", got[0])
	assert.Equal(t, "How are you?", got[1])
}

func TestSplitSentences_Exclamation(t *testing.T) {
	got := mdtext.SplitSentences("Wow! Amazing!")
	require.Len(t, got, 2)
}

func TestSplitSentences_Abbreviation(t *testing.T) {
	got := mdtext.SplitSentences("Dr. Smith went home.")
	require.Len(t, got, 1)
}

func TestSplitSentences_Decimal(t *testing.T) {
	got := mdtext.SplitSentences("The value is 3.14 today.")
	require.Len(t, got, 1)
}

func TestSplitSentences_Empty(t *testing.T) {
	got := mdtext.SplitSentences("")
	require.Empty(t, got)
}

func TestSplitSentences_WhitespaceOnly(t *testing.T) {
	got := mdtext.SplitSentences("   ")
	require.Empty(t, got)
}

// --- CountCharacters tests ---

func TestCountCharacters_Simple(t *testing.T) {
	assert.Equal(t, 10, mdtext.CountCharacters("Hello, world!"))
}

func TestCountCharacters_WithDigits(t *testing.T) {
	assert.Equal(t, 6, mdtext.CountCharacters("abc 123"))
}

func TestCountCharacters_Empty(t *testing.T) {
	assert.Equal(t, 0, mdtext.CountCharacters(""))
}

func TestCountCharacters_OnlyPunctuation(t *testing.T) {
	assert.Equal(t, 0, mdtext.CountCharacters("...!!!"))
}

// --- Slugify tests ---

func TestSlugify_Simple(t *testing.T) {
	assert.Equal(t, "hello-world", mdtext.Slugify("Hello World"))
}

func TestSlugify_Empty(t *testing.T) {
	assert.Equal(t, "", mdtext.Slugify(""))
}

func TestSlugify_TrimSpaces(t *testing.T) {
	assert.Equal(t, "hello", mdtext.Slugify("  Hello  "))
}

func TestSlugify_SpecialChars(t *testing.T) {
	assert.Equal(t, "hello-world", mdtext.Slugify("Hello, World!"))
}

func TestSlugify_MultipleDashes(t *testing.T) {
	assert.Equal(t, "hello-world", mdtext.Slugify("Hello  World"))
}

func TestSlugify_Underscores(t *testing.T) {
	assert.Equal(t, "foo-bar", mdtext.Slugify("foo_bar"))
}

func TestSlugify_LeadingTrailingDashes(t *testing.T) {
	assert.Equal(t, "hello", mdtext.Slugify("---hello---"))
}

func TestSlugify_Unicode(t *testing.T) {
	// Unicode letters are preserved.
	result := mdtext.Slugify("Ångström")
	assert.NotEmpty(t, result)
}

// --- CollectTOCItems tests ---

func TestCollectTOCItems_Basic(t *testing.T) {
	doc, src := parseDoc(t, "# Title\n\n## Section\n\n### Sub\n")
	items := mdtext.CollectTOCItems(doc, src)
	require.Len(t, items, 3)
	assert.Equal(t, 1, items[0].Level)
	assert.Equal(t, "Title", items[0].Text)
	assert.Equal(t, "title", items[0].Anchor)
	assert.Equal(t, 2, items[1].Level)
	assert.Equal(t, "section", items[1].Anchor)
	assert.Equal(t, 3, items[2].Level)
	assert.Equal(t, "sub", items[2].Anchor)
}

func TestCollectTOCItems_DuplicateAnchors(t *testing.T) {
	doc, src := parseDoc(t, "## Foo\n\n## Foo\n\n## Foo\n")
	items := mdtext.CollectTOCItems(doc, src)
	require.Len(t, items, 3)
	assert.Equal(t, "foo", items[0].Anchor)
	assert.Equal(t, "foo-1", items[1].Anchor)
	assert.Equal(t, "foo-2", items[2].Anchor)
}

func TestCollectTOCItems_Empty(t *testing.T) {
	doc, src := parseDoc(t, "Just a paragraph.\n")
	items := mdtext.CollectTOCItems(doc, src)
	assert.Empty(t, items)
}

func TestCollectTOCItems_HeadingWithEmptySlug(t *testing.T) {
	// A heading consisting only of punctuation produces an empty slug and
	// is skipped.
	doc, src := parseDoc(t, "## ---\n\n## Normal\n")
	items := mdtext.CollectTOCItems(doc, src)
	require.Len(t, items, 1, "heading with empty slug should be skipped")
	assert.Equal(t, "Normal", items[0].Text)
}
