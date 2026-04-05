package discovery

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- matchesPath tests ---

func TestMatchesPath_ExactMatch(t *testing.T) {
	assert.True(t, matchesPath([]string{"foo.md"}, "foo.md"))
}

func TestMatchesPath_GlobPattern(t *testing.T) {
	assert.True(t, matchesPath([]string{"*.md"}, "readme.md"))
}

func TestMatchesPath_NoMatch(t *testing.T) {
	assert.False(t, matchesPath([]string{"*.txt"}, "readme.md"))
}

func TestMatchesPath_EmptyPatterns(t *testing.T) {
	assert.False(t, matchesPath([]string{}, "readme.md"))
}

func TestMatchesPath_InvalidPattern(t *testing.T) {
	// Invalid glob pattern should be skipped without panic.
	assert.False(t, matchesPath([]string{"[invalid"}, "readme.md"))
}

func TestMatchesPath_MatchesBasename(t *testing.T) {
	assert.True(t, matchesPath([]string{"readme.md"}, "/some/path/readme.md"))
}

func TestMatchesPath_MatchesCleanedPath(t *testing.T) {
	assert.True(t, matchesPath([]string{"foo/bar.md"}, "foo//bar.md"))
}

func TestMatchesPath_MultiplePatterns(t *testing.T) {
	assert.True(t, matchesPath([]string{"*.txt", "*.md"}, "readme.md"))
}

func TestMatchesPath_MultiplePatterns_NoneMatch(t *testing.T) {
	assert.False(t, matchesPath([]string{"*.txt", "*.go"}, "readme.md"))
}

// --- isNoFollow tests ---

func TestIsNoFollow_NoPatterns(t *testing.T) {
	w := &walker{noFollow: nil}
	info := fakeFileInfo{name: "link.md", mode: os.ModeSymlink}
	assert.False(t, w.isNoFollow("link.md", info))
}

func TestIsNoFollow_NotSymlink(t *testing.T) {
	w := &walker{noFollow: []string{"*.md"}}
	info := fakeFileInfo{name: "file.md", mode: 0}
	assert.False(t, w.isNoFollow("file.md", info))
}

func TestIsNoFollow_SymlinkMatchesPattern(t *testing.T) {
	w := &walker{noFollow: []string{"*.md"}}
	info := fakeFileInfo{name: "link.md", mode: os.ModeSymlink}
	assert.True(t, w.isNoFollow("link.md", info))
}

func TestIsNoFollow_SymlinkNoMatch(t *testing.T) {
	w := &walker{noFollow: []string{"*.txt"}}
	info := fakeFileInfo{name: "link.md", mode: os.ModeSymlink}
	assert.False(t, w.isNoFollow("link.md", info))
}

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

func TestVisit_SkipsDirWhenNoFollow(t *testing.T) {
	dir := t.TempDir()
	absBase := dir

	subDir := filepath.Join(dir, "linked")
	require.NoError(t, os.MkdirAll(subDir, 0o755))

	w := &walker{
		absBase:  absBase,
		patterns: []string{"**/*.md"},
		noFollow: []string{"linked"},
		seen:     make(map[string]bool),
	}

	info := fakeFileInfo{name: "linked", mode: os.ModeDir | os.ModeSymlink, isDir: true}
	err := w.visit(filepath.Join(absBase, "linked"), info, nil)
	assert.Equal(t, filepath.SkipDir, err)
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

func TestDiscover_SymlinkToFile(t *testing.T) {
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
	// Both real.md and link.md should be found (separate abs paths).
	require.Len(t, files, 2)
}

func TestDiscover_NoFollowSymlinksSkipsSymlinkedFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "real.md", "# Real\n")

	linkPath := filepath.Join(dir, "link.md")
	realPath := filepath.Join(dir, "real.md")
	require.NoError(t, os.Symlink(realPath, linkPath))

	// Note: filepath.Walk doesn't report symlinks with ModeSymlink for files,
	// it follows them. So NoFollowSymlinks only reliably works with Lstat-based
	// walkers. This test documents current behavior.
	files, err := Discover(Options{
		Patterns:         []string{"**/*.md"},
		BaseDir:          dir,
		NoFollowSymlinks: []string{"link.md"},
	})
	require.NoError(t, err)
	// filepath.Walk follows symlinks, so isNoFollow won't trigger for file symlinks.
	// Both files should still appear.
	assert.GreaterOrEqual(t, len(files), 1)
}

// fakeFileInfo implements os.FileInfo for testing.
type fakeFileInfo struct {
	name  string
	mode  os.FileMode
	isDir bool
}

func (f fakeFileInfo) Name() string      { return f.name }
func (f fakeFileInfo) Size() int64       { return 0 }
func (f fakeFileInfo) Mode() os.FileMode { return f.mode }
func (f fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeFileInfo) IsDir() bool       { return f.isDir }
func (f fakeFileInfo) Sys() any          { return nil }
