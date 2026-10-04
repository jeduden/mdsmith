package lsp

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeStale creates path holding "x" with an mtime two hours ago.
func writeStale(t *testing.T, path string) {
	t.Helper()
	old := time.Now().Add(-2 * time.Hour)
	require.NoError(t, os.WriteFile(path, []byte("x"), 0o600))
	require.NoError(t, os.Chtimes(path, old, old))
}

// symlinkOrSkip creates link -> target, skipping the test where the
// platform or privileges forbid symlinks (Windows without developer
// mode, for one).
func symlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
}

// On the shared temp-dir fallback another user can plant the registry
// dir as a symlink to a directory of the victim's. Prune must refuse
// it, or it deletes the target's old *.owner files.
func TestPruneStaleSkipsSymlinkedRegistryDir(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	target := filepath.Join(base, "target")
	require.NoError(t, os.Mkdir(target, 0o700))
	victim := filepath.Join(target, "keep.owner")
	writeStale(t, victim)
	link := filepath.Join(base, "lsp-singleton")
	symlinkOrSkip(t, target, link)

	pruneStale(link, "pruner", time.Now().Add(-time.Hour))

	assert.FileExists(t, victim, "prune must not follow a symlinked registry dir")
}

// A planted symlink one level up (the shared "mdsmith" dir) redirects
// the registry dir just as well, so the parent is checked too.
func TestPruneStaleSkipsRegistryDirUnderSymlinkedParent(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	target := filepath.Join(base, "target")
	require.NoError(t, os.MkdirAll(filepath.Join(target, "lsp-singleton"), 0o700))
	victim := filepath.Join(target, "lsp-singleton", "keep.owner")
	writeStale(t, victim)
	link := filepath.Join(base, "mdsmith")
	symlinkOrSkip(t, target, link)

	pruneStale(filepath.Join(link, "lsp-singleton"), "pruner", time.Now().Add(-time.Hour))

	assert.FileExists(t, victim, "prune must not follow a symlinked parent dir")
}

// A registry dir that is a plain file, not a directory, is a no-op.
func TestPruneStaleSkipsNonDirectoryRegistry(t *testing.T) {
	t.Parallel()
	p := filepath.Join(t.TempDir(), "lsp-singleton")
	writeStale(t, p)
	pruneStale(p, "pruner", time.Now().Add(-time.Hour))
	assert.FileExists(t, p)
}

// Only regular files are pruned: a symlink named like a record is left
// in place (neither removed nor renamed to a quarantine path), and its
// target is untouched. The cutoff is in the future so the link's own
// fresh mtime would otherwise count as stale.
func TestPruneStaleSkipsSymlinkEntries(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.owner")
	writeStale(t, outside)
	link := filepath.Join(dir, "x.owner")
	symlinkOrSkip(t, outside, link)

	pruneStale(dir, "pruner", time.Now().Add(time.Hour))

	info, err := os.Lstat(link)
	require.NoError(t, err, "the symlink entry must not be removed or renamed")
	assert.NotZero(t, info.Mode()&os.ModeSymlink)
	assert.FileExists(t, outside)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "no quarantine file may be created")
}

// A quarantine path that turns out not to be a regular file (swapped
// between the move and the re-check) is never removed; it goes back.
func TestPruneStaleRestoresNonRegularQuarantine(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p := filepath.Join(dir, "x.owner")
	writeStale(t, p)
	q := p + ".pruner.prune"

	pruneStale(dir, "pruner", time.Now().Add(time.Hour), nil, func(string) {
		require.NoError(t, os.Remove(q))
		require.NoError(t, os.Mkdir(q, 0o700))
	})

	assert.DirExists(t, p, "the non-regular quarantine is moved back, not removed")
}
