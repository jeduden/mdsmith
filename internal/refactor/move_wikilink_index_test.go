package refactor

import (
	"maps"
	"slices"
	"strings"
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
// archive/guide.md) still counts as a same-stem holder: it sorts before
// docs/guide.md, so `[[guide]]` resolves to it and is left alone rather
// than retargeted at the moved file it never resolved to.
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
// does not count as a same-stem holder. The README is shallower than
// the moved file, so it would win `[[readme]]` if it were indexed.
func TestMove_NodeModulesReadmeDoesNotBlockWikilinkRewrite(t *testing.T) {
	src := "See [[readme]].\n"
	ws := newUnlistedWorkspace(map[string]string{
		"docs/api/v1/readme.md":      "# Readme\n",
		"node_modules/pkg/README.md": "# Pkg\n",
		"index.md":                   src,
	})
	plan, err := Move(ws, "docs/api/v1/readme.md", "docs/api/v1/intro.md")
	require.NoError(t, err)
	assert.Equal(t, "See [[intro]].\n", applyEditsToSource(t, src, plan.Edits["index.md"]))
}

// TestMove_UnlistedTypedNameBlocksWikilinkRewrite locks that a
// non-Markdown destination is blocked by a file the resolver indexes
// under its name, listed or not, that sorts before it: `[[logo.png]]`
// reaches the shallower root file or a/LOGO.PNG, and `[[guide.mdx]]`
// reaches a/guide.mdx.
func TestMove_UnlistedTypedNameBlocksWikilinkRewrite(t *testing.T) {
	for name, tc := range map[string]struct {
		src, dst, unlisted, link string
	}{
		"root logo.png":    {"docs/logo.md", "docs/logo.png", "logo.png", "[[logo]]"},
		"unlisted mdx":     {"docs/guide.md", "docs/guide.mdx", "a/guide.mdx", "[[guide]]"},
		"case-folded name": {"docs/logo.md", "docs/logo.png", "a/LOGO.PNG", "[[logo]]"},
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

// TestMove_UnindexedSourceKeepsWikilinks locks that a move out of
// `node_modules` or `.git` leaves `[[guide]]` as written: the resolver
// never indexes the source, so no `[[guide]]` link ever reached it, and
// retargeting one at `[[manual]]` would point it at a file it never
// named.
func TestMove_UnindexedSourceKeepsWikilinks(t *testing.T) {
	for _, src := range []string{"node_modules/pkg/guide.md", ".git/guide.md"} {
		t.Run(src, func(t *testing.T) {
			link := "See [[guide]].\n"
			ws := newMemWorkspace(map[string]string{
				src:        "# Guide\n",
				"index.md": link,
			})
			plan, err := Move(ws, src, "docs/manual.md")
			require.NoError(t, err)
			assert.Equal(t, link, applyEditsToSource(t, link, plan.Edits["index.md"]))
		})
	}
}

// nilIndexWorkspace is a memWorkspace whose WikilinkIndex is nil, as a
// root walk that fails at the top leaves it.
type nilIndexWorkspace struct{ *memWorkspace }

func (nilIndexWorkspace) WikilinkIndex() *linkgraph.WikilinkIndex { return nil }

// TestMove_NilWikilinkIndexCountsListedFiles locks that a workspace with
// no wikilink index still reads the files it lists: a listed same-stem
// sibling that `[[guide]]` resolves to blocks the rewrite, and a lone
// holder is still rewritten.
func TestMove_NilWikilinkIndexCountsListedFiles(t *testing.T) {
	src := "See [[guide]].\n"
	ws := nilIndexWorkspace{newMemWorkspace(map[string]string{
		"docs/guide.md": "# Guide\n",
		"a/guide.md":    "# A\n",
		"index.md":      src,
	})}
	plan, err := Move(ws, "docs/guide.md", "docs/manual.md")
	require.NoError(t, err)
	assert.Empty(t, plan.Edits["index.md"], "a listed sibling wins the stem")

	ws = nilIndexWorkspace{newMemWorkspace(map[string]string{
		"docs/guide.md": "# Guide\n",
		"index.md":      src,
	})}
	plan, err = Move(ws, "docs/guide.md", "docs/manual.md")
	require.NoError(t, err)
	assert.Equal(t, "See [[manual]].\n", applyEditsToSource(t, src, plan.Edits["index.md"]))
}

// markdownOnlyNilIndexWorkspace is a nilIndexWorkspace whose Files
// lists Markdown files only, as the CLI and LSP workspaces do: the
// fallback set then knows no non-Markdown file.
type markdownOnlyNilIndexWorkspace struct{ nilIndexWorkspace }

func (w markdownOnlyNilIndexWorkspace) Files() []string {
	var out []string
	for _, f := range w.memWorkspace.Files() {
		if strings.HasSuffix(f, ".md") {
			out = append(out, f)
		}
	}
	return out
}

// TestMove_NilWikilinkIndexKeepsTypedLinks locks that, with no wikilink
// index, a typed `[[name.ext]]` link is left as written whether its
// key is the source's or the destination's: the listed files need not
// include the non-Markdown files that hold the name, so a shallower
// img.png may win `[[img.png]]` or `[[photo.png]]` unseen.
func TestMove_NilWikilinkIndexKeepsTypedLinks(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		src, dst string
	}{
		{"source name", map[string]string{
			"img.png": "png", "x/img.png": "png", "n.md": "[[img.png]]\n",
		}, "x/img.png", "x/photo.png"},
		{"destination name", map[string]string{
			"photo.png": "png", "x/guide.md": "# G\n", "n.md": "[[guide]]\n",
		}, "x/guide.md", "x/photo.png"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ws := markdownOnlyNilIndexWorkspace{nilIndexWorkspace{newMemWorkspace(tt.files)}}
			plan, err := Move(ws, tt.src, tt.dst)
			require.NoError(t, err)
			assert.Empty(t, plan.Edits["n.md"])
		})
	}
}
