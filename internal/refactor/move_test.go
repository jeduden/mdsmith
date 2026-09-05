package refactor

import (
	"sort"
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/mdtext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// applyEditsToSource splices every single-line edit into source and
// returns the rewritten text, so a move test can assert on the final
// file rather than on raw ranges. Same-line edits apply right-to-left
// so an earlier edit's offsets stay valid.
func applyEditsToSource(source string, edits []Edit) string {
	lines := strings.Split(source, "\n")
	byLine := map[int][]Edit{}
	for _, e := range edits {
		byLine[e.Range.Start.Line] = append(byLine[e.Range.Start.Line], e)
	}
	for ln, es := range byLine {
		row := []byte(lines[ln])
		sort.SliceStable(es, func(i, j int) bool {
			return es[i].Range.Start.Character > es[j].Range.Start.Character
		})
		for _, e := range es {
			s := mdtext.UTF16ToByteOffset(row, e.Range.Start.Character)
			en := mdtext.UTF16ToByteOffset(row, e.Range.End.Character)
			next := append([]byte{}, row[:s]...)
			next = append(next, e.NewText...)
			next = append(next, row[en:]...)
			row = next
		}
		lines[ln] = string(row)
	}
	return strings.Join(lines, "\n")
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

	got := applyEditsToSource("See [a](a.md) and [sec](a.md#intro).\n", plan.Edits["b.md"])
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
	got := applyEditsToSource("Use [x][a].\n\n[a]: a.md\n", plan.Edits["b.md"])
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

	got := applyEditsToSource(src, plan.Edits["guide.md"])
	assert.Equal(t, "See [[service]] and [[service#usage]] and [[service|the API]].\n", got)
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

// TestMove_WikilinkAmbiguousStemLeftUntouched locks that a move whose
// basename stem is shared by another workspace file does not rewrite
// any wikilink: the index keys wikilink edges by stem alone and cannot
// tell which same-stem file `[[ref/Guide]]` points at, so rewriting it
// would break a reference to the sibling file that is not moving.
func TestMove_WikilinkAmbiguousStemLeftUntouched(t *testing.T) {
	src := "See [[ref/Guide]] and [[docs/Guide]].\n"
	ws := newMemWorkspace(map[string]string{
		"docs/Guide.md": "# Guide\n",
		"ref/Guide.md":  "# Guide\n",
		"index.md":      src,
	})
	plan, err := Move(ws, "docs/Guide.md", "docs/Manual.md")
	require.NoError(t, err)
	assert.Empty(t, plan.Edits["index.md"],
		"ambiguous stem: no wikilink is rewritten")
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
		applyEditsToSource("See [a](a.md).\n", plan.Edits["link.md"]))
	assert.Equal(t, "[ref]: new%20file.md\n",
		applyEditsToSource("[ref]: a.md\n", plan.Edits["refdef.md"]))
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
		applyEditsToSource("See [a](a.md \"in\").\n", plan.Edits["b.md"]))
	assert.Equal(t, "See [o](../sub/o.md \"out\").\n",
		applyEditsToSource("See [o](sub/o.md \"out\").\n", plan.Edits["a.md"]))
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
// reference definition inside the moved file is not rewritten. Its
// destination would be recomputed from the file's old directory,
// producing a path that breaks once the file relocates; leaving it
// alone matches the documented "src's own ref-defs are a follow-up"
// contract and stays correct for a same-directory move.
func TestMove_SelfRefDefLeftUntouched(t *testing.T) {
	src := "# A\n\n[self]: a.md\n"
	ws := newMemWorkspace(map[string]string{"a.md": src})
	plan, err := Move(ws, "a.md", "docs/a.md")
	require.NoError(t, err)
	assert.Empty(t, plan.Edits["a.md"],
		"self-referential ref-def is left to the tracked follow-up")
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

	got := applyEditsToSource(src, plan.Edits["docs/a.md"])
	assert.Equal(t,
		"# A\n\nSee [b](../../docs/b.md), [up](../../top.md), and [ext](https://x.example).\n",
		got)
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
	got := applyEditsToSource("See [a](./a.md).\n", plan.Edits["docs/b.md"])
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
