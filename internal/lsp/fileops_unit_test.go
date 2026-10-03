package lsp

import (
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
	t.Run("identical duplicates collapse to one", func(t *testing.T) {
		t.Parallel()
		got := dropConflictingTextEdits([]textEdit{edAt(1, 0, 3, "a"), edAt(1, 0, 3, "a")})
		assert.Equal(t, []textEdit{edAt(1, 0, 3, "a")}, got)
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
