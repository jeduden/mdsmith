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
