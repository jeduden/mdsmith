package markdown

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"

	"github.com/jeduden/mdsmith/internal/goldmark/linkrefparagraph"
)

func TestNewParser(t *testing.T) {
	p := NewParser()
	require.NotNil(t, p)
	root := p.Parse(text.NewReader([]byte("# H\n\n<?toc?>\n")))
	require.NotNil(t, root)
	assert.Equal(t, ast.KindDocument, root.Kind())
	// The PI block parser is registered, so <?toc?> is a PI node.
	assert.Len(t, findPINodes(root), 1)
}

// fakeTransformer is a no-op paragraph transformer used to verify
// substituteLinkRef preserves entries it does not recognise.
// Goldmark's current DefaultParagraphTransformers ships only the
// link-reference entry, so the pass-through branch is not reachable
// from a goldmark-as-shipped parser; this unit test drives it
// directly.
type fakeTransformer struct{}

func (fakeTransformer) Transform(*ast.Paragraph, text.Reader, parser.Context) {}

func TestSubstituteLinkRef_PreservesUnknownEntries(t *testing.T) {
	fake := fakeTransformer{}
	lrp := linkrefparagraph.New()
	defaults := []util.PrioritizedValue{
		util.Prioritized(fake, 200),
		util.Prioritized(parser.LinkReferenceParagraphTransformer, 100),
		util.Prioritized(fake, 50),
	}
	got := substituteLinkRef(defaults, lrp)
	require.Len(t, got, 3)
	assert.Equal(t, fake, got[0].Value)
	assert.Equal(t, 200, got[0].Priority)
	assert.Equal(t, lrp, got[1].Value)
	assert.Equal(t, 100, got[1].Priority)
	assert.Equal(t, fake, got[2].Value)
	assert.Equal(t, 50, got[2].Priority)
}

// TestParseContext_ConcurrentRaceFree drives the pooled parser from
// many goroutines at once. Parsing is multi-goroutine — the LSP serves
// concurrent documents and the check walk fans out across workers — so
// the parser pool must stay race-free and each parse must keep its own
// reference defs (per-call parser.Context isolation). Run with -race.
func TestParseContext_ConcurrentRaceFree(t *testing.T) {
	const goroutines = 32
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(n int) {
			defer wg.Done()
			src := []byte(fmt.Sprintf(
				"# Doc %d\n\nText [ref%d].\n\n[ref%d]: https://example.com/%d\n",
				n, n, n, n))
			ctx := parser.NewContext()
			root := ParseContext(src, ctx)
			require.NotNil(t, root)
			assert.Equal(t, ast.KindDocument, root.Kind())

			refs := ctx.References()
			require.Len(t, refs, 1, "each parse keeps its own reference defs")
			assert.Equal(t, fmt.Sprintf("ref%d", n), string(refs[0].Label()))
		}(i)
	}
	wg.Wait()
}
