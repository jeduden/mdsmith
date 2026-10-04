package refactor

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/index"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resolveCounter wraps stubWorkspace and counts Resolve calls per file.
type resolveCounter struct {
	stubWorkspace
	anchorEdges []index.Edge
	calls       map[string]int
}

func (c *resolveCounter) IncomingAnchorEdges(string, string) []index.Edge { return c.anchorEdges }

func (c *resolveCounter) Resolve(file string) (string, []byte, bool) {
	c.calls[file]++
	return c.stubWorkspace.Resolve(file)
}

// TestAppendWikilinkStemEdits_ReadsEachSourceOnce locks that a file
// holding several `[[stem]]` links is resolved once, not once per edge:
// the edges arrive sorted by SourceFile.
func TestAppendWikilinkStemEdits_ReadsEachSourceOnce(t *testing.T) {
	ws := &resolveCounter{calls: map[string]int{}, stubWorkspace: stubWorkspace{
		files: []string{"api.md", "index.md"},
		wikilinkEdges: []index.Edge{
			{Kind: index.EdgeWikilink, SourceFile: "index.md", SourceLine: 1, SourceCol: 1},
			{Kind: index.EdgeWikilink, SourceFile: "index.md", SourceLine: 2, SourceCol: 1},
			{Kind: index.EdgeWikilink, SourceFile: "index.md", SourceLine: 3, SourceCol: 1},
		},
		sources: map[string][]byte{"index.md": []byte("[[api]]\n[[api]]\n[[api]]\n")},
	}}
	changes := map[string][]Edit{}
	appendWikilinkStemEdits(changes, ws, soloResolver(ws, "api.md", "service.md"), "api.md", "service.md")
	require.Len(t, changes["index.md"], 3)
	assert.Equal(t, 1, ws.calls["index.md"])
}

// TestAppendAnchorEditsForHeading_ReadsEachSourceOnce locks the same
// for a heading rename's anchor edges.
func TestAppendAnchorEditsForHeading_ReadsEachSourceOnce(t *testing.T) {
	ws := &resolveCounter{calls: map[string]int{}, stubWorkspace: stubWorkspace{
		sources: map[string][]byte{"index.md": []byte("[a](g.md#setup)\n[b](g.md#setup)\n")},
	}, anchorEdges: []index.Edge{
		{Kind: index.EdgeFileLink, SourceFile: "index.md", SourceLine: 1, SourceCol: 1},
		{Kind: index.EdgeFileLink, SourceFile: "index.md", SourceLine: 2, SourceCol: 1},
	}}
	changes := map[string][]Edit{}
	appendAnchorEditsForHeading(changes, ws, "g.md", "setup", "install")
	require.Len(t, changes["index.md"], 2)
	assert.Equal(t, 1, ws.calls["index.md"])
}

// TestEdgeLines locks the reader's per-file cache: a new file resolves
// again, an unreadable file stays unreadable, and an out-of-range line
// is refused.
func TestEdgeLines(t *testing.T) {
	ws := &resolveCounter{calls: map[string]int{}, stubWorkspace: stubWorkspace{
		sources:      map[string][]byte{"a.md": []byte("x\ny\n"), "b.md": []byte("z\n")},
		unresolvable: map[string]bool{"gone.md": true},
	}}
	r := edgeLines{ws: ws}
	key, row, ok := r.row(index.Edge{SourceFile: "a.md", SourceLine: 2})
	require.True(t, ok)
	assert.Equal(t, "a.md", key)
	assert.Equal(t, "y", string(row))
	_, _, ok = r.row(index.Edge{SourceFile: "a.md", SourceLine: 9})
	assert.False(t, ok, "a line past EOF is refused")
	_, _, ok = r.row(index.Edge{SourceFile: "a.md", SourceLine: 0})
	assert.False(t, ok, "line 0 is refused")
	_, row, ok = r.row(index.Edge{SourceFile: "b.md", SourceLine: 1})
	require.True(t, ok)
	assert.Equal(t, "z", string(row))
	for range 2 {
		_, _, ok = r.row(index.Edge{SourceFile: "gone.md", SourceLine: 1})
		assert.False(t, ok)
	}
	assert.Equal(t, map[string]int{"a.md": 1, "b.md": 1, "gone.md": 1}, ws.calls)
}
