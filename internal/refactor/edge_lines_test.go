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

// TestMoveAll_ReadsEachFileOnce locks that a batch scans the workspace
// for incoming links once, not once per planned move: a hub linking
// every member is read once, and a moved file once, by validation.
func TestMoveAll_ReadsEachFileOnce(t *testing.T) {
	ws := &resolveCounter{calls: map[string]int{}, stubWorkspace: stubWorkspace{
		files: []string{"a.md", "b.md", "c.md", "hub.md"},
		sources: map[string][]byte{
			"a.md":   []byte("[b](b.md)\n"),
			"b.md":   []byte("# B\n"),
			"c.md":   []byte("# C\n"),
			"hub.md": []byte("[a](a.md) [b](b.md) [c](c.md)\n"),
		},
	}}
	bp := MoveAll(ws, []MovePair{{"a.md", "x/a.md"}, {"b.md", "x/b.md"}, {"c.md", "y/c.md"}})
	assert.Equal(t, []string{"y/c.md", "x/b.md", "x/a.md"}, texts(bp.Edits, "hub.md"))
	assert.Empty(t, bp.Edits["a.md"])
	assert.Equal(t, 1, ws.calls["hub.md"])
	assert.Equal(t, 1, ws.calls["a.md"])
}

// TestMoveAll_ShadowedPathReadsEachFileOnce locks that counting the
// links to a shadowed path rides on the referrer scan: b.md's refused
// move to the existing c.md lets a.md take its path, and hub.md, which
// links both, is still read once.
func TestMoveAll_ShadowedPathReadsEachFileOnce(t *testing.T) {
	ws := &resolveCounter{calls: map[string]int{}, stubWorkspace: stubWorkspace{
		files: []string{"a.md", "b.md", "c.md", "hub.md"},
		sources: map[string][]byte{
			"a.md":   []byte("# A\n"),
			"b.md":   []byte("# B\n"),
			"c.md":   []byte("# C\n"),
			"hub.md": []byte("[a](a.md) [b](b.md)\n"),
		},
	}}
	bp := MoveAll(ws, []MovePair{{"b.md", "c.md"}, {"a.md", "b.md"}})
	require.Equal(t, DestinationExistsError{Dst: "c.md"}, bp.Moves[0].Err)
	assert.Equal(t, []string{"b.md"}, texts(bp.Edits, "hub.md"))
	assert.Equal(t, 1, bp.Withheld)
	assert.Equal(t, 1, ws.calls["hub.md"])
}

// memResolveCounter counts Resolve calls on a memWorkspace.
type memResolveCounter struct {
	*memWorkspace
	calls map[string]int
}

func (w memResolveCounter) Resolve(file string) (string, []byte, bool) {
	w.calls[file]++
	return w.memWorkspace.Resolve(file)
}

// TestMoveAll_StemPassesReadEachFileOnce locks that the `[[stem]]`
// passes of one batch share their reads: hub.md, holding a link to
// each of three moved files, is read once by the referrer scan and
// once by every stem pass together, not once per move.
func TestMoveAll_StemPassesReadEachFileOnce(t *testing.T) {
	ws := memResolveCounter{calls: map[string]int{}, memWorkspace: newMemWorkspace(map[string]string{
		"a.md":   "# A\n",
		"b.md":   "# B\n",
		"c.md":   "# C\n",
		"hub.md": "# Hub\n\n[[a]] [[b]] [[c]]\n",
	})}
	bp := MoveAll(ws, []MovePair{{"a.md", "a2.md"}, {"b.md", "b2.md"}, {"c.md", "c2.md"}})
	assert.ElementsMatch(t, []string{"a2", "b2", "c2"}, texts(bp.Edits, "hub.md"))
	assert.Equal(t, 2, ws.calls["hub.md"])
}

// TestEdgeLines_Memo locks that a reader with a memo resolves a file
// once even when its edges are not consecutive, and that one without
// re-reads it.
func TestEdgeLines_Memo(t *testing.T) {
	edges := []index.Edge{
		{SourceFile: "a.md", SourceLine: 1}, {SourceFile: "b.md", SourceLine: 1}, {SourceFile: "a.md", SourceLine: 1},
	}
	for memo, want := range map[bool]int{true: 1, false: 2} {
		ws := &resolveCounter{calls: map[string]int{}, stubWorkspace: stubWorkspace{
			sources: map[string][]byte{"a.md": []byte("x\n"), "b.md": []byte("y\n")},
		}}
		r := &edgeLines{ws: ws}
		if memo {
			r.memo = map[string]edgeFile{}
		}
		for _, e := range edges {
			_, row, ok := r.row(e)
			require.True(t, ok)
			assert.NotEmpty(t, row)
		}
		assert.Equal(t, want, ws.calls["a.md"], "memo %v", memo)
	}
}
