//go:build !windows

package discovery

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/jeduden/mdsmith/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDiscover_SkipsFifoEntries covers the non-symlink
// non-regular skip branch in the discovery walker: a FIFO with
// a markdown-matching name must not be added to results.
func TestDiscover_SkipsFifoEntries(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "real.md"), []byte("# Real"), 0o644))
	require.NoError(t, syscall.Mkfifo(
		filepath.Join(dir, "pipe.md"), 0o644))

	files, err := Discover(Options{
		Patterns: []string{"**/*.md"},
		BaseDir:  dir,
	})
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, "real.md", filepath.Base(files[0]))
}

// TestDiscover_SymlinkToFifo_SkippedUnderOptIn covers the
// symlink-to-non-regular skip branch: even in opt-in mode, a
// symlink whose target is a FIFO must not be included.
func TestDiscover_SymlinkToFifo_SkippedUnderOptIn(t *testing.T) {
	testutil.SkipIfSymlinkUnsupported(t)
	dir := t.TempDir()

	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "real.md"), []byte("# Real"), 0o644))

	fifo := filepath.Join(dir, "pipe")
	require.NoError(t, syscall.Mkfifo(fifo, 0o644))
	require.NoError(t, os.Symlink(fifo, filepath.Join(dir, "link.md")))

	files, err := Discover(Options{
		Patterns:       []string{"**/*.md"},
		BaseDir:        dir,
		FollowSymlinks: true,
	})
	require.NoError(t, err)
	require.Len(t, files, 1,
		"symlink to FIFO must not be enqueued even under FollowSymlinks")
	assert.Equal(t, "real.md", filepath.Base(files[0]))
}
