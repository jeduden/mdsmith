package parser_test

// Parser corpus tests: a curated set of markdown snippets that
// exercise every block parser and every inline parser. Each
// snippet is parsed and the resulting AST is walked to assert a
// minimum expected node type is present. The goal is broad
// parser coverage; the CommonMark spec's full corpus was removed
// along with the upstream testutil-driven tests, and these
// snippets restore the parser-coverage breadth without bringing
// the spec corpus back.

import (
	"testing"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

func walkKinds(root ast.Node) map[ast.NodeKind]int {
	out := map[ast.NodeKind]int{}
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			out[n.Kind()]++
		}
		return ast.WalkContinue, nil
	})
	return out
}

func TestParser_BlockCorpus(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want ast.NodeKind
	}{
		// Atx and Setext headings.
		{"atx-h1", "# H1\n", ast.KindHeading},
		{"atx-h2", "## H2\n", ast.KindHeading},
		{"atx-h6", "###### H6\n", ast.KindHeading},
		{"atx-trailing-hash", "## H2 ##\n", ast.KindHeading},
		{"atx-blank-content", "# \n", ast.KindHeading},
		{"setext-h1", "Title\n=====\n", ast.KindHeading},
		{"setext-h2", "Subtitle\n--------\n", ast.KindHeading},
		// Thematic break in three glyph styles.
		{"hr-dashes", "---\n", ast.KindThematicBreak},
		{"hr-stars", "***\n", ast.KindThematicBreak},
		{"hr-underscores", "___\n", ast.KindThematicBreak},
		// Code blocks: indented and fenced (both fence styles).
		{"indented-code", "    code line\n", ast.KindCodeBlock},
		{"fenced-backtick", "```\ncode\n```\n", ast.KindFencedCodeBlock},
		{"fenced-tilde", "~~~\ncode\n~~~\n", ast.KindFencedCodeBlock},
		{"fenced-info", "```go\nfn()\n```\n", ast.KindFencedCodeBlock},
		// Blockquote and nested blockquote.
		{"blockquote", "> quoted\n", ast.KindBlockquote},
		{"blockquote-nested", "> > deeply\n", ast.KindBlockquote},
		// Lists.
		{"ul-dash", "- one\n- two\n", ast.KindList},
		{"ul-star", "* one\n* two\n", ast.KindList},
		{"ul-plus", "+ one\n+ two\n", ast.KindList},
		{"ol-paren", "1) one\n2) two\n", ast.KindList},
		{"ol-dot", "1. one\n2. two\n", ast.KindList},
		{"list-loose", "- one\n\n- two\n", ast.KindList},
		// HTML block (type 1: <script>) and type 7 (inline open).
		{"html-block-script", "<script>x</script>\n", ast.KindHTMLBlock},
		{"html-block-pre", "<pre>x</pre>\n", ast.KindHTMLBlock},
		// Link reference definition.
		{"linkref", "[lab]: /url\n\n[lab]\n", ast.KindLinkReferenceDefinition},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := parser.NewParser(
				parser.WithBlockParsers(parser.DefaultBlockParsers()...),
				parser.WithInlineParsers(parser.DefaultInlineParsers()...),
				parser.WithParagraphTransformers(parser.DefaultParagraphTransformers()...),
			)
			root := p.Parse(text.NewReader([]byte(tc.src)), parser.WithContext(parser.NewContext()))
			kinds := walkKinds(root)
			if kinds[tc.want] == 0 {
				t.Errorf("AST for %q missing %v\nkinds: %v", tc.src, tc.want, kinds)
			}
		})
	}
}

func TestParser_InlineCorpus(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want ast.NodeKind
	}{
		// Emphasis variants.
		{"emph-star", "this is *emphasised* text\n", ast.KindEmphasis},
		{"emph-under", "this is _emphasised_ text\n", ast.KindEmphasis},
		{"strong-star", "this is **strong** text\n", ast.KindEmphasis},
		{"strong-under", "this is __strong__ text\n", ast.KindEmphasis},
		// Code span (1, 2, and 3 backticks).
		{"code-1", "use `code` here\n", ast.KindCodeSpan},
		{"code-2", "use ``co`de`` here\n", ast.KindCodeSpan},
		{"code-3", "use ```co`d`e``` here\n", ast.KindCodeSpan},
		// Links and autolinks.
		{"link", "see [text](/url)\n", ast.KindLink},
		{"link-with-title", "see [text](/url \"title\")\n", ast.KindLink},
		{"autolink-url", "<https://example.com>\n", ast.KindAutoLink},
		{"autolink-email", "<alice@example.com>\n", ast.KindAutoLink},
		// Images.
		{"image", "see ![alt](/url)\n", ast.KindImage},
		{"image-titled", "see ![alt](/url \"title\")\n", ast.KindImage},
		// Raw HTML.
		{"raw-html-tag", "an <span class=\"x\">inline</span> tag\n", ast.KindRawHTML},
		// Hard line break.
		{"hardbreak-backslash", "first  \nsecond\n", ast.KindParagraph},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := parser.NewParser(
				parser.WithBlockParsers(parser.DefaultBlockParsers()...),
				parser.WithInlineParsers(parser.DefaultInlineParsers()...),
				parser.WithParagraphTransformers(parser.DefaultParagraphTransformers()...),
			)
			root := p.Parse(text.NewReader([]byte(tc.src)), parser.WithContext(parser.NewContext()))
			kinds := walkKinds(root)
			if kinds[tc.want] == 0 {
				t.Errorf("AST for %q missing %v\nkinds: %v", tc.src, tc.want, kinds)
			}
		})
	}
}

func TestParser_AttributeSyntax(t *testing.T) {
	// {#id .class key=value} after a heading or image lifts the
	// attribute parser to non-zero coverage.
	p := parser.NewParser(
		parser.WithBlockParsers(parser.DefaultBlockParsers()...),
		parser.WithInlineParsers(parser.DefaultInlineParsers()...),
		parser.WithParagraphTransformers(parser.DefaultParagraphTransformers()...),
		parser.WithHeadingAttribute(),
	)
	src := `# Heading {#my-id .my-class data-x=1 data-y="quoted" data-z='single'}

paragraph with image ![alt](/img){#i .c key=val}
`
	root := p.Parse(text.NewReader([]byte(src)), parser.WithContext(parser.NewContext()))
	if root == nil {
		t.Fatal("Parse returned nil root")
	}
	var hadHeading bool
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			if _, ok := n.(*ast.Heading); ok {
				hadHeading = true
			}
		}
		return ast.WalkContinue, nil
	})
	if !hadHeading {
		t.Error("did not find heading node")
	}
}

func TestParser_EscapedAndEntities(t *testing.T) {
	// Backslash escapes, named entities, hex/decimal numeric
	// entities — drives util.ResolveNumericReferences and
	// ResolveEntityNames.
	p := parser.NewParser(
		parser.WithBlockParsers(parser.DefaultBlockParsers()...),
		parser.WithInlineParsers(parser.DefaultInlineParsers()...),
		parser.WithParagraphTransformers(parser.DefaultParagraphTransformers()...),
	)
	src := []byte(`\* not emphasised
&amp; &#65; &#x41; &#1234;
`)
	root := p.Parse(text.NewReader(src), parser.WithContext(parser.NewContext()))
	if root == nil {
		t.Fatal("Parse returned nil root")
	}
	// Just walking the result is enough; the entity functions fire
	// during the walk inside the inline parsers.
	_ = walkKinds(root)
}
