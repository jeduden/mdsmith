package lsp

import (
	"maps"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	t.Run("an edit the batch did not plan for the moved file is dropped", func(t *testing.T) {
		t.Parallel()
		moves := []plannedMove{{key: "a", changesDir: true, own: []refactor.Edit{ref(1, "own")}}}
		merged := map[string][]textEdit{
			"a": {edAt(1, 0, 4, "own"), edAt(3, 0, 4, "x")},
			"c": {edAt(1, 0, 4, "y")},
		}
		dropCrossMoveEdits(merged, moves, nil)
		assert.Equal(t, map[string][]textEdit{
			"a": {edAt(1, 0, 4, "own")},
			"c": {edAt(1, 0, 4, "y")},
		}, merged)
	})
	t.Run("a key left empty is deleted", func(t *testing.T) {
		t.Parallel()
		merged := map[string][]textEdit{"a": {edAt(3, 0, 4, "x")}}
		dropCrossMoveEdits(merged, []plannedMove{{key: "a", changesDir: true}}, nil)
		assert.Empty(t, merged)
	})
	t.Run("same-directory rename keeps every edit", func(t *testing.T) {
		t.Parallel()
		merged := map[string][]textEdit{"a": {edAt(3, 0, 4, "x")}}
		dropCrossMoveEdits(merged, []plannedMove{{key: "a"}, {key: "b", changesDir: true}}, nil)
		assert.Equal(t, map[string][]textEdit{"a": {edAt(3, 0, 4, "x")}}, merged)
	})
}

// A `[[stem]]` rewrite inside a file whose move could not be planned
// is kept: it names its target by stem, not by a path from the
// holder's directory. A path rewrite there is dropped.
func TestDropCrossMoveEdits_KeepsStemRewrites(t *testing.T) {
	t.Parallel()
	moves := []plannedMove{{key: "a", changesDir: true}}
	merged := map[string][]textEdit{"a": {edAt(3, 0, 4, "c"), edAt(5, 0, 4, "x")}}
	dropCrossMoveEdits(merged, moves, map[string][]refactor.Edit{"a": {ref(3, "c")}})
	assert.Equal(t, map[string][]textEdit{"a": {edAt(3, 0, 4, "c")}}, merged)
}

// TestGuardRenameEdits_BatchCasesDropNothing locks that the two guards
// no longer fire on the cases the pre-batch planner needed them for:
// links between two moved files in one new folder or different ones,
// an agreeing pair of rewrites, a rewrite only the target's move
// touched, a right one-sided rewrite, a wikilink between moved files,
// a three-file cycle, and a `[[stem]]` rewrite inside a file whose
// move was refused.
func TestGuardRenameEdits_BatchCasesDropNothing(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	uri := func(rel string) string { return pathToURI(filepath.Join(root, filepath.FromSlash(rel))) }
	for name, tc := range map[string]struct {
		files map[string]string
		pairs []string
	}{
		"same new folder": {
			map[string]string{"a.md": "[b](b.md)\n", "b.md": "[a](a.md)\n"},
			[]string{"a.md", "x/a.md", "b.md", "x/b.md"},
		},
		"different new folders": {
			map[string]string{"a.md": "[b](b.md)\n", "b.md": "[a](a.md)\n"},
			[]string{"a.md", "x/a.md", "b.md", "y/b.md"},
		},
		"agreeing rewrites": {
			map[string]string{"docs/a.md": "[b](../b.md)\n", "b.md": "# B\n"},
			[]string{"docs/a.md", "a.md", "b.md", "docs/b.md"},
		},
		"only the target's move rewrites": {
			map[string]string{"docs/a.md": "[b](../docs/b.md)\n", "docs/b.md": "# B\n"},
			[]string{"docs/a.md", "other/a.md", "docs/b.md", "docs/sub/b.md"},
		},
		"right one-sided rewrite": {
			map[string]string{"docs/a.md": "[b](../b.md)\n", "b.md": "# B\n"},
			[]string{"docs/a.md", "other/a.md", "b.md", "b2.md"},
		},
		"wikilinks between moved files": {
			map[string]string{"a.md": "[[b]] [b](b.md)\n", "b.md": "[[a]]\n"},
			[]string{"a.md", "x/a2.md", "b.md", "y/b2.md"},
		},
		"three-file cycle": {
			map[string]string{"a.md": "[b](b.md)\n", "b.md": "[c](c.md)\n", "c.md": "[a](a.md)\n"},
			[]string{"a.md", "p/a.md", "b.md", "q/b.md", "c.md", "r/c.md"},
		},
		"refused move": {
			map[string]string{"docs/a.md": "[[b]]\n", "docs/b.md": "# B\n", "x/y/a.md": "# Old\n"},
			[]string{"docs/a.md", "x/y/a.md", "docs/b.md", "x/y/c.md"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var files []fileRename
			for i := 0; i < len(tc.pairs); i += 2 {
				files = append(files, fileRename{OldURI: uri(tc.pairs[i]), NewURI: uri(tc.pairs[i+1])})
			}
			batch := planRenameBatch(newMemRenameWorkspace(tc.files), root, files)
			merged, dropped := guardRenameEdits(batch)
			assert.Zero(t, dropped)
			assert.Zero(t, batch.withheld)
			for key, edits := range batch.edits {
				assert.ElementsMatch(t, toTextEdits(edits), merged[key], key)
			}
		})
	}
}

// TestPlanRenameBatch_SameFolderMoveWithholdsNothing locks that moving
// docs/a.md and docs/b.md into docs/sub/ together plans no edit for the
// links between them, which still resolve, and counts none as withheld.
func TestPlanRenameBatch_SameFolderMoveWithholdsNothing(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	uri := func(rel string) string { return pathToURI(filepath.Join(root, filepath.FromSlash(rel))) }
	batch := planRenameBatch(newMemRenameWorkspace(map[string]string{
		"docs/a.md": "# A\n\n[b](b.md)\n",
		"docs/b.md": "# B\n\n[a](./a.md)\n",
	}), root, []fileRename{
		{OldURI: uri("docs/a.md"), NewURI: uri("docs/sub/a.md")},
		{OldURI: uri("docs/b.md"), NewURI: uri("docs/sub/b.md")},
	})
	assert.Empty(t, batch.edits)
	assert.Zero(t, batch.withheld)
	_, dropped := guardRenameEdits(batch)
	assert.Zero(t, dropped)
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
	return linkgraph.NewWikilinkIndexFromPaths(slices.Collect(maps.Keys(w.files)))
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
	t.Run("an unplanned move is kept without own edits", func(t *testing.T) {
		t.Parallel()
		batch := planRenameBatch(ws, root, []fileRename{
			{OldURI: uri("docs/a.md"), NewURI: uri("x/y/a.md")},
			{OldURI: uri("docs/b.md"), NewURI: uri("x/y/c.md")},
		})
		require.Len(t, batch.moves, 2)
		assert.Equal(t, plannedMove{key: "docs/a.md", changesDir: true}, batch.moves[0])
		assert.Equal(t, "docs/b.md", batch.moves[1].key)
		assert.Len(t, batch.edits["docs/a.md"], 1, "the stem rewrite; `b.md` still resolves from x/y/")
		assert.Len(t, batch.stems["docs/a.md"], 1)
	})
	t.Run("skips empty, unchanged, repeated, and unreadable pairs", func(t *testing.T) {
		t.Parallel()
		pair := fileRename{OldURI: uri("docs/b.md"), NewURI: uri("docs/c.md")}
		batch := planRenameBatch(ws, root, []fileRename{
			pair, pair,
			{OldURI: uri("docs/a.md"), NewURI: uri("docs/a.md")},
			{OldURI: "untitled:x", NewURI: uri("docs/z.md")},
			{OldURI: uri("gone.md"), NewURI: uri("gone2.md")},
		})
		require.Len(t, batch.moves, 1)
		assert.False(t, batch.moves[0].changesDir)
	})
	t.Run("a planned move keeps the edits inside it", func(t *testing.T) {
		t.Parallel()
		batch := planRenameBatch(ws, root, []fileRename{
			{OldURI: uri("docs/a.md"), NewURI: uri("other/a.md")},
		})
		require.Len(t, batch.moves, 1)
		assert.Len(t, batch.moves[0].own, 1)
		assert.Equal(t, batch.edits["docs/a.md"], batch.moves[0].own)
	})
	t.Run("own holds only the moved file's outbound edits", func(t *testing.T) {
		t.Parallel()
		batch := planRenameBatch(ws, root, []fileRename{
			{OldURI: uri("docs/a.md"), NewURI: uri("docs/z/a.md")},
			{OldURI: uri("docs/b.md"), NewURI: uri("docs/z/c.md")},
		})
		require.Len(t, batch.moves, 2)
		assert.Equal(t, []string{"c", "c.md"}, editTexts(batch.edits["docs/a.md"]))
		assert.Equal(t, []string{"c.md"}, editTexts(batch.moves[0].own), "not the [[b]] stem rewrite")
	})
}

// editTexts returns the NewText of each edit, in order.
func editTexts(edits []refactor.Edit) []string {
	out := make([]string, 0, len(edits))
	for _, e := range edits {
		out = append(out, e.NewText)
	}
	return out
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

// TestServerRenameWorkspace_WikilinkIndex locks that every rename
// workspace the server builds carries a wikilink index walked once, at
// the root its paths were spelled against, so no move path can fall
// back to counting listed files. An unreadable root builds no index.
func TestServerRenameWorkspace_WikilinkIndex(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "guide.md"), []byte("# G\n"), 0o644))
	s := New(Options{})
	s.rootDir = t.TempDir() // a config reload moved the server's root
	ws := s.renameWorkspace(root)
	idx := ws.WikilinkIndex()
	require.NotNil(t, idx)
	assert.Equal(t, []string{"guide.md"}, idx.StemPaths("guide"))
	assert.Same(t, idx, ws.WikilinkIndex(), "the walk runs once per workspace")
	assert.Nil(t, s.renameWorkspace(filepath.Join(root, "missing")).WikilinkIndex())
}

// TestGuardRenameEdits locks that the guards still drop, and count,
// what a plan should never hold: two overlapping edits, and a path
// edit inside a moved file the batch did not plan for it.
func TestGuardRenameEdits(t *testing.T) {
	t.Parallel()
	merged, dropped := guardRenameEdits(renameBatch{
		moves: []plannedMove{{key: "m", changesDir: true}},
		edits: map[string][]refactor.Edit{
			"a": {ref(1, "x"), ref(1, "y"), ref(2, "z")},
			"m": {ref(1, "p")},
		},
	})
	assert.Equal(t, map[string][]textEdit{"a": {edAt(2, 0, 4, "z")}}, merged)
	assert.Equal(t, 3, dropped)
}
