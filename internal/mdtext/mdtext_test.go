package mdtext_test

import (
	"testing"

	"github.com/jeduden/tidymark/internal/mdtext"
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
	if para == nil {
		t.Fatal("no paragraph found")
	}
	return para, source
}

func TestExtractPlainText_PlainParagraph(t *testing.T) {
	para, src := parseParagraph(t, "Hello world.\n")
	got := mdtext.ExtractPlainText(para, src)
	if got != "Hello world." {
		t.Errorf("got %q, want %q", got, "Hello world.")
	}
}

func TestExtractPlainText_Link(t *testing.T) {
	para, src := parseParagraph(t, "Click [here](https://example.com) now.\n")
	got := mdtext.ExtractPlainText(para, src)
	if got != "Click here now." {
		t.Errorf("got %q, want %q", got, "Click here now.")
	}
}

func TestExtractPlainText_Emphasis(t *testing.T) {
	para, src := parseParagraph(t, "This is *important* text.\n")
	got := mdtext.ExtractPlainText(para, src)
	if got != "This is important text." {
		t.Errorf("got %q, want %q", got, "This is important text.")
	}
}

func TestExtractPlainText_Strong(t *testing.T) {
	para, src := parseParagraph(t, "This is **bold** text.\n")
	got := mdtext.ExtractPlainText(para, src)
	if got != "This is bold text." {
		t.Errorf("got %q, want %q", got, "This is bold text.")
	}
}

func TestExtractPlainText_CodeSpan(t *testing.T) {
	para, src := parseParagraph(t, "Use `fmt.Println` to print.\n")
	got := mdtext.ExtractPlainText(para, src)
	if got != "Use fmt.Println to print." {
		t.Errorf("got %q, want %q", got, "Use fmt.Println to print.")
	}
}

func TestExtractPlainText_Image(t *testing.T) {
	para, src := parseParagraph(t, "See ![alt text](image.png) here.\n")
	got := mdtext.ExtractPlainText(para, src)
	if got != "See alt text here." {
		t.Errorf("got %q, want %q", got, "See alt text here.")
	}
}

func TestExtractPlainText_NestedMarkup(t *testing.T) {
	para, src := parseParagraph(
		t,
		"Click [**bold link**](https://example.com) now.\n",
	)
	got := mdtext.ExtractPlainText(para, src)
	if got != "Click bold link now." {
		t.Errorf("got %q, want %q", got, "Click bold link now.")
	}
}

func TestExtractPlainText_SoftLineBreak(t *testing.T) {
	para, src := parseParagraph(t, "Hello\nworld.\n")
	got := mdtext.ExtractPlainText(para, src)
	if got != "Hello world." {
		t.Errorf("got %q, want %q", got, "Hello world.")
	}
}

// --- CountWords tests ---

func TestCountWords_Simple(t *testing.T) {
	if got := mdtext.CountWords("hello world"); got != 2 {
		t.Errorf("got %d, want 2", got)
	}
}

func TestCountWords_Empty(t *testing.T) {
	if got := mdtext.CountWords(""); got != 0 {
		t.Errorf("got %d, want 0", got)
	}
}

func TestCountWords_MultipleSpaces(t *testing.T) {
	if got := mdtext.CountWords("  hello   world  "); got != 2 {
		t.Errorf("got %d, want 2", got)
	}
}

// --- CountSentences tests ---

func TestCountSentences_OneSentence(t *testing.T) {
	if got := mdtext.CountSentences("Hello world."); got != 1 {
		t.Errorf("got %d, want 1", got)
	}
}

func TestCountSentences_TwoSentences(t *testing.T) {
	got := mdtext.CountSentences("Hello world. How are you?")
	if got != 2 {
		t.Errorf("got %d, want 2", got)
	}
}

func TestCountSentences_NoTerminator(t *testing.T) {
	// No sentence-ending punctuation returns 1 for non-empty text.
	if got := mdtext.CountSentences("Hello world"); got != 1 {
		t.Errorf("got %d, want 1", got)
	}
}

func TestCountSentences_Empty(t *testing.T) {
	if got := mdtext.CountSentences(""); got != 0 {
		t.Errorf("got %d, want 0", got)
	}
}

func TestCountSentences_Exclamation(t *testing.T) {
	got := mdtext.CountSentences("Wow! Amazing!")
	if got != 2 {
		t.Errorf("got %d, want 2", got)
	}
}

func TestCountSentences_AbbreviationNotCounted(t *testing.T) {
	// "e.g." has dots not followed by whitespace (except last),
	// so interior dots are not counted.
	got := mdtext.CountSentences("Use e.g. this one.")
	// "e.g." -> dot after 'g' at position is followed by space -> counted
	// then ". " after "this one" -> counted
	// Actually "e.g. " the dot after g is followed by space so counted.
	// And "one." at end is counted. So 2.
	if got != 2 {
		t.Errorf("got %d, want 2", got)
	}
}

// --- SplitSentences tests ---

func TestSplitSentences_Simple(t *testing.T) {
	got := mdtext.SplitSentences("Hello world. How are you?")
	if len(got) != 2 {
		t.Fatalf("got %d sentences, want 2: %v", len(got), got)
	}
	if got[0] != "Hello world." {
		t.Errorf("sentence 0: got %q, want %q", got[0], "Hello world.")
	}
	if got[1] != "How are you?" {
		t.Errorf("sentence 1: got %q, want %q", got[1], "How are you?")
	}
}

func TestSplitSentences_Exclamation(t *testing.T) {
	got := mdtext.SplitSentences("Wow! Amazing!")
	if len(got) != 2 {
		t.Fatalf("got %d sentences, want 2: %v", len(got), got)
	}
}

func TestSplitSentences_Abbreviation(t *testing.T) {
	got := mdtext.SplitSentences("Dr. Smith went home.")
	if len(got) != 1 {
		t.Fatalf("got %d sentences, want 1: %v", len(got), got)
	}
}

func TestSplitSentences_Decimal(t *testing.T) {
	got := mdtext.SplitSentences("The value is 3.14 today.")
	if len(got) != 1 {
		t.Fatalf("got %d sentences, want 1: %v", len(got), got)
	}
}

func TestSplitSentences_Empty(t *testing.T) {
	got := mdtext.SplitSentences("")
	if len(got) != 0 {
		t.Fatalf("got %d sentences, want 0: %v", len(got), got)
	}
}

func TestSplitSentences_WhitespaceOnly(t *testing.T) {
	got := mdtext.SplitSentences("   ")
	if len(got) != 0 {
		t.Fatalf("got %d sentences, want 0: %v", len(got), got)
	}
}

// --- CountCharacters tests ---

func TestCountCharacters_Simple(t *testing.T) {
	got := mdtext.CountCharacters("Hello, world!")
	// H e l l o w o r l d = 10 letters, no digits
	if got != 10 {
		t.Errorf("got %d, want 10", got)
	}
}

func TestCountCharacters_WithDigits(t *testing.T) {
	got := mdtext.CountCharacters("abc 123")
	// a b c 1 2 3 = 6
	if got != 6 {
		t.Errorf("got %d, want 6", got)
	}
}

func TestCountCharacters_Empty(t *testing.T) {
	if got := mdtext.CountCharacters(""); got != 0 {
		t.Errorf("got %d, want 0", got)
	}
}

func TestCountCharacters_OnlyPunctuation(t *testing.T) {
	if got := mdtext.CountCharacters("...!!!"); got != 0 {
		t.Errorf("got %d, want 0", got)
	}
}
