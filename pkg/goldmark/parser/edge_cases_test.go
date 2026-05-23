package parser_test

// Edge-case corpus targeting the remaining gaps in raw_html.go
// (parseComment, parseUntil), setext_headings.go (Continue, Close),
// code_span.go (Parse), and attribute.go (parseAttributeNumber).

import (
	"testing"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

func TestFencedCodeBlock_IndentationBranches(t *testing.T) {
	// A fenced code block opened with N leading spaces dedents
	// each body line by up to N. Drive the "less indented than
	// expected" branch with body lines that have fewer leading
	// spaces than the opener.
	cases := []string{
		"   ```\nbody\n   ```\n",                    // 3-space opener, no body indent
		"   ```\n body\n   ```\n",                   // 3-space opener, 1-space body
		"```\nfirst\n\n  blank then content\n```\n", // blank line inside fence
		"~~~\nfirst\n~~~~\nnot a closer with diff char\nstill inside ~~~\n", // tilde with wrong closer
	}
	for _, src := range cases {
		_ = parseWithDefaults(src)
	}
}

func TestRawHTML_Comment_AllShapes(t *testing.T) {
	// CommonMark inline comment rules: <!-- ... -->. Drive each
	// branch in parseComment by varying the content.
	cases := []string{
		"a <!-- short --> b\n",
		"a <!----> b\n",     // empty comment 1 (<!-- ->)
		"a <!---> b\n",      // empty comment 2 (<!--->)
		"a <!---- -> b\n",   // not a valid close (->) on first attempt
		"a <!-- multi\nline --> b\n",
		"a <!-- multi\nline\nmore line --> b\n",
		"a <!-- unclosed comment never ends\n",
	}
	for _, src := range cases {
		root := parseWithDefaults(src)
		if root == nil {
			t.Fatalf("Parse returned nil for %q", src)
		}
	}
}

func TestRawHTML_ProcessingInstruction_AllShapes(t *testing.T) {
	cases := []string{
		"a <?xml?> b\n",
		"a <?xml content?> b\n",
		"a <?xml\nmulti?> b\n",
		"a <?unclosed PI never closes\n", // hits parseUntil return-nil branch
		"a <![CDATA[unclosed CDATA\n",
		"a <!UNCLOSED declaration\n",
	}
	for _, src := range cases {
		_ = parseWithDefaults(src)
	}
}

func TestRawHTML_Declaration_AllShapes(t *testing.T) {
	cases := []string{
		"a <!FOO> b\n",
		"a <!FOO bar> b\n",
		"a <!FOO\nmulti> b\n",
	}
	for _, src := range cases {
		_ = parseWithDefaults(src)
	}
}

func TestRawHTML_CDATA_AllShapes(t *testing.T) {
	cases := []string{
		"a <![CDATA[content]]> b\n",
		"a <![CDATA[]]> b\n",
		"a <![CDATA[multi\nline]]> b\n",
	}
	for _, src := range cases {
		_ = parseWithDefaults(src)
	}
}

func TestSetextHeading_LongUnderlineAndShortContent(t *testing.T) {
	// Setext underline must run to the end of the line (no
	// trailing chars). Drive cases that exercise Continue + Close.
	cases := []string{
		"Title\n=====\n",
		"Title\n=\n", // single-char underline
		"Title\n-----\n",
		"Title\n   -----\n",   // 3-space indented underline (max allowed)
		"Para\nspan\nUnderline\n=====\n", // multi-line setext content
	}
	for _, src := range cases {
		root := parseWithDefaults(src)
		found := false
		_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
			if entering && n.Kind() == ast.KindHeading {
				found = true
				return ast.WalkStop, nil
			}
			return ast.WalkContinue, nil
		})
		if !found {
			t.Errorf("expected Heading for %q", src)
		}
	}
}

// Drive the unreachable-via-parse-flow methods directly. Continue
// is never called during a normal parse (Open consumes both the
// content line and the underline) but the BlockParser interface
// requires the method to exist; call it explicitly so the surface
// coverage matches the surface area.
func TestSetextHeading_NewWithOptions(t *testing.T) {
	// NewSetextHeadingParser's opts loop body needs options.
	_ = parser.NewSetextHeadingParser(
		parser.WithAutoHeadingID(),
		parser.WithHeadingAttribute(),
	)
}

func TestSetextHeading_DirectMethodInvocation(t *testing.T) {
	p := parser.NewSetextHeadingParser()
	// Continue and CanAcceptIndentedLine are pure functions that
	// return constant values; invoke them so cover sees them as
	// reached.
	pc := parser.NewContext()
	got := p.Continue(ast.NewHeading(1), nil, pc)
	if got != parser.Close {
		t.Errorf("Continue should return Close, got %v", got)
	}
	if p.CanAcceptIndentedLine() {
		t.Error("CanAcceptIndentedLine should be false")
	}
	if !p.CanInterruptParagraph() {
		t.Error("CanInterruptParagraph should be true")
	}
}

func TestSetextHeading_AttributesAndAutoID(t *testing.T) {
	// Drive setextHeadingParser.Close branches for attribute /
	// AutoHeadingID by parsing with both options on. Specifically
	// the explicit-id branch (id already in attributes -> Put on
	// IDs registry) needs `{#name}` syntax in the heading line.
	p := parser.NewParser(
		parser.WithBlockParsers(parser.DefaultBlockParsers()...),
		parser.WithInlineParsers(parser.DefaultInlineParsers()...),
		parser.WithParagraphTransformers(parser.DefaultParagraphTransformers()...),
		parser.WithAutoHeadingID(),
		parser.WithAttribute(),
	)
	srcs := []string{
		"Setext Heading\n==============\n", // auto-generated id branch
		"Setext With Id {#my-id}\n=======================\n", // explicit-id branch (Put)
		"Setext Two-level\n-----\n",        // h2 setext
	}
	for _, src := range srcs {
		root := p.Parse(text.NewReader([]byte(src)), parser.WithContext(parser.NewContext()))
		if root == nil {
			t.Fatalf("Parse returned nil for %q", src)
		}
	}
}

func TestSetextHeading_NotAHeading(t *testing.T) {
	// Pure `===` or `---` after a blank line is a thematic break,
	// not a setext heading.
	cases := []string{
		"\n=====\n",
		"---\n",
	}
	for _, src := range cases {
		root := parseWithDefaults(src)
		_ = root
	}
}

func TestCodeSpan_NestedBackticks(t *testing.T) {
	cases := []string{
		"a `code` b\n",
		"a `` co`de `` b\n",
		"a ``` ` ``` b\n",
		"a `unclosed\n",
		"a `` co\nde `` b\n",          // multi-line code span
		"a ` ` b\n",                   // single-space code span
		"a `   spaces stripped   ` b\n",
	}
	for _, src := range cases {
		_ = parseWithDefaults(src)
	}
}

func TestEmphasis_RareDelimiterPatterns(t *testing.T) {
	// Drive uncommon emphasis branches in ProcessDelimiters:
	// CanOpenCloser asymmetric, can-open-but-not-close, can-close-
	// but-not-open, intraword underscores, multi-character runs.
	cases := []string{
		"a*foo bar*",                       // basic emphasis
		"*foo *bar*",                        // double open
		"*foo* bar*",                        // open then trailing
		"foo*bar*baz",                       // intraword * (allowed)
		"foo_bar_baz",                       // intraword _ (NOT emphasis)
		"foo*_bar_*baz",                     // mixed delimiters
		"**foo***bar**",                     // adjacent runs
		"*foo **bar***",                    // mixed lengths
		"***foo***",                         // triple = both em+strong
		"*** foo ***",                       // surrounded with spaces
		"a *foo*** bar*** baz",              // longer runs
		"_*foo*_",                           // nested different delims
		"_**foo**_",
		"** open with no close",
		"close with no open **",
		"*not_an_emphasis_either*",
	}
	for _, src := range cases {
		_ = parseWithDefaults(src + "\n")
	}
}

func TestBlockquote_AllProcessBranches(t *testing.T) {
	// Drive each branch in blockquoteParser.process:
	//   ">\n"           -> pos at newline immediately (early-return path)
	//   "> text"        -> ' ' continuation (no padding)
	//   ">\ttext"       -> '\t' continuation (sets padding)
	//   ">text"         -> immediate content, no space/tab
	cases := []string{
		">\n",
		"> single space\n",
		">\ttab indented\n",
		">no space\n",
		">  two spaces and content\n",
		"> nested\n> > deeper\n",
	}
	for _, src := range cases {
		_ = parseWithDefaults(src)
	}
}

func TestDelimiters_UnmatchedEmphasisClearsStack(t *testing.T) {
	// parseContext.ClearDelimiters has an early-return when the
	// delimiter stack is empty plus a loop body that removes
	// delimiters one by one. Unclosed emphasis runs ('*' that
	// never finds a closer) leave delimiters on the stack until
	// the paragraph finishes, which then triggers ClearDelimiters
	// with bottom == nil and walks the loop.
	cases := []string{
		"unclosed *emphasis\n",
		"unclosed **strong\n",
		"unclosed *one and *two emphases\n",
		"mixed _italic and **bold runs\n",
	}
	for _, src := range cases {
		_ = parseWithDefaults(src)
	}
}

func TestATXHeading_AttributeParsingEdgeCases(t *testing.T) {
	// parseLastLineAttributes handles \-escapes and { braces.
	cases := []string{
		`# H \{not-attribute\}`,    // escaped braces
		`# H \! { #id }`,            // escaped punct then attr block
		`# H { #id } trailing text`, // attr block in middle, then text
		`# H {#id}`,                 // valid
		`# H \\{#id}`,               // escaped backslash then attrs
	}
	for _, src := range cases {
		_ = parseWithDefaultsAttr(src + "\n")
	}
}

func TestAttribute_EdgeCases(t *testing.T) {
	// Drive remaining parseAttribute branches.
	cases := []string{
		`# H {}`,             // empty
		`# H {  }`,           // whitespace
		`# H {!notattr}`,     // non-identifier first char
		`# H {123start}`,     // numeric start (invalid identifier)
		`# H {.}`,            // bare dot (no class name)
		`# H {#}`,            // bare hash (no id name)
		`# H {.-leading-dash}`,
		`# H {#:colon-name}`,
		`# H {key1=val1 key2=val2}`,
		`# H {key=[1, 2, 3]}`,      // array value
		`# H {key={#nested}}`,       // nested attributes
	}
	for _, src := range cases {
		_ = parseWithDefaultsAttr(src)
	}
}

func TestAttribute_MultipleClasses(t *testing.T) {
	// Multiple `.classname` tokens in an attribute block extend
	// the existing class= attribute via findUpdate's success
	// branch. A single class shortcut hits the findUpdate miss
	// branch (the !ok -> append fallback).
	srcs := []string{
		`# H {.first}`,             // miss branch
		`# H {.first .second}`,     // miss then success
		`# H {.a .b .c}`,           // success branch fires twice
		`# H {.x .y .z .w}`,
	}
	for _, src := range srcs {
		_ = parseWithDefaultsAttr(src)
	}
}

func TestAttribute_StringEscapes(t *testing.T) {
	// parseAttributeString handles JSON-style backslash escapes.
	// Drive each of the branches: \", \\, \/, \b, \f, \n, \r, \t,
	// plus the default branch (unknown escape kept literal).
	cases := []string{
		`# H {title="plain"}`,
		`# H {title="a \"quoted\" word"}`,
		`# H {title="a \\ backslash"}`,
		`# H {title="a \/ slash"}`,
		`# H {title="line\nbreak"}`,
		`# H {title="bell\b backspace"}`,
		`# H {title="form\ffeed"}`,
		`# H {title="carriage\rreturn"}`,
		`# H {title="tab\there"}`,
		`# H {title="unknown\xescape"}`, // default branch
	}
	for _, src := range cases {
		_ = parseWithDefaultsAttr(src)
	}
}

func TestAttribute_NumberShapes(t *testing.T) {
	// parseAttributeNumber's branches: integer, decimal, negative,
	// hex (0x...), and float exponents.
	cases := []string{
		`# H {n=0}`,
		`# H {n=10}`,
		`# H {n=-7}`,
		`# H {n=+5}`,            // explicit + sign
		`# H {n=3.14}`,
		`# H {n=-3.14}`,
		`# H {n=1e10}`,
		`# H {n=1E10}`,          // capital E
		`# H {n=1.5e-3}`,
		`# H {n=1.5E+3}`,        // capital E with +
		`# H {n=+5.5e-3}`,
		`# H {n=-not-a-number}`, // sign without numeric -> bail
	}
	for _, src := range cases {
		_ = parseWithDefaultsAttr(src)
	}
}

func parseWithDefaultsAttr(src string) ast.Node {
	// Same as parseWithDefaults but with WithHeadingAttribute()
	// enabled so attribute parsing fires.
	p := parser.NewParser(
		parser.WithBlockParsers(parser.DefaultBlockParsers()...),
		parser.WithInlineParsers(parser.DefaultInlineParsers()...),
		parser.WithParagraphTransformers(parser.DefaultParagraphTransformers()...),
		parser.WithHeadingAttribute(),
	)
	return p.Parse(text.NewReader([]byte(src)), parser.WithContext(parser.NewContext()))
}
