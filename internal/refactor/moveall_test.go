package refactor

import (
	"path"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/linkgraph"
)

// soloResolver returns a resolver for a batch of one planned move,
// src to dst, as Move builds it.
func soloResolver(ws MoveWorkspace, src, dst string) *destResolver {
	return &destResolver{ws: ws, batch: &moveBatch{members: map[string]batchMember{src: {dst: dst, planned: true}}}}
}

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

// TestMoveAll_StemAndPathEditsInMovedFile locks that a moved file gets
// both its outbound path rewrite and the `[[stem]]` rewrite another
// move plans inside it, one edit each.
func TestMoveAll_StemAndPathEditsInMovedFile(t *testing.T) {
	bp := moveAll(t, map[string]string{
		"a.md": "# A\n\n[[b]] [b](b.md)\n",
		"b.md": "# B\n",
	}, MovePair{"a.md", "x/a.md"}, MovePair{"b.md", "y/c.md"})
	assert.Equal(t, []string{"../y/c.md", "c"}, texts(bp.Edits, "a.md"))
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

// TestMoveAll_ChainOntoRefusedMove covers a planned pair landing on a
// path whose own move was refused: the host still vacates it, so every
// link to the refused file then reaches the newcomer. No rule can flag
// such a link, since it still resolves, so each one is counted.
func TestMoveAll_ChainOntoRefusedMove(t *testing.T) {
	bp := MoveAll(newMemWorkspace(map[string]string{
		"a.md": "# A\n",
		"b.md": "# B\n",
		"c.md": "# C\n",
		"n.md": "# N\n\n[b](b.md) [[b]] [a](a.md)\n",
	}), []MovePair{{"b.md", "c.md"}, {"a.md", "b.md"}})
	assert.Equal(t, DestinationExistsError{Dst: "c.md"}, bp.Moves[0].Err)
	require.NoError(t, bp.Moves[1].Err)
	assert.Equal(t, []string{"b.md"}, texts(bp.Edits, "n.md"))
	assert.Equal(t, 2, bp.Withheld)

	// A planned holder's link is counted once, by its outbound pass.
	bp = MoveAll(newMemWorkspace(map[string]string{
		"a.md": "# A\n",
		"b.md": "# B\n",
		"c.md": "# C\n",
		"d.md": "# D\n\n[b](b.md)\n",
	}), []MovePair{{"b.md", "c.md"}, {"a.md", "b.md"}, {"d.md", "e.md"}})
	require.NoError(t, bp.Moves[2].Err)
	assert.Equal(t, 1, bp.Withheld)
}

// TestMoveAll_ShadowedSelfLink covers a refused file's links to
// itself when a planned member takes its path. Once the host moves it,
// a path link and a `[[stem]]` link alike reach the newcomer, so both
// are counted; a path link that still names the file from where it
// lands is not.
func TestMoveAll_ShadowedSelfLink(t *testing.T) {
	for _, tc := range []struct {
		name, self, taken string
		want              int
	}{
		{"path", "[self](b.md)", "c.md", 1},
		{"stem", "[[b]]", "c.md", 1},
		{"path still names it", "[self](b.md)", "x/b.md", 0},
		{"stem reaches newcomer", "[[b]]", "x/b.md", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bp := MoveAll(newMemWorkspace(map[string]string{
				"a.md":   "# A\n",
				"b.md":   "# B\n\n" + tc.self + "\n",
				tc.taken: "# T\n",
			}), []MovePair{{"b.md", tc.taken}, {"a.md", "b.md"}})
			assert.Equal(t, DestinationExistsError{Dst: tc.taken}, bp.Moves[0].Err)
			require.NoError(t, bp.Moves[1].Err)
			assert.Equal(t, tc.want, bp.Withheld)
		})
	}
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

// TestMoveAll_SingleMatchesMove locks what a one-pair batch plans, the
// plan Move returns: the moved file's outbound link and self-link
// re-spelled from its new folder, the incoming link repointed, and
// `[[b]]`, which names another file, left alone. Move is MoveAll with
// one pair, so comparing the two alone would lock nothing.
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
	assert.Equal(t, []string{"c.md", "../docs/b.md"}, texts(bp.Edits, "docs/a.md"))
	assert.Equal(t, []string{"../x/c.md"}, texts(bp.Edits, "docs/b.md"))
	assert.Len(t, bp.Edits, 2)
	assert.Zero(t, bp.Withheld)
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

// TestMoveAll_CaseVariantDestinationsRefused covers two moves landing
// on destinations that differ in letter case alone: on a
// case-insensitive file system they are one file, so neither move is
// planned, as for two pairs naming one destination.
func TestMoveAll_CaseVariantDestinationsRefused(t *testing.T) {
	bp := MoveAll(newMemWorkspace(map[string]string{
		"x/a.md": "# A\n",
		"y/b.md": "# B\n",
		"n.md":   "# N\n\n[[a]] [[b]] [a](x/a.md)\n",
	}), []MovePair{{"x/a.md", "docs/c.md"}, {"y/b.md", "Docs/C.md"}})
	assert.ErrorIs(t, bp.Moves[0].Err, ErrDuplicateDestination)
	assert.ErrorIs(t, bp.Moves[1].Err, ErrDuplicateDestination)
	assert.Empty(t, bp.Edits)
}

func TestFoldPath(t *testing.T) {
	assert.Equal(t, foldPath("docs/c.md"), foldPath("Docs/C.MD"))
	assert.Equal(t, foldPath("k.md"), foldPath("\u212a.md"), "the Kelvin sign folds to k")
	assert.Equal(t, foldPath("\u03c3.md"), foldPath("\u03c2.md"), "final sigma folds to sigma")
	assert.NotEqual(t, foldPath("a.md"), foldPath("b.md"))
	assert.True(t, strings.EqualFold("\u03a3.md", "\u03c2.md"))
	assert.Equal(t, foldPath("\u03a3.md"), foldPath("\u03c2.md"), "as strings.EqualFold reads them")
}

// partialIndexWorkspace is a memWorkspace whose wikilink index holds
// only indexed, as a walk that skips a symlinked or unreadable
// directory leaves the rest out.
type partialIndexWorkspace struct {
	*memWorkspace
	indexed []string
}

func (w partialIndexWorkspace) WikilinkIndex() *linkgraph.WikilinkIndex {
	return holderIndex(w.indexed...)
}

// TestMoveAll_UnindexedSameStemSourcesPlanOneEdit covers two members
// sharing a stem that the wikilink index lacks, wholly or in part:
// only the one that sorts first wins `[[guide]]`, so each link gets
// one edit, not one per member.
func TestMoveAll_UnindexedSameStemSourcesPlanOneEdit(t *testing.T) {
	for name, indexed := range map[string][]string{
		"neither indexed": {"n.md"},
		"one indexed":     {"n.md", "y/guide.md"},
	} {
		t.Run(name, func(t *testing.T) {
			bp := MoveAll(partialIndexWorkspace{memWorkspace: newMemWorkspace(map[string]string{
				"x/guide.md": "# X\n",
				"y/guide.md": "# Y\n",
				"n.md":       "# N\n\n[[guide]]\n",
			}), indexed: indexed}, []MovePair{{"x/guide.md", "x/manual.md"}, {"y/guide.md", "y/howto.md"}})
			assertNoOverlap(t, bp.Edits)
			assert.Equal(t, []string{"manual"}, texts(bp.Edits, "n.md"))
		})
	}
}

func TestDestResolver_WinsKey(t *testing.T) {
	b := newMoveBatch()
	b.members["x/guide.md"] = batchMember{dst: "x/manual.md", planned: true}
	b.members["y/guide.md"] = batchMember{dst: "y/howto.md", planned: true}
	r := &destResolver{batch: b}
	none := holderIndex()
	assert.True(t, r.winsKey(none, stemKey("guide"), "x/guide.md"))
	assert.False(t, r.winsKey(none, stemKey("guide"), "y/guide.md"), "a member source outsorts it")
	assert.False(t, r.winsKey(holderIndex("guide.md"), stemKey("guide"), "x/guide.md"),
		"an indexed holder outsorts it")
	assert.True(t, r.winsKey(none, stemKey("other"), "z/other.md"), "no other member holds the stem")
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

// TestMoveAll_UnplannedTargetOnItsDestinationCounted covers a link to
// an unplanned move that, read from the holder's new folder, names the
// refused destination: docs/p.md's `r.md` reaches x/r.md from x/,
// which is docs/r.md only if the host overwrites it and the old x/r.md
// otherwise. No spelling is right both ways, so it is counted.
func TestMoveAll_UnplannedTargetOnItsDestinationCounted(t *testing.T) {
	bp := MoveAll(newMemWorkspace(map[string]string{
		"docs/p.md": "# P\n\n[r](r.md)\n",
		"docs/r.md": "# R\n",
		"x/r.md":    "# Old\n",
	}), []MovePair{{"docs/p.md", "x/p.md"}, {"docs/r.md", "x/r.md"}})
	require.NoError(t, bp.Moves[0].Err)
	assert.Equal(t, DestinationExistsError{Dst: "x/r.md"}, bp.Moves[1].Err)
	assert.NotContains(t, bp.Edits, "docs/p.md")
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

// TestMoveAll_UnplannedHolderInSameFolder covers a link inside a file
// whose move was refused but lands in its own folder: the link reads
// the same from there whether or not the host moves the file, so the
// rewrite spelled from that folder is planned, not withheld.
func TestMoveAll_UnplannedHolderInSameFolder(t *testing.T) {
	bp := MoveAll(newMemWorkspace(map[string]string{
		"docs/a.md": "# A\n\n[b](b.md)\n",
		"docs/b.md": "# B\n",
		"docs/c.md": "# C\n",
	}), []MovePair{{"docs/a.md", "docs/c.md"}, {"docs/b.md", "z/b.md"}})
	require.Error(t, bp.Moves[0].Err)
	require.NoError(t, bp.Moves[1].Err)
	assert.Equal(t, []string{"../z/b.md"}, texts(bp.Edits, "docs/a.md"))
	assert.Zero(t, bp.Withheld)
}

// TestMoveAll_KeptStemTakenByMember covers a move that keeps its stem
// while another member lands on a shallower file with that stem: every
// bare `[[guide]]` then reaches the newcomer, so it is counted.
func TestMoveAll_KeptStemTakenByMember(t *testing.T) {
	bp := moveAll(t, map[string]string{
		"x/guide.md": "# G\n",
		"q/other.md": "# O\n",
		"n.md":       "# N\n\n[[guide]] [[other]]\n",
	}, MovePair{"x/guide.md", "x/sub/guide.md"}, MovePair{"q/other.md", "guide.md"})
	assert.Equal(t, []string{"guide"}, texts(bp.Edits, "n.md"), "only [[other]] follows its file")
	assert.Equal(t, 1, bp.Withheld)
}

// TestMoveAll_KeptStemStillWins covers a kept stem another member also
// lands on, deeper: dst still wins it, so nothing is rewritten or
// counted.
func TestMoveAll_KeptStemStillWins(t *testing.T) {
	bp := moveAll(t, map[string]string{
		"x/guide.md": "# G\n",
		"q/other.md": "# O\n",
		"n.md":       "# N\n\n[[guide]]\n",
	}, MovePair{"x/guide.md", "a/guide.md"}, MovePair{"q/other.md", "z/z/guide.md"})
	assert.NotContains(t, bp.Edits, "n.md")
	assert.Zero(t, bp.Withheld)
}

// TestMoveAll_KeptStemFollowsRenamedSibling covers the file `[[guide]]`
// reaches keeping its stem while the batch renames a same-stem
// sibling: `[[y/guide]]`, which names the sibling, follows it.
func TestMoveAll_KeptStemFollowsRenamedSibling(t *testing.T) {
	bp := moveAll(t, map[string]string{
		"x/guide.md": "# X\n",
		"y/guide.md": "# Y\n",
		"n.md":       "# N\n\n[[guide]] [[y/guide]]\n",
	}, MovePair{"x/guide.md", "z/guide.md"}, MovePair{"y/guide.md", "y/howto.md"})
	assert.Equal(t, []string{"howto"}, texts(bp.Edits, "n.md"))
	assert.Zero(t, bp.Withheld)
}

// TestMoveAll_NewcomerTakesBlockedStem covers a `[[b]]` whose rewrite
// a file outside the batch blocks (c.md wins `c`) while another member
// lands on the path b.md vacates: the link, left as written, then
// reaches the newcomer and still resolves, so no rule can flag it. A
// lone move would leave it dangling instead, so it is counted.
func TestMoveAll_NewcomerTakesBlockedStem(t *testing.T) {
	bp := moveAll(t, map[string]string{
		"a.md": "# A\n",
		"b.md": "# B\n",
		"c.md": "# C\n",
		"n.md": "# N\n\n[[b]]\n",
	}, MovePair{"b.md", "x/c.md"}, MovePair{"a.md", "b.md"})
	assert.NotContains(t, bp.Edits, "n.md")
	assert.Equal(t, 1, bp.Withheld)
}

// TestMoveAll_NamedSiblingOutsortedByMember covers a `[[y/b]]` left as
// written for the sibling y/b.md it names: once the batch has run,
// x/z/b.md lands at x/b.md and outsorts y/b.md, so the link silently
// reaches that member and is counted. Where the named sibling, or the
// file the link reached before the batch, still wins, nothing is.
func TestMoveAll_NamedSiblingOutsortedByMember(t *testing.T) {
	files := map[string]string{
		"b.md":     "# B\n",
		"x/z/b.md": "# XZB\n",
		"y/b.md":   "# YB\n",
		"n.md":     "# N\n\n[[y/b]]\n",
	}
	for _, tc := range []struct {
		name string
		kept string // where x/z/b.md lands, keeping its stem
		want int
	}{
		{"member outsorts the sibling", "x/b.md", 1},
		{"sibling still wins", "x/q/b.md", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bp := moveAll(t, files, MovePair{"x/z/b.md", tc.kept}, MovePair{"b.md", "a.md"})
			assert.NotContains(t, bp.Edits, "n.md")
			assert.Equal(t, tc.want, bp.Withheld)
		})
	}
	// b.md keeps its stem and still outsorts y/b.md: the link reaches
	// the file it reached before the batch, so it is not counted.
	bp := moveAll(t, files, MovePair{"b.md", "a/b.md"}, MovePair{"x/z/b.md", "x/z/q.md"})
	assert.Zero(t, bp.Withheld)
}

func TestDestResolver_KeptWikilinkTarget(t *testing.T) {
	batch := func(members map[string]batchMember) *destResolver {
		return &destResolver{batch: &moveBatch{members: members}}
	}
	self := batchMember{dst: "z/guide.md", planned: true}
	want := wikilinkTarget{dst: "z/guide.md", wikilinkKey: stemKey("guide")}

	_, ok := soloResolver(nil, "x/guide.md", "z/guide.md").keptWikilinkTarget(stemKey("guide"), "z/guide.md")
	assert.False(t, ok, "a lone move")
	_, ok = batch(map[string]batchMember{"x/guide.md": self, "a.md": {dst: "b.md"}}).
		keptWikilinkTarget(stemKey("guide"), "z/guide.md")
	assert.False(t, ok, "no other member holds the stem")
	r := batch(map[string]batchMember{"x/guide.md": self, "a.md": {dst: "guide.md"}})
	got, ok := r.keptWikilinkTarget(stemKey("guide"), "z/guide.md")
	require.True(t, ok, "another member lands on the stem")
	assert.Equal(t, want, got)
	got, ok = batch(map[string]batchMember{"x/guide.md": self, "y/Guide.md": {dst: "y/howto.md"}}).
		keptWikilinkTarget(stemKey("guide"), "z/guide.md")
	require.True(t, ok, "another member leaves the stem")
	assert.Equal(t, want, got)
	_, ok = r.keptWikilinkTarget(stemKey("guide"), "z/manual.md")
	assert.False(t, ok, "a new stem")
	_, ok = r.keptWikilinkTarget(stemKey("guide"), "node_modules/guide.md")
	assert.False(t, ok, "an unindexed destination")
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

func TestNewWikilinkTarget(t *testing.T) {
	got, ok := newWikilinkTarget(stemKey("guide"), "docs/Manual.md")
	require.True(t, ok)
	assert.Equal(t, wikilinkTarget{dst: "docs/Manual.md", spelling: "Manual", wikilinkKey: stemKey("manual")}, got)
	got, ok = newWikilinkTarget(stemKey("guide"), "img/Logo.png")
	require.True(t, ok)
	assert.Equal(t, "logo.png", got.key)
	assert.False(t, got.isStem)
	_, ok = newWikilinkTarget(stemKey("guide"), "other/Guide.md")
	assert.False(t, ok, "a kept stem needs no rewrite")
	_, ok = newWikilinkTarget(stemKey("guide"), "node_modules/x.md")
	assert.False(t, ok, "an unindexed destination is unreachable")
	_, ok = newWikilinkTarget(stemKey("guide"), "a#b.md")
	assert.False(t, ok, "no token reaches the name")
}

func TestWikilinkTarget_Reaches(t *testing.T) {
	post := holderIndex("a/manual.md", "a/logo.png")
	assert.True(t, wikilinkTarget{dst: "manual.md", wikilinkKey: stemKey("manual")}.reaches(post))
	assert.False(t, wikilinkTarget{dst: "z/manual.md", wikilinkKey: stemKey("manual")}.reaches(post))
	assert.True(t, wikilinkTarget{dst: "logo.png", wikilinkKey: nameKey("logo.png")}.reaches(post))
	assert.False(t, wikilinkTarget{dst: "z/logo.png", wikilinkKey: nameKey("logo.png")}.reaches(post))
}

func TestDestResolver_SiblingTarget(t *testing.T) {
	r := &destResolver{batch: &moveBatch{members: map[string]batchMember{
		"y/guide.md": {dst: "y/howto.md", planned: true},
		"z/guide.md": {dst: "q/guide.md", planned: true},
		"w/guide.md": {dst: "w/other.md"},
	}}}
	got, ok := r.siblingTarget(stemKey("guide"), "y/guide.md")
	require.True(t, ok)
	assert.Equal(t, "y/howto.md", got.dst)
	_, ok = r.siblingTarget(stemKey("guide"), "z/guide.md")
	assert.False(t, ok, "a sibling keeping its stem")
	_, ok = r.siblingTarget(stemKey("guide"), "w/guide.md")
	assert.False(t, ok, "an unplanned sibling")
	_, ok = r.siblingTarget(stemKey("guide"), "v/guide.md")
	assert.False(t, ok, "a sibling the batch does not move")
}

func TestDestResolver_Stolen(t *testing.T) {
	r := &destResolver{batch: &moveBatch{dsts: map[string]bool{"b.md": true, "a/b.md": true}}}
	post := linkgraph.NewWikilinkIndexFromPaths([]string{"b.md", "y/b.md"})
	assert.True(t, r.stolen(post, stemKey("b"), "x/c.md", "y/b.md"), "a member's destination wins")
	assert.False(t, r.stolen(post, stemKey("b"), "b.md", "y/b.md"), "the moving file's own landing wins")
	assert.False(t, r.stolen(post, stemKey("b"), "x/c.md", "b.md"), "the file the link names wins")
	assert.False(t, r.stolen(post, stemKey("q"), "x/c.md", "y/b.md"), "no file holds the stem")
	unmoved := linkgraph.NewWikilinkIndexFromPaths([]string{"c/b.md", "y/b.md"})
	assert.False(t, r.stolen(unmoved, stemKey("b"), "x/c.md", "y/b.md"), "a file outside the batch wins")
}

func TestDestResolver_CountStolen(t *testing.T) {
	r := &destResolver{batch: &moveBatch{dsts: map[string]bool{"b.md": true}}}
	post := linkgraph.NewWikilinkIndexFromPaths([]string{"b.md", "y/b.md"})
	r.countStolen(post, stemKey("b"), "x/c.md", "y/b.md")
	assert.Equal(t, 1, r.batch.withheld, "a member's destination takes the link")
	r.countStolen(post, stemKey("b"), "x/c.md", "b.md")
	assert.Equal(t, 1, r.batch.withheld, "the link reaches the file it names")
}

func TestDestResolver_Landing(t *testing.T) {
	r := &destResolver{batch: &moveBatch{members: map[string]batchMember{
		"a.md": {dst: "x/a.md", planned: true},
		"b.md": {dst: "y/b.md"},
		"c.md": {},
	}}}
	assert.Equal(t, "x/a.md", r.landing("a.md"))
	assert.Equal(t, "y/b.md", r.landing("b.md"), "an unplanned member lands where the host moves it")
	assert.Empty(t, r.landing("c.md"), "a member leaving the workspace")
	assert.Equal(t, "d.md", r.landing("d.md"), "a file the batch does not move")
}

func TestDestResolver_Member(t *testing.T) {
	r := soloResolver(nil, "a.md", "b.md")
	m, ok := r.member("a.md")
	assert.True(t, ok)
	assert.Equal(t, batchMember{dst: "b.md", planned: true}, m)
	_, ok = r.member("b.md")
	assert.False(t, ok, "a destination is no member")
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
	alone := soloResolver(nil, "a.md", "x/c.md").postIndex(idx)
	assert.Equal(t, []string{"x/c.md"}, alone.StemPaths("c"))
	assert.Empty(t, alone.StemPaths("a"))

	r := &destResolver{batch: &moveBatch{members: map[string]batchMember{
		"a.md": {dst: "x/c.md", planned: true},
		"b.md": {},
	}}}
	post := r.postIndex(idx)
	assert.Same(t, post, r.postIndex(idx), "built once per batch")
	assert.Empty(t, post.StemPaths("b"), "a member leaving the workspace is removed")
}

func TestResolves(t *testing.T) {
	ws := newMemWorkspace(map[string]string{"a.md": "# A\n"})
	assert.True(t, resolves(ws, "a.md"))
	assert.False(t, resolves(ws, "b.md"))
}

func TestMoveBatch_Admit(t *testing.T) {
	ws := newMemWorkspace(map[string]string{"a.md": "# A\n"})
	b := newMoveBatch()
	landing := map[string]int{}
	m := b.admit(ws, MovePair{"./a.md", "x/A.md"}, landing)
	assert.Equal(t, BatchMove{Src: "a.md", Dst: "x/A.md", Key: "a.md"}, m)
	assert.Equal(t, batchMember{dst: "x/A.md"}, b.members["a.md"], "planned only once validated")
	assert.Equal(t, 1, landing[foldPath("X/a.MD")], "counted under its case-folded key")
	assert.Equal(t, map[string]bool{"x/A.md": true}, b.dsts)
	assert.Equal(t, []byte("# A\n"), b.sources["a.md"], "the source is read once, here")
	assert.ErrorIs(t, b.admit(ws, MovePair{"a.md", "y.md"}, landing).Err, ErrDuplicateSource)
}

func TestDestResolver_ReferrerEdit(t *testing.T) {
	dest := func(d string) inlineDest {
		row := "[x](" + d + ")"
		return inlineDest{dest: []byte(d), row: []byte(row), ps: 4}
	}
	b := newMoveBatch()
	b.members["docs/t.md"] = batchMember{dst: "z/t.md", planned: true}
	b.members["docs/v.md"] = batchMember{dst: "docs/w.md"}
	b.members["docs/h.md"] = batchMember{dst: "y/h.md"}
	b.shadowed["docs/v.md"] = true
	r := &destResolver{ws: stubWorkspace{}, batch: b}

	e, ok := r.referrerEdit(dest("t.md"), "docs/a.md", batchMember{}, false)
	require.True(t, ok, "a planned target is repointed")
	assert.Equal(t, "../z/t.md", e.NewText)
	_, ok = r.referrerEdit(dest("https://x"), "docs/a.md", batchMember{}, false)
	assert.False(t, ok, "an external link")
	_, ok = r.referrerEdit(dest("other.md"), "docs/a.md", batchMember{}, false)
	assert.False(t, ok, "a target the batch does not move")
	assert.Zero(t, b.withheld)

	_, ok = r.referrerEdit(dest("v.md"), "docs/a.md", batchMember{}, false)
	assert.False(t, ok)
	assert.Equal(t, 1, b.withheld, "a link to a shadowed path is counted")
	_, ok = r.referrerEdit(dest("v.md"), "docs/v.md", b.members["docs/v.md"], true)
	assert.False(t, ok)
	assert.Equal(t, 2, b.withheld, "the shadowed file's link to itself reaches the newcomer")
	_, ok = r.referrerEdit(dest("v.md"), "docs/v.md", batchMember{dst: "q/v.md"}, true)
	assert.False(t, ok)
	assert.Equal(t, 2, b.withheld, "a self-link that still names the file is not counted")

	_, ok = r.referrerEdit(dest("t.md"), "docs/h.md", b.members["docs/h.md"], true)
	assert.False(t, ok, "a refused holder leaving its folder")
	assert.Equal(t, 3, b.withheld, "its link stops resolving from y/")
	e, ok = r.referrerEdit(dest("t.md"), "docs/h.md", batchMember{dst: "docs/h2.md"}, true)
	require.True(t, ok, "a refused holder kept in its folder")
	assert.Equal(t, "../z/t.md", e.NewText)
}

func TestMoveBatch_KeyHolders(t *testing.T) {
	b := newMoveBatch()
	b.members["x/guide.md"] = batchMember{dst: "z/guide.md", planned: true}
	b.members["y/Guide.md"] = batchMember{dst: "y/howto.md"}
	b.members["q/other.md"] = batchMember{dst: "guide.md", planned: true}
	b.members["img/a.png"] = batchMember{}
	assert.Equal(t, 3, b.keyHolders(stemKey("guide")), "a member holding the stem at both ends counts once")
	assert.Equal(t, 1, b.keyHolders(stemKey("howto")))
	assert.Equal(t, 1, b.keyHolders(stemKey("other")))
	assert.Zero(t, b.keyHolders(stemKey("a")), "a non-Markdown member holds no stem")
	b.members["n.md"] = batchMember{dst: "guide.md"}
	assert.Equal(t, 3, b.keyHolders(stemKey("guide")), "built once, on the first call")
}

func TestMoveBatch_KeySources(t *testing.T) {
	b := newMoveBatch()
	b.members["x/guide.md"] = batchMember{dst: "z/guide.md", planned: true}
	b.members["y/Guide.md"] = batchMember{dst: "y/howto.md"}
	b.members["q/other.md"] = batchMember{dst: "guide.md", planned: true}
	assert.ElementsMatch(t, []string{"x/guide.md", "y/Guide.md"}, b.keySources(stemKey("guide")),
		"sources only; a destination holding the stem is not one")
	assert.Nil(t, b.keySources(stemKey("howto")))
}

func TestMoveBatch_ScanBases(t *testing.T) {
	b := newMoveBatch()
	b.members["x/a.md"] = batchMember{dst: "z/a.md", planned: true}
	b.members["y/a.md"] = batchMember{dst: "q/a.md", planned: true}
	b.members["b.md"] = batchMember{dst: "c.md"}
	assert.Equal(t, [][]byte{[]byte("a.md")}, b.scanBases(), "each planned base once; unplanned left out")
	b.shadowed["b.md"] = true
	assert.ElementsMatch(t, [][]byte{[]byte("a.md"), []byte("b.md")}, b.scanBases(), "a shadowed path is scanned")
}

// TestValidateBatch_Shadowed locks that a refused member whose path a
// planned member takes is recorded as shadowed, and a vacated path
// whose own move is planned is not.
func TestValidateBatch_Shadowed(t *testing.T) {
	ws := newMemWorkspace(map[string]string{
		"a.md": "# A\n", "b.md": "# B\n", "c.md": "# C\n", "d.md": "# D\n", "e.md": "# E\n",
	})
	moves, b := validateBatch(ws, []MovePair{{"b.md", "c.md"}, {"a.md", "b.md"}, {"d.md", "z.md"}, {"e.md", "d.md"}})
	require.Equal(t, DestinationExistsError{Dst: "c.md"}, moves[0].Err)
	for _, m := range moves[1:] {
		require.NoError(t, m.Err)
	}
	assert.Equal(t, map[string]bool{"b.md": true}, b.shadowed)
}

// TestMoveAll_ShadowedWithoutStem covers a shadowed file that no
// `[[stem]]` link can name: a non-Markdown file, and a Markdown file
// under a directory the wikilink index skips. Its path links are
// still counted, by the referrer scan. A typed `[[img.png]]` link
// reaches the newcomer too and is counted beside it; the skipped
// Markdown file's `[[b.md]]` never reached it and is not.
func TestMoveAll_ShadowedWithoutStem(t *testing.T) {
	for _, tc := range []struct {
		old, taken, newcomer string
		withheld             int
	}{
		{"img.png", "img2.png", "a.png", 2},
		{"node_modules/b.md", "node_modules/c.md", "node_modules/a.md", 1},
	} {
		t.Run(tc.old, func(t *testing.T) {
			bp := MoveAll(newMemWorkspace(map[string]string{
				tc.old:      "x\n",
				tc.taken:    "x\n",
				tc.newcomer: "x\n",
				"n.md":      "# N\n\n[o](" + tc.old + ") [[" + path.Base(tc.old) + "]]\n",
			}), []MovePair{{tc.old, tc.taken}, {tc.newcomer, tc.old}})
			assert.Equal(t, DestinationExistsError{Dst: tc.taken}, bp.Moves[0].Err)
			require.NoError(t, bp.Moves[1].Err)
			assert.Equal(t, tc.withheld, bp.Withheld)
		})
	}
}

func TestMoveBatch_BuildKeys(t *testing.T) {
	b := newMoveBatch()
	b.members["x/guide.md"] = batchMember{dst: "x/Guide.md"}
	b.members["a.md"] = batchMember{dst: "guide.md"}
	b.members["gone.md"] = batchMember{}
	b.buildKeys()
	assert.Equal(t, map[wikilinkKey]int{stemKey("guide"): 2, stemKey("a"): 1, stemKey("gone"): 1}, b.keys,
		"a member holding one stem at both ends counts once")
	assert.Equal(t, map[wikilinkKey][]string{
		stemKey("guide"): {"x/guide.md"}, stemKey("a"): {"a.md"}, stemKey("gone"): {"gone.md"},
	}, b.srcKeys)
	b.members["n.md"] = batchMember{}
	b.buildKeys()
	assert.NotContains(t, b.keys, stemKey("n"), "built once")
}

func TestDestResolver_CountStale(t *testing.T) {
	r := &destResolver{batch: newMoveBatch()}
	r.countStale("x/a.md", "b.md", "x/b.md")
	assert.Zero(t, r.batch.withheld, "the link still names its target")
	r.countStale("x/a.md", "b.md", "y/b.md")
	assert.Equal(t, 1, r.batch.withheld, "the link stops resolving")
	r.countStale("", "b.md", "b.md")
	r.countStale("a.md", "b.md", "")
	assert.Equal(t, 3, r.batch.withheld, "an end leaves the workspace")
}

func TestUnplannedInPlace(t *testing.T) {
	assert.True(t, unplannedInPlace(batchMember{dst: "docs/c.md"}, "docs/a.md"))
	assert.False(t, unplannedInPlace(batchMember{dst: "z/c.md"}, "docs/a.md"), "another folder")
	assert.False(t, unplannedInPlace(batchMember{dst: "docs/c.md", planned: true}, "docs/a.md"), "a planned move")
	assert.False(t, unplannedInPlace(batchMember{}, "a.md"), "a file leaving the workspace")
}

func TestCountShadowed(t *testing.T) {
	ws := newMemWorkspace(map[string]string{
		"b.md":    "# B\n",
		"x/c.md":  "# C\n",
		"c.md":    "# C\n",
		"img.png": "png",
		"x/a.png": "png",
		"n.md":    "# N\n\n[[b]] [[x/b]] [[c]] ![[img.png]] [[a.png]]\n",
	})
	for vacated, want := range map[string]int{
		"b.md":    2, // wins `b`: both `[[b]]` links are counted
		"x/c.md":  0, // c.md wins `c`
		"img.png": 1, // wins the name `img.png`
		"x/a.png": 1, // the only `a.png`
		"q.md":    0, // no `[[q]]` link
	} {
		r := &destResolver{ws: ws, batch: newMoveBatch()}
		countShadowed(ws, r, vacated)
		assert.Equal(t, want, r.batch.withheld, vacated)
	}
}

func TestDestResolver_EdgeReader(t *testing.T) {
	r := &destResolver{ws: stubWorkspace{}}
	lines := r.edgeReader()
	require.NotNil(t, lines.memo, "the shared reader keeps every file it reads")
	assert.Same(t, lines, r.edgeReader())
}

func TestWikilinkTarget_Holders(t *testing.T) {
	post := holderIndex("a/guide.md", "guide.md", "img/guide.png")
	assert.Equal(t, []string{"guide.md", "a/guide.md"}, wikilinkTarget{wikilinkKey: stemKey("guide")}.holders(post))
	assert.Equal(t, []string{"img/guide.png"}, wikilinkTarget{wikilinkKey: nameKey("guide.png")}.holders(post))
}

func TestDestResolver_CountBlocked(t *testing.T) {
	b := newMoveBatch()
	b.dsts["c.md"], b.dsts["x/c.md"], b.dsts["b.md"] = true, true, true
	r := &destResolver{batch: b}
	t1 := wikilinkTarget{dst: "x/c.md", wikilinkKey: stemKey("c")}
	r.countBlocked(holderIndex("c.md", "x/c.md"), t1, stemKey("a"), "x/c.md")
	assert.Equal(t, 1, b.withheld, "a member's destination wins the new key")
	r.countBlocked(holderIndex("y/c.md", "x/c.md"), t1, stemKey("a"), "x/c.md")
	assert.Equal(t, 1, b.withheld, "a file outside the batch wins it")
	r.countBlocked(holderIndex("y/c.md", "x/c.md", "b.md"), t1, stemKey("b"), "x/c.md")
	assert.Equal(t, 2, b.withheld, "the link left as written reaches a member's destination")
}
