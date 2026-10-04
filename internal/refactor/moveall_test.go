package refactor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/linkgraph"
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
	out := make([]string, 0, len(edits[key]))
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

func TestMoveAll_WikilinksBetweenMovedFiles(t *testing.T) {
	bp := moveAll(t, map[string]string{
		"a.md": "# A\n\nSee [[b]] and [b](b.md).\n",
		"b.md": "# B\n\nSee [[a]].\n",
	}, MovePair{"a.md", "x/a2.md"}, MovePair{"b.md", "y/b2.md"})
	assert.Equal(t, []string{"../y/b2.md", "b2"}, texts(bp.Edits, "a.md"))
	assert.Equal(t, []string{"a2"}, texts(bp.Edits, "b.md"))
	assert.Equal(t, []string{"b2"}, texts(bp.StemEdits, "a.md"))
	assert.Zero(t, bp.Withheld)
}

// TestMoveAll_SharedNewStem covers two moves landing on one new stem:
// only the move whose destination wins the stem after the batch
// rewrites its links, and the other's are withheld and counted.
func TestMoveAll_SharedNewStem(t *testing.T) {
	bp := moveAll(t, map[string]string{
		"x/a.md": "# A\n",
		"y/b.md": "# B\n",
		"n.md":   "# N\n\n[[a]] [[b]]\n",
	}, MovePair{"x/a.md", "x/c.md"}, MovePair{"y/b.md", "y/c.md"})
	assert.Equal(t, []string{"c"}, texts(bp.Edits, "n.md"))
	assert.Equal(t, 1, bp.Withheld)
}

// TestMoveAll_StemAndSiblingShareNewStem covers the file `[[Guide]]`
// reaches and its same-stem sibling both moving to one new stem: the
// sibling's destination wins `[[Manual]]`, so the bare link is
// withheld, while `[[ref/Guide]]`, which names the sibling, follows it.
func TestMoveAll_StemAndSiblingShareNewStem(t *testing.T) {
	bp := moveAll(t, map[string]string{
		"docs/Guide.md": "# G\n",
		"ref/Guide.md":  "# R\n",
		"n.md":          "# N\n\n[[Guide]] [[ref/Guide]]\n",
	}, MovePair{"docs/Guide.md", "z/Manual.md"}, MovePair{"ref/Guide.md", "a/Manual.md"})
	assert.Equal(t, []string{"Manual"}, texts(bp.Edits, "n.md"))
	require.Len(t, bp.Edits["n.md"], 1)
	assert.Equal(t, 16, bp.Edits["n.md"][0].Range.Start.Character, "the ref/Guide link")
	assert.Equal(t, 1, bp.Withheld)
}

// TestMoveAll_FolderPrefixedStemFollowsSibling covers two moves
// leaving a shared stem: `[[y/guide]]` names the sibling's folder, so
// it follows y/guide.md, not the file the bare stem reaches.
func TestMoveAll_FolderPrefixedStemFollowsSibling(t *testing.T) {
	bp := moveAll(t, map[string]string{
		"x/guide.md": "# X\n",
		"y/guide.md": "# Y\n",
		"n.md":       "# N\n\n[[guide]] [[y/guide]]\n",
	}, MovePair{"x/guide.md", "x/manual.md"}, MovePair{"y/guide.md", "y/howto.md"})
	assert.Equal(t, []string{"howto", "manual"}, texts(bp.Edits, "n.md"))
	assert.Zero(t, bp.Withheld)
}

// TestMoveAll_SiblingKeepingStemLeavesPrefixedLink covers a named
// sibling whose move keeps its stem: its link still reaches it, so it
// is left as written.
func TestMoveAll_SiblingKeepingStemLeavesPrefixedLink(t *testing.T) {
	bp := moveAll(t, map[string]string{
		"x/guide.md": "# X\n",
		"y/guide.md": "# Y\n",
		"n.md":       "# N\n\n[[guide]] [[y/Guide]]\n",
	}, MovePair{"x/guide.md", "x/manual.md"}, MovePair{"y/guide.md", "z/guide.md"})
	assert.Equal(t, []string{"manual"}, texts(bp.Edits, "n.md"))
	assert.Zero(t, bp.Withheld)
}

// TestMoveAll_UnmovedStemHolderIsNotCounted locks that a new stem a
// file outside the batch already wins blocks the rewrite silently, as
// Move always has: no batch member caused it.
func TestMoveAll_UnmovedStemHolderIsNotCounted(t *testing.T) {
	bp := moveAll(t, map[string]string{
		"x/a.md": "# A\n",
		"c.md":   "# C\n",
		"n.md":   "# N\n\n[[a]]\n",
	}, MovePair{"x/a.md", "x/c.md"})
	assert.NotContains(t, bp.Edits, "n.md")
	assert.Zero(t, bp.Withheld)
}

// TestMoveAll_TypedNameSharedByTwoMoves covers two moves to one
// non-Markdown name: `[[logo.png]]` reaches the shallower destination,
// so the other move's link is withheld.
func TestMoveAll_TypedNameSharedByTwoMoves(t *testing.T) {
	bp := moveAll(t, map[string]string{
		"x/a.md":   "# A\n",
		"y/z/b.md": "# B\n",
		"n.md":     "# N\n\n[[a]] [[b]]\n",
	}, MovePair{"x/a.md", "x/logo.png"}, MovePair{"y/z/b.md", "y/z/logo.png"})
	assert.Equal(t, []string{"logo.png"}, texts(bp.Edits, "n.md"))
	assert.Equal(t, 1, bp.Withheld)
}

// TestMoveAll_UnplannedTargetCounted covers a rewrite that would
// assume an unplanned move stayed put: docs/b.md moves onto the
// existing x/b.md, which Move refuses but the host still performs, so
// docs/a.md's `b.md` gets no edit and is counted.
func TestMoveAll_UnplannedTargetCounted(t *testing.T) {
	bp := MoveAll(newMemWorkspace(map[string]string{
		"docs/a.md": "# A\n\n[b](b.md)\n",
		"docs/b.md": "# B\n",
		"x/b.md":    "# Old\n",
	}), []MovePair{{"docs/a.md", "other/a.md"}, {"docs/b.md", "x/b.md"}})
	require.NoError(t, bp.Moves[0].Err)
	assert.Equal(t, DestinationExistsError{Dst: "x/b.md"}, bp.Moves[1].Err)
	assert.NotContains(t, bp.Edits, "docs/a.md")
	assert.Equal(t, 1, bp.Withheld)
}

// TestMoveAll_UnplannedHolder covers a link inside a file whose move
// was not planned: it is counted only when it stops resolving.
func TestMoveAll_UnplannedHolder(t *testing.T) {
	files := map[string]string{
		"docs/a.md": "# A\n\n[b](b.md)\n",
		"docs/b.md": "# B\n",
		"x/y/a.md":  "# Old\n",
	}
	for dst, want := range map[string]int{"x/y/b.md": 0, "z/b.md": 1} {
		t.Run(dst, func(t *testing.T) {
			bp := MoveAll(newMemWorkspace(files),
				[]MovePair{{"docs/a.md", "x/y/a.md"}, {"docs/b.md", dst}})
			require.Error(t, bp.Moves[0].Err)
			require.NoError(t, bp.Moves[1].Err)
			assert.NotContains(t, bp.Edits, "docs/a.md")
			assert.Equal(t, want, bp.Withheld)
		})
	}
}

// TestMoveAll_TargetLeavesWorkspace covers a member moved out of the
// workspace: a link to it from another moved file can never resolve.
func TestMoveAll_TargetLeavesWorkspace(t *testing.T) {
	bp := MoveAll(newMemWorkspace(map[string]string{
		"a.md": "# A\n\n[b](b.md)\n",
		"b.md": "# B\n",
	}), []MovePair{{"a.md", "x/a.md"}, {"b.md", "../out.md"}})
	assert.ErrorIs(t, bp.Moves[1].Err, ErrTraversalPath)
	assert.NotContains(t, bp.Edits, "a.md")
	assert.Equal(t, 1, bp.Withheld)
}

// TestDestResolver_CountsNeedABatch locks that a pass run without a
// batch counts nothing.
func TestDestResolver_CountsNeedABatch(t *testing.T) {
	r := &destResolver{}
	r.countStale("a.md", "b.md", "c.md")
	r.countBlocked(holderIndex("c.md"), stemTarget{dst: "z/c.md", key: "c", isStem: true})
	assert.Nil(t, r.batch)
}

func TestNewStemTarget(t *testing.T) {
	got, ok := newStemTarget("guide", "docs/Manual.md")
	require.True(t, ok)
	assert.Equal(t, stemTarget{dst: "docs/Manual.md", spelling: "Manual", key: "manual", isStem: true}, got)
	got, ok = newStemTarget("guide", "img/Logo.png")
	require.True(t, ok)
	assert.Equal(t, "logo.png", got.key)
	assert.False(t, got.isStem)
	_, ok = newStemTarget("guide", "other/Guide.md")
	assert.False(t, ok, "a kept stem needs no rewrite")
	_, ok = newStemTarget("guide", "node_modules/x.md")
	assert.False(t, ok, "an unindexed destination is unreachable")
	_, ok = newStemTarget("guide", "a#b.md")
	assert.False(t, ok, "no token reaches the name")
}

func TestStemTarget_Reaches(t *testing.T) {
	post := holderIndex("a/manual.md", "a/logo.png")
	assert.True(t, stemTarget{dst: "manual.md", key: "manual", isStem: true}.reaches(post))
	assert.False(t, stemTarget{dst: "z/manual.md", key: "manual", isStem: true}.reaches(post))
	assert.True(t, stemTarget{dst: "logo.png", key: "logo.png"}.reaches(post))
	assert.False(t, stemTarget{dst: "z/logo.png", key: "logo.png"}.reaches(post))
}

func TestDestResolver_SiblingTarget(t *testing.T) {
	r := &destResolver{batch: &moveBatch{members: map[string]batchMember{
		"y/guide.md": {dst: "y/howto.md", planned: true},
		"z/guide.md": {dst: "q/guide.md", planned: true},
		"w/guide.md": {dst: "w/other.md"},
	}}}
	got, ok := r.siblingTarget("guide", "y/guide.md")
	require.True(t, ok)
	assert.Equal(t, "y/howto.md", got.dst)
	_, ok = r.siblingTarget("guide", "z/guide.md")
	assert.False(t, ok, "a sibling keeping its stem")
	_, ok = r.siblingTarget("guide", "w/guide.md")
	assert.False(t, ok, "an unplanned sibling")
	_, ok = r.siblingTarget("guide", "v/guide.md")
	assert.False(t, ok, "a sibling the batch does not move")
}

func TestDestResolver_Member(t *testing.T) {
	_, ok := (&destResolver{}).member("a.md")
	assert.False(t, ok, "no batch, no members")
	r := &destResolver{batch: &moveBatch{members: map[string]batchMember{"a.md": {dst: "b.md", planned: true}}}}
	m, ok := r.member("a.md")
	assert.True(t, ok)
	assert.Equal(t, batchMember{dst: "b.md", planned: true}, m)
}

// wikilinkIndexCountingWorkspace counts WikilinkIndex calls.
type wikilinkIndexCountingWorkspace struct {
	stubWorkspace
	calls *int
	idx   *linkgraph.WikilinkIndex
}

func (w wikilinkIndexCountingWorkspace) WikilinkIndex() *linkgraph.WikilinkIndex {
	*w.calls++
	return w.idx
}

func TestDestResolver_WikilinkIndex(t *testing.T) {
	calls := 0
	idx := holderIndex("a.md")
	r := &destResolver{ws: wikilinkIndexCountingWorkspace{calls: &calls, idx: idx}}
	assert.Same(t, idx, r.wikilinkIndex())
	assert.Same(t, idx, r.wikilinkIndex())
	assert.Equal(t, 1, calls, "read once per resolver")

	r = &destResolver{ws: wikilinkIndexCountingWorkspace{
		stubWorkspace: stubWorkspace{files: []string{"./docs/b.md"}}, calls: &calls,
	}}
	assert.Equal(t, []string{"docs/b.md"}, r.wikilinkIndex().StemPaths("b"), "a nil index falls back to Files")
}

func TestDestResolver_PostIndex(t *testing.T) {
	idx := holderIndex("a.md", "b.md")
	alone := (&destResolver{}).postIndex(idx, "a.md", "x/c.md")
	assert.Equal(t, []string{"x/c.md"}, alone.StemPaths("c"))
	assert.Empty(t, alone.StemPaths("a"))

	r := &destResolver{batch: &moveBatch{members: map[string]batchMember{
		"a.md": {dst: "x/c.md", planned: true},
		"b.md": {},
	}}}
	post := r.postIndex(idx, "a.md", "x/c.md")
	assert.Same(t, post, r.postIndex(idx, "a.md", "x/c.md"), "built once per batch")
	assert.Empty(t, post.StemPaths("b"), "a member leaving the workspace is removed")
}

func TestResolves(t *testing.T) {
	ws := newMemWorkspace(map[string]string{"a.md": "# A\n"})
	assert.True(t, resolves(ws, "a.md"))
	assert.False(t, resolves(ws, "b.md"))
}

func TestMoveBatch_Admit(t *testing.T) {
	ws := newMemWorkspace(map[string]string{"a.md": "# A\n"})
	b := &moveBatch{members: map[string]batchMember{}}
	landing := map[string]int{}
	m := b.admit(ws, MovePair{"./a.md", "x/a.md"}, landing)
	assert.Equal(t, BatchMove{Src: "a.md", Dst: "x/a.md", Key: "a.md"}, m)
	assert.Equal(t, batchMember{dst: "x/a.md"}, b.members["a.md"], "planned only once validated")
	assert.Equal(t, 1, landing["x/a.md"])
	assert.ErrorIs(t, b.admit(ws, MovePair{"a.md", "y.md"}, landing).Err, ErrDuplicateSource)
}
