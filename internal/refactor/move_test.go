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
