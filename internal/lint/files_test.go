package lint

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

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

// --- NoFollowSymlinks tests ---

func TestResolveFilesWithOpts_NoFollowSymlinks(t *testing.T) {
	dir := t.TempDir()

	realFile := filepath.Join(dir, "real.md")
	require.NoError(t, os.WriteFile(realFile, []byte("# Real"), 0o644))

	subDir := filepath.Join(dir, "target")
	require.NoError(t, os.MkdirAll(subDir, 0o755))
	targetFile := filepath.Join(subDir, "doc.md")
	require.NoError(t, os.WriteFile(targetFile, []byte("# Target"), 0o644))

	linkFile := filepath.Join(dir, "link.md")
	require.NoError(t, os.Symlink(targetFile, linkFile))

	// Without no-follow-symlinks: all files should be found.
	files, err := ResolveFilesWithOpts([]string{dir}, DefaultResolveOpts())
	require.NoError(t, err)
	require.Len(t, files, 3) // real.md, link.md, target/doc.md

	// With no-follow-symlinks matching all .md: symlink should be skipped.
	noGitignore := false
	opts := ResolveOpts{
		UseGitignore:     &noGitignore,
		NoFollowSymlinks: []string{"*.md"},
	}
	files, err = ResolveFilesWithOpts([]string{dir}, opts)
	require.NoError(t, err)
	require.Len(t, files, 2, "expected 2 files (symlink skipped)")
	for _, f := range files {
		assert.NotEqual(t, "link.md", filepath.Base(f), "link.md should have been skipped (symlink)")
	}
}

func TestResolveFilesWithOpts_NoFollowSymlinks_PatternSpecific(t *testing.T) {
	dir := t.TempDir()

	targetA := filepath.Join(dir, "a.md")
	targetB := filepath.Join(dir, "b.md")
	require.NoError(t, os.WriteFile(targetA, []byte("# A"), 0o644))
	require.NoError(t, os.WriteFile(targetB, []byte("# B"), 0o644))

	linkDir := filepath.Join(dir, "links")
	require.NoError(t, os.MkdirAll(linkDir, 0o755))
	require.NoError(t, os.Symlink(targetA, filepath.Join(linkDir, "link-a.md")))
	require.NoError(t, os.Symlink(targetB, filepath.Join(linkDir, "link-b.md")))

	noGitignore := false
	opts := ResolveOpts{
		UseGitignore:     &noGitignore,
		NoFollowSymlinks: []string{"**/links/*"},
	}
	files, err := ResolveFilesWithOpts([]string{dir}, opts)
	require.NoError(t, err)
	require.Len(t, files, 2)
}
