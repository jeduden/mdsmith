package linkgraph

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
	"github.com/jeduden/mdsmith/pkg/goldmark/text"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/runcache"
)

func TestExtractWikiLinks_NilFileReturnsNil(t *testing.T) {
	assert.Nil(t, ExtractWikiLinks(nil))
}

func TestWikilinkStem(t *testing.T) {
	cases := []struct {
		name     string
		target   string
		wantStem string
		wantOK   bool
	}{
		{"bare page", "Page", "page", true},
		{"markdown extension", "Notes.md", "notes", true},
		{"foldered stem uses basename", "folder/API", "api", true},
		{"mixed case lowercased", "MyDoc", "mydoc", true},
		{"typed non-markdown returns false", "diagram.png", "", false},
		{"empty returns false", "", "", false},
		{"traversal returns false", "../secret", "", false},
		{"absolute returns false", "/etc/passwd", "", false},
		{"padded absolute returns false", " /etc/passwd", "", false},
		{"backslash root-relative returns false", `\notes`, "", false},
		{"backslash UNC returns false", `\\host\share\notes`, "", false},
		{"drive letter returns false", `C:\notes`, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stem, ok := WikilinkStem(tc.target)
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.wantStem, stem)
		})
	}
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

func TestWikilinkIndexFor_NilCacheBuildsDirectly(t *testing.T) {
	// A nil cache (or empty key) falls through to NewWikilinkIndex
	// — used by `mdsmith list backlinks` which has no engine-wide
	// RunCache. Two calls then walk twice; the cache path is what
	// turns the second call into an O(1) lookup.
	mfs := fstest.MapFS{"page.md": &fstest.MapFile{Data: []byte{}}}
	idx := linkgraphWikilinkIndexFor(nil, "", mfs)
	require.NotNil(t, idx)
	got, ok := idx.Resolve("page")
	require.True(t, ok)
	assert.Equal(t, "page.md", got)
}

func TestWikilinkIndexFor_CachedAcrossCallers(t *testing.T) {
	// With a shared cache + key, two callers see the same index.
	// MDS027 takes this branch via the engine's RunCache so a
	// workspace walked for host file A serves host file B too.
	mfs := fstest.MapFS{"page.md": &fstest.MapFile{Data: []byte{}}}
	cache := runcache.New()
	a := linkgraphWikilinkIndexFor(cache, "/root", mfs)
	b := linkgraphWikilinkIndexFor(cache, "/root", mfs)
	require.NotNil(t, a)
	assert.Same(t, a, b, "cache must hand out the same *WikilinkIndex per key")
}

// linkgraphWikilinkIndexFor is a thin alias so the test reads
// naturally as a unit test of the package-local helper.
func linkgraphWikilinkIndexFor(cache *runcache.Cache, key string, root fs.FS) *WikilinkIndex {
	return WikilinkIndexFor(cache, key, root)
}

func TestSkipHeavyDirs(t *testing.T) {
	assert.Nil(t, skipHeavyDirs("."))
	assert.Nil(t, skipHeavyDirs("docs"))
	assert.Nil(t, skipHeavyDirs("plan/sub"))
	assert.Equal(t, fs.SkipDir, skipHeavyDirs(".git"))
	assert.Equal(t, fs.SkipDir, skipHeavyDirs("vendor/dep/node_modules"))
	assert.Equal(t, fs.SkipDir, skipHeavyDirs("sub/.git"))
}

func TestNewWikilinkIndex_PrunesHeavyDirs(t *testing.T) {
	// A wikilink target that lives under .git/ or node_modules/
	// must not show up in the index — both directories carry no
	// content users intend to wikilink against, and skipping them
	// keeps the workspace walk bounded on real repos.
	mfs := fstest.MapFS{
		"page.md":                                &fstest.MapFile{Data: []byte{}},
		".git/HEAD":                              &fstest.MapFile{Data: []byte{}},
		".git/sub/page.md":                       &fstest.MapFile{Data: []byte{}},
		"node_modules/lib/page.md":               &fstest.MapFile{Data: []byte{}},
		"vendor/dep/node_modules/inside/page.md": &fstest.MapFile{Data: []byte{}},
	}
	idx := NewWikilinkIndex(mfs)
	require.NotNil(t, idx)
	got, ok := idx.Resolve("page")
	require.True(t, ok)
	assert.Equal(t, "page.md", got,
		"only the top-level page.md should survive pruning")
}

func TestResolveWikiLink_PrunesHeavyDirs(t *testing.T) {
	// The fallback per-call walk must apply the same pruning so
	// a vault without a cached index does not pay for walking
	// node_modules.
	mfs := fstest.MapFS{
		"page.md":                  &fstest.MapFile{Data: []byte{}},
		"node_modules/lib/page.md": &fstest.MapFile{Data: []byte{}},
	}
	got, ok := ResolveWikiLink(mfs, "from.md", "page")
	require.True(t, ok)
	assert.Equal(t, "page.md", got)
}

func TestNewWikilinkIndex_NilRoot(t *testing.T) {
	assert.Nil(t, NewWikilinkIndex(nil))
}

func TestNewWikilinkIndex_RootWalkErrorReturnsNil(t *testing.T) {
	// When fs.WalkDir cannot read the root (e.g. ReadDir(".")
	// fails), NewWikilinkIndex returns nil so the resolver can
	// fall back to per-call walks instead of an empty index that
	// would silently report every target as "not found".
	root := &rootFailFS{}
	assert.Nil(t, NewWikilinkIndex(root))
}

// rootFailFS rejects the root ReadDir but accepts every other
// call. fs.WalkDir starts with Stat(".") which goes through
// Open(".") on the fallback path, then calls ReadDirFile on the
// returned file. Returning a Stat-only file forces WalkDir to
// fall back to ReadDir(".") on the fsys, which we reject.
type rootFailFS struct{}

func (rootFailFS) Open(name string) (fs.File, error) {
	if name == "." {
		return &rootStubDir{}, nil
	}
	return nil, fs.ErrNotExist
}

func (rootFailFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return nil, fs.ErrPermission
}

type rootStubDir struct{}

func (rootStubDir) Stat() (fs.FileInfo, error) { return rootStubInfo{}, nil }
func (rootStubDir) Read([]byte) (int, error)   { return 0, fs.ErrInvalid }
func (rootStubDir) Close() error               { return nil }

type rootStubInfo struct{}

func (rootStubInfo) Name() string       { return "." }
func (rootStubInfo) Size() int64        { return 0 }
func (rootStubInfo) Mode() fs.FileMode  { return fs.ModeDir }
func (rootStubInfo) ModTime() time.Time { return time.Time{} }
func (rootStubInfo) IsDir() bool        { return true }
func (rootStubInfo) Sys() any           { return nil }

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
		{"padded absolute rejected", " /notes.md", "", false},
		{"backslash root-relative rejected", `\notes.md`, "", false},
		{"backslash UNC rejected", `\\host\share\notes.md`, "", false},
		{"empty rejected", "", "", false},
		{"dot rejected", "./", "", false},
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

func TestExtractWikiLinks_NoBracketsAllocBudget(t *testing.T) {
	// The vast majority of files in a workspace contain no `[[` at
	// all, so ExtractWikiLinks must not pay for CollectCodeBlockLines,
	// CollectPIBlockLines, or the code-span AST walk before it has
	// even confirmed the source can match wikilinkRE
	// (docs/development/high-performance-go.md, "Gate expensive
	// analyzers behind a cheap pre-check").
	f := newFile(t, "# Doc\n\nSome ordinary prose with `a code span` and "+
		"a [normal link](x.md), but no wikilinks at all.\n")
	allocs := testing.AllocsPerRun(100, func() {
		_ = ExtractWikiLinks(f)
	})
	if allocs > 0 {
		t.Fatalf("ExtractWikiLinks allocs per call on bracket-free source: want 0, got %v", allocs)
	}
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

func TestInCodeSpan_ManySpans(t *testing.T) {
	// 1000 ordered, disjoint spans: [10i, 10i+4).
	spans := make([]byteRange, 0, 1000)
	for i := 0; i < 1000; i++ {
		spans = append(spans, byteRange{start: 10 * i, end: 10*i + 4})
	}
	for i := 0; i < 1000; i++ {
		assert.True(t, inCodeSpan(spans, 10*i), "start of span %d", i)
		assert.True(t, inCodeSpan(spans, 10*i+3), "last byte of span %d", i)
		assert.False(t, inCodeSpan(spans, 10*i+4), "end of span %d is exclusive", i)
		assert.False(t, inCodeSpan(spans, 10*i+9), "gap after span %d", i)
	}
	assert.False(t, inCodeSpan(spans, -1))
	assert.False(t, inCodeSpan(spans, 20000))
	assert.False(t, inCodeSpan([]byteRange{{start: 5, end: 5}}, 5), "zero-width span is empty")
}

func BenchmarkInCodeSpan(b *testing.B) {
	spans := make([]byteRange, 0, 2000)
	for i := 0; i < 2000; i++ {
		spans = append(spans, byteRange{start: 10 * i, end: 10*i + 4})
	}
	b.ReportAllocs()
	for b.Loop() {
		for i := 0; i < 2000; i++ {
			inCodeSpan(spans, 10*i+7)
		}
	}
}

// inCodeSpan binary-searches, so collectCodeSpanRanges must return
// sorted, disjoint spans for real parsed documents.
func TestCollectCodeSpanRanges_SortedDisjoint(t *testing.T) {
	src := "`a` text [[x]] ``b`` and\n\n- item `c`\n\n> quote `d` `e`\n\n[^1]: note `f`\n\nText[^1] `g`\n"
	f, err := lint.NewFile("t.md", []byte(src))
	require.NoError(t, err)
	spans := collectCodeSpanRanges(f)
	require.NotEmpty(t, spans)
	for i := 1; i < len(spans); i++ {
		assert.LessOrEqual(t, spans[i-1].end, spans[i].start, "span %d overlaps or precedes span %d", i, i-1)
	}
}

func TestWikilinkIndex_StemAndNamePaths(t *testing.T) {
	idx := NewWikilinkIndex(fstest.MapFS{
		"docs/Guide.md":                 {},
		"archive/guide.md":              {},
		"node_modules/pkg/guide.md":     {},
		"img/logo.png":                  {},
		"logo.png":                      {},
		"a/guide.mdx":                   {},
		"notes/license":                 {},
		".git/hooks/guide.md":           {},
		"docs/nested/deep/different.md": {},
	})
	require.NotNil(t, idx)
	assert.Equal(t, []string{"archive/guide.md", "docs/Guide.md"}, idx.StemPaths("guide"))
	assert.Equal(t, []string{"logo.png", "img/logo.png"}, idx.NamePaths("logo.png"))
	assert.Equal(t, []string{"a/guide.mdx"}, idx.NamePaths("guide.mdx"))
	assert.Empty(t, idx.StemPaths("license"), "an extensionless file has no stem key")
	assert.Empty(t, idx.StemPaths("missing"))
	assert.Empty(t, idx.NamePaths("missing"))
}

// TestWikilinkIndexed locks that WikilinkIndexed answers the walk's own
// skip rule: a file under `.git` or `node_modules` at any depth is not
// indexed, and a file merely named like one is.
func TestWikilinkIndexed(t *testing.T) {
	for p, want := range map[string]bool{
		"guide.md":                  true,
		"docs/guide.md":             true,
		"node_modules":              true,
		"docs/node_modules.md":      true,
		"node_modules/pkg/guide.md": false,
		"docs/node_modules/x.md":    false,
		".git/guide.md":             false,
		"a/b/.git/c/guide.md":       false,
	} {
		assert.Equal(t, want, WikilinkIndexed(p), p)
		fsys := fstest.MapFS{p: {}}
		assert.Equal(t, want, len(NewWikilinkIndex(fsys).NamePaths(FileNameKey(path.Base(p)))) == 1,
			"%s: the walk agrees", p)
	}
}

func TestWikilinkIndex_PathsNilReceiver(t *testing.T) {
	var idx *WikilinkIndex
	assert.Empty(t, idx.StemPaths("a"))
	assert.Empty(t, idx.NamePaths("a.md"))
}

// TestWikilinkStemAt_Span locks the base span WikilinkStemAt returns:
// the last segment of the trimmed target, with `\` read as `/`, outside
// any folder prefix, anchor, alias, or table-cell `\|` escape.
func TestWikilinkStemAt_Span(t *testing.T) {
	t.Run("not a wikilink returns false", func(t *testing.T) {
		_, _, _, ok := WikilinkStemAt([]byte("[x](y)"), 0)
		assert.False(t, ok)
	})
	t.Run("out-of-range bracket start returns false", func(t *testing.T) {
		_, _, _, ok := WikilinkStemAt([]byte("[["), 0)
		assert.False(t, ok)
		_, _, _, ok = WikilinkStemAt([]byte("[[a]]"), -1)
		assert.False(t, ok)
		_, _, _, ok = WikilinkStemAt([]byte("[[a]]"), 9)
		assert.False(t, ok)
	})
	t.Run("a link that starts later is not read", func(t *testing.T) {
		_, _, _, ok := WikilinkStemAt([]byte("x [[a]]"), 0)
		assert.False(t, ok)
	})
	t.Run("empty target returns false", func(t *testing.T) {
		_, _, _, ok := WikilinkStemAt([]byte("[[#frag]]"), 0)
		assert.False(t, ok)
	})
	t.Run("offsets are relative to the row", func(t *testing.T) {
		row := []byte("see ![[folder/Page#f|alias]] now")
		_, s, e, ok := WikilinkStemAt(row, 5)
		require.True(t, ok)
		assert.Equal(t, "Page", string(row[s:e]))
	})
	// The resolver turns `\` into `/` and reads path.Base of the
	// trimmed target, so the span is the last segment the same way.
	for row, want := range map[string]string{
		`[[docs\Page]]`:        "Page",
		`[[Page\|alias]]`:      "Page",
		"[[docs/Page/ ]]":      "Page",
		`[[docs\Page\#f|a]]`:   "Page",
		"[[ Page ]]":           "Page",
		"[[x/Page.md#f]]":      "Page.md",
		`[[a\b/c\Page.md|al]]`: "Page.md",
		"[[x/ guide]]":         " guide",
		"[[api /]]":            "api ",
	} {
		t.Run(row, func(t *testing.T) {
			_, s, e, ok := WikilinkStemAt([]byte(row), 0)
			require.True(t, ok)
			assert.Equal(t, want, row[s:e])
		})
	}
	for _, row := range []string{`[[/\ ]]`, "[[/abs]]", "[[../up]]", "[[C:\\x]]"} {
		t.Run("unresolvable "+row, func(t *testing.T) {
			_, _, _, ok := WikilinkStemAt([]byte(row), 0)
			assert.False(t, ok)
		})
	}
}

// TestNewWikilinkIndexFromPaths locks that an index built from a path
// list keys and orders files the way the walk does, skipping `.git`
// and `node_modules`.
func TestNewWikilinkIndexFromPaths(t *testing.T) {
	paths := []string{"docs/guide.md", "guide.md", "img/Logo.PNG", "node_modules/p/guide.md", ".git/x.md"}
	fsys := fstest.MapFS{}
	for _, p := range paths {
		fsys[p] = &fstest.MapFile{}
	}
	walked := NewWikilinkIndex(fsys)
	listed := NewWikilinkIndexFromPaths(paths)
	assert.Equal(t, []string{"guide.md", "docs/guide.md"}, listed.StemPaths("guide"))
	assert.Equal(t, walked.StemPaths("guide"), listed.StemPaths("guide"))
	assert.Equal(t, walked.NamePaths("logo.png"), listed.NamePaths("logo.png"))
	assert.Empty(t, listed.StemPaths("x"))
}

// TestWikilinkStemAt locks the stem key read at a `[[` column: folder,
// anchor, alias, casing, and outer spaces do not change it (a space
// before a trailing slash stays, as the index keys it), and a typed name, a
// refused target, or a column with no wikilink returns ok=false.
func TestWikilinkStemAt(t *testing.T) {
	for row, want := range map[string]string{
		"[[docs/Guide#a|G]]": "guide",
		"[[guide.md]]":       "guide",
		"[[ Guide ]]":        "guide",
		"[[Guide /]]":        "guide ",
	} {
		got, _, _, ok := WikilinkStemAt([]byte(row), 0)
		assert.True(t, ok, row)
		assert.Equal(t, want, got, row)
	}
	for _, row := range []string{"[[logo.png]]", "[[../x]]", "x [[a]]", "[["} {
		_, _, _, ok := WikilinkStemAt([]byte(row), 0)
		assert.False(t, ok, row)
	}
	_, _, _, ok := WikilinkStemAt([]byte("[[a]]"), -1)
	assert.False(t, ok)
}

// TestWikilinkIndex_Moved locks that Moved returns the index as it
// reads once every listed move has run: each source leaves its stem and
// name keys, each non-empty destination joins its own in resolver
// order, a destination already present is held once, an empty
// destination only removes, and the receiver is left unchanged.
func TestWikilinkIndex_Moved(t *testing.T) {
	idx := NewWikilinkIndexFromPaths([]string{"x/a.md", "y/b.md", "x/c.md", "img/a.png", "gone.md"})
	moved := idx.Moved(map[string]string{
		"x/a.md":  "y/c.md",
		"y/b.md":  "b/c.md",
		"x/c.md":  "x/c.md",
		"gone.md": "",
		"nope.md": "node_modules/c.md",
	})
	assert.Empty(t, moved.StemPaths("a"))
	assert.Empty(t, moved.StemPaths("b"))
	assert.Empty(t, moved.StemPaths("gone"))
	assert.Equal(t, []string{"b/c.md", "x/c.md", "y/c.md"}, moved.StemPaths("c"))
	assert.Equal(t, []string{"b/c.md", "x/c.md", "y/c.md"}, moved.NamePaths("c.md"))
	assert.Equal(t, []string{"img/a.png"}, moved.NamePaths("a.png"))
	assert.Equal(t, []string{"x/a.md"}, idx.StemPaths("a"), "receiver unchanged")
	assert.Equal(t, []string{"x/c.md"}, idx.StemPaths("c"), "receiver unchanged")
	assert.Equal(t, []string{"w/new.md"}, idx.Moved(map[string]string{"nope.md": "w/new.md"}).StemPaths("new"),
		"a source idx lacks still lands at its destination")
	assert.Nil(t, (*WikilinkIndex)(nil).Moved(map[string]string{"a.md": "b.md"}))
}

func TestAppendIf(t *testing.T) {
	assert.Equal(t, []string{"a", "b"}, appendIf([]string{"a"}, "b", true))
	assert.Equal(t, []string{"a"}, appendIf([]string{"a"}, "b", false))
	assert.Nil(t, appendIf(nil, "b", false))
}

// TestMovedPaths locks that one key's holders lose every source and
// gain each joining destination once, in resolver order.
func TestMovedPaths(t *testing.T) {
	moves := map[string]string{"x/a.md": "b/a.md", "y/a.md": "b/a.md"}
	got := movedPaths([]string{"a.md", "x/a.md", "y/a.md"}, moves, []string{"b/a.md", "b/a.md", "a.md"})
	assert.Equal(t, []string{"a.md", "b/a.md"}, got)
}

// TestWikilinkIndex_MovedSharesUntouchedKeys locks that Moved costs
// what the moves touch, not the workspace: a key no move touches is
// read from the receiver, Resolve reads through the overlay, and a
// Moved index can be moved again.
func TestWikilinkIndex_MovedSharesUntouchedKeys(t *testing.T) {
	idx := NewWikilinkIndexFromPaths([]string{"x/a.md", "z.md", "img/z.png"})
	moved := idx.Moved(map[string]string{"x/a.md": "b.md"})
	require.NotEmpty(t, moved.StemPaths("z"))
	assert.Same(t, &idx.StemPaths("z")[0], &moved.StemPaths("z")[0])
	assert.Same(t, &idx.NamePaths("z.png")[0], &moved.NamePaths("z.png")[0])
	got, ok := moved.Resolve("b")
	assert.True(t, ok)
	assert.Equal(t, "b.md", got)
	_, ok = moved.Resolve("a")
	assert.False(t, ok)
	got, ok = moved.Resolve("z.png")
	assert.True(t, ok)
	assert.Equal(t, "img/z.png", got)
	again := moved.Moved(map[string]string{"b.md": "", "z.md": "q/z.md"})
	assert.Empty(t, again.StemPaths("b"))
	assert.Equal(t, []string{"q/z.md"}, again.StemPaths("z"))
	assert.Equal(t, []string{"img/z.png"}, again.NamePaths("z.png"))
}

// TestWikilinkName locks the exact-name key a typed `[[name.ext]]`
// target resolves by: the lowercased basename. A stem-mode target
// (bare or Markdown) and a refused target have none.
func TestWikilinkName(t *testing.T) {
	for target, want := range map[string]string{
		"diagram.png":      "diagram.png",
		"img/Logo.PNG":     "logo.png",
		` a\b\Guide.mdx `:  "guide.mdx",
		"v1.3":             "v1.3",
		"folder/x.png/":    "x.png",
		"folder\\x.tar.gz": "x.tar.gz",
	} {
		got, ok := WikilinkName(target)
		assert.True(t, ok, target)
		assert.Equal(t, want, got, target)
	}
	for _, target := range []string{"Page", "Notes.md", "x.markdown", "", "../x.png", "/x.png", `C:\x.png`} {
		got, ok := WikilinkName(target)
		assert.False(t, ok, target)
		assert.Empty(t, got, target)
	}
}

// TestWikilinkNameAt locks the name key and base span read at a `[[`
// column for a typed link; a stem-mode link, a refused target, or a
// column with no wikilink returns ok=false.
func TestWikilinkNameAt(t *testing.T) {
	for row, want := range map[string][2]string{
		"[[img/Logo.PNG#a|G]]": {"logo.png", "Logo.PNG"},
		"![[ x.png ]]":         {"x.png", "x.png"},
		`[[a\b.png\|alias]]`:   {"b.png", "b.png"},
	} {
		at := 0
		if row[0] == '!' {
			at = 1
		}
		got, s, e, ok := WikilinkNameAt([]byte(row), at)
		require.True(t, ok, row)
		assert.Equal(t, want[0], got, row)
		assert.Equal(t, want[1], row[s:e], row)
	}
	for _, row := range []string{"[[logo]]", "[[logo.md]]", "[[../x.png]]", "x [[a.png]]", "[["} {
		_, _, _, ok := WikilinkNameAt([]byte(row), 0)
		assert.False(t, ok, row)
	}
}

// TestWikilinkBaseSpan locks the span of the last trimmed segment of a
// raw target, with `\` read as `/` and trailing separators dropped.
func TestWikilinkBaseSpan(t *testing.T) {
	for raw, want := range map[string]string{
		"a/b.png":    "b.png",
		` a\b.png `:  "b.png",
		"x/ y.png//": " y.png",
		"plain":      "plain",
	} {
		lo, hi := wikilinkBaseSpan([]byte(raw))
		assert.Equal(t, want, raw[lo:hi], raw)
	}
}
