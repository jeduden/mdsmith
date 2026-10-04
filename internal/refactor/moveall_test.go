package refactor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// moveAll runs MoveAll over files and fails the test when any pair
// could not be planned.
func moveAll(t *testing.T, files map[string]string, pairs ...MovePair) BatchPlan {
	t.Helper()
	bp := MoveAll(newMemWorkspace(files), pairs)
	for _, m := range bp.Moves {
		require.NoError(t, m.Err, "%s -> %s", m.Src, m.Dst)
	}
	assertNoOverlap(t, bp.Edits)
	return bp
}

// assertNoOverlap fails when one key holds two edits whose ranges
// intersect, or two inserts at one position.
func assertNoOverlap(t *testing.T, edits map[string][]Edit) {
	t.Helper()
	for key, es := range edits {
		for i := range es {
			for j := i + 1; j < len(es); j++ {
				a, b := es[i].Range, es[j].Range
				sameInsert := a.Start == a.End && b.Start == b.End && a.Start == b.Start
				overlap := ComparePositionsBottomUp(a.Start, b.End) > 0 &&
					ComparePositionsBottomUp(b.Start, a.End) > 0
				assert.False(t, sameInsert || overlap, "%s: edits %d and %d overlap", key, i, j)
			}
		}
	}
}

// texts returns the NewText of each edit under key, in plan order.
func texts(edits map[string][]Edit, key string) []string {
	var out []string
	for _, e := range edits[key] {
		out = append(out, e.NewText)
	}
	return out
}

func TestMoveAll_SameNewFolderLeavesLinkAlone(t *testing.T) {
	bp := moveAll(t, map[string]string{
		"a.md": "# A\n\n[b](b.md)\n",
		"b.md": "# B\n\n[a](a.md)\n",
		"c.md": "# C\n\n[a](a.md) [b](b.md)\n",
	}, MovePair{"a.md", "x/a.md"}, MovePair{"b.md", "x/b.md"})
	assert.NotContains(t, bp.Edits, "a.md")
	assert.NotContains(t, bp.Edits, "b.md")
	assert.Equal(t, []string{"x/b.md", "x/a.md"}, texts(bp.Edits, "c.md"))
	assert.Zero(t, bp.Withheld)
	assert.Nil(t, bp.FileOp)
}

func TestMoveAll_DifferentNewFolders(t *testing.T) {
	bp := moveAll(t, map[string]string{
		"a.md": "# A\n\n[b](b.md)\n",
		"b.md": "# B\n\n[a](./a.md)\n",
	}, MovePair{"a.md", "x/a.md"}, MovePair{"b.md", "y/b.md"})
	assert.Equal(t, []string{"../y/b.md"}, texts(bp.Edits, "a.md"))
	assert.Equal(t, []string{"../x/a.md"}, texts(bp.Edits, "b.md"))
	assert.Zero(t, bp.Withheld)
}

func TestMoveAll_ThreeFileCycle(t *testing.T) {
	bp := moveAll(t, map[string]string{
		"a.md": "# A\n\n[b](b.md)\n",
		"b.md": "# B\n\n[c](c.md)\n",
		"c.md": "# C\n\n[a](a.md)\n",
	}, MovePair{"a.md", "p/a.md"}, MovePair{"b.md", "q/b.md"}, MovePair{"c.md", "r/s/c.md"})
	assert.Equal(t, []string{"../q/b.md"}, texts(bp.Edits, "a.md"))
	assert.Equal(t, []string{"../r/s/c.md"}, texts(bp.Edits, "b.md"))
	assert.Equal(t, []string{"../../p/a.md"}, texts(bp.Edits, "c.md"))
}

// TestMoveAll_OnlyTargetMoveRewrites covers a link only the target's
// move used to rewrite: docs/a.md's `../docs/b.md` still resolves from
// other/, but docs/b.md moves too, so the right text is spelled from
// other/ to docs/sub/b.md.
func TestMoveAll_OnlyTargetMoveRewrites(t *testing.T) {
	bp := moveAll(t, map[string]string{
		"docs/a.md": "# A\n\n[b](../docs/b.md)\n",
		"docs/b.md": "# B\n",
	}, MovePair{"docs/a.md", "other/a.md"}, MovePair{"docs/b.md", "docs/sub/b.md"})
	assert.Equal(t, []string{"../docs/sub/b.md"}, texts(bp.Edits, "docs/a.md"))
}

// TestMoveAll_KeepsRightOneSidedRewrite covers a rewrite that only one
// move plans and that is already right from the holder's new folder.
func TestMoveAll_KeepsRightOneSidedRewrite(t *testing.T) {
	bp := moveAll(t, map[string]string{
		"docs/a.md": "# A\n\n[b](../b.md)\n",
		"b.md":      "# B\n",
	}, MovePair{"docs/a.md", "other/a.md"}, MovePair{"b.md", "b2.md"})
	assert.Equal(t, []string{"../b2.md"}, texts(bp.Edits, "docs/a.md"))
}

// TestMoveAll_AgreeingRewritesFollowBothMoves covers two moves whose
// pre-batch rewrites agreed but were both wrong: docs/a.md moving to
// the root and b.md moving into docs/ need `docs/b.md`.
func TestMoveAll_AgreeingRewritesFollowBothMoves(t *testing.T) {
	bp := moveAll(t, map[string]string{
		"docs/a.md": "# A\n\n[b](../b.md)\n",
		"b.md":      "# B\n",
	}, MovePair{"docs/a.md", "a.md"}, MovePair{"b.md", "docs/b.md"})
	assert.Equal(t, []string{"docs/b.md"}, texts(bp.Edits, "docs/a.md"))
}

// TestMoveAll_Chain covers a destination another pair of the batch
// vacates: b.md moves to z.md and a.md takes its place.
func TestMoveAll_Chain(t *testing.T) {
	bp := moveAll(t, map[string]string{
		"a.md": "# A\n\n[b](b.md)\n",
		"b.md": "# B\n",
		"c.md": "# C\n\n[a](a.md) [b](b.md)\n",
	}, MovePair{"b.md", "z.md"}, MovePair{"a.md", "b.md"})
	assert.Equal(t, []string{"z.md"}, texts(bp.Edits, "a.md"))
	assert.Equal(t, []string{"z.md", "b.md"}, texts(bp.Edits, "c.md"))
}

// TestMoveAll_Swap covers two files trading places.
func TestMoveAll_Swap(t *testing.T) {
	bp := moveAll(t, map[string]string{
		"a.md": "# A\n\n[b](b.md)\n",
		"b.md": "# B\n\n[a](a.md)\n",
		"c.md": "# C\n\n[a](a.md)\n",
	}, MovePair{"a.md", "b.md"}, MovePair{"b.md", "a.md"})
	assert.Equal(t, []string{"a.md"}, texts(bp.Edits, "a.md"))
	assert.Equal(t, []string{"b.md"}, texts(bp.Edits, "b.md"))
	assert.Equal(t, []string{"b.md"}, texts(bp.Edits, "c.md"))
}

func TestMoveAll_Validation(t *testing.T) {
	ws := newMemWorkspace(map[string]string{
		"a.md": "# A\n", "b.md": "# B\n", "c.md": "# C\n", "d.md": "# D\n", "e.md": "# E\n", "taken.md": "# T\n",
	})
	bp := MoveAll(ws, []MovePair{
		{"../x.md", "y.md"},     // traversal source
		{"a.md", "./a.md"},      // same file
		{"gone.md", "g2.md"},    // unreadable source
		{"b.md", "../out.md"},   // traversal destination
		{"c.md", "taken.md"},    // destination exists
		{"c.md", "c2.md"},       // repeated source
		{"d.md", "same.md"},     // shared destination
		{"e.md", "same.md"},     // shared destination
		{"gone2.md", "../o.md"}, // traversal wins over a missing source
	})
	require.Len(t, bp.Moves, 9)
	assert.ErrorIs(t, bp.Moves[0].Err, ErrTraversalPath)
	assert.Empty(t, bp.Moves[0].Key)
	assert.ErrorIs(t, bp.Moves[1].Err, ErrSameFile)
	assert.Equal(t, SourceNotFoundError{Src: "gone.md"}, bp.Moves[2].Err)
	assert.ErrorIs(t, bp.Moves[3].Err, ErrTraversalPath)
	assert.Equal(t, "b.md", bp.Moves[3].Key, "a readable source is still a batch member")
	assert.Equal(t, DestinationExistsError{Dst: "taken.md"}, bp.Moves[4].Err)
	assert.ErrorIs(t, bp.Moves[5].Err, ErrDuplicateSource)
	assert.ErrorIs(t, bp.Moves[6].Err, ErrDuplicateDestination)
	assert.ErrorIs(t, bp.Moves[7].Err, ErrDuplicateDestination)
	assert.ErrorIs(t, bp.Moves[8].Err, ErrTraversalPath)
}

// TestMoveAll_SingleMatchesMove locks that a one-pair batch plans the
// same edits Move does.
func TestMoveAll_SingleMatchesMove(t *testing.T) {
	files := map[string]string{
		"docs/a.md": "# A\n\n[b](b.md) [[b]] [self](a.md)\n",
		"docs/b.md": "# B\n\n[a](a.md)\n",
	}
	ws := newMemWorkspace(files)
	p, err := Move(ws, "docs/a.md", "x/c.md")
	require.NoError(t, err)
	bp := MoveAll(ws, []MovePair{{"docs/a.md", "x/c.md"}})
	require.NoError(t, bp.Moves[0].Err)
	assert.Equal(t, p.Edits, bp.Edits)
	assert.Equal(t, &FileOp{From: "docs/a.md", To: "x/c.md"}, p.FileOp)
}
