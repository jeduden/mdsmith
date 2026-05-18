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
