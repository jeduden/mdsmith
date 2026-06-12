package build

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSnapshotDirs_RecordsFiles(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "a.txt"), []byte("hello"), 0o644))

	snap, err := snapshotDirs([]string{root}, snapshotCap, nil)
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
	_, err := snapshotDirs([]string{root}, 3, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "2 000")
	assert.Contains(t, err.Error(), root)
}

func TestDiffSnapshots_DetectsAddedFile(t *testing.T) {
	root := t.TempDir()
	declared := filepath.Join(root, "declared.txt")
	require.NoError(t, os.WriteFile(declared, []byte("orig"), 0o644))

	before, err := snapshotDirs([]string{root}, snapshotCap, nil)
	require.NoError(t, err)

	// Recipe writes an undeclared sibling.
	undeclared := filepath.Join(root, "sneaky.txt")
	require.NoError(t, os.WriteFile(undeclared, []byte("evil"), 0o644))

	after, err := snapshotDirs([]string{root}, snapshotCap, before)
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
	// Pin the mtime BEFORE the before-snapshot so that, after a same-size
	// rewrite and an identical Chtimes, the after-file's cheap fields match
	// the before-snapshot. That makes statFile hash the after-file too, so
	// the verdict comes from two genuinely-computed content hashes — the
	// content-preserving-rewrite path, not a zero-vs-real hash artifact.
	fixedTime := time.Unix(1700000000, 0)
	require.NoError(t, os.Chtimes(other, fixedTime, fixedTime))

	before, err := snapshotDirs([]string{root}, snapshotCap, nil)
	require.NoError(t, err)
	require.NotEqual(t, [32]byte{}, before[other].hash, "before hashes eagerly")

	// Same size, different content, same mtime.
	require.NoError(t, os.WriteFile(other, []byte("bbbbb"), 0o644))
	require.NoError(t, os.Chtimes(other, fixedTime, fixedTime))
	after, err := snapshotDirs([]string{root}, snapshotCap, before)
	require.NoError(t, err)
	// The after-file's cheap fields matched the before snapshot, so it was
	// hashed; the two real hashes differ.
	require.NotEqual(t, [32]byte{}, after[other].hash, "after hashes on cheap-field match")
	require.NotEqual(t, before[other].hash, after[other].hash)

	violations := diffSnapshots(before, after, map[string]struct{}{})
	require.Len(t, violations, 1)
	assert.Equal(t, "modified", violations[0].kind)
}

func TestSnapshotDirs_AfterSkipsHashWhenCheapFieldsDiffer(t *testing.T) {
	root := t.TempDir()
	f := filepath.Join(root, "n.txt")
	require.NoError(t, os.WriteFile(f, []byte("aaaaa"), 0o644))

	before, err := snapshotDirs([]string{root}, snapshotCap, nil)
	require.NoError(t, err)
	require.NotEqual(t, [32]byte{}, before[f].hash, "before-snapshot hashes eagerly")

	// Change the size so the cheap fields differ from the before-snapshot.
	require.NoError(t, os.WriteFile(f, []byte("bb"), 0o644))
	after, err := snapshotDirs([]string{root}, snapshotCap, before)
	require.NoError(t, err)
	// The after-snapshot skipped the content hash: the cheap fields already
	// settle the verdict, so no bytes were read.
	assert.Equal(t, [32]byte{}, after[f].hash)

	// The verdict is still correct.
	violations := diffSnapshots(before, after, map[string]struct{}{})
	require.Len(t, violations, 1)
	assert.Equal(t, "modified", violations[0].kind)
}

func TestDiffSnapshots_DeclaredOutputIgnored(t *testing.T) {
	root := t.TempDir()
	declared := filepath.Join(root, "out.txt")

	before, err := snapshotDirs([]string{root}, snapshotCap, nil)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(declared, []byte("new"), 0o644))
	after, err := snapshotDirs([]string{root}, snapshotCap, before)
	require.NoError(t, err)

	violations := diffSnapshots(before, after, map[string]struct{}{declared: {}})
	assert.Empty(t, violations, "writes to declared outputs are allowed")
}

func TestDiffSnapshots_DetectsModeChange(t *testing.T) {
	root := t.TempDir()
	f := filepath.Join(root, "m.txt")
	require.NoError(t, os.WriteFile(f, []byte("x"), 0o644))

	before, err := snapshotDirs([]string{root}, snapshotCap, nil)
	require.NoError(t, err)
	require.NoError(t, os.Chmod(f, 0o600))
	after, err := snapshotDirs([]string{root}, snapshotCap, before)
	require.NoError(t, err)

	violations := diffSnapshots(before, after, map[string]struct{}{})
	require.Len(t, violations, 1)
	assert.Equal(t, "modified", violations[0].kind)
}

func TestDiffSnapshots_DetectsRemoval(t *testing.T) {
	root := t.TempDir()
	f := filepath.Join(root, "gone.txt")
	require.NoError(t, os.WriteFile(f, []byte("x"), 0o644))

	before, err := snapshotDirs([]string{root}, snapshotCap, nil)
	require.NoError(t, err)
	require.NoError(t, os.Remove(f))
	after, err := snapshotDirs([]string{root}, snapshotCap, before)
	require.NoError(t, err)

	violations := diffSnapshots(before, after, map[string]struct{}{})
	require.Len(t, violations, 1)
	assert.Equal(t, "removed", violations[0].kind)
}

func TestSnapshotDirs_DeduplicatesDirs(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "a.txt"), []byte("x"), 0o644))

	// Passing the same dir twice must not double-count entries.
	snap, err := snapshotDirs([]string{root, root}, snapshotCap, nil)
	require.NoError(t, err)
	assert.Len(t, snap, 1)
}

func TestSnapshotDirs_ReadDirError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("ENOTDIR semantics differ on Windows")
	}
	// Point a "directory" path at a regular file so os.ReadDir returns
	// ENOTDIR — a non-ErrNotExist error that hits the "scanning" branch
	// regardless of whether the process runs as root.
	root := t.TempDir()
	notadir := filepath.Join(root, "file")
	require.NoError(t, os.WriteFile(notadir, []byte("x"), 0o644))

	_, err := snapshotDirs([]string{notadir}, snapshotCap, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "scanning")
}

func TestSnapshotDirs_NotExistDir(t *testing.T) {
	// A not-yet-created output parent contributes nothing and is not an
	// error: the IsNotExist branch continues past it.
	snap, err := snapshotDirs([]string{filepath.Join(t.TempDir(), "nonexistent")}, snapshotCap, nil)
	require.NoError(t, err)
	assert.Empty(t, snap)
}

func TestSnapshotDirs_StatFileError(t *testing.T) {
	old := statFileFn
	statFileFn = func(string, fileState, bool) (fileState, error) {
		return fileState{}, errors.New("stat failed")
	}
	t.Cleanup(func() { statFileFn = old })

	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "a.txt"), []byte("x"), 0o644))
	_, err := snapshotDirs([]string{root}, snapshotCap, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stat failed")
}

func TestStatFile_LstatError(t *testing.T) {
	// A nonexistent path makes os.Lstat fail with ErrNotExist, surfacing the
	// "inspecting" error branch.
	_, err := statFile(filepath.Join(t.TempDir(), "nonexistent"), fileState{}, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "inspecting")
}

func TestStatFile_ReadlinkError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on Windows")
	}
	old := readlinkFn
	readlinkFn = func(string) (string, error) { return "", errors.New("readlink failed") }
	t.Cleanup(func() { readlinkFn = old })

	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	link := filepath.Join(root, "link.txt")
	require.NoError(t, os.WriteFile(target, []byte("x"), 0o644))
	require.NoError(t, os.Symlink(target, link))

	_, err := statFile(link, fileState{}, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading symlink")
}

func TestStatFile_HashFileSumError(t *testing.T) {
	old := hashFileSumFn
	hashFileSumFn = func(string) ([32]byte, error) {
		return [32]byte{}, errors.New("hash failed")
	}
	t.Cleanup(func() { hashFileSumFn = old })

	root := t.TempDir()
	f := filepath.Join(root, "a.txt")
	require.NoError(t, os.WriteFile(f, []byte("x"), 0o644))

	_, err := statFile(f, fileState{}, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "hash failed")
}

func TestHashFileSum_OpenError(t *testing.T) {
	_, err := hashFileSum(filepath.Join(t.TempDir(), "nonexistent.txt"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "hashing")
}

func TestSnapshotDirs_SymlinkEntry(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on Windows")
	}
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	link := filepath.Join(root, "link.txt")
	require.NoError(t, os.WriteFile(target, []byte("hello"), 0o644))
	require.NoError(t, os.Symlink(target, link))

	snap, err := snapshotDirs([]string{root}, snapshotCap, nil)
	require.NoError(t, err)

	entry, ok := snap[link]
	require.True(t, ok, "symlink entry must be snapshotted")
	assert.Equal(t, target, entry.link)
}

func TestDiffSnapshots_SortsTwoViolations(t *testing.T) {
	root := t.TempDir()
	// Take an empty before-snapshot, then add two files so both appear
	// as "added" violations. The sort comparison function only fires with
	// 2+ violations.
	before, err := snapshotDirs([]string{root}, snapshotCap, nil)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(root, "b.txt"), []byte("b"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "a.txt"), []byte("a"), 0o644))

	after, err := snapshotDirs([]string{root}, snapshotCap, before)
	require.NoError(t, err)

	violations := diffSnapshots(before, after, map[string]struct{}{})
	require.Len(t, violations, 2)
	// Violations must be sorted by path.
	assert.Less(t, violations[0].path, violations[1].path)
	assert.Equal(t, "added", violations[0].kind)
}
