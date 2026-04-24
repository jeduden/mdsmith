package mdtext

import (
	"fmt"
	"strings"
	"sync"
	"unicode"

	"github.com/neurosnap/sentences/english"
	"github.com/yuin/goldmark/ast"

	sentlib "github.com/neurosnap/sentences"
)

// Slugify converts heading text to a GitHub-compatible URL anchor slug.
// Lowercase, letters/digits preserved, spaces and hyphens become a single dash.
func Slugify(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return ""
	}
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			prevDash = false
		case unicode.IsSpace(r) || r == '-' || r == '_':
			if b.Len() > 0 && !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// TOCItem represents a single heading entry for table-of-contents generation.
type TOCItem struct {
	Level  int
	Text   string
	Anchor string
}

// CollectTOCItems returns all headings from the AST as TOC items, in document
// order. Anchors are disambiguated by insertion order: first occurrence keeps
// the plain slug, subsequent duplicates get -1, -2, … suffixes — matching the
// anchor computation in crossfilereferenceintegrity. Tracks used anchors (not
// just base slugs) to guarantee unique anchors even when a later heading's
// base slug matches an earlier heading's disambiguated anchor.
func CollectTOCItems(root ast.Node, source []byte) []TOCItem {
	var items []TOCItem
	usedAnchors := make(map[string]bool)
	slugCounts := make(map[string]int)
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		h, ok := n.(*ast.Heading)
		if !ok {
			return ast.WalkContinue, nil
		}
		text := ExtractPlainText(h, source)
		slug := Slugify(text)
		if slug == "" {
			return ast.WalkContinue, nil
		}

		// Find a unique anchor by incrementing suffix until unused.
		anchor := slug
		if usedAnchors[anchor] {
			count := slugCounts[slug]
			for {
				count++
				anchor = fmt.Sprintf("%s-%d", slug, count)
				if !usedAnchors[anchor] {
					break
				}
			}
			slugCounts[slug] = count
		}

		usedAnchors[anchor] = true
		items = append(items, TOCItem{Level: h.Level, Text: text, Anchor: anchor})
		return ast.WalkContinue, nil
	})
	return items
}

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
