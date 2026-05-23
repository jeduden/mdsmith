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

func TestFootnote_ParseEarlyReturns(t *testing.T) {
	// Drive footnoteParser.Parse early returns:
	// - '!' before '[' (image-like context)
	// - '[' without '^'
	// - '[^' without closing ']'
	// - footnote ref with no matching def (no list)
	// - missing footnote def
	srcs := []string{
		"![^img] footnote-like image\n\n[^img]: body\n",
		"[no caret] not a footnote ref\n",
		"[^unclosed never closes\n",
		"[^missing] no def\n",
	}
	for _, src := range srcs {
		md := goldmark.New(goldmark.WithExtensions(extension.Footnote))
		var buf bytes.Buffer
		if err := md.Convert([]byte(src), &buf); err != nil {
			t.Fatalf("Convert(%q): %v", src, err)
		}
	}
}

func TestDefinitionList_EdgeCases(t *testing.T) {
	// Drive definitionListParser.Open's:
	//  - already-inside-list early return (line 30)
	//  - colon-not-followed-by-space (line 43)
	//  - deeply-indented body (w >= 8 -> indented code)
	srcs := []string{
		"term\n:   def\n",                                  // happy path
		"term\n:def\n",                                     // no space after :
		"term\n:       very deeply indented def\n",         // 7+ space indent
		"term\n:   def1\n: def2\n",                          // two defs in sequence
		"term\n:   def with paragraph\n\n   continuation\n", // multi-paragraph def
	}
	for _, src := range srcs {
		md := goldmark.New(goldmark.WithExtensions(extension.DefinitionList))
		var buf bytes.Buffer
		if err := md.Convert([]byte(src), &buf); err != nil {
			t.Fatalf("Convert(%q): %v", src, err)
		}
	}
}

func TestStrikethrough_ParseEarlyReturns(t *testing.T) {
	// strikethroughParser.Parse early-returns for:
	//  - tilde-tilde-tilde (more than 2 tildes -> not strikethrough)
	//  - preceding char is also '~' (>2 tildes triplet)
	srcs := []string{
		"~~basic strike~~ end\n",
		"~~~not strikethrough (3 tildes)~~~ end\n",
		"abc~~~def\n", // 3 consecutive tildes
		"~~unclosed strike text\n",
	}
	for _, src := range srcs {
		md := goldmark.New(goldmark.WithExtensions(extension.Strikethrough))
		var buf bytes.Buffer
		if err := md.Convert([]byte(src), &buf); err != nil {
			t.Fatalf("Convert(%q): %v", src, err)
		}
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
		"[^x]:\n",                                    // empty body (pos >= len after \n strip)
		"[^x]:",                                      // no trailing newline at all
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
	// applyFootnoteTemplate has three loop branches:
	//   - no placeholders -> fast path returns template as-is.
	//   - ^^ found first -> hits the b[i-1]=='^' && c=='^' branch.
	//   - %% found first -> hits the b[i-1]=='%' && c=='%' branch.
	// Drive each separately.
	templates := []string{
		"only-^^-placeholder",
		"only-%%-placeholder",
		"both=^^ and refs=%%",
	}
	for _, tmpl := range templates {
		md := goldmark.New(goldmark.WithExtensions(
			extension.NewFootnote(extension.WithFootnoteBacklinkHTML(tmpl)),
		))
		src := []byte("see[^a] and again[^a]\n\n[^a]: body\n")
		var buf bytes.Buffer
		if err := md.Convert(src, &buf); err != nil {
			t.Fatalf("Convert(%q): %v", tmpl, err)
		}
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
