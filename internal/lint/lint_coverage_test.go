package lint

import (
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"

	"github.com/jeduden/mdsmith/internal/gitignore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- GetGitignore tests ---

func TestGetGitignore_NilFunc(t *testing.T) {
	f := &File{}
	m := f.GetGitignore()
	assert.Nil(t, m)
}

func TestGetGitignore_WithFunc(t *testing.T) {
	called := 0
	matcher := &gitignore.Matcher{}
	f := &File{
		GitignoreFunc: func() *gitignore.Matcher {
			called++
			return matcher
		},
	}
	m := f.GetGitignore()
	assert.Same(t, matcher, m)
	assert.Equal(t, 1, called)
}

func TestGetGitignore_Cached(t *testing.T) {
	called := 0
	matcher := &gitignore.Matcher{}
	f := &File{
		GitignoreFunc: func() *gitignore.Matcher {
			called++
			return matcher
		},
	}
	m1 := f.GetGitignore()
	m2 := f.GetGitignore()
	assert.Same(t, m1, m2)
	assert.Equal(t, 1, called, "GitignoreFunc should be called only once")
}

// --- useGitignore tests ---

func TestUseGitignore_NilPointer(t *testing.T) {
	opts := ResolveOpts{UseGitignore: nil}
	assert.True(t, opts.useGitignore(), "nil UseGitignore should default to true")
}

func TestUseGitignore_True(t *testing.T) {
	b := true
	opts := ResolveOpts{UseGitignore: &b}
	assert.True(t, opts.useGitignore())
}

func TestUseGitignore_False(t *testing.T) {
	b := false
	opts := ResolveOpts{UseGitignore: &b}
	assert.False(t, opts.useGitignore())
}

// --- resolveGlob tests ---

func TestResolveGlob_InvalidPattern(t *testing.T) {
	err := resolveGlob("[invalid", DefaultResolveOpts(), func(_ string) {})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid glob pattern")
}

func TestResolveGlob_NoMatches(t *testing.T) {
	var files []string
	pattern := filepath.Join(t.TempDir(), "no-match-*.md")
	err := resolveGlob(pattern, DefaultResolveOpts(), func(f string) {
		files = append(files, f)
	})
	require.NoError(t, err)
	assert.Empty(t, files)
}

func TestResolveGlob_MatchesFiles(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.md"), []byte("# A"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.md"), []byte("# B"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "c.txt"), []byte("text"), 0o644))

	var files []string
	pattern := filepath.Join(dir, "*.md")
	err := resolveGlob(pattern, DefaultResolveOpts(), func(f string) {
		files = append(files, f)
	})
	require.NoError(t, err)
	assert.Len(t, files, 2)
}

func TestResolveGlob_MatchesDirectory(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "docs")
	require.NoError(t, os.MkdirAll(subDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(subDir, "guide.md"), []byte("# Guide"), 0o644))

	var files []string
	pattern := filepath.Join(dir, "doc*")
	err := resolveGlob(pattern, DefaultResolveOpts(), func(f string) {
		files = append(files, f)
	})
	require.NoError(t, err)
	assert.Len(t, files, 1)
}

// --- isGitignored tests ---

func TestIsGitignored_MatchAndNoMatch(t *testing.T) {
	// isGitignored with a real matcher: matching path returns true,
	// non-matching path returns false.
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.log\n"), 0o644))

	matcher := gitignore.NewMatcher(dir)
	// A file matching the pattern should be ignored.
	logFile := filepath.Join(dir, "test.log")
	require.NoError(t, os.WriteFile(logFile, []byte("log"), 0o644))
	assert.True(t, isGitignored(matcher, logFile, false))

	// A file not matching should not be ignored.
	mdFile := filepath.Join(dir, "test.md")
	require.NoError(t, os.WriteFile(mdFile, []byte("# Test"), 0o644))
	assert.False(t, isGitignored(matcher, mdFile, false))
}

// PI node and parser unit tests live with the canonical
// implementation in pkg/markdown (TestKind_ProcessingInstruction,
// TestIsRaw_ProcessingInstruction, TestPIBlockParser_*, …). The
// lint-level integration smoke is TestNewFile_MultiPIs below.

// --- LineOfOffset tests ---

func TestLineIndex(t *testing.T) {
	f := &File{Source: []byte("a\nbb\n\n")}
	got := f.lineIndex()
	assert.Equal(t, []int{1, 4, 5}, got, "newline offsets")

	again := f.lineIndex()
	require.NotEmpty(t, got)
	if &got[0] != &again[0] {
		t.Fatal("lineIndex rebuilt the slice instead of caching it")
	}

	empty := &File{Source: nil}
	assert.Empty(t, empty.lineIndex(), "no newlines in empty source")
}

func TestLineIndex_ConcurrentFirstCall(t *testing.T) {
	// sync.Once must make the lazy build race-free even when the
	// first calls land on different goroutines. Run under -race.
	f := &File{Source: []byte("x\ny\nz\nw\n")}
	const n = 16
	var wg sync.WaitGroup
	results := make([][]int, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = f.lineIndex()
		}(i)
	}
	wg.Wait()
	for i := 0; i < n; i++ {
		assert.Equal(t, []int{1, 3, 5, 7}, results[i], "goroutine %d", i)
		if i > 0 && &results[i][0] != &results[0][0] {
			t.Fatal("concurrent callers saw different backing slices")
		}
	}
}

func TestLineOfOffset_Basic(t *testing.T) {
	f := &File{Source: []byte("line1\nline2\nline3\n")}
	assert.Equal(t, 1, f.LineOfOffset(0))
	assert.Equal(t, 1, f.LineOfOffset(4))
	assert.Equal(t, 2, f.LineOfOffset(6))
	assert.Equal(t, 3, f.LineOfOffset(12))
}

// lineOfOffsetOracle is the original O(n) definition. The optimized
// LineOfOffset must agree with it for every offset, including the
// out-of-range and at-newline boundaries.
func lineOfOffsetOracle(src []byte, offset int) int {
	line := 1
	for i := 0; i < offset && i < len(src); i++ {
		if src[i] == '\n' {
			line++
		}
	}
	return line
}

func TestLineOfOffset_MatchesOracle(t *testing.T) {
	sources := [][]byte{
		nil,
		[]byte(""),
		[]byte("\n"),
		[]byte("no newline"),
		[]byte("a\nb\nc"),
		[]byte("line1\nline2\nline3\n"),
		[]byte("\n\n\nx\n\n"),
		[]byte("héllo\nwörld\n€\n"), // multibyte: offsets are byte-based
	}
	for _, src := range sources {
		f := &File{Source: src}
		// Cover every byte boundary plus out-of-range on both ends.
		for off := -3; off <= len(src)+3; off++ {
			assert.Equalf(t, lineOfOffsetOracle(src, off), f.LineOfOffset(off),
				"src=%q offset=%d", src, off)
		}
	}
}

func TestLineOfOffset_StableAcrossRepeatedCalls(t *testing.T) {
	// The line index is built lazily and cached; a second call must
	// return the same answer as the first.
	f := &File{Source: []byte("a\nbb\nccc\n\nd")}
	for _, off := range []int{0, 1, 2, 5, 9, 10, 11, 99} {
		first := f.LineOfOffset(off)
		assert.Equal(t, first, f.LineOfOffset(off), "offset %d", off)
		assert.Equal(t, lineOfOffsetOracle(f.Source, off), first, "offset %d", off)
	}
}

// --- ColumnOfOffset ---

func TestColumnOfOffset_FirstColumn(t *testing.T) {
	f := &File{Source: []byte("line1\nline2\n")}
	assert.Equal(t, 1, f.ColumnOfOffset(0))
	assert.Equal(t, 1, f.ColumnOfOffset(6), "offset just past newline is column 1")
}

func TestColumnOfOffset_MidLine(t *testing.T) {
	f := &File{Source: []byte("line1\nline2\n")}
	// 'i' in "line1" is at offset 1 → column 2.
	assert.Equal(t, 2, f.ColumnOfOffset(1))
	// '1' in "line2" is at offset 10 → 10-6+1 = 5.
	assert.Equal(t, 5, f.ColumnOfOffset(10))
}

func TestColumnOfOffset_PastEOFClamps(t *testing.T) {
	// Offsets past len(Source) clamp to EOF; the result reflects the
	// position one past the final character on the last line.
	src := []byte("abc")
	f := &File{Source: src}
	// EOF is offset 3, line starts at 0 → column 4.
	assert.Equal(t, 4, f.ColumnOfOffset(999))
}

func TestColumnOfOffset_AtNewline(t *testing.T) {
	// The newline itself sits at the end of its line.
	f := &File{Source: []byte("ab\nc")}
	// '\n' is at offset 2 → column 3.
	assert.Equal(t, 3, f.ColumnOfOffset(2))
}

// --- NewParser / PIBlockParserPrioritized forwarders ---

// TestNewParser_ReturnsNonNil pins the public forwarder: a rule
// re-parsing a document (for example, to consult the link-reference
// map after a fix has rewritten anchors) must get a parser, not a
// nil interface that would panic on the first Parse call.
func TestNewParser_ReturnsNonNil(t *testing.T) {
	p := NewParser()
	require.NotNil(t, p, "NewParser must return a usable parser.Parser")
}

// The PI block-parser forwarder now lives in internal/pi
// (TestBlockParserPrioritized); the lint package only consumes it via
// NewParser. PIBlockParser edge cases and extractPINameBytes unit tests live
// with the canonical implementation in pkg/markdown
// (TestPIBlockParser_*, TestExtractPINameBytes).

// --- Additional walk coverage ---

func TestWalkDir_NonexistentDir(t *testing.T) {
	_, err := walkDir(filepath.Join(t.TempDir(), "no-such-dir"), false, false)
	assert.Error(t, err)
}

// --- hasGlobChars tests ---

func TestHasGlobChars(t *testing.T) {
	assert.True(t, hasGlobChars("*.md"))
	assert.True(t, hasGlobChars("file?.md"))
	assert.True(t, hasGlobChars("[abc].md"))
	assert.True(t, hasGlobChars("docs/{a,b}.md")) // brace expansion with comma
	assert.False(t, hasGlobChars("readme.md"))
	assert.False(t, hasGlobChars("{draft}.md")) // single-item brace — not expansion
}

func TestHasBraceExpansion(t *testing.T) {
	assert.True(t, hasBraceExpansion("docs/{a,b}.md"))
	assert.True(t, hasBraceExpansion("{guide,ref}.md"))
	assert.True(t, hasBraceExpansion("a/{x,y}/b"))               // nested depth with comma
	assert.False(t, hasBraceExpansion("{draft}.md"))             // no comma inside braces
	assert.False(t, hasBraceExpansion("readme.md"))              // no braces at all
	assert.False(t, hasBraceExpansion("a,b"))                    // comma but no enclosing brace
	assert.False(t, hasBraceExpansion("notes/{draft,review.md")) // unclosed brace
}

// TestResolveFiles_BraceExpansion verifies that a brace-expansion CLI argument
// is expanded via doublestar rather than treated as a literal filename.
func TestResolveFiles_BraceExpansion(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.md"), []byte("# A\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.md"), []byte("# B\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "c.md"), []byte("# C\n"), 0o644))

	orig, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	defer func() { _ = os.Chdir(orig) }()

	files, err := ResolveFiles([]string{"{a,b}.md"})
	require.NoError(t, err)
	names := make([]string, len(files))
	for i, f := range files {
		names[i] = filepath.Base(f)
	}
	sort.Strings(names)
	assert.Equal(t, []string{"a.md", "b.md"}, names)
	assert.NotContains(t, names, "c.md")
}

// TestResolveFiles_LiteralBrace verifies that a filename with a single-item
// brace group (no comma) is treated as a literal path, not a glob.
func TestResolveFiles_LiteralBrace(t *testing.T) {
	dir := t.TempDir()
	// Create a file with a literal brace in its name.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "{draft}.md"), []byte("# D\n"), 0o644))

	orig, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	defer func() { _ = os.Chdir(orig) }()

	files, err := ResolveFiles([]string{"{draft}.md"})
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, "{draft}.md", filepath.Base(files[0]))
}

// TestResolveFiles_DoubleStarRecursive verifies that ** in a CLI argument
// recurses into subdirectories via doublestar.FilepathGlob.
func TestResolveFiles_DoubleStarRecursive(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "top.md"), []byte("# T\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(sub, "nested.md"), []byte("# N\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(sub, "other.txt"), []byte("txt"), 0o644))

	orig, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	defer func() { _ = os.Chdir(orig) }()

	files, err := ResolveFiles([]string{"**/*.md"})
	require.NoError(t, err)
	names := make([]string, len(files))
	for i, f := range files {
		names[i] = filepath.Base(f)
	}
	sort.Strings(names)
	assert.Equal(t, []string{"nested.md", "top.md"}, names, "** should recurse into subdirs")
}

// --- isMarkdown tests ---

func TestIsMarkdown(t *testing.T) {
	assert.True(t, isMarkdown("readme.md"))
	assert.True(t, isMarkdown("readme.MD"))
	assert.True(t, isMarkdown("file.markdown"))
	assert.True(t, isMarkdown("file.MARKDOWN"))
	assert.False(t, isMarkdown("file.txt"))
	assert.False(t, isMarkdown("file.go"))
}

// --- Walk with PI nodes ---

// TestNewFile_MultiPIs is the lint-level integration smoke that
// lint.NewFile (delegating to pkg/markdown's pooled parser) still
// yields ProcessingInstruction nodes. The PI node/parser unit tests
// proper live in pkg/markdown.
func TestNewFile_MultiPIs(t *testing.T) {
	src := "<?foo?>\n\n<?bar\nbaz\n?>\n"
	f, err := NewFile("test.md", []byte(src))
	require.NoError(t, err)
	pis := findPINodes(f.AST)
	require.Len(t, pis, 2)
	assert.Equal(t, "foo", pis[0].Name)
	assert.Equal(t, "bar", pis[1].Name)
}

// --- SetRootDir tests ---

// TestSetRootDir_SetsRootDirAndRootFS verifies that SetRootDir sets both the
// RootDir string and initialises RootFS as an os.DirFS rooted at that directory.
func TestSetRootDir_SetsRootDirAndRootFS(t *testing.T) {
	dir := t.TempDir()
	f := &File{}
	f.SetRootDir(dir)

	assert.Equal(t, dir, f.RootDir)
	require.NotNil(t, f.RootFS, "RootFS must be non-nil after SetRootDir")

	// Write a sentinel file and verify RootFS can open it, which confirms
	// that RootFS is rooted at dir and not at some other location.
	sentinel := "sentinel.md"
	require.NoError(t, os.WriteFile(filepath.Join(dir, sentinel), []byte("# hi"), 0o644))
	fh, err := f.RootFS.Open(sentinel)
	require.NoError(t, err, "RootFS must be able to open files under dir")
	_ = fh.Close()
}

// --- addDirFiles error path ---

// TestAddDirFiles_NonexistentDir exercises the error return path of addDirFiles
// when the target directory does not exist.
func TestAddDirFiles_NonexistentDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "does-not-exist")
	err := addDirFiles(dir, DefaultResolveOpts(), func(_ string) {})
	assert.Error(t, err, "addDirFiles must propagate walkDir errors")
}

// --- isDescendantOf unreachable-Rel branch (via filepath.Rel on Windows
//     volume-root mismatch; on POSIX we use the closest equivalent) ---

// TestIsDescendantOf_RelError exercises the filepath.Rel-error branch of
// isDescendantOf. filepath.Rel returns an error when the two paths reside on
// different Windows drive letters; on POSIX the function never returns an
// error for absolute paths, so we verify the happy-path variant that hits the
// same coverage statement via the `rel == "."` guard (already covered by the
// existing SelfReturnsFalse test).  This additional test strengthens the
// assertion that sibling paths are correctly classified.
func TestIsDescendantOf_Sibling(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "a")
	sibling := filepath.Join(root, "sibling")
	parent := filepath.Join(root, "other")
	child := filepath.Join(parent, "child")
	assert.False(t, isDescendantOf(sibling, parent),
		"sibling directory must not be a descendant")
	assert.True(t, isDescendantOf(child, parent),
		"child must be a descendant of parent")
}

// --- isGitignored: Abs-error branch ---
// filepath.Abs virtually never returns an error on POSIX (only in the
// unreachable os.Getwd failure scenario), so we verify the positive/negative
// paths that are reachable without simulating a Getwd failure.
func TestIsGitignored_DirectorySentinel(t *testing.T) {
	dir := t.TempDir()
	gitignorePath := filepath.Join(dir, ".gitignore")
	require.NoError(t, os.WriteFile(gitignorePath, []byte("logs/\n"), 0o644))

	m := gitignore.NewMatcher(dir)
	logsDir := filepath.Join(dir, "logs")
	// isGitignored for a directory entry.
	assert.True(t, isGitignored(m, logsDir, true),
		"directory matching dir-only pattern must be ignored")
	assert.False(t, isGitignored(m, logsDir, false),
		"file with same name must not be ignored by dir-only pattern")
}
