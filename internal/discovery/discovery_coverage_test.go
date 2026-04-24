package discovery

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jeduden/mdsmith/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- visit tests ---

func TestVisit_WalkError(t *testing.T) {
	w := &walker{
		absBase:  "/tmp/base",
		patterns: []string{"**/*.md"},
		seen:     make(map[string]bool),
	}
	err := w.visit("/tmp/base/foo.md", nil, os.ErrPermission)
	assert.ErrorIs(t, err, os.ErrPermission)
}

// TestVisit_SkipsSymlinkDirByDefault confirms the walker returns nil
// (no descent) for a symlinked directory entry when FollowSymlinks is
// false. filepath.Walk reports symlinks via Lstat, so the entry's
// info has ModeSymlink set but IsDir()==false — the test uses real
// Lstat info rather than a synthetic fakeFileInfo so the assertion
// reflects actual Walk semantics.
func TestVisit_SkipsSymlinkDirByDefault(t *testing.T) {
	skipIfSymlinkUnsupported(t)
	dir := t.TempDir()

	target := filepath.Join(dir, "real")
	require.NoError(t, os.MkdirAll(target, 0o755))
	linked := filepath.Join(dir, "linked")
	require.NoError(t, os.Symlink(target, linked))

	info, err := os.Lstat(linked)
	require.NoError(t, err)
	require.NotZero(t, info.Mode()&os.ModeSymlink, "fixture must be a symlink")

	w := &walker{
		absBase:  dir,
		patterns: []string{"**/*.md"},
		seen:     make(map[string]bool),
	}
	visitErr := w.visit(linked, info, nil)
	assert.NoError(t, visitErr)
	assert.Empty(t, w.result, "symlink must be skipped")
}

// TestVisit_FollowsSymlinkFileWhenOptedIn asserts a symlinked markdown
// file is NOT skipped when FollowSymlinks=true. This is the primary
// opt-in case — symlinked directories are still not recursed into,
// per the Options.FollowSymlinks doc comment.
func TestVisit_FollowsSymlinkFileWhenOptedIn(t *testing.T) {
	skipIfSymlinkUnsupported(t)
	dir := t.TempDir()

	target := filepath.Join(dir, "real.md")
	require.NoError(t, os.WriteFile(target, []byte("# Real\n"), 0o644))
	linked := filepath.Join(dir, "linked.md")
	require.NoError(t, os.Symlink(target, linked))

	info, err := os.Lstat(linked)
	require.NoError(t, err)

	w := &walker{
		absBase:        dir,
		patterns:       []string{"**/*.md"},
		followSymlinks: true,
		seen:           make(map[string]bool),
	}
	visitErr := w.visit(linked, info, nil)
	assert.NoError(t, visitErr)
	assert.Len(t, w.result, 1, "symlinked file must be included under opt-in")
}

func TestVisit_SkipsFileWhenGitignored(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".gitignore", "ignored.md\n")
	writeFile(t, dir, "ignored.md", "# Ignored\n")
	writeFile(t, dir, "kept.md", "# Kept\n")

	files, err := Discover(Options{
		Patterns:     []string{"**/*.md"},
		BaseDir:      dir,
		UseGitignore: true,
	})
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, "kept.md", filepath.Base(files[0]))
}

func TestVisit_RootDirSkipped(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "readme.md", "# Hello\n")

	w := &walker{
		absBase:  dir,
		patterns: []string{"**/*.md"},
		seen:     make(map[string]bool),
	}

	// visit the root itself (rel == ".")
	info, err := os.Stat(dir)
	require.NoError(t, err)
	err = w.visit(dir, info, nil)
	assert.NoError(t, err)
	assert.Empty(t, w.result, "root dir should be skipped")
}

// --- Discover edge cases ---

func TestDiscover_InvalidPatterns(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "readme.md", "# Hello\n")

	// All patterns are invalid, so no files discovered.
	files, err := Discover(Options{
		Patterns: []string{"[invalid"},
		BaseDir:  dir,
	})
	require.NoError(t, err)
	assert.Empty(t, files)
}

func TestDiscover_MixedValidAndInvalidPatterns(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "readme.md", "# Hello\n")

	files, err := Discover(Options{
		Patterns: []string{"[invalid", "**/*.md"},
		BaseDir:  dir,
	})
	require.NoError(t, err)
	require.Len(t, files, 1)
}

func TestDiscover_DefaultBaseDir(t *testing.T) {
	// When BaseDir is empty, it defaults to ".".
	// Just verify it doesn't error out.
	_, err := Discover(Options{
		Patterns: []string{"nonexistent-pattern-xyz-*.md"},
		BaseDir:  "",
	})
	require.NoError(t, err)
}

// TestDiscover_SymlinkToFile_SkippedByDefault asserts the secure
// default: a symlinked file is not discovered.
func TestDiscover_SymlinkToFile_SkippedByDefault(t *testing.T) {
	skipIfSymlinkUnsupported(t)
	dir := t.TempDir()
	writeFile(t, dir, "real.md", "# Real\n")

	linkPath := filepath.Join(dir, "link.md")
	realPath := filepath.Join(dir, "real.md")
	require.NoError(t, os.Symlink(realPath, linkPath))

	files, err := Discover(Options{
		Patterns: []string{"**/*.md"},
		BaseDir:  dir,
	})
	require.NoError(t, err)
	require.Len(t, files, 1, "symlink must be skipped by default")
	assert.Equal(t, "real.md", filepath.Base(files[0]))
}

// TestDiscover_FollowSymlinks_OptIn asserts that FollowSymlinks=true
// surfaces the symlink as a distinct discovery result.
func TestDiscover_FollowSymlinks_OptIn(t *testing.T) {
	skipIfSymlinkUnsupported(t)
	dir := t.TempDir()
	writeFile(t, dir, "real.md", "# Real\n")

	linkPath := filepath.Join(dir, "link.md")
	realPath := filepath.Join(dir, "real.md")
	require.NoError(t, os.Symlink(realPath, linkPath))

	files, err := Discover(Options{
		Patterns:       []string{"**/*.md"},
		BaseDir:        dir,
		FollowSymlinks: true,
	})
	require.NoError(t, err)
	require.Len(t, files, 2, "both real and linked entries are discovered")
}

// skipIfSymlinkUnsupported forwards to the shared testutil helper.
func skipIfSymlinkUnsupported(t *testing.T) {
	t.Helper()
	testutil.SkipIfSymlinkUnsupported(t)
}
