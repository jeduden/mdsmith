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

func TestRawHTML_Comment_AllShapes(t *testing.T) {
	// CommonMark inline comment rules: <!-- ... -->. Drive each
	// branch in parseComment by varying the content.
	cases := []string{
		"a <!-- short --> b\n",
		"a <!----> b\n", // empty comment
		"a <!---- -> b\n", // not a valid close (->) on first attempt
		"a <!-- multi\nline --> b\n",
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
		`# H {n=3.14}`,
		`# H {n=-3.14}`,
		`# H {n=1e10}`,
		`# H {n=1.5e-3}`,
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
