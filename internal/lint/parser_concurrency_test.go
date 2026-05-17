package lint

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"
)

// TestNewFile_ConcurrentParseRaceFree drives NewFile (and the pooled
// parser behind it) plus LinkReferences from many goroutines at once.
// Linting is multi-goroutine — the LSP serves concurrent documents and
// the check walk fans out across workers — so the parser pool and the
// per-File caches must stay race-free. Run with -race; each parse must
// keep its own reference defs (per-call parser.Context isolation).
func TestNewFile_ConcurrentParseRaceFree(t *testing.T) {
	const goroutines = 32
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(n int) {
			defer wg.Done()
			src := []byte(fmt.Sprintf(
				"# Doc %d\n\nText [ref%d].\n\n[ref%d]: https://example.com/%d\n",
				n, n, n, n))
			f, err := NewFile(fmt.Sprintf("doc%d.md", n), src)
			require.NoError(t, err)
			require.NotNil(t, f.AST)
			assert.Equal(t, ast.KindDocument, f.AST.Kind())

			refs := f.LinkReferences()
			require.Len(t, refs, 1, "each parse keeps its own reference defs")
			assert.Equal(t, fmt.Sprintf("ref%d", n), string(refs[0].Label()))

			// Cached collectors must also be race-free under the
			// concurrent readers the LSP path creates for one File.
			_ = CollectCodeBlockLines(f)
			_ = CollectPIBlockLines(f)
		}(i)
	}
	wg.Wait()
}

// TestNewFile_SharedFileConcurrentReaders exercises the LSP shape: one
// *File read by several goroutines at once. The sync.Once-guarded
// caches (linkRefs, codeBlockLines, piBlockLines) must produce one
// stable result without a data race.
func TestNewFile_SharedFileConcurrentReaders(t *testing.T) {
	f, err := NewFile("shared.md", []byte(
		"# H\n\n```go\nx := 1\n```\n\nSee [a].\n\n[a]: https://example.com/a\n"))
	require.NoError(t, err)

	const readers = 16
	var wg sync.WaitGroup
	wg.Add(readers)
	for i := 0; i < readers; i++ {
		go func() {
			defer wg.Done()
			require.Len(t, f.LinkReferences(), 1)
			require.NotEmpty(t, CollectCodeBlockLines(f))
			_ = CollectPIBlockLines(f)
		}()
	}
	wg.Wait()
}
