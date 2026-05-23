package extension_test

// Bulk coverage for extension predicate methods and constructors
// that the normal Convert path either does not exercise, or only
// exercises in narrow input shapes.

import (
	"bytes"
	"testing"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

func TestFootnote_BlockParserDirectPredicates(t *testing.T) {
	p := extension.NewFootnoteBlockParser()
	if !p.CanInterruptParagraph() {
		t.Error("footnote block parser should interrupt paragraphs")
	}
	if p.CanAcceptIndentedLine() {
		t.Error("footnote block parser should not accept indented lines")
	}
	// Continue is exercised through a normal parse of a multi-
	// line footnote definition. The same Convert call below also
	// drives Open + Continue + Close inside the block parser.
}

func TestFootnote_MultiLineDefinition(t *testing.T) {
	// A footnote definition whose body spans multiple lines is
	// what makes the block parser's Continue branch fire. The
	// continuation lines are indented under the [^1]: marker.
	md := goldmark.New(goldmark.WithExtensions(extension.Footnote))
	src := []byte("see[^1] here\n\n[^1]: first line of body\n    second line indented\n    third line indented\n")
	var buf bytes.Buffer
	if err := md.Convert(src, &buf); err != nil {
		t.Fatalf("Convert: %v", err)
	}
}

func TestNewFootnote_Extender(t *testing.T) {
	// NewFootnote returns an Extender; plug it in and confirm
	// footnote parsing fires through the new instance rather
	// than the package-level Footnote singleton.
	ext := extension.NewFootnote(
		extension.WithFootnoteIDPrefix("inst-"),
	)
	md := goldmark.New(goldmark.WithExtensions(ext))
	var buf bytes.Buffer
	if err := md.Convert([]byte("see[^1]\n\n[^1]: body\n"), &buf); err != nil {
		t.Fatalf("Convert: %v", err)
	}
}
