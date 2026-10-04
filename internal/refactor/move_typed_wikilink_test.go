package refactor

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/index"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// nameKey returns the wikilinkKey a typed `[[name.ext]]` link reaches
// a non-Markdown file by.
func nameKey(name string) wikilinkKey {
	return wikilinkKey{key: name}
}

// TestMove_TypedWikilinkFollowsRename locks that a typed `[[name.ext]]`
// link or `![[name.ext]]` embed follows a moved non-Markdown file to its
// new name: only the base segment is rewritten, in the destination's
// own casing, so any folder prefix, anchor, and alias stay.
func TestMove_TypedWikilinkFollowsRename(t *testing.T) {
	src := "![[img.png]] [[IMG.PNG|x]] [[assets/img.png#a]] [[img]]\n"
	ws := newMemWorkspace(map[string]string{
		"img.png": "png",
		"img.md":  "# Img\n",
		"n.md":    src,
	})
	plan, err := Move(ws, "img.png", "pics/Photo.png")
	require.NoError(t, err)
	assert.Equal(t, "![[Photo.png]] [[Photo.png|x]] [[assets/Photo.png#a]] [[img]]\n",
		applyEditsToSource(t, src, plan.Edits["n.md"]))
}

// TestMove_TypedWikilinkOnlyWhenSourceWinsName locks the "reached it
// before the move" guard: with a shallower img.png that wins the name,
// `[[img.png]]` never reached x/img.png and is left alone.
func TestMove_TypedWikilinkOnlyWhenSourceWinsName(t *testing.T) {
	ws := newMemWorkspace(map[string]string{
		"img.png":   "png",
		"x/img.png": "png",
		"n.md":      "[[img.png]]\n",
	})
	plan, err := Move(ws, "x/img.png", "x/photo.png")
	require.NoError(t, err)
	assert.Empty(t, plan.Edits["n.md"])
}

// TestMove_TypedWikilinkKeptName locks that a move that keeps the name
// (img.png to pics/img.png) plans no wikilink edit: the resolver reads
// the base name alone, so `[[img.png]]` still reaches the file and no
// other spelling would serve better.
func TestMove_TypedWikilinkKeptName(t *testing.T) {
	ws := newMemWorkspace(map[string]string{
		"img.png": "png",
		"n.md":    "[[img.png]]\n",
	})
	plan, err := Move(ws, "img.png", "pics/IMG.png")
	require.NoError(t, err)
	assert.Empty(t, plan.Edits["n.md"])
}

// TestMove_TypedWikilinkToMarkdownDestination locks that a typed link
// to a non-Markdown file moved to a Markdown name becomes a stem link.
func TestMove_TypedWikilinkToMarkdownDestination(t *testing.T) {
	src := "[[notes.txt]]\n"
	ws := newMemWorkspace(map[string]string{"notes.txt": "x", "n.md": src})
	plan, err := Move(ws, "notes.txt", "docs/Notes.md")
	require.NoError(t, err)
	assert.Equal(t, "[[Notes]]\n", applyEditsToSource(t, src, plan.Edits["n.md"]))
}

// TestMove_TypedWikilinkDestinationNameTaken locks that a rewrite whose
// new name a shallower file already holds is left alone, as a stem
// rewrite is.
func TestMove_TypedWikilinkDestinationNameTaken(t *testing.T) {
	ws := newMemWorkspace(map[string]string{
		"img.png":     "png",
		"photo.png":   "png",
		"n.md":        "[[img.png]]\n",
		"x/other.txt": "x",
	})
	plan, err := Move(ws, "img.png", "x/photo.png")
	require.NoError(t, err)
	assert.Empty(t, plan.Edits["n.md"])
}

// TestMoveAll_TypedWikilinkBlockedByMember locks that a typed rewrite
// whose new name another member's destination wins is withheld.
func TestMoveAll_TypedWikilinkBlockedByMember(t *testing.T) {
	bp := moveAll(t, map[string]string{
		"img.png": "png",
		"a.png":   "png",
		"n.md":    "# N\n\n[[img.png]]\n",
	}, MovePair{"img.png", "x/photo.png"}, MovePair{"a.png", "photo.png"})
	assert.Empty(t, texts(bp.Edits, "n.md"))
	assert.Equal(t, 1, bp.Withheld)
}

// TestMoveAll_TypedWikilinkSwap locks that two files swapping names
// each keep their typed links: every link follows its own file.
func TestMoveAll_TypedWikilinkSwap(t *testing.T) {
	src := "# N\n\n[[a.png]] [[b.png]]\n"
	bp := moveAll(t, map[string]string{
		"a.png": "png",
		"b.png": "png",
		"n.md":  src,
	}, MovePair{"a.png", "x/b.png"}, MovePair{"b.png", "x/a.png"})
	assert.Equal(t, "# N\n\n[[b.png]] [[a.png]]\n", applyEditsToSource(t, src, bp.Edits["n.md"]))
	assert.Zero(t, bp.Withheld)
}

func TestFileWikilinkKey(t *testing.T) {
	assert.Equal(t, stemKey("guide"), fileWikilinkKey("docs/Guide.md"))
	assert.Equal(t, stemKey("img.png"), fileWikilinkKey("img.png.markdown"))
	assert.Equal(t, nameKey("img.png"), fileWikilinkKey("a/IMG.png"))
	assert.Equal(t, nameKey("makefile"), fileWikilinkKey("Makefile"))
}

func TestWikilinkKey_Edges(t *testing.T) {
	ws := stubWorkspace{
		wikilinkEdges: []index.Edge{{SourceFile: "stem.md"}},
		nameEdges:     []index.Edge{{SourceFile: "name.md"}},
	}
	assert.Equal(t, "stem.md", stemKey("a").edges(ws)[0].SourceFile)
	assert.Equal(t, "name.md", nameKey("a.png").edges(ws)[0].SourceFile)
}

func TestWikilinkKey_HoldersAndResolvesTo(t *testing.T) {
	idx := holderIndex("x/img.png", "img.png", "img.png.md")
	assert.Equal(t, []string{"img.png", "x/img.png"}, nameKey("img.png").holders(idx))
	assert.Equal(t, []string{"img.png.md"}, stemKey("img.png").holders(idx))
	assert.True(t, nameKey("img.png").resolvesTo(idx, "img.png"))
	assert.False(t, nameKey("img.png").resolvesTo(idx, "x/img.png"))
	assert.True(t, stemKey("img.png").resolvesTo(idx, "img.png.md"))
}

func TestWikilinkKey_At(t *testing.T) {
	row := []byte("[[a/IMG.png|x]] [[img.png.md]]")
	start, end, ok := nameKey("img.png").at(row, 0)
	require.True(t, ok)
	assert.Equal(t, "IMG.png", string(row[start:end]))
	start, end, ok = stemKey("img.png").at(row, 16)
	require.True(t, ok)
	assert.Equal(t, "img.png.md", string(row[start:end]))
	_, _, ok = stemKey("img.png").at(row, 0)
	assert.False(t, ok, "a typed link has no stem key")
	_, _, ok = nameKey("img.png").at(row, 16)
	assert.False(t, ok, "a Markdown link has no name key")
	_, _, ok = nameKey("other.png").at(row, 0)
	assert.False(t, ok, "a stale edge whose link names another file")
}

// TestTypedKeys_BatchHelpers locks the name-key cases of the helpers
// the stem path already covers: a name key matches only a destination
// of the same name, and members are counted by name.
func TestTypedKeys_BatchHelpers(t *testing.T) {
	got, ok := newWikilinkTarget(nameKey("img.png"), "x/photo.png")
	require.True(t, ok)
	assert.Equal(t, wikilinkTarget{dst: "x/photo.png", spelling: "photo.png", wikilinkKey: nameKey("photo.png")}, got)
	_, ok = newWikilinkTarget(nameKey("img.png"), "x/IMG.png")
	assert.False(t, ok, "the name is kept")
	_, ok = newWikilinkTarget(stemKey("img.png"), "x/img.png")
	assert.True(t, ok, "a stem and a name are separate keys")

	b := newMoveBatch()
	b.members["img.png"] = batchMember{dst: "x/img.png", planned: true}
	b.members["a.png"] = batchMember{dst: "y/IMG.png", planned: true}
	b.members["img.md"] = batchMember{dst: "z/img.md", planned: true}
	assert.Equal(t, 2, b.keyHolders(nameKey("img.png")))
	assert.Equal(t, 1, b.keyHolders(stemKey("img")))
	assert.Equal(t, []string{"img.png"}, b.keySources(nameKey("img.png")))
	r := &destResolver{batch: b}
	kept, ok := r.keptWikilinkTarget(nameKey("img.png"), "x/img.png")
	require.True(t, ok)
	assert.Equal(t, wikilinkTarget{dst: "x/img.png", wikilinkKey: nameKey("img.png")}, kept)
	_, ok = r.keptWikilinkTarget(nameKey("img.png"), "x/img.md")
	assert.False(t, ok)
}
