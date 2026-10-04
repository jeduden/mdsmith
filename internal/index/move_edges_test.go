package index

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIncomingPathEdges_ResolvedPathTargetsOnly asserts the move query
// returns exactly the edges that address a file by a rewritable path —
// file links (with or without an anchor) and include directives — and
// excludes same-file anchor links and label-based reference links,
// which carry no path token to rewrite.
func TestIncomingPathEdges_ResolvedPathTargetsOnly(t *testing.T) {
	idx := New("/root")
	idx.Update("docs/a.md", []byte("# A\n\nJump [self](#a).\n"))
	idx.Update("docs/b.md", []byte("See [a](a.md) and [sec](a.md#a).\n\nUse [it][ref].\n\n[ref]: a.md\n"))
	idx.Update("docs/c.md", []byte("<?include\nfile: a.md\n?>\n<?/include?>\n"))

	got := idx.IncomingPathEdges("docs/a.md")
	require.Len(t, got, 3)

	var fileLinks, includes int
	for _, e := range got {
		assert.Equal(t, "docs/a.md", e.TargetFile)
		switch e.Kind {
		case EdgeFileLink:
			fileLinks++
		case EdgeInclude:
			includes++
		default:
			t.Fatalf("unexpected edge kind %v in path edges", e.Kind)
		}
	}
	assert.Equal(t, 2, fileLinks, "both b.md file links (plain and anchored)")
	assert.Equal(t, 1, includes, "c.md include directive")
}

func TestIncomingPathEdges_NilReceiver(t *testing.T) {
	var idx *Index
	assert.Nil(t, idx.IncomingPathEdges("a.md"))
}

// TestIncomingPathEdges_SkipsOtherTargets ensures a resolved path edge
// to a different file is filtered out of the query for docs/a.md.
func TestIncomingPathEdges_SkipsOtherTargets(t *testing.T) {
	idx := New("/root")
	idx.Update("docs/a.md", []byte("# A\n"))
	idx.Update("docs/b.md", []byte("# B\n"))
	idx.Update("c.md", []byte("Links [a](docs/a.md) and [b](docs/b.md).\n"))

	got := idx.IncomingPathEdges("docs/a.md")
	require.Len(t, got, 1)
	assert.Equal(t, "docs/a.md", got[0].TargetFile)
}

// TestIncomingWikilinkEdges matches Obsidian-style links by lowercased
// basename stem, so both `[[api]]` and `[[API]]` count against stem
// "api".
func TestIncomingWikilinkEdges(t *testing.T) {
	idx := New("/root")
	idx.Update("notes/api.md", []byte("# API\n"))
	idx.Update("notes/guide.md", []byte("See [[api]] and also [[API]] here.\n"))

	got := idx.IncomingWikilinkEdges("api")
	require.Len(t, got, 2)
	for _, e := range got {
		assert.Equal(t, EdgeWikilink, e.Kind)
		assert.Equal(t, "api", e.TargetLabel)
		assert.Equal(t, "notes/guide.md", e.SourceFile)
	}
	// A caller may pass any casing; the query lowercases before matching.
	assert.Len(t, idx.IncomingWikilinkEdges("API"), 2)
}

func TestIncomingWikilinkEdges_NilReceiver(t *testing.T) {
	var idx *Index
	assert.Nil(t, idx.IncomingWikilinkEdges("api"))
}

// TestIncomingWikilinkEdges_SortOrder exercises every tie-breaker in the
// (SourceFile, SourceLine, SourceCol) ordering: two files, and two
// wikilinks on different lines within one file.
func TestIncomingWikilinkEdges_SortOrder(t *testing.T) {
	idx := New("/root")
	idx.Update("notes/api.md", []byte("# API\n"))
	idx.Update("a.md", []byte("[[api]]\nmore text\n[[api]]\n"))
	idx.Update("z.md", []byte("[[api]]\n"))

	got := idx.IncomingWikilinkEdges("api")
	require.Len(t, got, 3)
	// a.md line 1, a.md line 3, then z.md line 1.
	assert.Equal(t, "a.md", got[0].SourceFile)
	assert.Equal(t, 1, got[0].SourceLine)
	assert.Equal(t, "a.md", got[1].SourceFile)
	assert.Equal(t, 3, got[1].SourceLine)
	assert.Equal(t, "z.md", got[2].SourceFile)
}

func TestCollectWikilinkEdges_SkipsNonMarkdownEmbed(t *testing.T) {
	idx := New("/root")
	idx.Update("notes/diagram.png.md", []byte("# D\n"))
	// An `![[diagram.png]]` embed resolves by exact filename, not stem,
	// so it produces no wikilink edge keyed by a Markdown stem.
	idx.Update("guide.md", []byte("Embed: ![[diagram.png]] and [[page]]\n"))
	assert.Empty(t, idx.IncomingWikilinkEdges("diagram"))
	assert.Len(t, idx.IncomingWikilinkEdges("page"), 1)
}

// TestWikilinkEdgesStayOutOfBacklinks locks that adding the wikilink
// edge kind does not change the reverse-edge (path) queries: wikilinks
// are emitted Unresolved, so BacklinksFor and IncomingPathEdges skip
// them.
func TestWikilinkEdgesStayOutOfBacklinks(t *testing.T) {
	idx := New("/root")
	idx.Update("notes/api.md", []byte("# API\n"))
	idx.Update("notes/guide.md", []byte("See [[api]].\n"))

	assert.Empty(t, idx.BacklinksFor("notes/api.md"))
	assert.Empty(t, idx.IncomingPathEdges("notes/api.md"))
}

// TestIncomingWikilinkNameEdges locks the exact-name lookup for typed
// `[[name.ext]]` links: keyed by the lowercased base name, kept apart
// from the stem lookup (a `[[diagram.png]]` never answers a stem query
// for `diagram.png`, the stem of diagram.png.md), sorted by source, and
// skipped by the path queries like every unresolved edge.
func TestIncomingWikilinkNameEdges(t *testing.T) {
	idx := New("/root")
	idx.Update("z.md", []byte("![[img/Diagram.PNG]]\n"))
	idx.Update("a.md", []byte("x [[diagram.png|D]]\n\n![[diagram.png.md]] [[other.png]]\n"))
	got := idx.IncomingWikilinkNameEdges("Diagram.png")
	require.Len(t, got, 2)
	assert.Equal(t, "a.md", got[0].SourceFile)
	assert.Equal(t, 3, got[0].SourceCol)
	assert.Equal(t, "z.md", got[1].SourceFile)
	assert.Equal(t, EdgeWikilinkName, got[1].Kind)
	assert.Equal(t, "diagram.png", got[1].TargetLabel)
	assert.True(t, got[1].Unresolved)
	assert.Len(t, idx.IncomingWikilinkEdges("diagram.png"), 1, "only the Markdown [[diagram.png.md]]")
	assert.Empty(t, idx.IncomingWikilinkNameEdges("diagram"))
	assert.Empty(t, idx.IncomingPathEdges("diagram.png"))
	var nilIdx *Index
	assert.Nil(t, nilIdx.IncomingWikilinkNameEdges("diagram.png"))
}

// TestEdgeKind_IsWikilink locks which kinds are wikilink edges: the
// stem and the exact-name kind, and no other.
func TestEdgeKind_IsWikilink(t *testing.T) {
	assert.True(t, EdgeWikilink.IsWikilink())
	assert.True(t, EdgeWikilinkName.IsWikilink())
	for _, k := range []EdgeKind{EdgeAnchorLink, EdgeFileLink, EdgeRefLink, EdgeInclude, EdgeCatalog, EdgeBuild} {
		assert.False(t, k.IsWikilink(), k)
	}
}

// TestWikilinkEdges locks the shared lookup: only edges of the asked
// kind whose label equals the key, as given (callers key it first).
func TestWikilinkEdges(t *testing.T) {
	idx := New("/root")
	idx.Update("a.md", []byte("[[x.png]] [[x.png.md]] [[X.png]]\n"))
	assert.Len(t, idx.wikilinkEdges(EdgeWikilinkName, "x.png"), 2)
	assert.Len(t, idx.wikilinkEdges(EdgeWikilink, "x.png"), 1)
	assert.Empty(t, idx.wikilinkEdges(EdgeWikilinkName, "X.png"), "the key is not folded here")
}
