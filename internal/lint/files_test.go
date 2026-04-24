package lint

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveFiles_SingleFile(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "test.md")
	require.NoError(t, os.WriteFile(mdFile, []byte("# Hello"), 0o644))

	files, err := ResolveFiles([]string{mdFile})
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, mdFile, files[0])
}

func TestResolveFiles_NonMarkdownFile(t *testing.T) {
	dir := t.TempDir()
	txtFile := filepath.Join(dir, "test.txt")
	require.NoError(t, os.WriteFile(txtFile, []byte("hello"), 0o644))

	// Non-markdown files are still returned when given explicitly as args.
	files, err := ResolveFiles([]string{txtFile})
	require.NoError(t, err)
	require.Len(t, files, 1)
}

func TestResolveFiles_Directory(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "sub")
	require.NoError(t, os.MkdirAll(subDir, 0o755))

	// Create markdown files at various levels.
	for _, name := range []string{
		filepath.Join(dir, "a.md"),
		filepath.Join(dir, "b.markdown"),
		filepath.Join(dir, "c.txt"), // should be excluded
		filepath.Join(subDir, "d.md"),
	} {
		require.NoError(t, os.WriteFile(name, []byte("# Test"), 0o644))
	}

	files, err := ResolveFiles([]string{dir})
	require.NoError(t, err)

	// Should find a.md, b.markdown, sub/d.md (not c.txt).
	require.Len(t, files, 3)

	// Check that all returned files are markdown.
	for _, f := range files {
		ext := filepath.Ext(f)
		assert.True(t, ext == ".md" || ext == ".markdown", "unexpected non-markdown file: %s", f)
	}
}

func TestResolveFiles_GlobPattern(t *testing.T) {
	dir := t.TempDir()

	for _, name := range []string{"a.md", "b.md", "c.txt"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("# Test"), 0o644))
	}

	pattern := filepath.Join(dir, "*.md")
	files, err := ResolveFiles([]string{pattern})
	require.NoError(t, err)
	require.Len(t, files, 2)
}

func TestResolveFiles_NonexistentPath(t *testing.T) {
	_, err := ResolveFiles([]string{"/nonexistent/path/file.md"})
	require.Error(t, err, "expected error for nonexistent path")
}

func TestResolveFiles_EmptyArgs(t *testing.T) {
	files, err := ResolveFiles([]string{})
	require.NoError(t, err)
	require.Empty(t, files)
}

func TestResolveFiles_NilArgs(t *testing.T) {
	files, err := ResolveFiles(nil)
	require.NoError(t, err)
	require.Empty(t, files)
}

func TestResolveFiles_Deduplicated(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "test.md")
	require.NoError(t, os.WriteFile(mdFile, []byte("# Hello"), 0o644))

	// Pass the same file twice.
	files, err := ResolveFiles([]string{mdFile, mdFile})
	require.NoError(t, err)
	require.Len(t, files, 1, "expected 1 file (deduplicated)")
}

func TestResolveFiles_Sorted(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"z.md", "a.md", "m.md"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("# Test"), 0o644))
	}

	files, err := ResolveFiles([]string{dir})
	require.NoError(t, err)
	assert.True(t, sort.StringsAreSorted(files), "expected sorted files, got %v", files)
}

func TestResolveFiles_MarkdownExtension(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "doc.markdown")
	require.NoError(t, os.WriteFile(mdFile, []byte("# Hello"), 0o644))

	files, err := ResolveFiles([]string{dir})
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, ".markdown", filepath.Ext(files[0]))
}

func TestResolveFiles_GlobMatchingDirectory(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "docs")
	require.NoError(t, os.MkdirAll(subDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(subDir, "guide.md"), []byte("# Guide"), 0o644))

	// Glob that matches a directory should recurse into it.
	pattern := filepath.Join(dir, "doc*")
	files, err := ResolveFiles([]string{pattern})
	require.NoError(t, err)
	require.Len(t, files, 1)
}

// --- Gitignore-aware walking tests ---

func TestResolveFilesWithOpts_GitignoreSkipsMatchedFiles(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "ignored")
	require.NoError(t, os.MkdirAll(subDir, 0o755))

	for _, name := range []string{
		filepath.Join(dir, "keep.md"),
		filepath.Join(subDir, "skip.md"),
	} {
		require.NoError(t, os.WriteFile(name, []byte("# Test"), 0o644))
	}

	gitignore := filepath.Join(dir, ".gitignore")
	require.NoError(t, os.WriteFile(gitignore, []byte("ignored/\n"), 0o644))

	files, err := ResolveFilesWithOpts([]string{dir}, DefaultResolveOpts())
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, "keep.md", filepath.Base(files[0]))
}

func TestResolveFilesWithOpts_NestedGitignore(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "sub")
	require.NoError(t, os.MkdirAll(subDir, 0o755))

	for _, name := range []string{
		filepath.Join(dir, "root.md"),
		filepath.Join(subDir, "included.md"),
		filepath.Join(subDir, "draft.md"),
	} {
		require.NoError(t, os.WriteFile(name, []byte("# Test"), 0o644))
	}

	gitignore := filepath.Join(subDir, ".gitignore")
	require.NoError(t, os.WriteFile(gitignore, []byte("draft.md\n"), 0o644))

	files, err := ResolveFilesWithOpts([]string{dir}, DefaultResolveOpts())
	require.NoError(t, err)
	require.Len(t, files, 2)
	for _, f := range files {
		assert.NotEqual(t, "draft.md", filepath.Base(f), "draft.md should have been excluded by nested .gitignore")
	}
}

func TestResolveFilesWithOpts_UseGitignoreFalse(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "ignored")
	require.NoError(t, os.MkdirAll(subDir, 0o755))

	for _, name := range []string{
		filepath.Join(dir, "keep.md"),
		filepath.Join(subDir, "skip.md"),
	} {
		require.NoError(t, os.WriteFile(name, []byte("# Test"), 0o644))
	}

	gitignore := filepath.Join(dir, ".gitignore")
	require.NoError(t, os.WriteFile(gitignore, []byte("ignored/\n"), 0o644))

	// With UseGitignore=false, all files should be included.
	f := false
	opts := ResolveOpts{UseGitignore: &f}
	files, err := ResolveFilesWithOpts([]string{dir}, opts)
	require.NoError(t, err)
	require.Len(t, files, 2, "expected 2 files (gitignore disabled)")
}

func TestResolveFilesWithOpts_NoGitignorePresent(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{
		filepath.Join(dir, "a.md"),
		filepath.Join(dir, "b.md"),
	} {
		require.NoError(t, os.WriteFile(name, []byte("# Test"), 0o644))
	}

	files, err := ResolveFilesWithOpts([]string{dir}, DefaultResolveOpts())
	require.NoError(t, err)
	require.Len(t, files, 2)
}

func TestResolveFilesWithOpts_ExplicitFileNotFilteredByGitignore(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "ignored.md")
	require.NoError(t, os.WriteFile(mdFile, []byte("# Test"), 0o644))

	gitignore := filepath.Join(dir, ".gitignore")
	require.NoError(t, os.WriteFile(gitignore, []byte("*.md\n"), 0o644))

	files, err := ResolveFilesWithOpts([]string{mdFile}, DefaultResolveOpts())
	require.NoError(t, err)
	require.Len(t, files, 1, "expected 1 file (explicit path not filtered)")
	assert.Equal(t, mdFile, files[0])
}

func TestResolveFilesWithOpts_GitignoreWildcardPattern(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "build")
	require.NoError(t, os.MkdirAll(subDir, 0o755))

	for _, name := range []string{
		filepath.Join(dir, "readme.md"),
		filepath.Join(subDir, "output.md"),
	} {
		require.NoError(t, os.WriteFile(name, []byte("# Test"), 0o644))
	}

	gitignore := filepath.Join(dir, ".gitignore")
	require.NoError(t, os.WriteFile(gitignore, []byte("build/\n"), 0o644))

	files, err := ResolveFilesWithOpts([]string{dir}, DefaultResolveOpts())
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, "readme.md", filepath.Base(files[0]))
}

func TestResolveFilesWithOpts_GitignoreNegation(t *testing.T) {
	dir := t.TempDir()

	for _, name := range []string{
		filepath.Join(dir, "a.md"),
		filepath.Join(dir, "b.md"),
		filepath.Join(dir, "keep.md"),
	} {
		require.NoError(t, os.WriteFile(name, []byte("# Test"), 0o644))
	}

	gitignore := filepath.Join(dir, ".gitignore")
	require.NoError(t, os.WriteFile(gitignore, []byte("*.md\n!keep.md\n"), 0o644))

	files, err := ResolveFilesWithOpts([]string{dir}, DefaultResolveOpts())
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, "keep.md", filepath.Base(files[0]))
}

// --- FollowSymlinks tests ---

// TestResolveFilesWithOpts_SkipsSymlinksByDefault asserts that the
// secure default (FollowSymlinks=false) skips both symlinked files
// and symlinked directories encountered during the walk.
func TestResolveFilesWithOpts_SkipsSymlinksByDefault(t *testing.T) {
	skipIfSymlinkUnsupported(t)
	dir := t.TempDir()

	realFile := filepath.Join(dir, "real.md")
	require.NoError(t, os.WriteFile(realFile, []byte("# Real"), 0o644))

	// Target for the file-symlink case.
	subDir := filepath.Join(dir, "target")
	require.NoError(t, os.MkdirAll(subDir, 0o755))
	targetFile := filepath.Join(subDir, "doc.md")
	require.NoError(t, os.WriteFile(targetFile, []byte("# Target"), 0o644))

	// Symlinked file (link.md -> target/doc.md).
	linkFile := filepath.Join(dir, "link.md")
	require.NoError(t, os.Symlink(targetFile, linkFile))

	// Symlinked directory (linked-dir -> target/). A dirty markdown
	// file inside the target would surface if the walker descended.
	require.NoError(t, os.Symlink(subDir, filepath.Join(dir, "linked-dir")))

	// Default: only the real files under target/ are walked; both
	// symlinked entries are skipped.
	files, err := ResolveFilesWithOpts([]string{dir}, DefaultResolveOpts())
	require.NoError(t, err)
	require.Len(t, files, 2,
		"expected only real.md and target/doc.md, got %v", files)
	for _, f := range files {
		base := filepath.Base(f)
		assert.NotEqual(t, "link.md", base,
			"symlinked file must be skipped by default")
		assert.False(t,
			strings.Contains(filepath.ToSlash(f), "/linked-dir/"),
			"symlinked directory must not be descended")
	}
}

// TestResolveFilesWithOpts_FollowSymlinks_OptIn asserts that
// FollowSymlinks=true restores the pre-plan-84 behavior of walking
// symlinked entries.
func TestResolveFilesWithOpts_FollowSymlinks_OptIn(t *testing.T) {
	skipIfSymlinkUnsupported(t)
	dir := t.TempDir()

	realFile := filepath.Join(dir, "real.md")
	require.NoError(t, os.WriteFile(realFile, []byte("# Real"), 0o644))

	subDir := filepath.Join(dir, "target")
	require.NoError(t, os.MkdirAll(subDir, 0o755))
	targetFile := filepath.Join(subDir, "doc.md")
	require.NoError(t, os.WriteFile(targetFile, []byte("# Target"), 0o644))

	linkFile := filepath.Join(dir, "link.md")
	require.NoError(t, os.Symlink(targetFile, linkFile))

	noGitignore := false
	opts := ResolveOpts{
		UseGitignore:   &noGitignore,
		FollowSymlinks: true,
	}
	files, err := ResolveFilesWithOpts([]string{dir}, opts)
	require.NoError(t, err)
	require.Len(t, files, 3,
		"with FollowSymlinks=true, symlink is walked alongside real files")
}

// TestResolveFilesWithOpts_Glob_SkipsSymlinksByDefault asserts the
// same default-deny applies when paths come in through a glob
// expansion (resolveGlob path) rather than a directory walk.
func TestResolveFilesWithOpts_Glob_SkipsSymlinksByDefault(t *testing.T) {
	skipIfSymlinkUnsupported(t)
	dir := t.TempDir()

	realFile := filepath.Join(dir, "real.md")
	require.NoError(t, os.WriteFile(realFile, []byte("# Real"), 0o644))

	target := filepath.Join(dir, "target.md")
	require.NoError(t, os.WriteFile(target, []byte("# Target"), 0o644))
	linkFile := filepath.Join(dir, "link.md")
	require.NoError(t, os.Symlink(target, linkFile))

	pattern := filepath.Join(dir, "*.md")
	files, err := ResolveFilesWithOpts([]string{pattern}, DefaultResolveOpts())
	require.NoError(t, err)
	require.Len(t, files, 2,
		"expected only the two real markdown files, got %v", files)
	bases := []string{filepath.Base(files[0]), filepath.Base(files[1])}
	assert.ElementsMatch(t, []string{"real.md", "target.md"}, bases,
		"glob expansion must yield real.md and target.md and skip link.md")
}

// --- hasSymlinkAncestor / helpers ---

// TestHasSymlinkAncestor_RelativeUnderCwd exercises the common case:
// a project-relative path whose ancestor is a symbolic link.
func TestHasSymlinkAncestor_RelativeUnderCwd(t *testing.T) {
	skipIfSymlinkUnsupported(t)
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "real"), 0o755))
	require.NoError(t, os.Symlink(
		filepath.Join(dir, "real"), filepath.Join(dir, "linked")))

	t.Chdir(dir)
	assert.True(t, hasSymlinkAncestor("linked/foo.md"),
		"linked/foo.md must be flagged as crossing a symlinked ancestor")
	assert.False(t, hasSymlinkAncestor("real/foo.md"),
		"real/foo.md has no symlink ancestors")
}

// TestHasSymlinkAncestor_CacheReusedAcrossSiblings covers the
// memoization path in hasSymlinkAncestorCached: two sibling paths
// under the same symlinked directory share one Lstat result.
func TestHasSymlinkAncestor_CacheReusedAcrossSiblings(t *testing.T) {
	skipIfSymlinkUnsupported(t)
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "real"), 0o755))
	require.NoError(t, os.Symlink(
		filepath.Join(dir, "real"), filepath.Join(dir, "linked")))
	t.Chdir(dir)

	cache := make(map[string]bool)
	assert.True(t, hasSymlinkAncestorCached("linked/a.md", cache))
	assert.True(t, hasSymlinkAncestorCached("linked/b.md", cache))
	// The shared parent is cached as "true"; both paths return true
	// without re-Lstat'ing the ancestor.
	assert.Contains(t, cache, filepath.Join(dir, "linked"),
		"shared ancestor must be memoised")
}

// TestHasSymlinkAncestor_AbsPathOutsideCwdWithGitRoot ensures an
// absolute path outside cwd but inside a .git-rooted project still
// gets its ancestor chain scanned.
func TestHasSymlinkAncestor_AbsPathOutsideCwdWithGitRoot(t *testing.T) {
	skipIfSymlinkUnsupported(t)
	root := t.TempDir()
	project := filepath.Join(root, "project")
	cwd := filepath.Join(root, "other")
	require.NoError(t, os.MkdirAll(filepath.Join(project, ".git"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(project, "real"), 0o755))
	require.NoError(t, os.MkdirAll(cwd, 0o755))
	require.NoError(t, os.Symlink(
		filepath.Join(project, "real"),
		filepath.Join(project, "linked")))

	t.Chdir(cwd)
	assert.True(t,
		hasSymlinkAncestor(filepath.Join(project, "linked", "foo.md")),
		"abs path into a .git-rooted project must scan its ancestors")
}

// TestHasSymlinkAncestor_SkipsPathOutsideProjects confirms that an
// absolute path with no cwd or .git anchor is trusted (no probe).
// This keeps system-level symlinks like /tmp on macOS out of scope.
func TestHasSymlinkAncestor_SkipsPathOutsideProjects(t *testing.T) {
	// cwd and target live in unrelated temp dirs; neither has a
	// .git ancestor, so hasSymlinkAncestor should find no anchor
	// and return false. Using temp dirs keeps the test portable
	// across OSes — unlike a hardcoded `/etc/...` path.
	cwd := t.TempDir()
	outside := t.TempDir()
	t.Chdir(cwd)
	assert.False(t, hasSymlinkAncestor(
		filepath.Join(outside, "nonexistent", "foo.md")))
}

// TestGitProjectRoot_FindsAncestor and _None cover the boundary
// helper used for absolute paths outside cwd.
func TestGitProjectRoot_FindsAncestor(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	sub := filepath.Join(dir, "docs", "guides")
	require.NoError(t, os.MkdirAll(sub, 0o755))

	got := gitProjectRoot(sub)
	assert.Equal(t, dir, got,
		"gitProjectRoot must walk up to the nearest .git-containing dir")
}

func TestGitProjectRoot_NoAncestor(t *testing.T) {
	dir := t.TempDir()
	assert.Empty(t, gitProjectRoot(dir),
		"gitProjectRoot must return \"\" when no .git ancestor exists")
}

// TestHasSymlinkAncestor_DotDotAfterSymlinkedDir covers the
// physics-preserving component walk: `linked/../dirty.md` where
// `linked` is a real symlink must be rejected (the walker probes
// `linked` before the `..` pops back, catching the symlink).
func TestHasSymlinkAncestor_DotDotAfterSymlinkedDir(t *testing.T) {
	skipIfSymlinkUnsupported(t)
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "real"), 0o755))
	require.NoError(t, os.Symlink(
		filepath.Join(dir, "real"), filepath.Join(dir, "linked")))
	t.Chdir(dir)

	assert.True(t, hasSymlinkAncestor("linked/../dirty.md"),
		"`linked/../dirty.md` with symlinked `linked` must be flagged")
}

// TestHasSymlinkAncestor_DotDotAfterRealDir confirms the
// component walk does NOT over-reject: `a/b/../c.md` where no
// component is a symlink is allowed through.
func TestHasSymlinkAncestor_DotDotAfterRealDir(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "a", "b"), 0o755))
	t.Chdir(dir)

	assert.False(t, hasSymlinkAncestor("a/b/../c.md"),
		"`..` after a non-symlink dir must not trigger rejection")
}

// TestHasSymlinkAncestorCache_SkipsRepeatLstat confirms that once
// a directory has been Lstat'd, a second hit uses the cached value
// instead of re-probing.
func TestHasSymlinkAncestorCache_SkipsRepeatLstat(t *testing.T) {
	skipIfSymlinkUnsupported(t)
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "real"), 0o755))
	require.NoError(t, os.Symlink(
		filepath.Join(dir, "real"), filepath.Join(dir, "linked")))

	cache := make(map[string]bool)
	first := hasSymlinkAncestorWithCwd("linked/a.md", dir, cache)
	second := hasSymlinkAncestorWithCwd("linked/b.md", dir, cache)
	assert.True(t, first)
	assert.True(t, second)
	assert.True(t, cache[filepath.Join(dir, "linked")],
		"shared ancestor must be memoised as a symlink")
}

// TestHasSymlinkAncestorWithCwd_EmptyCwdFallsBackToGetwd exercises
// the Getwd fallback for both the stop-boundary and the component
// walk when the caller passes an empty cwd.
func TestHasSymlinkAncestorWithCwd_EmptyCwdFallsBackToGetwd(t *testing.T) {
	skipIfSymlinkUnsupported(t)
	dir := t.TempDir()
	// .git marker so gitProjectRoot can anchor the stop boundary.
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "real"), 0o755))
	require.NoError(t, os.Symlink(
		filepath.Join(dir, "real"), filepath.Join(dir, "linked")))
	t.Chdir(dir)

	cache := make(map[string]bool)
	got := hasSymlinkAncestorWithCwd("linked/a.md", "", cache)
	assert.True(t, got,
		"empty cwd arg must fall back to os.Getwd for resolution")
}

// TestIsDescendantOf_SelfReturnsFalse pins the `p == base` branch
// (strict-descendant semantics) so `current == stop` in the walker
// correctly skips probing.
func TestIsDescendantOf_SelfReturnsFalse(t *testing.T) {
	assert.False(t, isDescendantOf("/foo", "/foo"),
		"path equal to base is not a strict descendant")
	assert.True(t, isDescendantOf("/foo/bar", "/foo"))
	assert.False(t, isDescendantOf("/foo-other", "/foo"),
		"must not mistake sibling with shared prefix for descendant")
}

// TestAbsWithCwd_EmptyCwdUsesGetwd covers the Getwd fallback.
func TestAbsWithCwd_EmptyCwdUsesGetwd(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	got := absWithCwd("foo.md", "")
	assert.Equal(t, filepath.Join(dir, "foo.md"), got)
}

// TestHasSymlinkAncestorWithCwd_HonorsCwdArg confirms that the
// `cwd` parameter — not the process working directory — is what
// gets joined to a relative path. Otherwise the precomputed-cwd
// optimisation in resolveGlob would be a lie and wouldn't avoid
// the per-match `os.Getwd` it claims to.
func TestHasSymlinkAncestorWithCwd_HonorsCwdArg(t *testing.T) {
	skipIfSymlinkUnsupported(t)
	target := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(target, "real"), 0o755))
	require.NoError(t, os.Symlink(
		filepath.Join(target, "real"), filepath.Join(target, "linked")))

	// Process cwd is NOT `target`. Pass `target` explicitly.
	processCwd := t.TempDir()
	t.Chdir(processCwd)

	cache := make(map[string]bool)
	got := hasSymlinkAncestorWithCwd("linked/foo.md", target, cache)
	assert.True(t, got,
		"helper must resolve `linked/foo.md` against the supplied cwd, "+
			"not os.Getwd; otherwise the precomputed-cwd optimisation is moot")
}

// skipIfSymlinkUnsupported forwards to the shared testutil helper.
func skipIfSymlinkUnsupported(t *testing.T) {
	t.Helper()
	testutil.SkipIfSymlinkUnsupported(t)
}
