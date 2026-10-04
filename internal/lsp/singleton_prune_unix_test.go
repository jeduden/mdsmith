//go:build unix

package lsp

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDirOwnedByUID(t *testing.T) {
	t.Parallel()
	info, err := os.Lstat(t.TempDir())
	require.NoError(t, err)
	uid := os.Getuid()
	assert.True(t, dirOwnedBy(info, uid), "a dir we created is ours")
	assert.False(t, dirOwnedBy(info, uid+1), "another uid does not own it")
}

// A FileInfo without a *syscall.Stat_t (a fake) cannot prove ownership,
// so it is refused.
func TestDirOwnedByRejectsUnknownSys(t *testing.T) {
	t.Parallel()
	assert.False(t, dirOwnedBy(fakeInfo{}, os.Getuid()))
}

// A world-writable registry dir lets any local user swap entries, so
// prune skips it.
func TestPruneStaleSkipsWorldWritableRegistryDir(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "lsp-singleton")
	require.NoError(t, os.Mkdir(dir, 0o700))
	require.NoError(t, os.Chmod(dir, 0o777))
	p := filepath.Join(dir, "x.owner")
	writeStale(t, p)

	pruneStale(dir, "pruner", time.Now().Add(-time.Hour))

	assert.FileExists(t, p)
}

// A world-writable parent (the shared "mdsmith" dir another user made)
// lets that user rename the registry dir away mid-prune, so prune skips.
func TestPruneStaleSkipsWorldWritableParent(t *testing.T) {
	t.Parallel()
	parent := filepath.Join(t.TempDir(), "mdsmith")
	dir := filepath.Join(parent, "lsp-singleton")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	require.NoError(t, os.Chmod(parent, 0o777))
	p := filepath.Join(dir, "x.owner")
	writeStale(t, p)

	pruneStale(dir, "pruner", time.Now().Add(-time.Hour))

	assert.FileExists(t, p)
}

type fakeInfo struct{ os.FileInfo }

func (fakeInfo) Sys() any { return nil }

// A registry dir that passes the safety check but cannot be listed
// leaves prune a no-op rather than an error.
func TestPruneStaleToleratesUnreadableRegistryDir(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root reads any directory")
	}
	dir := filepath.Join(t.TempDir(), "lsp-singleton")
	require.NoError(t, os.Mkdir(dir, 0o700))
	p := filepath.Join(dir, "x.owner")
	writeStale(t, p)
	require.NoError(t, os.Chmod(dir, 0o300))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	assert.NotPanics(t, func() {
		pruneStale(dir, "pruner", time.Now().Add(-time.Hour))
	})
	require.NoError(t, os.Chmod(dir, 0o700))
	assert.FileExists(t, p)
}
