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
