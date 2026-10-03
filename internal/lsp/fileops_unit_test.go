package lsp

import (
	"math/rand/v2"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"

	"github.com/jeduden/mdsmith/internal/index"
	"github.com/jeduden/mdsmith/internal/linkgraph"
	"github.com/jeduden/mdsmith/internal/refactor"
)

func edAt(line, from, to int, text string) textEdit {
	return textEdit{
		Range:   Range{Start: Position{Line: line, Character: from}, End: Position{Line: line, Character: to}},
		NewText: text,
	}
}

func TestDropConflictingTextEdits(t *testing.T) {
	t.Parallel()
	t.Run("disjoint edits kept", func(t *testing.T) {
		t.Parallel()
		in := []textEdit{edAt(2, 5, 8, "b"), edAt(1, 0, 3, "a"), edAt(1, 4, 6, "c")}
		got := dropConflictingTextEdits(in)
		assert.ElementsMatch(t, in, got)
	})
	t.Run("identical overlapping edits drop both", func(t *testing.T) {
		t.Parallel()
		got := dropConflictingTextEdits([]textEdit{
			edAt(1, 0, 3, "a"), edAt(1, 0, 3, "a"), edAt(4, 0, 1, "k"),
		})
		assert.Equal(t, []textEdit{edAt(4, 0, 1, "k")}, got)
	})
	t.Run("same range different text drops both", func(t *testing.T) {
		t.Parallel()
		got := dropConflictingTextEdits([]textEdit{
			edAt(1, 0, 3, "a"), edAt(1, 0, 3, "z"), edAt(4, 0, 1, "k"),
		})
		assert.Equal(t, []textEdit{edAt(4, 0, 1, "k")}, got)
	})
	t.Run("partial overlap drops both", func(t *testing.T) {
		t.Parallel()
		got := dropConflictingTextEdits([]textEdit{edAt(1, 0, 5, "a"), edAt(1, 3, 8, "b")})
		assert.Empty(t, got)
	})
	t.Run("touching ranges are not overlapping", func(t *testing.T) {
		t.Parallel()
		in := []textEdit{edAt(1, 0, 3, "a"), edAt(1, 3, 6, "b")}
		assert.ElementsMatch(t, in, dropConflictingTextEdits(in))
	})
	t.Run("empty input", func(t *testing.T) {
		t.Parallel()
		assert.Empty(t, dropConflictingTextEdits(nil))
	})
	t.Run("survivors return sorted bottom-up", func(t *testing.T) {
		t.Parallel()
		got := dropConflictingTextEdits([]textEdit{
			edAt(1, 0, 3, "a"), edAt(2, 5, 8, "b"), edAt(1, 4, 6, "c"),
		})
		assert.Equal(t, []textEdit{
			edAt(2, 5, 8, "b"), edAt(1, 4, 6, "c"), edAt(1, 0, 3, "a"),
		}, got)
	})
	t.Run("single edit kept", func(t *testing.T) {
		t.Parallel()
		in := []textEdit{edAt(1, 0, 3, "a")}
		assert.Equal(t, in, dropConflictingTextEdits(in))
	})
}

// Inserts follow refactor.ApplyEdits and the LSP spec: one may touch a
// replacement's start or end, but two at one point, or one strictly
// inside a replacement, conflict.
func TestDropConflictingTextEdits_Inserts(t *testing.T) {
	t.Parallel()
	t.Run("insert at a replacement's start is kept", func(t *testing.T) {
		t.Parallel()
		in := []textEdit{edAt(1, 3, 3, "i"), edAt(1, 3, 6, "r")}
		assert.ElementsMatch(t, in, dropConflictingTextEdits(in))
	})
	t.Run("insert at a replacement's end is kept", func(t *testing.T) {
		t.Parallel()
		in := []textEdit{edAt(1, 0, 3, "r"), edAt(1, 3, 3, "i")}
		assert.ElementsMatch(t, in, dropConflictingTextEdits(in))
	})
	t.Run("two inserts at one point drop both", func(t *testing.T) {
		t.Parallel()
		got := dropConflictingTextEdits([]textEdit{
			edAt(1, 3, 3, "i"), edAt(1, 3, 6, "r"), edAt(1, 3, 3, "j"),
		})
		assert.Equal(t, []textEdit{edAt(1, 3, 6, "r")}, got)
	})
	t.Run("insert inside a replacement drops both", func(t *testing.T) {
		t.Parallel()
		got := dropConflictingTextEdits([]textEdit{edAt(1, 0, 6, "r"), edAt(1, 3, 3, "i")})
		assert.Empty(t, got)
	})
}

// rangesOverlap is the pairwise oracle for dropConflictingTextEdits:
// edits over a and b touch the same text when their spans intersect
// (an insert strictly inside a replacement included) or both are
// inserts at one position, whose relative order no single move fixes.
// Ranges that merely touch end-to-start do not overlap, and neither
// does an insert at a replacement's start or end — refactor.ApplyEdits
// and the LSP spec both accept those.
func rangesOverlap(a, b Range) bool {
	if a.Start == b.Start && a.Start == a.End && b.Start == b.End {
		return true
	}
	return posLess(a.Start, b.End) && posLess(b.Start, a.End)
}

// An overlap cluster drops every member, even ones that touch only a
// third edit.
func TestDropConflictingTextEdits_Clusters(t *testing.T) {
	t.Parallel()
	t.Run("an edit nesting two disjoint edits drops all three", func(t *testing.T) {
		t.Parallel()
		got := dropConflictingTextEdits([]textEdit{
			edAt(1, 5, 6, "c"), edAt(1, 0, 10, "a"), edAt(1, 2, 3, "b"), edAt(1, 10, 12, "k"),
		})
		assert.Equal(t, []textEdit{edAt(1, 10, 12, "k")}, got)
	})
	t.Run("a chain of overlaps drops every link", func(t *testing.T) {
		t.Parallel()
		got := dropConflictingTextEdits([]textEdit{
			edAt(1, 6, 9, "c"), edAt(1, 0, 4, "a"), edAt(1, 3, 7, "b"),
		})
		assert.Empty(t, got)
	})
}

// The sort-and-sweep keeps exactly the edits the pairwise oracle finds
// overlapping no other edit, on random multi-line ranges including
// zero-width ones and shared starts.
func TestDropConflictingTextEdits_MatchesPairwiseOracle(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewPCG(1, 2))
	for round := range 2000 {
		n := rng.IntN(7)
		in := make([]textEdit, n)
		for i := range in {
			sl, sc := rng.IntN(3), rng.IntN(6)
			el, ec := sl, sc+rng.IntN(4)
			if rng.IntN(4) == 0 {
				el, ec = sl+1, rng.IntN(6)
			}
			in[i] = textEdit{
				Range: Range{
					Start: Position{Line: sl, Character: sc},
					End:   Position{Line: el, Character: ec},
				},
				NewText: strconv.Itoa(i),
			}
		}
		var want []textEdit
		for i, a := range in {
			lone := true
			for j, b := range in {
				if i != j && rangesOverlap(a.Range, b.Range) {
					lone = false
				}
			}
			if lone {
				want = append(want, a)
			}
		}
		got := dropConflictingTextEdits(slices.Clone(in))
		assert.ElementsMatch(t, want, got, "round %d input %v", round, in)
	}
}

func TestRangesOverlap(t *testing.T) {
	t.Parallel()
	assert.True(t, rangesOverlap(edAt(1, 0, 5, "").Range, edAt(1, 4, 6, "").Range))
	assert.False(t, rangesOverlap(edAt(1, 0, 3, "").Range, edAt(1, 3, 6, "").Range))
	assert.False(t, rangesOverlap(edAt(1, 0, 3, "").Range, edAt(2, 0, 3, "").Range))
	assert.True(t, rangesOverlap(edAt(1, 2, 2, "").Range, edAt(1, 2, 2, "").Range))
	assert.True(t, rangesOverlap(edAt(1, 2, 5, "").Range, edAt(1, 2, 4, "").Range))
	assert.False(t, rangesOverlap(edAt(1, 2, 2, "").Range, edAt(1, 2, 5, "").Range))
	assert.True(t, rangesOverlap(edAt(1, 0, 5, "").Range, edAt(1, 2, 2, "").Range))
}

func TestPosLess(t *testing.T) {
	t.Parallel()
	assert.True(t, posLess(Position{Line: 1, Character: 9}, Position{Line: 2, Character: 0}))
	assert.True(t, posLess(Position{Line: 1, Character: 2}, Position{Line: 1, Character: 3}))
	assert.False(t, posLess(Position{Line: 1, Character: 3}, Position{Line: 1, Character: 3}))
	assert.False(t, posLess(Position{Line: 2, Character: 0}, Position{Line: 1, Character: 9}))
}

// ref is the refactor.Edit that edAt(line, 0, 4, text) converts from.
func ref(line int, text string) refactor.Edit {
	return refactor.Edit{
		Range: refactor.Range{
			Start: refactor.Position{Line: line, Character: 0},
			End:   refactor.Position{Line: line, Character: 4},
		},
		NewText: text,
	}
}

func TestDropCrossMoveEdits(t *testing.T) {
	t.Parallel()
	t.Run("edit inside a file moved to another directory is dropped", func(t *testing.T) {
		t.Parallel()
		moves := []plannedMove{
			{key: "a", changesDir: true, edits: map[string][]refactor.Edit{"a": {ref(1, "own")}}},
			{key: "b", changesDir: true, edits: map[string][]refactor.Edit{"a": {ref(3, "x")}, "c": {ref(1, "y")}}},
		}
		merged := map[string][]textEdit{
			"a": {edAt(1, 0, 4, "own"), edAt(3, 0, 4, "x")},
			"c": {edAt(1, 0, 4, "y")},
		}
		dropCrossMoveEdits(merged, moves)
		assert.Equal(t, map[string][]textEdit{
			"a": {edAt(1, 0, 4, "own")},
			"c": {edAt(1, 0, 4, "y")},
		}, merged)
	})
	t.Run("a key left empty is deleted", func(t *testing.T) {
		t.Parallel()
		moves := []plannedMove{
			{key: "a", changesDir: true},
			{key: "b", edits: map[string][]refactor.Edit{"a": {ref(3, "x")}}},
		}
		merged := map[string][]textEdit{"a": {edAt(3, 0, 4, "x")}}
		dropCrossMoveEdits(merged, moves)
		assert.Empty(t, merged)
	})
	t.Run("same-directory rename keeps foreign edits", func(t *testing.T) {
		t.Parallel()
		moves := []plannedMove{
			{key: "a", changesDir: false},
			{key: "b", changesDir: true, edits: map[string][]refactor.Edit{"a": {ref(3, "x")}}},
		}
		merged := map[string][]textEdit{"a": {edAt(3, 0, 4, "x")}}
		dropCrossMoveEdits(merged, moves)
		assert.Equal(t, map[string][]textEdit{"a": {edAt(3, 0, 4, "x")}}, merged)
	})
}

// A `[[stem]]` rewrite another move plans inside a file moved to
// another directory is kept beside that file's own edits, while the
// other move's path rewrite there is dropped.
func TestDropCrossMoveEdits_KeepsStemRewrites(t *testing.T) {
	t.Parallel()
	stem := ref(3, "c")
	moves := []plannedMove{
		{key: "a", changesDir: true, edits: map[string][]refactor.Edit{"a": {ref(1, "own")}}},
		{
			key: "b", changesDir: true,
			edits:     map[string][]refactor.Edit{"a": {stem, ref(5, "x")}},
			stemEdits: map[string][]refactor.Edit{"a": {stem}},
		},
	}
	merged := map[string][]textEdit{"a": {edAt(1, 0, 4, "own"), edAt(3, 0, 4, "c"), edAt(5, 0, 4, "x")}}
	dropCrossMoveEdits(merged, moves)
	assert.Equal(t, map[string][]textEdit{"a": {edAt(1, 0, 4, "own"), edAt(3, 0, 4, "c")}}, merged)
}

// memRenameWorkspace is a refactor.Workspace over an in-memory file
// set: the production index behind refactor.IndexEdges, with Resolve
// keying each file by its workspace-relative path.
type memRenameWorkspace struct {
	refactor.IndexEdges
	files map[string]string
}

func newMemRenameWorkspace(files map[string]string) memRenameWorkspace {
	rels := make([]string, 0, len(files))
	for rel := range files {
		rels = append(rels, rel)
	}
	idx := index.New(".")
	idx.BuildSerial(rels, func(rel string) ([]byte, error) { return []byte(files[rel]), nil })
	return memRenameWorkspace{IndexEdges: refactor.NewIndexEdges(idx), files: files}
}

func (w memRenameWorkspace) WikilinkIndex() *linkgraph.WikilinkIndex {
	fsys := fstest.MapFS{}
	for rel := range w.files {
		fsys[rel] = &fstest.MapFile{}
	}
	return linkgraph.NewWikilinkIndex(fsys)
}

func (w memRenameWorkspace) Resolve(file string) (string, []byte, bool) {
	rel := index.NormalizePath(file)
	src, ok := w.files[rel]
	return rel, []byte(src), ok
}

func TestPlanRenameBatch(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	uri := func(rel string) string { return pathToURI(filepath.Join(root, filepath.FromSlash(rel))) }
	ws := newMemRenameWorkspace(map[string]string{
		"docs/a.md": "# A\n\n[b](b.md) and [[b]]\n",
		"docs/b.md": "# B\n",
		"x/y/a.md":  "# Old\n",
	})
	t.Run("an unplanned move is kept without edits", func(t *testing.T) {
		t.Parallel()
		moves := planRenameBatch(ws, root, []fileRename{
			{OldURI: uri("docs/a.md"), NewURI: uri("x/y/a.md")},
			{OldURI: uri("docs/b.md"), NewURI: uri("x/y/c.md")},
		})
		assert.Len(t, moves, 2)
		assert.Equal(t, plannedMove{key: "docs/a.md", changesDir: true}, moves[0])
		assert.Equal(t, "docs/b.md", moves[1].key)
		assert.Len(t, moves[1].edits["docs/a.md"], 2, "one path and one stem rewrite")
		assert.Len(t, moves[1].stemEdits["docs/a.md"], 1)
	})
	t.Run("skips empty, unchanged, repeated, and unreadable pairs", func(t *testing.T) {
		t.Parallel()
		pair := fileRename{OldURI: uri("docs/b.md"), NewURI: uri("docs/c.md")}
		moves := planRenameBatch(ws, root, []fileRename{
			pair, pair,
			{OldURI: uri("docs/a.md"), NewURI: uri("docs/a.md")},
			{OldURI: "untitled:x", NewURI: uri("docs/z.md")},
			{OldURI: uri("gone.md"), NewURI: uri("gone2.md")},
		})
		assert.Len(t, moves, 1)
		assert.False(t, moves[0].changesDir)
	})
	t.Run("a single rename keeps its stem subset", func(t *testing.T) {
		t.Parallel()
		moves := planRenameBatch(ws, root, []fileRename{
			{OldURI: uri("docs/b.md"), NewURI: uri("docs/c.md")},
		})
		assert.Len(t, moves, 1)
		assert.Len(t, moves[0].edits["docs/a.md"], 2)
		assert.Len(t, moves[0].stemEdits["docs/a.md"], 1)
	})
}

func TestCompareTextEditsTopDown(t *testing.T) {
	t.Parallel()
	assert.Negative(t, compareTextEditsTopDown(edAt(1, 0, 2, ""), edAt(1, 1, 2, "")))
	assert.Positive(t, compareTextEditsTopDown(edAt(2, 0, 2, ""), edAt(1, 1, 2, "")))
	assert.Negative(t, compareTextEditsTopDown(edAt(1, 3, 3, ""), edAt(1, 3, 5, "")))
	assert.Positive(t, compareTextEditsTopDown(edAt(1, 3, 5, ""), edAt(1, 3, 3, "")))
	assert.Zero(t, compareTextEditsTopDown(edAt(1, 3, 5, ""), edAt(1, 3, 6, "")))
}

func TestIsInsert(t *testing.T) {
	t.Parallel()
	assert.True(t, isInsert(edAt(1, 3, 3, "x")))
	assert.False(t, isInsert(edAt(1, 3, 4, "")))
}
