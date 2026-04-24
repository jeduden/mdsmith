//go:build !windows

package lint

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResolveFiles_SkipsNonRegularEntries asserts that FIFOs, and
// by extension other non-regular file types, are never enqueued —
// even when their name has a markdown extension. Reading such
// entries via the lint pipeline could block indefinitely.
//
// The test is gated behind `!windows` because `syscall.Mkfifo` is
// not available on Windows (the syscall package elides it at
// compile time for that platform).
func TestResolveFiles_SkipsNonRegularEntries(t *testing.T) {
	dir := t.TempDir()
	// Real file + FIFO-with-.md-name in the same directory.
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "real.md"), []byte("# Real"), 0o644))
	fifo := filepath.Join(dir, "pipe.md")
	require.NoError(t, syscall.Mkfifo(fifo, 0o644))

	// Explicit arg (resolveArg path).
	gotExplicit, err := ResolveFiles([]string{fifo})
	require.NoError(t, err)
	assert.Empty(t, gotExplicit,
		"explicit FIFO arg must not be enqueued")

	// Directory walk (walkDir path).
	gotWalk, err := ResolveFiles([]string{dir})
	require.NoError(t, err)
	assert.Equal(t, []string{filepath.Join(dir, "real.md")}, gotWalk,
		"walkDir must include the regular file and skip the FIFO")

	// Glob expansion (resolveGlob path).
	gotGlob, err := ResolveFiles([]string{filepath.Join(dir, "*.md")})
	require.NoError(t, err)
	assert.Equal(t, []string{filepath.Join(dir, "real.md")}, gotGlob,
		"resolveGlob must skip the FIFO even with a matching name")
}

// TestResolveFiles_SymlinkToFifo_SkippedUnderOptIn covers the
// "symlink to non-regular file" branch: even with FollowSymlinks
// enabled, a symlink pointing at a FIFO must not be enqueued.
func TestResolveFiles_SymlinkToFifo_SkippedUnderOptIn(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "pipe")
	require.NoError(t, syscall.Mkfifo(fifo, 0o644))
	link := filepath.Join(dir, "link.md")
	require.NoError(t, os.Symlink(fifo, link))

	opts := DefaultResolveOpts()
	opts.FollowSymlinks = true

	// Explicit arg via resolveArg.
	gotExplicit, err := ResolveFilesWithOpts([]string{link}, opts)
	require.NoError(t, err)
	assert.Empty(t, gotExplicit,
		"symlink to FIFO must be skipped even under --follow-symlinks")

	// Directory walk via walkDir.
	gotWalk, err := ResolveFilesWithOpts([]string{dir}, opts)
	require.NoError(t, err)
	assert.Empty(t, gotWalk,
		"walkDir must skip symlinks whose target is a FIFO")

	// Glob expansion via resolveGlob.
	gotGlob, err := ResolveFilesWithOpts(
		[]string{filepath.Join(dir, "*.md")}, opts)
	require.NoError(t, err)
	assert.Empty(t, gotGlob,
		"resolveGlob must skip symlinks whose target is a FIFO")
}

// TestResolveFiles_BrokenSymlink_SilentlySkipped covers the
// broken-symlink branches: a symlink whose target was removed
// must be silently skipped (not error out) across all entry
// points, matching the FollowSymlinks contract.
func TestResolveFiles_BrokenSymlink_SilentlySkipped(t *testing.T) {
	dir := t.TempDir()
	// Create a symlink pointing at a path that does not exist.
	link := filepath.Join(dir, "dangling.md")
	require.NoError(t, os.Symlink(
		filepath.Join(dir, "does-not-exist.md"), link))

	opts := DefaultResolveOpts()
	opts.FollowSymlinks = true

	// Explicit arg.
	got, err := ResolveFilesWithOpts([]string{link}, opts)
	require.NoError(t, err,
		"broken symlink arg must not surface an error")
	assert.Empty(t, got)

	// Glob expansion.
	got, err = ResolveFilesWithOpts(
		[]string{filepath.Join(dir, "*.md")}, opts)
	require.NoError(t, err)
	assert.Empty(t, got)

	// Directory walk.
	got, err = ResolveFilesWithOpts([]string{dir}, opts)
	require.NoError(t, err)
	assert.Empty(t, got)
}
