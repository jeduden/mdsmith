package mdtext

import (
	"strings"
	"sync"
	"unicode"

	"github.com/neurosnap/sentences/english"
	"github.com/yuin/goldmark/ast"

	sentlib "github.com/neurosnap/sentences"
)

// ExtractPlainText extracts readable text from a goldmark AST node,
// stripping markdown syntax. Keeps: text content, link display text,
// emphasis inner text, image alt text, code span text.
func ExtractPlainText(node ast.Node, source []byte) string {
	var buf strings.Builder
	extractText(&buf, node, source)
	return buf.String()
}

func extractText(buf *strings.Builder, node ast.Node, source []byte) {
	// For text nodes, write the content.
	if t, ok := node.(*ast.Text); ok {
		buf.Write(t.Segment.Value(source))
		if t.SoftLineBreak() || t.HardLineBreak() {
			buf.WriteByte(' ')
		}
		return
	}

	// For code spans, extract the text content from children.
	if _, ok := node.(*ast.CodeSpan); ok {
		for c := node.FirstChild(); c != nil; c = c.NextSibling() {
			if t, ok := c.(*ast.Text); ok {
				buf.Write(t.Segment.Value(source))
			}
		}
		return
	}

	// For images, use alt text from children.
	if _, ok := node.(*ast.Image); ok {
		for c := node.FirstChild(); c != nil; c = c.NextSibling() {
			extractText(buf, c, source)
		}
		return
	}

	// For links, emphasis, strong, and other nodes: recurse into children.
	for c := node.FirstChild(); c != nil; c = c.NextSibling() {
		extractText(buf, c, source)
	}
}

// CountWords counts words in text by splitting on whitespace.
func CountWords(text string) int {
	return len(strings.Fields(text))
}

// CountSentences counts sentences by splitting on sentence-ending
// punctuation (., !, ?) followed by whitespace or end of text.
// Returns at least 1 for non-empty text.
func CountSentences(text string) int {
	if strings.TrimSpace(text) == "" {
		return 0
	}
	count := 0
	runes := []rune(text)
	for i, r := range runes {
		if r == '.' || r == '!' || r == '?' {
			if i == len(runes)-1 {
				count++
			} else if unicode.IsSpace(runes[i+1]) {
				count++
			}
		}
	}
	if count == 0 {
		return 1
	}
	return count
}

var (
	tokenizer *sentlib.DefaultSentenceTokenizer
	initOnce  sync.Once
)

func initTokenizer() {
	t, _ := english.NewSentenceTokenizer(nil)
	tokenizer = t
}

// SplitSentences splits text into individual sentences using a
// Punkt sentence tokenizer. Handles abbreviations, decimals,
// and ellipses.
func SplitSentences(text string) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	initOnce.Do(initTokenizer)
	sents := tokenizer.Tokenize(text)
	result := make([]string, 0, len(sents))
	for _, s := range sents {
		t := strings.TrimSpace(s.Text)
		if t != "" {
			result = append(result, t)
		}
	}
	return result
}

// CountCharacters counts letters and digits in text
// (no spaces or punctuation).
func CountCharacters(text string) int {
	count := 0
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			count++
		}
	}
	return count
}
