package linkgraph

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractWikiLinks_NilFileReturnsNil(t *testing.T) {
	assert.Nil(t, ExtractWikiLinks(nil))
}

func TestExtractWikiLinks_EmptySource(t *testing.T) {
	f := newFile(t, "")
	assert.Nil(t, ExtractWikiLinks(f))
}

func TestResolveWikiLink_WhitespaceTarget(t *testing.T) {
	mfs := fstest.MapFS{"page.md": &fstest.MapFile{Data: []byte{}}}
	_, ok := ResolveWikiLink(mfs, "from.md", "   ")
	assert.False(t, ok)
}

func TestExtractWikiLinks_BarePage(t *testing.T) {
	f := newFile(t, "# Doc\n\nSee [[Page]] for context.\n")
	got := ExtractWikiLinks(f)
	require.Len(t, got, 1)
	assert.Equal(t, "Page", got[0].Target)
	assert.Empty(t, got[0].Anchor)
	assert.Empty(t, got[0].Alias)
	assert.False(t, got[0].Embed)
	assert.Equal(t, 3, got[0].Line)
	assert.Equal(t, 5, got[0].Column)
}

func TestExtractWikiLinks_AnchorAndAlias(t *testing.T) {
	f := newFile(t, "Refer to [[Notes#Heading|the notes]].\n")
	got := ExtractWikiLinks(f)
	require.Len(t, got, 1)
	assert.Equal(t, "Notes", got[0].Target)
	assert.Equal(t, "Heading", got[0].Anchor)
	assert.Equal(t, "the notes", got[0].Alias)
}

func TestExtractWikiLinks_AliasOnly(t *testing.T) {
	f := newFile(t, "See [[Page|Display]].\n")
	got := ExtractWikiLinks(f)
	require.Len(t, got, 1)
	assert.Equal(t, "Page", got[0].Target)
	assert.Equal(t, "Display", got[0].Alias)
	assert.Empty(t, got[0].Anchor)
}

func TestExtractWikiLinks_Embed(t *testing.T) {
	f := newFile(t, "Inline ![[image.png]] embed.\n")
	got := ExtractWikiLinks(f)
	require.Len(t, got, 1)
	assert.Equal(t, "image.png", got[0].Target)
	assert.True(t, got[0].Embed)
	// Column points at the leading '[', not the '!'.
	assert.Equal(t, 9, got[0].Column)
}

func TestExtractWikiLinks_SkipsCodeSpan(t *testing.T) {
	f := newFile(t, "Inline `[[NotALink]]` should be ignored.\n")
	got := ExtractWikiLinks(f)
	assert.Empty(t, got)
}

func TestExtractWikiLinks_EmptyCodeSpan(t *testing.T) {
	// Empty backticks (`` `` ``) parse as a CodeSpan with no Text
	// children — codeSpanTextBounds returns first<0 and the range
	// is dropped from the span list, so the extractor must not
	// panic and must still find the wikilink that follows.
	f := newFile(t, "A `` `` literal, then [[Page]].\n")
	got := ExtractWikiLinks(f)
	require.Len(t, got, 1)
	assert.Equal(t, "Page", got[0].Target)
}

func TestExtractWikiLinks_SkipsFencedCode(t *testing.T) {
	src := "```\n[[InFence]]\n```\n"
	f := newFile(t, src)
	got := ExtractWikiLinks(f)
	assert.Empty(t, got)
}

func TestExtractWikiLinks_SkipsPIBlock(t *testing.T) {
	// A wikilink on a directive marker line is skipped — every line
	// goldmark reports inside the `<?...?>` block (open, body, close)
	// counts as PI content, the same exclusion MDS054's scanner uses.
	src := "<?some-directive\n[[InPI]]\n?>\n"
	f := newFile(t, src)
	got := ExtractWikiLinks(f)
	assert.Empty(t, got)
}

func TestExtractWikiLinks_Multiple(t *testing.T) {
	src := "See [[One]] and [[Two|x]] and [[Three#frag]].\n"
	f := newFile(t, src)
	got := ExtractWikiLinks(f)
	require.Len(t, got, 3)
	assert.Equal(t, "One", got[0].Target)
	assert.Equal(t, "Two", got[1].Target)
	assert.Equal(t, "Three", got[2].Target)
	assert.Equal(t, "frag", got[2].Anchor)
}

func TestExtractWikiLinks_NoNewlinesInsideBrackets(t *testing.T) {
	// A "wikilink" split across a newline is not a wikilink; the regex
	// rejects internal newlines so this paragraph yields zero matches.
	src := "See [[Page\nname]].\n"
	f := newFile(t, src)
	got := ExtractWikiLinks(f)
	assert.Empty(t, got)
}

func TestResolveWikiLink_ExactStem(t *testing.T) {
	mfs := fstest.MapFS{
		"notes.md": &fstest.MapFile{Data: []byte("# Notes\n")},
	}
	path, ok := ResolveWikiLink(mfs, "from.md", "notes")
	require.True(t, ok)
	assert.Equal(t, "notes.md", path)
}

func TestResolveWikiLink_CaseInsensitive(t *testing.T) {
	mfs := fstest.MapFS{
		"Notes.md": &fstest.MapFile{Data: []byte("# Notes\n")},
	}
	path, ok := ResolveWikiLink(mfs, "from.md", "notes")
	require.True(t, ok)
	assert.Equal(t, "Notes.md", path)
}

func TestResolveWikiLink_ShortestPathWins(t *testing.T) {
	mfs := fstest.MapFS{
		"deep/sub/notes.md": &fstest.MapFile{Data: []byte{}},
		"notes.md":          &fstest.MapFile{Data: []byte{}},
		"other/notes.md":    &fstest.MapFile{Data: []byte{}},
	}
	path, ok := ResolveWikiLink(mfs, "from.md", "notes")
	require.True(t, ok)
	assert.Equal(t, "notes.md", path)
}

func TestResolveWikiLink_AlphabeticalTieBreak(t *testing.T) {
	mfs := fstest.MapFS{
		"a/notes.md": &fstest.MapFile{Data: []byte{}},
		"b/notes.md": &fstest.MapFile{Data: []byte{}},
	}
	path, ok := ResolveWikiLink(mfs, "from.md", "notes")
	require.True(t, ok)
	assert.Equal(t, "a/notes.md", path)
}

func TestResolveWikiLink_NotFound(t *testing.T) {
	mfs := fstest.MapFS{
		"other.md": &fstest.MapFile{Data: []byte{}},
	}
	_, ok := ResolveWikiLink(mfs, "from.md", "missing")
	assert.False(t, ok)
}

func TestResolveWikiLink_EmbedExactName(t *testing.T) {
	mfs := fstest.MapFS{
		"assets/diagram.png": &fstest.MapFile{Data: []byte{}},
		"diagram.md":         &fstest.MapFile{Data: []byte{}},
	}
	path, ok := ResolveWikiLink(mfs, "from.md", "diagram.png")
	require.True(t, ok)
	assert.Equal(t, "assets/diagram.png", path)
}

func TestResolveWikiLink_EmbedNotFound(t *testing.T) {
	mfs := fstest.MapFS{
		"other.png": &fstest.MapFile{Data: []byte{}},
	}
	_, ok := ResolveWikiLink(mfs, "from.md", "missing.png")
	assert.False(t, ok)
}

func TestResolveWikiLink_RejectsRootEscape(t *testing.T) {
	mfs := fstest.MapFS{
		"notes.md": &fstest.MapFile{Data: []byte{}},
	}
	_, ok := ResolveWikiLink(mfs, "from.md", "../etc/passwd")
	assert.False(t, ok)
}

func TestResolveWikiLink_AcceptsDoubleDotInName(t *testing.T) {
	// A bare ".." in the middle of a stem must not be confused with a
	// parent-dir traversal. The wikilink writes the full filename
	// (`v1..v2.md`) so path.Ext can identify ".md" as the extension and
	// the search falls into stem mode against the matching file.
	mfs := fstest.MapFS{
		"v1..v2.md": &fstest.MapFile{Data: []byte{}},
	}
	got, ok := ResolveWikiLink(mfs, "from.md", "v1..v2.md")
	require.True(t, ok)
	assert.Equal(t, "v1..v2.md", got)
}

func TestResolveWikiLink_RejectsCollapsedTraversal(t *testing.T) {
	// path.Clean reduces "a/../../etc" to "../etc" — the check must
	// catch traversal hidden behind a leading legitimate segment.
	mfs := fstest.MapFS{
		"notes.md": &fstest.MapFile{Data: []byte{}},
	}
	_, ok := ResolveWikiLink(mfs, "from.md", "a/../../etc/passwd")
	assert.False(t, ok)
}

func TestResolveWikiLink_RejectsAbsolutePath(t *testing.T) {
	mfs := fstest.MapFS{
		"notes.md": &fstest.MapFile{Data: []byte{}},
	}
	_, ok := ResolveWikiLink(mfs, "from.md", "/notes.md")
	assert.False(t, ok)
}

func TestResolveWikiLink_EmptyTarget(t *testing.T) {
	mfs := fstest.MapFS{}
	_, ok := ResolveWikiLink(mfs, "from.md", "")
	assert.False(t, ok)
}

func TestResolveWikiLink_NilFS(t *testing.T) {
	_, ok := ResolveWikiLink(nil, "from.md", "page")
	assert.False(t, ok)
}

func TestResolveWikiLink_WalkDirCallbackError(t *testing.T) {
	// fs.WalkDir invokes the callback with err != nil when ReadDir
	// on a child directory fails. ResolveWikiLink must swallow the
	// error and keep walking the rest of the tree. erroringFS
	// rejects ReadDir("broken") while serving every other path
	// normally; resolution finds page.md in the sibling subtree.
	mfs := &erroringFS{
		inner: fstest.MapFS{
			"broken":        &fstest.MapFile{Mode: fs.ModeDir},
			"other/page.md": &fstest.MapFile{Data: []byte{}},
		},
		failDir: "broken",
	}
	got, ok := ResolveWikiLink(mfs, "from.md", "page")
	require.True(t, ok)
	assert.Equal(t, "other/page.md", got)
}

// erroringFS rejects ReadDir on a specific subdirectory while
// serving Open and other paths normally. fs.WalkDir then invokes
// its callback with err != nil for the rejected directory — the
// exact branch ResolveWikiLink swallows.
type erroringFS struct {
	inner   fs.FS
	failDir string
}

func (e *erroringFS) Open(name string) (fs.File, error) {
	return e.inner.Open(name)
}

func (e *erroringFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if name == e.failDir {
		return nil, &fsErr{name: name}
	}
	return fs.ReadDir(e.inner, name)
}

type fsErr struct{ name string }

func (e *fsErr) Error() string { return "synthetic read failure on " + e.name }

func TestResolveWikiLink_OnDiskFS(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sub", "page.md"), []byte("#h\n"), 0o644))
	root, err := openDirFS(dir)
	require.NoError(t, err)
	path, ok := ResolveWikiLink(root, "from.md", "page")
	require.True(t, ok)
	assert.Equal(t, "sub/page.md", path)
}

// openDirFS is a tiny wrapper so the helper above can return an error
// alongside the FS, without leaking os.DirFS internals into the test
// body.
func openDirFS(dir string) (fs.FS, error) {
	return os.DirFS(dir), nil
}
