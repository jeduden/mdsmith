package refactor

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/index"
	"github.com/stretchr/testify/assert"
)

func indexedWorkspace(t *testing.T) *index.Index {
	t.Helper()
	return newMemWorkspace(map[string]string{
		"a.md": "# Setup\n",
		"b.md": "See [go](a.md#setup) and [[a]].\n",
	}).idx
}

func TestIndexEdges_ForwardsToIndex(t *testing.T) {
	idx := indexedWorkspace(t)
	var ws Workspace = struct {
		IndexEdges
		resolveStub
	}{IndexEdges: NewIndexEdges(idx)}
	assert.Len(t, ws.IncomingAnchorEdges("a.md", "setup"), 1)
	assert.Len(t, ws.IncomingPathEdges("a.md"), 1)
	assert.Len(t, ws.IncomingWikilinkEdges("a"), 1)
	assert.ElementsMatch(t, []string{"a.md", "b.md"}, ws.Files())
}

type resolveStub struct{}

func (resolveStub) Resolve(string) (string, []byte, bool) { return "", nil, false }

func TestIndexEdges_GetterIsLazy(t *testing.T) {
	idx := indexedWorkspace(t)
	calls := 0
	e := IndexEdges{Get: func() *index.Index { calls++; return idx }}
	assert.Zero(t, calls, "constructing reads nothing")
	assert.Len(t, e.IncomingPathEdges("a.md"), 1)
	assert.Equal(t, 1, calls)
}

func TestIndexEdges_NilAnswersNothing(t *testing.T) {
	for name, e := range map[string]IndexEdges{
		"nil getter":  {},
		"nil index":   NewIndexEdges(nil),
		"nil from fn": {Get: func() *index.Index { return nil }},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Empty(t, e.IncomingAnchorEdges("a.md", "x"))
			assert.Empty(t, e.IncomingPathEdges("a.md"))
			assert.Empty(t, e.IncomingWikilinkEdges("a"))
			assert.Empty(t, e.Files())
		})
	}
}

func TestNewIndexEdges_Eager(t *testing.T) {
	idx := indexedWorkspace(t)
	e := NewIndexEdges(idx)
	assert.Same(t, idx, e.Get())
}
