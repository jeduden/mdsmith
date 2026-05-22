package linkgraph

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"

	"github.com/jeduden/mdsmith/internal/lint"
)

func TestExtractWikiLinks_NilFileReturnsNil(t *testing.T) {
	assert.Nil(t, ExtractWikiLinks(nil))
}

func TestExtractWikiLinks_EmptySource(t *testing.T) {
	f := newFile(t, "")
	assert.Nil(t, ExtractWikiLinks(f))
}

func TestExtractWikiLinks_NilASTReturnsNilNoPanic(t *testing.T) {
	// lint.File explicitly supports the struct-literal construction
	// path where AST is never populated. The extractor walks the
	// AST via CollectCodeBlockLines / CollectPIBlockLines, so it
	// must short-circuit instead of panicking on a nil tree.
	f := &lint.File{Source: []byte("[[Page]]\n")}
	assert.NotPanics(t, func() {
		assert.Nil(t, ExtractWikiLinks(f))
	})
}

func TestNewWikilinkIndex_NilRoot(t *testing.T) {
	assert.Nil(t, NewWikilinkIndex(nil))
}

func TestWikilinkIndex_ResolveSemantics(t *testing.T) {
	// One index should serve every shape ResolveWikiLink supports:
	// stem (.md), embed (any extension), case-insensitive match,
	// shortest-path tie-break, traversal rejection, drive-letter
	// rejection, backslash normalisation.
	mfs := fstest.MapFS{
		"notes.md":          &fstest.MapFile{Data: []byte{}},
		"deep/sub/notes.md": &fstest.MapFile{Data: []byte{}},
		"assets/img.png":    &fstest.MapFile{Data: []byte{}},
		"sub/page.md":       &fstest.MapFile{Data: []byte{}},
	}
	idx := NewWikilinkIndex(mfs)
	require.NotNil(t, idx)

	cases := []struct {
		name   string
		target string
		want   string
		wantOK bool
	}{
		{"stem case-insensitive", "Notes", "notes.md", true},
		{"shortest path", "notes", "notes.md", true},
		{"embed exact name", "img.png", "assets/img.png", true},
		{"backslash normalised", `sub\page`, "sub/page.md", true},
		{"missing", "absent", "", false},
		{"traversal rejected", "../etc/passwd", "", false},
		{"drive rejected", "C:/Windows/notes.md", "", false},
		{"UNC rejected", "//host/share/notes.md", "", false},
		{"absolute rejected", "/notes.md", "", false},
		{"empty rejected", "", "", false},
		{"nil index", "notes", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			target := idx
			if tc.name == "nil index" {
				target = nil
			}
			got, ok := target.Resolve(tc.target)
			assert.Equal(t, tc.wantOK, ok)
			if ok {
				assert.Equal(t, tc.want, got)
			}
		})
	}
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

func TestResolveWikiLink_NormalizesBackslashSegments(t *testing.T) {
	// A Windows-authored wikilink like `[[sub\page]]` arrives on
	// Linux CI as the literal string "sub\page" — filepath.ToSlash
	// is a no-op on POSIX. The resolver must collapse backslashes
	// to slashes itself so cross-host vaults still resolve.
	mfs := fstest.MapFS{
		"sub/page.md": &fstest.MapFile{Data: []byte{}},
	}
	got, ok := ResolveWikiLink(mfs, "from.md", `sub\page`)
	require.True(t, ok)
	assert.Equal(t, "sub/page.md", got)
}

func TestResolveWikiLink_RejectsAbsolutePath(t *testing.T) {
	mfs := fstest.MapFS{
		"notes.md": &fstest.MapFile{Data: []byte{}},
	}
	_, ok := ResolveWikiLink(mfs, "from.md", "/notes.md")
	assert.False(t, ok)
}

func TestResolveWikiLink_RejectsWindowsAbsoluteForms(t *testing.T) {
	// On POSIX hosts a Windows drive-letter or UNC path would
	// otherwise pass the leading-slash check and be searched as a
	// workspace-relative stem. The drive/UNC guard matches the one
	// linkgraph.ResolveRelTarget uses for the same reason.
	mfs := fstest.MapFS{
		"system.md": &fstest.MapFile{Data: []byte{}},
	}
	for _, target := range []string{
		"C:/Windows/system.md",
		"c:/Windows/system.md",
		"//host/share/system.md",
	} {
		_, ok := ResolveWikiLink(mfs, "from.md", target)
		assert.Falsef(t, ok, "Windows-absolute %q must be rejected", target)
	}
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

func TestWikilinkSearchKey(t *testing.T) {
	cases := []struct {
		name     string
		target   string
		wantName string
		wantStem string
		stemMode bool
	}{
		{"no extension → stem mode", "Notes", "", "Notes", true},
		{"md extension → stem mode", "Notes.md", "", "Notes", true},
		{"markdown extension → stem mode", "Notes.markdown", "", "Notes", true},
		{"PNG embed → exact name", "image.png", "image.png", "", false},
		{"backslash normalised", `sub\page`, "", "page", true},
		{"nested path → basename only", "deep/sub/page", "", "page", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n, s, mode := wikilinkSearchKey(tc.target)
			assert.Equal(t, tc.wantName, n, "wantName")
			assert.Equal(t, tc.wantStem, s, "wantStem")
			assert.Equal(t, tc.stemMode, mode, "stemMode")
		})
	}
}

func TestIsMarkdownName(t *testing.T) {
	assert.True(t, isMarkdownName("page.md"))
	assert.True(t, isMarkdownName("page.MD"))
	assert.True(t, isMarkdownName("page.markdown"))
	assert.False(t, isMarkdownName("page.txt"))
	assert.False(t, isMarkdownName("page"))
}

func TestSortByDepthThenName(t *testing.T) {
	// Mixed depths and matching depths exercise both keys of the
	// sort: shorter paths come first, ties break alphabetically.
	paths := []string{
		"b/page.md",
		"a/page.md",
		"page.md",
		"a/sub/page.md",
	}
	sortByDepthThenName(paths)
	assert.Equal(t, []string{
		"page.md",
		"a/page.md",
		"b/page.md",
		"a/sub/page.md",
	}, paths)
}

func TestCodeSpanTextBounds(t *testing.T) {
	// One Text child → bounds equal that text's segment. A non-Text
	// child is skipped (continue). Two Text children expand the
	// range. Zero Text children → -1, -1.
	src := []byte("`abc`")
	cs := ast.NewCodeSpan()
	t1 := ast.NewTextSegment(text.NewSegment(1, 3))
	cs.AppendChild(cs, t1)
	first, last := codeSpanTextBounds(cs)
	assert.Equal(t, 1, first)
	assert.Equal(t, 3, last)

	csNoText := ast.NewCodeSpan()
	first, last = codeSpanTextBounds(csNoText)
	assert.Equal(t, -1, first)
	assert.Equal(t, -1, last)

	csMixed := ast.NewCodeSpan()
	csMixed.AppendChild(csMixed, ast.NewAutoLink(ast.AutoLinkURL, ast.NewTextSegment(text.NewSegment(0, 0))))
	csMixed.AppendChild(csMixed, ast.NewTextSegment(text.NewSegment(2, 4)))
	first, last = codeSpanTextBounds(csMixed)
	assert.Equal(t, 2, first)
	assert.Equal(t, 4, last)
	_ = src
}

func TestInCodeSpan(t *testing.T) {
	spans := []byteRange{{start: 5, end: 10}, {start: 20, end: 25}}
	assert.True(t, inCodeSpan(spans, 5))
	assert.True(t, inCodeSpan(spans, 9))
	assert.False(t, inCodeSpan(spans, 10), "end is exclusive")
	assert.False(t, inCodeSpan(spans, 4))
	assert.False(t, inCodeSpan(spans, 100))
	assert.False(t, inCodeSpan(nil, 0))
}

func TestCollectCodeSpanRanges_EmptyCodeSpan(t *testing.T) {
	// Drive the `first < 0` early return in collectCodeSpanRanges by
	// handing it an AST with a CodeSpan that has no Text children.
	// goldmark won't usually emit one, but a struct-literal node
	// proves the guard works without relying on parser quirks.
	f := newFile(t, "ignored\n")
	root := ast.NewDocument()
	root.AppendChild(root, ast.NewCodeSpan())
	f.AST = root
	got := collectCodeSpanRanges(f)
	assert.Empty(t, got, "empty CodeSpan must yield no range")
}

func TestWikilinkIndex_Resolve_EmbedNotFound(t *testing.T) {
	// An embed lookup (exact-name) that misses falls through to the
	// final `return "", false`. The stem case already hits its own
	// "", false path via TestWikilinkIndex_ResolveSemantics.
	mfs := fstest.MapFS{
		"img.png": &fstest.MapFile{Data: []byte{}},
	}
	idx := NewWikilinkIndex(mfs)
	_, ok := idx.Resolve("missing.png")
	assert.False(t, ok)
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
