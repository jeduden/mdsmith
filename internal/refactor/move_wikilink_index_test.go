package refactor

import (
	"maps"
	"slices"
	"testing"

	"github.com/jeduden/mdsmith/internal/linkgraph"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// unlistedWorkspace is a memWorkspace whose backing filesystem also
// holds files the workspace file list omits, as a gitignored path or a
// non-Markdown file does in the CLI and a `.mdx` file does in the LSP.
// Its WikilinkIndex reads the whole backing filesystem, the way the
// resolver does.
type unlistedWorkspace struct {
	*memWorkspace
	unlisted []string
}

func newUnlistedWorkspace(files map[string]string, unlisted ...string) *unlistedWorkspace {
	return &unlistedWorkspace{memWorkspace: newMemWorkspace(files), unlisted: unlisted}
}

func (w *unlistedWorkspace) WikilinkIndex() *linkgraph.WikilinkIndex {
	return holderIndex(append(slices.Collect(maps.Keys(w.files)), w.unlisted...)...)
}

// TestMove_UnlistedStemSiblingBlocksWikilinkRewrite locks that a file
// the resolver indexes but the workspace does not list (a gitignored
// archive/guide.md) still counts as a same-stem holder, so `[[guide]]`
// is left alone rather than retargeted at a file it never resolved to.
func TestMove_UnlistedStemSiblingBlocksWikilinkRewrite(t *testing.T) {
	ws := newUnlistedWorkspace(map[string]string{
		"docs/guide.md": "# Guide\n",
		"index.md":      "See [[guide]].\n",
	}, "archive/guide.md")
	plan, err := Move(ws, "docs/guide.md", "docs/manual.md")
	require.NoError(t, err)
	assert.Empty(t, plan.Edits["index.md"])
}

// TestMove_NodeModulesReadmeDoesNotBlockWikilinkRewrite locks that a
// listed file under node_modules, which the resolver never indexes,
// does not count as a same-stem holder.
func TestMove_NodeModulesReadmeDoesNotBlockWikilinkRewrite(t *testing.T) {
	src := "See [[readme]].\n"
	ws := newUnlistedWorkspace(map[string]string{
		"docs/readme.md":             "# Readme\n",
		"node_modules/pkg/README.md": "# Pkg\n",
		"index.md":                   src,
	})
	plan, err := Move(ws, "docs/readme.md", "docs/intro.md")
	require.NoError(t, err)
	assert.Equal(t, "See [[intro]].\n", applyEditsToSource(t, src, plan.Edits["index.md"]))
}

// TestMove_UnlistedTypedNameBlocksWikilinkRewrite locks that a
// non-Markdown destination is blocked by any file the resolver indexes
// under its name, listed or not: `[[logo.png]]` reaches the shallower
// root file, and `[[guide.mdx]]` reaches a/guide.mdx.
func TestMove_UnlistedTypedNameBlocksWikilinkRewrite(t *testing.T) {
	for name, tc := range map[string]struct {
		src, dst, unlisted, link string
	}{
		"root logo.png":    {"docs/logo.md", "docs/logo.png", "logo.png", "[[logo]]"},
		"unlisted mdx":     {"docs/guide.md", "docs/guide.mdx", "a/guide.mdx", "[[guide]]"},
		"case-folded name": {"docs/logo.md", "docs/logo.png", "img/LOGO.PNG", "[[logo]]"},
	} {
		t.Run(name, func(t *testing.T) {
			ws := newUnlistedWorkspace(map[string]string{
				tc.src:     "# Doc\n",
				"index.md": "See " + tc.link + ".\n",
			}, tc.unlisted)
			plan, err := Move(ws, tc.src, tc.dst)
			require.NoError(t, err)
			assert.Empty(t, plan.Edits["index.md"])
		})
	}
}

// TestMove_UnindexedDestinationKeepsWikilinks locks that a move into
// `node_modules` or `.git` leaves `[[guide]]` as written: the resolver
// never indexes the destination, so `[[manual]]` would reach nothing.
// A destination merely named like one of those folders is rewritten.
func TestMove_UnindexedDestinationKeepsWikilinks(t *testing.T) {
	for dst, want := range map[string]string{
		"node_modules/pkg/manual.md": "See [[guide]].\n",
		".git/manual.md":             "See [[guide]].\n",
		"docs/node_modules.md":       "See [[node_modules]].\n",
	} {
		t.Run(dst, func(t *testing.T) {
			src := "See [[guide]].\n"
			ws := newMemWorkspace(map[string]string{
				"docs/guide.md": "# Guide\n",
				"index.md":      src,
			})
			plan, err := Move(ws, "docs/guide.md", dst)
			require.NoError(t, err)
			assert.Equal(t, want, applyEditsToSource(t, src, plan.Edits["index.md"]))
		})
	}
}
