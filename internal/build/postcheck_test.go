package build

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSnapshotDirs_RecordsFiles(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "a.txt"), []byte("hello"), 0o644))

	snap, err := snapshotDirs([]string{root}, snapshotCap)
	require.NoError(t, err)
	entry, ok := snap[filepath.Join(root, "a.txt")]
	require.True(t, ok)
	assert.Equal(t, int64(5), entry.size)
}

func TestSnapshotDirs_CapExceeded(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 5; i++ {
		require.NoError(t, os.WriteFile(filepath.Join(root, string(rune('a'+i))+".txt"), []byte("x"), 0o644))
	}
	_, err := snapshotDirs([]string{root}, 3)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "2 000")
	assert.Contains(t, err.Error(), root)
}

func TestDiffSnapshots_DetectsAddedFile(t *testing.T) {
	root := t.TempDir()
	declared := filepath.Join(root, "declared.txt")
	require.NoError(t, os.WriteFile(declared, []byte("orig"), 0o644))

	before, err := snapshotDirs([]string{root}, snapshotCap)
	require.NoError(t, err)

	// Recipe writes an undeclared sibling.
	undeclared := filepath.Join(root, "sneaky.txt")
	require.NoError(t, os.WriteFile(undeclared, []byte("evil"), 0o644))

	after, err := snapshotDirs([]string{root}, snapshotCap)
	require.NoError(t, err)

	violations := diffSnapshots(before, after, map[string]struct{}{declared: {}})
	require.Len(t, violations, 1)
	assert.Equal(t, undeclared, violations[0].path)
	assert.Equal(t, "added", violations[0].kind)
}

func TestDiffSnapshots_DetectsModifiedContent(t *testing.T) {
	root := t.TempDir()
	other := filepath.Join(root, "other.txt")
	require.NoError(t, os.WriteFile(other, []byte("aaaaa"), 0o644))

	before, err := snapshotDirs([]string{root}, snapshotCap)
	require.NoError(t, err)
	// Same size, different content, with mtime reset to match so only the
	// eagerly-captured content hash can distinguish the two snapshots —
	// exercising the content-preserving-rewrite path.
	fixedTime := time.Unix(1700000000, 0)
	require.NoError(t, os.WriteFile(other, []byte("bbbbb"), 0o644))
	require.NoError(t, os.Chtimes(other, fixedTime, fixedTime))
	after, err := snapshotDirs([]string{root}, snapshotCap)
	require.NoError(t, err)
	// Force the before-state mtime to match the after-state mtime.
	bs := before[other]
	bs.mtime = after[other].mtime
	before[other] = bs

	violations := diffSnapshots(before, after, map[string]struct{}{})
	require.Len(t, violations, 1)
	assert.Equal(t, "modified", violations[0].kind)
}

func TestDiffSnapshots_DeclaredOutputIgnored(t *testing.T) {
	root := t.TempDir()
	declared := filepath.Join(root, "out.txt")

	before, err := snapshotDirs([]string{root}, snapshotCap)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(declared, []byte("new"), 0o644))
	after, err := snapshotDirs([]string{root}, snapshotCap)
	require.NoError(t, err)

	violations := diffSnapshots(before, after, map[string]struct{}{declared: {}})
	assert.Empty(t, violations, "writes to declared outputs are allowed")
}

func TestDiffSnapshots_DetectsModeChange(t *testing.T) {
	root := t.TempDir()
	f := filepath.Join(root, "m.txt")
	require.NoError(t, os.WriteFile(f, []byte("x"), 0o644))

	before, err := snapshotDirs([]string{root}, snapshotCap)
	require.NoError(t, err)
	require.NoError(t, os.Chmod(f, 0o600))
	after, err := snapshotDirs([]string{root}, snapshotCap)
	require.NoError(t, err)

	violations := diffSnapshots(before, after, map[string]struct{}{})
	require.Len(t, violations, 1)
	assert.Equal(t, "modified", violations[0].kind)
}

func TestDiffSnapshots_DetectsRemoval(t *testing.T) {
	root := t.TempDir()
	f := filepath.Join(root, "gone.txt")
	require.NoError(t, os.WriteFile(f, []byte("x"), 0o644))

	before, err := snapshotDirs([]string{root}, snapshotCap)
	require.NoError(t, err)
	require.NoError(t, os.Remove(f))
	after, err := snapshotDirs([]string{root}, snapshotCap)
	require.NoError(t, err)

	violations := diffSnapshots(before, after, map[string]struct{}{})
	require.Len(t, violations, 1)
	assert.Equal(t, "removed", violations[0].kind)
}
