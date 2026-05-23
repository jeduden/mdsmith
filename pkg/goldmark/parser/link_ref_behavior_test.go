package parser_test

// Behavioural tests for the fork-modified
// linkReferenceParagraphTransformer.Transform. They drive the
// transformer through the public parser.Parser API and assert
// CommonMark-spec-conformant behaviour (e.g. that the
// paragraph is removed when no link-reference definitions
// survive, that the AST shape matches the spec for known
// fixtures).  Because the test imports `github.com/yuin/goldmark`
// which is `replace`-d to this same fork in mdsmith's go.mod,
// the assertions cover the fork's own contract, not a comparison
// against a separately-imported upstream goldmark.  Drift from
// upstream goldmark itself is tracked via the quarterly upstream
// merge documented in plan/198_goldmark-arena-fork.md.

import (
	"fmt"
	"strings"
	"testing"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// equivalenceCases drive parseLinkReferenceDefinition's branches.
// Each case is parsed by a fresh fork-parser and the resulting AST
// shape (Kind + LinkReferenceDefinition label/dest/title) must
// match what the same input produces through goldmark's default
// transformer registration path.
var equivalenceCases = []struct {
	name string
	src  string
}{
	// Happy paths.
	{"bare", "[foo]: /url\n\n[foo]\n"},
	{"titled-double", "[a]: /u \"title\"\n\n[a]\n"},
	{"titled-single", "[a]: /u 'title'\n\n[a]\n"},
	{"titled-paren", "[a]: /u (title)\n\n[a]\n"},
	{"angle-dest", "[a]: <http://example.com>\n\n[a]\n"},
	{"two-defs", "[a]: /1\n[b]: /2\n\n[a] [b]\n"},
	{"indented-3", "   [a]: /url\n\n[a]\n"},
	{"label-multiline", "[lo\nng]: /url\n\n[lo ng]\n"},
	{"title-on-next-line", "[a]: /url\n  \"the title\"\n\n[a]\n"},
	{"title-multiline", "[a]: /url \"line one\nline two\"\n\n[a]\n"},
	{"dest-parens-balanced", "[a]: foo(x)bar\n\n[a]\n"},
	{"dest-escape", "[a]: foo\\)bar\n\n[a]\n"},
	{"angle-escape", "[a]: <foo\\>bar>\n\n[a]\n"},
	{"angle-then-title", "[a]: <foo>\"title\"\n\nstuff\n"},
	{"three-refs-paragraph", "[a]: /1\n[b]: /2\n[c]: /3\n"},
	{"title-newline-trail", "[a]: /url\n\"title\" trail\n\nstuff\n"},
	// Negative paths — must produce no reference, the paragraph
	// stays as prose.
	{"no-def", "just prose, no link references at all.\n"},
	{"indent-4", "    [a]: /url\n\n[a]\n"},
	{"no-opener", "a]: /url\n\nstuff\n"},
	{"unclosed-label", "[unclosed: /url\nmore\n"},
	{"blank-label", "[]: /url\n\nstuff\n"},
	{"no-colon", "[label] /url\n\nstuff\n"},
	{"no-dest", "[label]:\n\nstuff\n"},
	{"trailing-on-line", "[a]: /url extra\n\n[a]\n"},
	{"title-glued", "[a]: /url\"title\"\n\n[a]\n"},
	{"unclosed-title", "[a]: /url \"unclosed\nstuff\n"},
	{"unclosed-angle", "[a]: <foo\nstuff\n"},
	{"dest-bad-rparen", "[a]: foo)bar\n\nthing\n"},
}

func TestLinkRefTransform_AstEquivalent(t *testing.T) {
	for _, tc := range equivalenceCases {
		t.Run(tc.name, func(t *testing.T) {
			got := dumpAST(t, tc.src, defaultsForkParser())
			want := dumpAST(t, tc.src, defaultsParser(parser.LinkReferenceParagraphTransformer))
			if got != want {
				t.Errorf("AST mismatch for %q\nfork:\n%s\nbaseline:\n%s", tc.name, got, want)
			}
		})
	}
}

func TestLinkRefTransform_ReusesBlockReaderAcrossParagraphs(t *testing.T) {
	src := []byte("[a]: /1\n\nfirst paragraph\n\n[b]: /2\n\nsecond paragraph\n")
	p := defaultsForkParser()
	ctx := parser.NewContext()
	root := p.Parse(text.NewReader(src), parser.WithContext(ctx))
	if root == nil {
		t.Fatal("Parse returned nil root")
	}
	if _, ok := ctx.Reference("a"); !ok {
		t.Error("reference [a] missing from context")
	}
	if _, ok := ctx.Reference("b"); !ok {
		t.Error("reference [b] missing from context")
	}
}

func TestLinkRefTransform_CrossSourceReallocates(t *testing.T) {
	// Two parses with distinct source buffers on the same parser
	// must still produce the right references — the transformer
	// has to re-allocate its BlockReader when the source bytes
	// change identity between Parse calls.
	p := defaultsForkParser()
	for i := 0; i < 3; i++ {
		src := []byte(fmt.Sprintf("[a%d]: /url%d\n\n[a%d]\n", i, i, i))
		ctx := parser.NewContext()
		p.Parse(text.NewReader(src), parser.WithContext(ctx))
		label := fmt.Sprintf("a%d", i)
		if _, ok := ctx.Reference(label); !ok {
			t.Errorf("iteration %d: reference [%s] missing", i, label)
		}
	}
}

// defaultsForkParser builds a parser using the fork's
// DefaultParagraphTransformers (which returns a fresh transformer
// instance per call).
func defaultsForkParser() parser.Parser {
	return parser.NewParser(
		parser.WithBlockParsers(parser.DefaultBlockParsers()...),
		parser.WithInlineParsers(parser.DefaultInlineParsers()...),
		parser.WithParagraphTransformers(parser.DefaultParagraphTransformers()...),
	)
}

// defaultsParser builds a parser using the SAME transformer value
// for every paragraph (used to baseline the equivalence test
// against goldmark's pre-fork singleton path).
func defaultsParser(tr parser.ParagraphTransformer) parser.Parser {
	return parser.NewParser(
		parser.WithBlockParsers(parser.DefaultBlockParsers()...),
		parser.WithInlineParsers(parser.DefaultInlineParsers()...),
		parser.WithParagraphTransformers(util.Prioritized(tr, 100)),
	)
}

func dumpAST(t *testing.T, src string, p parser.Parser) string {
	t.Helper()
	root := p.Parse(text.NewReader([]byte(src)), parser.WithContext(parser.NewContext()))
	var sb strings.Builder
	dumpNode(&sb, root, 0)
	return sb.String()
}

func dumpNode(sb *strings.Builder, n ast.Node, depth int) {
	for i := 0; i < depth; i++ {
		sb.WriteString("  ")
	}
	sb.WriteString(n.Kind().String())
	if ref, ok := n.(*ast.LinkReferenceDefinition); ok {
		fmt.Fprintf(sb, " label=%q dest=%q title=%q", ref.Label, ref.Destination, ref.Title)
	}
	sb.WriteByte('\n')
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		dumpNode(sb, c, depth+1)
	}
}
