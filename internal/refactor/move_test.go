package refactor

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/index"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// applyEditsToSource splices a plan's edits into source through
// ApplyEdits — the splice the CLI runs — so a move test asserts on the
// final file, and fails if the planner emits an overlapping or
// out-of-range edit.
func applyEditsToSource(t *testing.T, source string, edits []Edit) string {
	t.Helper()
	out, err := ApplyEdits([]byte(source), edits)
	require.NoError(t, err)
	return string(out)
}

func TestMove_IncomingFileLinksAndFileOp(t *testing.T) {
	ws := newMemWorkspace(map[string]string{
		"a.md": "# A\n",
		"b.md": "See [a](a.md) and [sec](a.md#intro).\n",
	})
	plan, err := Move(ws, "a.md", "docs/a.md")
	require.NoError(t, err)

	require.NotNil(t, plan.FileOp)
	assert.Equal(t, "a.md", plan.FileOp.From)
	assert.Equal(t, "docs/a.md", plan.FileOp.To)

	got := applyEditsToSource(t, "See [a](a.md) and [sec](a.md#intro).\n", plan.Edits["b.md"])
	assert.Equal(t, "See [a](docs/a.md) and [sec](docs/a.md#intro).\n", got)
}

func TestMove_RefDefDestination(t *testing.T) {
	ws := newMemWorkspace(map[string]string{
		"a.md": "# A\n",
		"b.md": "Use [x][a].\n\n[a]: a.md\n",
	})
	plan, err := Move(ws, "a.md", "sub/a.md")
	require.NoError(t, err)

	// The ref-def destination is rewritten; the [x][a] use (a label,
	// not a path) is untouched.
	got := applyEditsToSource(t, "Use [x][a].\n\n[a]: a.md\n", plan.Edits["b.md"])
	assert.Equal(t, "Use [x][a].\n\n[a]: sub/a.md\n", got)
}

func TestMove_WikilinkStemRewrittenWhenBasenameChanges(t *testing.T) {
	src := "See [[api]] and [[api#usage]] and [[api|the API]].\n"
	ws := newMemWorkspace(map[string]string{
		"api.md":   "# API\n",
		"guide.md": src,
	})
	plan, err := Move(ws, "api.md", "service.md")
	require.NoError(t, err)

	got := applyEditsToSource(t, src, plan.Edits["guide.md"])
	assert.Equal(t, "See [[service]] and [[service#usage]] and [[service|the API]].\n", got)
}

// MoveAll's StemEdits holds exactly the `[[stem]]` rewrites among its
// edits, keyed the same way, leaving out its path rewrites, and its
// edits match Move's plan.
func TestMoveAll_StemEdits(t *testing.T) {
	ws := newMemWorkspace(map[string]string{
		"api.md":   "# API\n",
		"guide.md": "See [[api]] and [the API](api.md).\n",
	})
	want, err := Move(ws, "api.md", "./service.md")
	require.NoError(t, err)

	bp := MoveAll(ws, []MovePair{{"./api.md", "service.md"}})
	require.NoError(t, bp.Moves[0].Err)
	assert.Equal(t, want.Edits, bp.Edits)
	require.Len(t, bp.Edits["guide.md"], 2, "one stem and one path rewrite")
	stems := bp.StemEdits
	require.Len(t, stems["guide.md"], 1)
	assert.Equal(t, "service", stems["guide.md"][0].NewText)
	assert.Contains(t, bp.Edits["guide.md"], stems["guide.md"][0])

	kept := MoveAll(ws, []MovePair{{"api.md", "docs/api.md"}})
	assert.Empty(t, kept.StemEdits, "a kept stem needs no rewrite")

	refused := MoveAll(ws, []MovePair{{"api.md", "guide.md"}})
	assert.ErrorAs(t, refused.Moves[0].Err, &DestinationExistsError{})
	assert.Empty(t, refused.StemEdits)
}

func TestMove_WikilinksUntouchedWhenBasenameKept(t *testing.T) {
	ws := newMemWorkspace(map[string]string{
		"api.md":   "# API\n",
		"guide.md": "See [[api]].\n",
	})
	plan, err := Move(ws, "api.md", "docs/api.md")
	require.NoError(t, err)
	// A stem still resolves to the moved file, so the wikilink is left
	// alone — the documented asymmetry with path links.
	assert.Empty(t, plan.Edits["guide.md"])
	require.NotNil(t, plan.FileOp)
}

// TestMove_WikilinkAmbiguousStemLeftUntouched locks that a move of a
// file that shares its basename stem with a sibling the resolver picks
// first rewrites no wikilink: the resolver reads the basename alone, so
// `[[ref/Guide]]` and `[[docs/Guide]]` both reach docs/Guide.md, and
// retargeting them would steal links from that sibling.
func TestMove_WikilinkAmbiguousStemLeftUntouched(t *testing.T) {
	src := "See [[ref/Guide]] and [[docs/Guide]].\n"
	ws := newMemWorkspace(map[string]string{
		"docs/Guide.md": "# Guide\n",
		"ref/Guide.md":  "# Guide\n",
		"index.md":      src,
	})
	plan, err := Move(ws, "ref/Guide.md", "ref/Manual.md")
	require.NoError(t, err)
	assert.Empty(t, plan.Edits["index.md"],
		"a sibling wins the stem: no wikilink is rewritten")
}

// TestMove_WikilinkSharedStemRewrittenWhenSourceWins locks that a move
// of the same-stem file the resolver picks first rewrites every
// `[[stem]]` that reaches it: left alone, each would silently reach the
// sibling once the file is gone. A link whose folder prefix names a
// sibling's folder, not the moved file's, is left as written: it
// already says which file it means, and it reaches that one once the
// moved file is gone. The prefix matches whole trailing folders,
// ignoring case and reading `\` as `/`.
func TestMove_WikilinkSharedStemRewrittenWhenSourceWins(t *testing.T) {
	src := "See [[Guide]], [[ref/Guide]], [[REF\\Guide]], [[a/ref/Guide]],\n" +
		"[[docs/Guide]], [[f/Guide]], and [[./Guide]].\n"
	ws := newMemWorkspace(map[string]string{
		"docs/Guide.md": "# Guide\n",
		"ref/Guide.md":  "# Guide\n",
		"index.md":      src,
	})
	plan, err := Move(ws, "docs/Guide.md", "docs/Manual.md")
	require.NoError(t, err)
	assert.Equal(t,
		"See [[Manual]], [[ref/Guide]], [[REF\\Guide]], [[a/ref/Manual]],\n"+
			"[[docs/Manual]], [[f/Manual]], and [[./Manual]].\n",
		applyEditsToSource(t, src, plan.Edits["index.md"]))
}

// TestMove_WikilinkRewrittenWhenDestinationWinsNewStem locks that a
// file already holding the new stem blocks the rewrite only when it
// sorts before dst: a shallower dst is the file `[[manual]]` reaches
// once it exists, so `[[guide]]` follows the move.
func TestMove_WikilinkRewrittenWhenDestinationWinsNewStem(t *testing.T) {
	src := "See [[guide]].\n"
	files := map[string]string{
		"docs/guide.md":   "# Guide\n",
		"z/x/y/manual.md": "# Other\n",
		"index.md":        src,
	}
	plan, err := Move(newMemWorkspace(files), "docs/guide.md", "a/manual.md")
	require.NoError(t, err)
	assert.Equal(t, "See [[manual]].\n", applyEditsToSource(t, src, plan.Edits["index.md"]))

	plan, err = Move(newMemWorkspace(files), "docs/guide.md", "z/x/y/w/manual.md")
	require.NoError(t, err)
	assert.Empty(t, plan.Edits["index.md"], "the shallower holder keeps [[manual]]")
}

// TestMove_DestinationWithSpaceIsPercentEncoded locks that relocating a
// file to a path containing a space emits a link destination that still
// parses: a bare space would terminate the CommonMark destination, so
// the recomputed token percent-encodes it (and the index decodes it
// back when resolving).
func TestMove_DestinationWithSpaceIsPercentEncoded(t *testing.T) {
	ws := newMemWorkspace(map[string]string{
		"a.md":      "# A\n",
		"link.md":   "See [a](a.md).\n",
		"refdef.md": "[ref]: a.md\n",
	})
	plan, err := Move(ws, "a.md", "new file.md")
	require.NoError(t, err)

	assert.Equal(t, "See [a](new%20file.md).\n",
		applyEditsToSource(t, "See [a](a.md).\n", plan.Edits["link.md"]))
	assert.Equal(t, "[ref]: new%20file.md\n",
		applyEditsToSource(t, "[ref]: a.md\n", plan.Edits["refdef.md"]))
}

// TestMove_TitledInlineLinksRewritten locks that an inline link
// carrying an optional CommonMark title has its path rewritten while the
// title is preserved — both for an incoming link that points at the
// moved file and for one of the moved file's own outbound links. A bare
// destination ends at the first space, so the title bytes must not be
// folded into the path token (which would leave the link resolving to
// the vacated location and never rewrite it).
func TestMove_TitledInlineLinksRewritten(t *testing.T) {
	ws := newMemWorkspace(map[string]string{
		"a.md":     "See [o](sub/o.md \"out\").\n",
		"sub/o.md": "# O\n",
		"b.md":     "See [a](a.md \"in\").\n",
	})
	plan, err := Move(ws, "a.md", "docs/a.md")
	require.NoError(t, err)

	assert.Equal(t, "See [a](docs/a.md \"in\").\n",
		applyEditsToSource(t, "See [a](a.md \"in\").\n", plan.Edits["b.md"]))
	assert.Equal(t, "See [o](../sub/o.md \"out\").\n",
		applyEditsToSource(t, "See [o](sub/o.md \"out\").\n", plan.Edits["a.md"]))
}

// TestMove_WikilinkLeftUntouchedWhenDestStemCollides locks that a move
// whose destination basename stem is already used by another workspace
// file does not rewrite `[[oldStem]]`: retargeting it to `[[newStem]]`
// would make the link resolve to that sibling (or become ambiguous)
// rather than the moved file. It mirrors the source-side ambiguity
// guard.
func TestMove_WikilinkLeftUntouchedWhenDestStemCollides(t *testing.T) {
	ws := newMemWorkspace(map[string]string{
		"api.md":         "# API\n",
		"other/guide.md": "# Other\n",
		"index.md":       "See [[api]].\n",
	})
	plan, err := Move(ws, "api.md", "svc/guide.md")
	require.NoError(t, err)
	assert.Empty(t, plan.Edits["index.md"],
		"colliding destination stem: no wikilink is rewritten")
}

// TestMove_SelfRefDefLeftUntouched locks that a self-referential
// reference definition inside the moved file is not rewritten when
// the basename is kept: it is recomputed against the file's new
// location, where `a.md` still names the file. Recomputing it from the
// old directory, or by the incoming pass, would break it.
func TestMove_SelfRefDefLeftUntouched(t *testing.T) {
	src := "# A\n\n[self]: a.md\n"
	ws := newMemWorkspace(map[string]string{"a.md": src})
	plan, err := Move(ws, "a.md", "docs/a.md")
	require.NoError(t, err)
	assert.Empty(t, plan.Edits["a.md"],
		"self-referential ref-def still names the moved file")
}

func TestMove_OutboundRelativeLinksRecomputed(t *testing.T) {
	src := "# A\n\nSee [b](b.md), [up](../top.md), and [ext](https://x.example).\n"
	ws := newMemWorkspace(map[string]string{
		"docs/a.md": src,
		"docs/b.md": "# B\n",
		"top.md":    "# Top\n",
	})
	plan, err := Move(ws, "docs/a.md", "guide/sub/a.md")
	require.NoError(t, err)

	got := applyEditsToSource(t, src, plan.Edits["docs/a.md"])
	assert.Equal(t,
		"# A\n\nSee [b](../../docs/b.md), [up](../../top.md), and [ext](https://x.example).\n",
		got)
}

func TestMove_OutboundImageDestinationsRecomputed(t *testing.T) {
	src := "# Guide\n\n[api](api.md#options)\n\n![chart](../assets/chart.svg)\n\n" +
		"[![badge](../assets/chart.svg)](../assets/chart.svg)\n"
	ws := newMemWorkspace(map[string]string{
		"docs/guide.md":    src,
		"docs/api.md":      "# Options\n",
		"assets/chart.svg": "<svg/>",
	})
	plan, err := Move(ws, "docs/guide.md", "reference/manual/guide.md")
	require.NoError(t, err)

	got := applyEditsToSource(t, src, plan.Edits["docs/guide.md"])
	assert.Equal(t,
		"# Guide\n\n[api](../../docs/api.md#options)\n\n![chart](../../assets/chart.svg)\n\n"+
			"[![badge](../../assets/chart.svg)](../../assets/chart.svg)\n",
		got)
}

func TestMove_OutboundEmptyAltImagesRecomputed(t *testing.T) {
	src := "# Guide\n\n![](../assets/chart.svg)\n\n" +
		"[![](../assets/chart.svg)](../assets/chart.svg)\n\n" +
		"```\n![](../assets/chart.svg)\n```\n"
	ws := newMemWorkspace(map[string]string{
		"docs/guide.md":    src,
		"assets/chart.svg": "<svg/>",
	})
	plan, err := Move(ws, "docs/guide.md", "reference/manual/guide.md")
	require.NoError(t, err)

	got := applyEditsToSource(t, src, plan.Edits["docs/guide.md"])
	assert.Equal(t,
		"# Guide\n\n![](../../assets/chart.svg)\n\n"+
			"[![](../../assets/chart.svg)](../../assets/chart.svg)\n\n"+
			"```\n![](../assets/chart.svg)\n```\n",
		got)
}

// TestMove_OutboundEmptyAltImageAfterSameTargetLink pins that a text-less
// image after a link to the same file gets its own edit. Neither node may
// take the other's destination, or the image path is left stale.
func TestMove_OutboundEmptyAltImageAfterSameTargetLink(t *testing.T) {
	src := "# Guide\n\n[see chart](../assets/chart.svg)\n\n![](../assets/chart.svg)\n"
	ws := newMemWorkspace(map[string]string{
		"docs/guide.md":    src,
		"assets/chart.svg": "<svg/>",
	})
	plan, err := Move(ws, "docs/guide.md", "reference/manual/guide.md")
	require.NoError(t, err)

	got := applyEditsToSource(t, src, plan.Edits["docs/guide.md"])
	assert.Equal(t,
		"# Guide\n\n[see chart](../../assets/chart.svg)\n\n![](../../assets/chart.svg)\n",
		got)
}

// TestMove_OutboundRewritesOnlyRealDestinations pins that `](path)` bytes
// which are not a link or image destination stay as written: a code span,
// an inline or block HTML comment, and a code span, raw HTML or autolink
// inside a node's own label. The real destination is rewritten instead.
func TestMove_OutboundRewritesOnlyRealDestinations(t *testing.T) {
	src := "# A\n\n" +
		"See `[x](./b.md)` and <!-- [y](./b.md) --> here.\n\n" +
		"<!-- [z](./b.md) -->\n\n" +
		"[](./b.md)\n\n" +
		"[`](./b.md)` code](./b.md)\n\n" +
		"[<span title=\"](./b.md)\">s</span>](./b.md)\n\n" +
		"[<img alt=\"](./b.md)\">](./b.md)\n\n" +
		"![<http://h](./b.md)>](./b.md)\n"
	ws := newMemWorkspace(map[string]string{"a.md": src, "b.md": "# B\n"})
	plan, err := Move(ws, "a.md", "docs/a.md")
	require.NoError(t, err)

	got := applyEditsToSource(t, src, plan.Edits["a.md"])
	assert.Equal(t, "# A\n\n"+
		"See `[x](./b.md)` and <!-- [y](./b.md) --> here.\n\n"+
		"<!-- [z](./b.md) -->\n\n"+
		"[](../b.md)\n\n"+
		"[`](./b.md)` code](../b.md)\n\n"+
		"[<span title=\"](./b.md)\">s</span>](../b.md)\n\n"+
		"[<img alt=\"](./b.md)\">](../b.md)\n\n"+
		"![<http://h](./b.md)>](../b.md)\n",
		got)
}

// TestMove_OutboundQueryAndPaddedDestinations pins that an image with a
// query string (`?raw=true`) and a destination padded with whitespace
// inside its parentheses are both re-spelled, keeping the query.
func TestMove_OutboundQueryAndPaddedDestinations(t *testing.T) {
	src := "# A\n\n![c](chart.svg?raw=true) [b]( b.md )\n"
	ws := newMemWorkspace(map[string]string{
		"a.md": src, "b.md": "# B\n", "chart.svg": "<svg/>",
	})
	plan, err := Move(ws, "a.md", "docs/a.md")
	require.NoError(t, err)

	assert.Equal(t, "# A\n\n![c](../chart.svg?raw=true) [b]( ../b.md )\n",
		applyEditsToSource(t, src, plan.Edits["a.md"]))
}

// TestMove_IncomingQueryAndPaddedDestinations pins the same two forms on
// the incoming side: a link in another file with a query string or a
// padded destination is repointed at dst, keeping the query.
func TestMove_IncomingQueryAndPaddedDestinations(t *testing.T) {
	b := "See [q](a.md?plain=1#x) and [p]( a.md ).\n"
	ws := newMemWorkspace(map[string]string{"a.md": "# A\n", "b.md": b})
	plan, err := Move(ws, "a.md", "docs/a.md")
	require.NoError(t, err)

	assert.Equal(t, "See [q](docs/a.md?plain=1#x) and [p]( docs/a.md ).\n",
		applyEditsToSource(t, b, plan.Edits["b.md"]))
}

// TestMove_OutboundSkipsReferenceStyleImages pins that a reference-style
// `![][logo]` is left alone: its destination lives in the ref-def, which
// the move rewrites once. It must not take the `](path)` of a code span
// or of a later image.
func TestMove_OutboundSkipsReferenceStyleImages(t *testing.T) {
	src := "# A\n\n`![](./logo.png)`\n\n![][logo] ![](./logo.png)\n\n[logo]: ./logo.png\n"
	ws := newMemWorkspace(map[string]string{"a.md": src, "logo.png": "png"})
	plan, err := Move(ws, "a.md", "docs/a.md")
	require.NoError(t, err)

	edits := plan.Edits["a.md"]
	require.Len(t, edits, 2)
	assert.Equal(t,
		"# A\n\n`![](./logo.png)`\n\n![][logo] ![](../logo.png)\n\n[logo]: ../logo.png\n",
		applyEditsToSource(t, src, edits))
}

func TestMove_LeavesNonWorkspaceLinksUntouched(t *testing.T) {
	// A root-anchored `/a.md` is treated as absolute — it never resolves
	// to a workspace file (no site-root), so a move has nothing to
	// rewrite and must not touch it.
	ws := newMemWorkspace(map[string]string{
		"a.md":      "# A\n",
		"docs/b.md": "See [a](/a.md) and [x](https://x.example/a.md).\n",
	})
	plan, err := Move(ws, "a.md", "moved/a.md")
	require.NoError(t, err)
	assert.Empty(t, plan.Edits["docs/b.md"])
}

func TestMove_PreservesExplicitRelativeSpelling(t *testing.T) {
	ws := newMemWorkspace(map[string]string{
		"docs/a.md": "# A\n",
		"docs/b.md": "See [a](./a.md).\n",
	})
	plan, err := Move(ws, "docs/a.md", "docs/c.md")
	require.NoError(t, err)
	got := applyEditsToSource(t, "See [a](./a.md).\n", plan.Edits["docs/b.md"])
	assert.Equal(t, "See [a](./c.md).\n", got)
}

func TestMove_SafetyErrors(t *testing.T) {
	ws := newMemWorkspace(map[string]string{
		"a.md": "# A\n",
		"b.md": "# B\n",
	})
	t.Run("destination exists", func(t *testing.T) {
		_, err := Move(ws, "a.md", "b.md")
		var de DestinationExistsError
		assert.ErrorAs(t, err, &de)
	})
	t.Run("traversal destination", func(t *testing.T) {
		_, err := Move(ws, "a.md", "../evil.md")
		assert.ErrorIs(t, err, ErrTraversalPath)
	})
	t.Run("same file", func(t *testing.T) {
		_, err := Move(ws, "a.md", "a.md")
		assert.ErrorIs(t, err, ErrSameFile)
	})
	t.Run("missing source", func(t *testing.T) {
		_, err := Move(ws, "ghost.md", "x.md")
		var se SourceNotFoundError
		assert.ErrorAs(t, err, &se)
	})
}

// TestMove_WikilinkRewrittenWithExtensionlessSibling locks that an
// extensionless file such as LICENSE is not a wikilink stem target: the
// wikilink index maps only Markdown files to stems, so it must not count
// as a same-stem sibling of docs/license.md. The root LICENSE is
// shallower, so it would win `[[license]]` if it held the stem.
func TestMove_WikilinkRewrittenWithExtensionlessSibling(t *testing.T) {
	src := "See [[license]].\n"
	ws := newMemWorkspace(map[string]string{
		"docs/license.md": "# License\n",
		"LICENSE":         "MIT\n",
		"index.md":        src,
	})
	plan, err := Move(ws, "docs/license.md", "docs/terms.md")
	require.NoError(t, err)
	assert.Equal(t, "See [[terms]].\n", applyEditsToSource(t, src, plan.Edits["index.md"]))
}

// TestMove_WikilinkLeftUntouchedWhenTypedDestNameCollides locks that a
// move to a non-Markdown extension keeps an exact-name collision guard:
// `[[guide.mdx]]` resolves by exact file name, so with a/guide.mdx
// already present the rewrite could land on the wrong file.
func TestMove_WikilinkLeftUntouchedWhenTypedDestNameCollides(t *testing.T) {
	ws := newMemWorkspace(map[string]string{
		"docs/guide.md": "# Guide\n",
		"a/guide.mdx":   "# Other\n",
		"index.md":      "See [[guide]].\n",
	})
	plan, err := Move(ws, "docs/guide.md", "docs/guide.mdx")
	require.NoError(t, err)
	assert.Empty(t, plan.Edits["index.md"],
		"typed destination name already taken: no wikilink is rewritten")
}

// TestMove_NonMarkdownSourceLeavesWikilinksAlone locks that moving a
// non-Markdown file never rewrites `[[stem]]` links: no stem resolves to
// it, so `[[license]]` still points at docs/license.md. The Markdown
// destination pins the source guard on its own: an extensionless
// destination such as COPYING is skipped by the destination guard too.
func TestMove_NonMarkdownSourceLeavesWikilinksAlone(t *testing.T) {
	for _, dst := range []string{"COPYING", "docs/terms.md"} {
		for name, listed := range map[string]bool{"listed": true, "unlisted": false} {
			t.Run(dst+"/"+name, func(t *testing.T) {
				files := map[string]string{
					"docs/license.md": "# License\n",
					"index.md":        "See [[license]].\n",
				}
				if listed {
					files["LICENSE"] = "MIT\n"
				}
				ws := newMemWorkspace(files)
				// An unlisted source is still resolvable on disk.
				var w Workspace = ws
				if !listed {
					w = unlistedSource{memWorkspace: ws, rel: "LICENSE", body: "MIT\n"}
				}
				plan, err := Move(w, "LICENSE", dst)
				require.NoError(t, err)
				assert.Empty(t, plan.Edits["index.md"])
			})
		}
	}
}

// TestMove_WikilinkNotRewrittenToExtensionlessName locks that a move to
// an extensionless name rewrites nothing: a bare `[[name]]` finds only
// Markdown files, so it could never reach the destination.
func TestMove_WikilinkNotRewrittenToExtensionlessName(t *testing.T) {
	ws := newMemWorkspace(map[string]string{
		"docs/api.md": "# API\n",
		"index.md":    "See [[api]].\n",
	})
	plan, err := Move(ws, "docs/api.md", "docs/COPYING")
	require.NoError(t, err)
	assert.Empty(t, plan.Edits["index.md"])
}

// TestMove_WikilinkNotRewrittenToUnspellableName locks that a move to a
// name no wikilink can spell rewrites nothing: an empty stem (`.md`)
// leaves `[[]]`, and a `#`, `|`, `[`, `]`, or newline in the stem splits
// or ends the link, so the rewrite would not name the destination. A
// trailing space is trimmed off the link target, so `[[guide.md ]]`
// would reach x/guide.md instead of the moved file.
func TestMove_WikilinkNotRewrittenToUnspellableName(t *testing.T) {
	for _, dst := range []string{
		"docs/.md", "docs/C#.md", "docs/a|b.md", "docs/[x].md", "docs/x].txt",
		"docs/guide.md ", "docs/a\nb.md", "docs/a\rb.md", "docs/a`b.md",
	} {
		t.Run(dst, func(t *testing.T) {
			ws := newMemWorkspace(map[string]string{
				"docs/api.md": "# API\n",
				"x/guide.md":  "# Guide\n",
				"index.md":    "See [[api]].\n",
			})
			plan, err := Move(ws, "docs/api.md", dst)
			require.NoError(t, err)
			assert.Empty(t, plan.Edits["index.md"])
		})
	}
}

// TestMove_WikilinkKeepsMarkdownExtForDottedStem locks that a
// destination stem holding a dot keeps its Markdown extension: a bare
// `[[v1.3]]` reads `.3` as a typed extension and looks up a file named
// exactly `v1.3`, so only `[[v1.3.md]]` reaches docs/v1.3.md.
func TestMove_WikilinkKeepsMarkdownExtForDottedStem(t *testing.T) {
	src := "See [[v1.2.md]] and [[v1.2.md#notes|old]].\n"
	ws := newMemWorkspace(map[string]string{
		"docs/v1.2.md": "# V1.2\n",
		"index.md":     src,
	})
	plan, err := Move(ws, "docs/v1.2.md", "docs/v1.3.md")
	require.NoError(t, err)
	assert.Equal(t, "See [[v1.3.md]] and [[v1.3.md#notes|old]].\n",
		applyEditsToSource(t, src, plan.Edits["index.md"]))
}

// TestMove_WikilinkDriveShapedNameGetsDotSlash locks that a destination
// whose name reads as a drive letter (`C:x.md`) is still reached: a
// bare `[[C:x]]` is refused as a drive path, so it is written
// `[[./C:x]]`, while a link with a folder prefix already starts with
// that folder and keeps the bare name.
func TestMove_WikilinkDriveShapedNameGetsDotSlash(t *testing.T) {
	src := "See [[api]] and [[ref/api|R]].\n"
	ws := newMemWorkspace(map[string]string{
		"docs/api.md": "# API\n",
		"index.md":    src,
	})
	plan, err := Move(ws, "docs/api.md", "docs/C:x.md")
	require.NoError(t, err)
	assert.Equal(t, "See [[./C:x]] and [[ref/C:x|R]].\n",
		applyEditsToSource(t, src, plan.Edits["index.md"]))
}

// TestMove_WikilinkKeepsBackslashPrefixAndTableEscape locks that a
// rewrite replaces only the stem segment the resolver reads: a `\`
// folder prefix is kept, and so is the `\` that escapes a `|` inside a
// table cell, so the alias stays in the cell.
func TestMove_WikilinkKeepsBackslashPrefixAndTableEscape(t *testing.T) {
	src := "See [[docs\\api]].\n\n| a |\n| - |\n| [[api\\|API]] |\n"
	ws := newMemWorkspace(map[string]string{
		"docs/api.md": "# API\n",
		"index.md":    src,
	})
	plan, err := Move(ws, "docs/api.md", "docs/service.md")
	require.NoError(t, err)
	assert.Equal(t, "See [[docs\\service]].\n\n| a |\n| - |\n| [[service\\|API]] |\n",
		applyEditsToSource(t, src, plan.Edits["index.md"]))
}

// TestMove_WikilinkKeepsMarkdownExtForTrailingSpaceStem locks that a
// destination stem ending in a space keeps its Markdown extension: the
// link target is trimmed, so `[[guide ]]` would look up `guide`, while
// `[[guide .md]]` keeps the space inside the target and reaches the file.
func TestMove_WikilinkKeepsMarkdownExtForTrailingSpaceStem(t *testing.T) {
	src := "See [[api]].\n"
	ws := newMemWorkspace(map[string]string{
		"docs/api.md": "# API\n",
		"index.md":    src,
	})
	plan, err := Move(ws, "docs/api.md", "docs/guide .md")
	require.NoError(t, err)
	assert.Equal(t, "See [[guide .md]].\n", applyEditsToSource(t, src, plan.Edits["index.md"]))
}

// TestMove_WikilinkRewrittenWhenTypedDestNameEqualsOldStem locks that a
// typed destination is compared by name, not against the Markdown stem:
// docs/guide.png.md has stem `guide.png`, the same string as the
// destination's name, yet `[[guide.png.md]]` stops resolving once the
// file is docs/guide.png and must become `[[guide.png]]`.
func TestMove_WikilinkRewrittenWhenTypedDestNameEqualsOldStem(t *testing.T) {
	src := "See [[guide.png.md]].\n"
	ws := newMemWorkspace(map[string]string{
		"docs/guide.png.md": "# Guide\n",
		"index.md":          src,
	})
	plan, err := Move(ws, "docs/guide.png.md", "docs/guide.png")
	require.NoError(t, err)
	assert.Equal(t, "See [[guide.png]].\n", applyEditsToSource(t, src, plan.Edits["index.md"]))
}

// TestMove_EmptyStemSourceLeavesWikilinksAlone locks that a source with
// an empty stem (`docs/.md`) rewrites no wikilink: no `[[stem]]` edge
// keys to it, and `[[.md.md]]` names a different file, `.md.md`.
func TestMove_EmptyStemSourceLeavesWikilinksAlone(t *testing.T) {
	ws := newMemWorkspace(map[string]string{
		"docs/.md": "# Empty\n",
		"index.md": "See [[.md.md]].\n",
	})
	plan, err := Move(ws, "docs/.md", "docs/x.md")
	require.NoError(t, err)
	assert.Empty(t, plan.Edits["index.md"])
}

// TestMove_WhitespaceNamedFileIsNotAStemHolder locks that a listed
// `x/ guide.md` does not block a `[[guide]]` rewrite: the resolver keys
// it as ` guide`, and a trimmed `[[guide]]` never reaches it.
func TestMove_WhitespaceNamedFileIsNotAStemHolder(t *testing.T) {
	src := "See [[guide]].\n"
	ws := newMemWorkspace(map[string]string{
		"docs/guide.md": "# Guide\n",
		"x/ guide.md":   "# Spaced\n",
		"index.md":      src,
	})
	plan, err := Move(ws, "docs/guide.md", "docs/manual.md")
	require.NoError(t, err)
	assert.Equal(t, "See [[manual]].\n", applyEditsToSource(t, src, plan.Edits["index.md"]))
}

// TestMove_WhitespaceNamedSourceLeavesWikilinksAlone locks that moving
// `docs/ guide.md` leaves `[[guide]]` alone: the trimmed link keys to
// `guide`, which never named the spaced source.
func TestMove_WhitespaceNamedSourceLeavesWikilinksAlone(t *testing.T) {
	ws := newMemWorkspace(map[string]string{
		"docs/ guide.md": "# Spaced\n",
		"index.md":       "See [[guide]].\n",
	})
	plan, err := Move(ws, "docs/ guide.md", "docs/manual.md")
	require.NoError(t, err)
	assert.Empty(t, plan.Edits["index.md"])
}

// unlistedSource resolves one file that Files() does not list, as
// Resolve reads any file on disk.
type unlistedSource struct {
	*memWorkspace
	rel, body string
}

func (u unlistedSource) Resolve(file string) (string, []byte, bool) {
	if n := index.NormalizePath(file); n == u.rel {
		return n, []byte(u.body), true
	}
	return u.memWorkspace.Resolve(file)
}
