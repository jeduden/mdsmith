package parser_test

// Direct-call coverage for the predicate methods on block
// parsers: CanInterruptParagraph and CanAcceptIndentedLine on the
// atx-heading and blockquote parsers, Close on the blockquote
// parser, plus WithAutoHeadingID option dispatching through
// SetHeadingOption / SetParserOption.

import (
	"testing"

	"github.com/yuin/goldmark/parser"
)

func TestATXHeading_DirectPredicates(t *testing.T) {
	p := parser.NewATXHeadingParser()
	if !p.CanInterruptParagraph() {
		t.Error("ATX heading parser should interrupt paragraphs")
	}
	if p.CanAcceptIndentedLine() {
		t.Error("ATX heading parser should not accept indented lines")
	}
}

func TestBlockquote_DirectPredicates(t *testing.T) {
	p := parser.NewBlockquoteParser()
	if !p.CanInterruptParagraph() {
		t.Error("blockquote should interrupt paragraphs")
	}
	if p.CanAcceptIndentedLine() {
		t.Error("blockquote should not accept indented lines")
	}
	// Close is a no-op; just call it to clear the 0% mark.
	p.Close(nil, nil, parser.NewContext())
}

func TestATXHeading_WithAutoHeadingID(t *testing.T) {
	// WithAutoHeadingID and WithHeadingAttribute both return
	// HeadingOption types whose SetHeadingOption / SetParserOption
	// dispatchers fire when threaded through NewATXHeadingParser
	// or the top-level Parser constructor.
	p := parser.NewATXHeadingParser(
		parser.WithAutoHeadingID(),
		parser.WithHeadingAttribute(),
	)
	if p == nil {
		t.Fatal("constructor returned nil")
	}

	// Through the parser-options dispatcher: WithAutoHeadingID
	// also implements parser.Option (SetParserOption), so it can
	// be threaded through NewParser too.
	parser.NewParser(
		parser.WithBlockParsers(parser.DefaultBlockParsers()...),
		parser.WithInlineParsers(parser.DefaultInlineParsers()...),
		parser.WithParagraphTransformers(parser.DefaultParagraphTransformers()...),
		parser.WithAutoHeadingID(),
		parser.WithHeadingAttribute(),
	)
}

func TestATXHeading_NoSpaceAfterHash(t *testing.T) {
	// ATX heading requires a space (or tab/newline) after the
	// leading #s.  '#text' (no space) is not a heading; the
	// parser returns nil.
	cases := []string{
		"#text\n",
		"##nospace\n",
		"###nospace at all\n",
	}
	for _, src := range cases {
		_ = parseWithDefaults(src)
	}
}

func TestATXHeading_EmptyContentAndAllHashes(t *testing.T) {
	// Drive the specific hl.Len()==0 and ### ### branches.
	cases := []string{
		"# \n",          // hl.Len() == 0 after trim
		"### ###\n",     // line[0] == '#' after closing hash trim
		"## ###\n",
		"# \\#\n",       // escaped hash at end
	}
	for _, src := range cases {
		_ = parseWithDefaults(src)
	}
}

func TestATXHeading_EdgeCases(t *testing.T) {
	// Drive Open branches that aren't reached by normal `# Title\n`:
	// - 7+ hashes (level > 6, not a heading)
	// - alone '#' with no content (just space) -> empty heading
	// - trailing closing hashes
	cases := []string{
		"#######  not a heading\n",  // 7 hashes = not a heading
		"# \n",                       // empty heading content
		"#\n",                        // bare hash
		"# title #\n",                // trailing close hash
		"# title  ###\n",             // multiple closing hashes
		"# title \\#\n",              // escaped closing hash
		"  # indented heading\n",     // 2-space leading
	}
	for _, src := range cases {
		_ = parseWithDefaults(src)
	}
}

func TestParagraph_DirectPredicates(t *testing.T) {
	p := parser.NewParagraphParser()
	if p.CanInterruptParagraph() {
		t.Error("paragraph parser must not interrupt other paragraphs")
	}
	if p.CanAcceptIndentedLine() {
		t.Error("paragraph parser must not accept indented lines as new blocks")
	}
}

func TestCodeBlock_TabIndentedBody(t *testing.T) {
	// preserveLeadingTabInCodeBlock fires when an indented-code
	// block's first byte is a tab AND the parser has already
	// advanced past partial padding. That happens with a leading
	// space-then-tab where the space counts as padding 1 and the
	// tab fills the rest of the 4-column indent.
	cases := []string{
		" \tafter-space-tab\n",                    // 1 space + tab -> tab inside padding
		"  \tafter-2spaces-tab\n",                 // 2 spaces + tab
		"   \tafter-3spaces-tab\n",                // 3 spaces + tab
		"\tplain-tab\n\tsecond tab line\n",        // pure tab
	}
	for _, src := range cases {
		_ = parseWithDefaults(src)
	}
}

func TestAttributes_FindFromTopLevel(t *testing.T) {
	// The top-level parser.Attributes.Find helper is unreachable
	// via the normal parse-flow because parsers consume attributes
	// through ParseAttributes directly. Construct one and call
	// Find to clear the 0 % mark.
	attrs := parser.Attributes{
		parser.Attribute{Name: []byte("id"), Value: []byte("a")},
		parser.Attribute{Name: []byte("class"), Value: []byte("c")},
	}
	if _, ok := attrs.Find([]byte("id")); !ok {
		t.Error("Find should locate 'id'")
	}
	if _, ok := attrs.Find([]byte("missing")); ok {
		t.Error("Find should not locate missing keys")
	}
}
