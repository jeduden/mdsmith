package goldmark_test

// Coverage for the top-level goldmark.Convert helper plus the
// Markdown setters and parser-option dispatchers.

import (
	"bytes"
	"strings"
	"testing"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/parser"
)

func TestConvert_TopLevel(t *testing.T) {
	// Convert() is the package-level convenience wrapper around
	// defaultMarkdown.Convert(). Drive a small markdown sample
	// through it.
	var buf bytes.Buffer
	if err := goldmark.Convert([]byte("# Title\n\nbody\n"), &buf); err != nil {
		t.Fatalf("Convert: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "<h1>Title</h1>") {
		t.Errorf("Convert output missing <h1>Title</h1>: %q", out)
	}
}

func TestNew_WithParserAndOptions(t *testing.T) {
	// WithParser swaps the parser entirely; WithParserOptions
	// passes parser-level options at New time.
	customParser := parser.NewParser(
		parser.WithBlockParsers(parser.DefaultBlockParsers()...),
		parser.WithInlineParsers(parser.DefaultInlineParsers()...),
	)
	md := goldmark.New(
		goldmark.WithParser(customParser),
		goldmark.WithParserOptions(parser.WithAttribute()),
	)
	var buf bytes.Buffer
	if err := md.Convert([]byte("# Title {#id}\n"), &buf); err != nil {
		t.Fatalf("Convert: %v", err)
	}
}

func TestMarkdown_SetParserAndSetRenderer(t *testing.T) {
	// SetParser replaces the underlying parser after construction;
	// SetRenderer does the same for the renderer.
	md := goldmark.New()
	originalParser := md.Parser()
	newParser := parser.NewParser(
		parser.WithBlockParsers(parser.DefaultBlockParsers()...),
		parser.WithInlineParsers(parser.DefaultInlineParsers()...),
		parser.WithParagraphTransformers(parser.DefaultParagraphTransformers()...),
	)
	md.SetParser(newParser)
	if md.Parser() == originalParser {
		t.Error("SetParser did not replace the underlying parser")
	}
	originalRenderer := md.Renderer()
	md.SetRenderer(goldmark.DefaultRenderer())
	if md.Renderer() == originalRenderer {
		t.Error("SetRenderer did not replace the underlying renderer")
	}
	// Convert must still work after both swaps.
	var buf bytes.Buffer
	if err := md.Convert([]byte("# X\n"), &buf); err != nil {
		t.Fatalf("Convert after swap: %v", err)
	}
}
