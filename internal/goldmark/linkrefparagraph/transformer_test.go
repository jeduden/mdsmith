package linkrefparagraph_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"

	"github.com/jeduden/mdsmith/internal/goldmark/linkrefparagraph"
)

// equivalenceCases exercise the link-reference parser branches the
// fork inherits from upstream — bare, titled (three quote variants),
// bracket-wrapped destination, indented-too-far, multiple defs per
// paragraph, and a no-definition control case. The fork's AST must
// match upstream byte-for-byte on each.
var equivalenceCases = []struct {
	name string
	src  string
}{
	{"bare", "[foo]: /url\n\n[foo]\n"},
	{"titled-double", "[a]: /u \"title\"\n\n[a]\n"},
	{"titled-single", "[a]: /u 'title'\n\n[a]\n"},
	{"titled-paren", "[a]: /u (title)\n\n[a]\n"},
	{"angle-dest", "[a]: <http://example.com>\n\n[a]\n"},
	{"two-defs", "[a]: /1\n[b]: /2\n\n[a] [b]\n"},
	{"indented-3", "   [a]: /url\n\n[a]\n"},
	{"no-def", "just prose, no link references at all.\n"},
	{"def-then-text", "[a]: /url\nlonger paragraph below\n\n[a]\n"},
}

func TestTransformer_EquivalentToUpstream(t *testing.T) {
	for _, tc := range equivalenceCases {
		t.Run(tc.name, func(t *testing.T) {
			gotFork := parseDump(t, tc.src, newForkParser())
			gotUp := parseDump(t, tc.src, newUpstreamParser())
			if gotFork != gotUp {
				t.Errorf("AST mismatch for %q\nfork:\n%s\nupstream:\n%s", tc.name, gotFork, gotUp)
			}
		})
	}
}

func TestTransformer_ReusesBlockReaderAcrossParagraphs(t *testing.T) {
	src := []byte("[a]: /1\n\nfirst paragraph\n\n[b]: /2\n\nsecond paragraph\n")
	tr := linkrefparagraph.New()
	p := newParserWith(tr)
	ctx := parser.NewContext()
	root := p.Parse(text.NewReader(src), parser.WithContext(ctx))
	if root == nil {
		t.Fatal("Parse returned nil root")
	}
	if _, ok := ctx.Reference("a"); !ok {
		t.Errorf("reference [a] missing from context")
	}
	if _, ok := ctx.Reference("b"); !ok {
		t.Errorf("reference [b] missing from context")
	}
}

func TestTransformer_Reset(t *testing.T) {
	tr := linkrefparagraph.New()
	p := newParserWith(tr)
	p.Parse(text.NewReader([]byte("[a]: /url\n\n[a]\n")), parser.WithContext(parser.NewContext()))
	tr.Reset()
	// After Reset, parsing a brand-new source must still work; this
	// also covers the post-Reset "first call with a new source" path
	// in Transform.
	root := p.Parse(text.NewReader([]byte("[b]: /other\n\n[b]\n")), parser.WithContext(parser.NewContext()))
	if root == nil {
		t.Fatal("post-Reset Parse returned nil")
	}
}

func TestTransformer_CrossSourceReallocates(t *testing.T) {
	tr := linkrefparagraph.New()
	p := newParserWith(tr)
	// Two parses with distinct source buffers (no Reset in between)
	// exercise the !sameSlice branch in Transform.
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

func newForkParser() parser.Parser {
	return newParserWith(linkrefparagraph.New())
}

func newParserWith(tr *linkrefparagraph.Transformer) parser.Parser {
	defs := parser.DefaultParagraphTransformers()
	out := make([]util.PrioritizedValue, len(defs))
	for i, pv := range defs {
		if pv.Value == parser.LinkReferenceParagraphTransformer {
			out[i] = util.Prioritized(tr, pv.Priority)
			continue
		}
		out[i] = pv
	}
	return parser.NewParser(
		parser.WithBlockParsers(parser.DefaultBlockParsers()...),
		parser.WithInlineParsers(parser.DefaultInlineParsers()...),
		parser.WithParagraphTransformers(out...),
	)
}

func newUpstreamParser() parser.Parser {
	return parser.NewParser(
		parser.WithBlockParsers(parser.DefaultBlockParsers()...),
		parser.WithInlineParsers(parser.DefaultInlineParsers()...),
		parser.WithParagraphTransformers(parser.DefaultParagraphTransformers()...),
	)
}

func parseDump(t *testing.T, src string, p parser.Parser) string {
	t.Helper()
	srcBytes := []byte(src)
	root := p.Parse(text.NewReader(srcBytes), parser.WithContext(parser.NewContext()))
	var sb strings.Builder
	dumpNode(&sb, root, srcBytes, 0)
	return sb.String()
}

func dumpNode(sb *strings.Builder, n ast.Node, src []byte, depth int) {
	for i := 0; i < depth; i++ {
		sb.WriteString("  ")
	}
	sb.WriteString(n.Kind().String())
	if ref, ok := n.(*ast.LinkReferenceDefinition); ok {
		fmt.Fprintf(sb, " label=%q dest=%q title=%q",
			ref.Label, ref.Destination, ref.Title)
	}
	sb.WriteByte('\n')
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		dumpNode(sb, c, src, depth+1)
	}
}
