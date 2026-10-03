package lsp

import (
	"math/rand/v2"
	"slices"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
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

// rangesOverlap is the pairwise oracle for dropConflictingTextEdits:
// edits over a and b touch the same text when their spans intersect or
// both start at the same position. Ranges that merely touch
// end-to-start do not overlap.
func rangesOverlap(a, b Range) bool {
	if a.Start == b.Start {
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
}

func TestPosLess(t *testing.T) {
	t.Parallel()
	assert.True(t, posLess(Position{Line: 1, Character: 9}, Position{Line: 2, Character: 0}))
	assert.True(t, posLess(Position{Line: 1, Character: 2}, Position{Line: 1, Character: 3}))
	assert.False(t, posLess(Position{Line: 1, Character: 3}, Position{Line: 1, Character: 3}))
	assert.False(t, posLess(Position{Line: 2, Character: 0}, Position{Line: 1, Character: 9}))
}
