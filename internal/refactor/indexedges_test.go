package refactor

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/index"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sampleIndex indexes a.md (heading "Setup") and b.md, whose one line
// holds an anchor link and a wikilink to a.md.
func sampleIndex(t *testing.T) *index.Index {
	t.Helper()
	files := map[string][]byte{
		"a.md": []byte("# Setup\n"),
		"b.md": []byte("See [go](a.md#setup) and [[a]].\n"),
	}
	idx := index.New(".")
	idx.BuildSerial([]string{"a.md", "b.md"}, func(rel string) ([]byte, error) {
		return files[rel], nil
	})
	return idx
}

// indexHost embeds IndexEdges beside a stub Resolve, the way every
// production host does, so each test also pins that the value-receiver
// methods satisfy Workspace.
type indexHost struct {
	IndexEdges
	resolveStub
}

type resolveStub struct{}

func (resolveStub) Resolve(string) (string, []byte, bool) { return "", nil, false }

// emptyIndexEdges lists every IndexEdges that has no index to forward
// to; each must answer every query with nothing.
func emptyIndexEdges() map[string]IndexEdges {
	return map[string]IndexEdges{
		"nil getter":  {},
		"nil index":   NewIndexEdges(nil),
		"nil from fn": {Get: func() *index.Index { return nil }},
	}
}

func TestIndexEdges_IncomingAnchorEdges(t *testing.T) {
	var ws Workspace = indexHost{IndexEdges: NewIndexEdges(sampleIndex(t))}
	edges := ws.IncomingAnchorEdges("a.md", "setup")
	require.Len(t, edges, 1)
	assert.Equal(t, "b.md", edges[0].SourceFile)
	assert.Empty(t, ws.IncomingAnchorEdges("a.md", "missing"))
	for name, e := range emptyIndexEdges() {
		t.Run(name, func(t *testing.T) {
			assert.Empty(t, e.IncomingAnchorEdges("a.md", "setup"))
		})
	}
}

func TestIndexEdges_IncomingPathEdges(t *testing.T) {
	var ws Workspace = indexHost{IndexEdges: NewIndexEdges(sampleIndex(t))}
	edges := ws.IncomingPathEdges("a.md")
	require.Len(t, edges, 1)
	assert.Equal(t, "b.md", edges[0].SourceFile)
	assert.Empty(t, ws.IncomingPathEdges("b.md"))
	for name, e := range emptyIndexEdges() {
		t.Run(name, func(t *testing.T) {
			assert.Empty(t, e.IncomingPathEdges("a.md"))
		})
	}
}

func TestIndexEdges_IncomingWikilinkEdges(t *testing.T) {
	var ws Workspace = indexHost{IndexEdges: NewIndexEdges(sampleIndex(t))}
	edges := ws.IncomingWikilinkEdges("a")
	require.Len(t, edges, 1)
	assert.Equal(t, "b.md", edges[0].SourceFile)
	assert.Empty(t, ws.IncomingWikilinkEdges("b"))
	for name, e := range emptyIndexEdges() {
		t.Run(name, func(t *testing.T) {
			assert.Empty(t, e.IncomingWikilinkEdges("a"))
		})
	}
}

func TestIndexEdges_Files(t *testing.T) {
	var ws Workspace = indexHost{IndexEdges: NewIndexEdges(sampleIndex(t))}
	assert.ElementsMatch(t, []string{"a.md", "b.md"}, ws.Files())
	for name, e := range emptyIndexEdges() {
		t.Run(name, func(t *testing.T) {
			assert.Empty(t, e.Files())
		})
	}
}

func TestIndexEdges_Index(t *testing.T) {
	t.Run("nil getter", func(t *testing.T) {
		assert.Nil(t, IndexEdges{}.index())
	})
	t.Run("calls the getter on every query", func(t *testing.T) {
		idx := sampleIndex(t)
		calls := 0
		e := IndexEdges{Get: func() *index.Index { calls++; return idx }}
		assert.Same(t, idx, e.index())
		assert.Same(t, idx, e.index())
		assert.Equal(t, 2, calls, "IndexEdges caches nothing itself")
	})
}

func TestNewIndexEdges(t *testing.T) {
	idx := sampleIndex(t)
	e := NewIndexEdges(idx)
	assert.Same(t, idx, e.Get())
}

func TestNewLazyIndexEdges(t *testing.T) {
	idx := sampleIndex(t)
	builds := 0
	e := NewLazyIndexEdges(func() *index.Index { builds++; return idx })
	assert.Zero(t, builds, "constructing builds nothing")
	assert.Len(t, e.IncomingPathEdges("a.md"), 1)
	assert.Len(t, e.IncomingWikilinkEdges("a"), 1)
	cp := e
	assert.Len(t, cp.Files(), 2)
	assert.Equal(t, 1, builds, "built once, then reused by every copy")
}
