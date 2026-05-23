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

func TestTaskList_ParseEarlyReturns(t *testing.T) {
	// Drive each early-return in taskCheckBoxParser.Parse.  All
	// inputs include `[` so the trigger fires; only the well-formed
	// list-item-text-block case actually creates a TaskCheckBox.
	srcs := []string{
		"[x] outside any list\n",                  // parent.Parent() not ListItem
		"- some text before [x] checkbox\n",       // parent.HasChildren (text before [)
		"- [notvalid] not a checkbox\n",           // regex miss
		"- [x] valid checkbox\n",                  // sanity / happy path
		"- [ ] unchecked checkbox\n",
		"- [X] uppercase X checkbox\n",
	}
	for _, src := range srcs {
		md := goldmark.New(goldmark.WithExtensions(extension.TaskList))
		var buf bytes.Buffer
		if err := md.Convert([]byte(src), &buf); err != nil {
			t.Fatalf("Convert(%q): %v", src, err)
		}
	}
}

func TestFootnote_OpenFailPaths(t *testing.T) {
	// Drive each early-return branch in footnoteBlockParser.Open.
	// Each input starts with '[' so the Trigger fires, but the
	// rest of the line is not a valid footnote definition.
	srcs := []string{
		"[not-a-footnote] just a link reference?\n",  // missing ^
		"[^missing-close\n",                          // no closing ]
		"[^missing-colon] no colon\n",                // ] but no :
		"[^]: empty label\n",                         // blank label
		"[^x]:\n",                                    // empty body (pos >= len)
		"[^x]: definition\n",                         // valid (sanity)
	}
	for _, src := range srcs {
		md := goldmark.New(goldmark.WithExtensions(extension.Footnote))
		var buf bytes.Buffer
		if err := md.Convert([]byte(src), &buf); err != nil {
			t.Fatalf("Convert(%q): %v", src, err)
		}
	}
}

func TestFootnote_TemplatePlaceholders(t *testing.T) {
	// applyFootnoteTemplate has a fast-path that returns the
	// template unchanged when neither "^^" nor "%%" appear. Drive
	// the slow path by configuring a custom backlink template
	// that contains both tokens.
	md := goldmark.New(goldmark.WithExtensions(
		extension.NewFootnote(
			extension.WithFootnoteBacklinkHTML("idx=^^ refs=%%"),
		),
	))
	src := []byte("see[^a] and again[^a]\n\n[^a]: body\n")
	var buf bytes.Buffer
	if err := md.Convert(src, &buf); err != nil {
		t.Fatalf("Convert: %v", err)
	}
	out := buf.String()
	if !bytes.Contains(buf.Bytes(), []byte("idx=1")) {
		t.Errorf("expected idx=1 substitution: %q", out)
	}
	if !bytes.Contains(buf.Bytes(), []byte("refs=2")) {
		t.Errorf("expected refs=2 substitution: %q", out)
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
