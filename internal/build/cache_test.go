package build

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadCache_MissingFileReturnsEmpty(t *testing.T) {
	root := t.TempDir()
	c, err := LoadCache(root)
	require.NoError(t, err)
	assert.Equal(t, CacheVersion, c.Version)
	assert.Empty(t, c.Entries)
}

func TestCache_SaveLoadRoundTrip(t *testing.T) {
	root := t.TempDir()
	c := NewCache()
	c.Put(CacheEntry{
		Outputs:  []OutputHash{{Path: "out.txt", Hash: "sha256-abc"}},
		Inputs:   []string{"src.txt"},
		ActionID: "sha256-deadbeef",
		Recipe:   "copy",
		BuiltAt:  "2026-06-11T00:00:00Z",
	})
	require.NoError(t, c.Save(root))

	got, err := LoadCache(root)
	require.NoError(t, err)
	require.Len(t, got.Entries, 1)
	e := got.Entries[0]
	assert.Equal(t, "sha256-deadbeef", e.ActionID)
	assert.Equal(t, "copy", e.Recipe)
	require.Len(t, e.Outputs, 1)
	assert.Equal(t, "out.txt", e.Outputs[0].Path)
	assert.Equal(t, "sha256-abc", e.Outputs[0].Hash)

	// File lives at .mdsmith/build-cache.json.
	_, statErr := os.Stat(filepath.Join(root, ".mdsmith", "build-cache.json"))
	assert.NoError(t, statErr)
}

func TestCache_LookupBySortedOutputSet(t *testing.T) {
	c := NewCache()
	c.Put(CacheEntry{
		Outputs:  []OutputHash{{Path: "b.txt", Hash: "h2"}, {Path: "a.txt", Hash: "h1"}},
		ActionID: "sha256-id",
	})
	// Lookup with a differently-ordered output set must still match.
	e, ok := c.Lookup([]string{"a.txt", "b.txt"})
	require.True(t, ok)
	assert.Equal(t, "sha256-id", e.ActionID)

	_, ok = c.Lookup([]string{"a.txt"})
	assert.False(t, ok)
}

func TestCache_PutReplacesByOutputSet(t *testing.T) {
	c := NewCache()
	c.Put(CacheEntry{
		Outputs:  []OutputHash{{Path: "a.txt", Hash: "h1"}},
		ActionID: "sha256-old",
	})
	c.Put(CacheEntry{
		Outputs:  []OutputHash{{Path: "a.txt", Hash: "h2"}},
		ActionID: "sha256-new",
	})
	assert.Len(t, c.Entries, 1)
	e, ok := c.Lookup([]string{"a.txt"})
	require.True(t, ok)
	assert.Equal(t, "sha256-new", e.ActionID)
}

func TestLoadCache_CorruptFileReturnsError(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".mdsmith"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, ".mdsmith", "build-cache.json"), []byte("{not json"), 0o644))
	_, err := LoadCache(root)
	require.Error(t, err)
}

func TestCache_SaveIsAtomic_NoTempLeftBehind(t *testing.T) {
	root := t.TempDir()
	c := NewCache()
	c.Put(CacheEntry{Outputs: []OutputHash{{Path: "a", Hash: "h"}}, ActionID: "id"})
	require.NoError(t, c.Save(root))

	entries, err := os.ReadDir(filepath.Join(root, ".mdsmith"))
	require.NoError(t, err)
	for _, e := range entries {
		assert.Equal(t, "build-cache.json", e.Name(), "no temp file should remain")
	}
}

func TestLoadCache_VersionZeroNormalized(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".mdsmith"), 0o755))
	// Write a cache JSON with version: 0 — should be normalised to CacheVersion on load.
	raw := `{"version":0,"entries":[]}`
	require.NoError(t, os.WriteFile(
		filepath.Join(root, ".mdsmith", "build-cache.json"), []byte(raw), 0o644))
	c, err := LoadCache(root)
	require.NoError(t, err)
	assert.Equal(t, CacheVersion, c.Version)
}

func TestLoadCache_DotMdsmithIsFile(t *testing.T) {
	root := t.TempDir()
	// .mdsmith as a regular file makes ReadFile on .mdsmith/build-cache.json
	// fail with ENOTDIR — a non-ENOENT error that hits the "reading build
	// cache" error path regardless of whether the process is root.
	require.NoError(t, os.WriteFile(filepath.Join(root, ".mdsmith"), []byte("file"), 0o644))
	_, err := LoadCache(root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading build cache")
}

func TestLoadCache_UnreadableFile(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root ignores file permissions")
	}
	root := t.TempDir()
	dir := filepath.Join(root, ".mdsmith")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	p := filepath.Join(dir, "build-cache.json")
	require.NoError(t, os.WriteFile(p, []byte(`{"version":1}`), 0o000))
	t.Cleanup(func() { _ = os.Chmod(p, 0o644) })

	_, err := LoadCache(root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading build cache")
}

func TestCache_Save_UnwritableDir(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	root := t.TempDir()
	dir := filepath.Join(root, ".mdsmith")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	c := NewCache()
	err := c.Save(root)
	require.Error(t, err)
}

func TestCache_Save_MkdirAllError(t *testing.T) {
	root := t.TempDir()
	// .mdsmith as a regular file: MkdirAll(".mdsmith") fails with ENOTDIR
	// regardless of whether the process runs as root.
	require.NoError(t, os.WriteFile(filepath.Join(root, ".mdsmith"), []byte("file"), 0o644))
	c := NewCache()
	err := c.Save(root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "creating .mdsmith dir")
}

func TestCache_Save_RenameError(t *testing.T) {
	root := t.TempDir()
	mdsmithDir := filepath.Join(root, ".mdsmith")
	require.NoError(t, os.MkdirAll(mdsmithDir, 0o755))
	// build-cache.json as a directory: os.Rename(tmpFile → dir) fails with
	// EISDIR on POSIX, which hits the "committing build cache" error path.
	require.NoError(t, os.MkdirAll(filepath.Join(mdsmithDir, "build-cache.json"), 0o755))
	c := NewCache()
	err := c.Save(root)
	require.Error(t, err)
}

func TestCache_Save_VersionZeroNormalized(t *testing.T) {
	// Ensure the c.Version == 0 branch in Save normalises to CacheVersion.
	root := t.TempDir()
	c := &Cache{Version: 0}
	c.Put(CacheEntry{Outputs: []OutputHash{{Path: "a.txt", Hash: "h"}}, ActionID: "id"})
	require.NoError(t, c.Save(root))
	got, err := LoadCache(root)
	require.NoError(t, err)
	assert.Equal(t, CacheVersion, got.Version)
}

// --- writeTempFile error paths ---

type writeFailCloser struct{}

func (w *writeFailCloser) Write([]byte) (int, error) { return 0, errors.New("disk full") }
func (w *writeFailCloser) Close() error              { return nil }

type closeFailWriter struct{}

func (w *closeFailWriter) Write(p []byte) (int, error) { return len(p), nil }
func (w *closeFailWriter) Close() error                { return errors.New("flush failed") }

func TestWriteTempFile_WriteError(t *testing.T) {
	err := writeTempFile(&writeFailCloser{}, []byte("data"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "writing temp file")
}

func TestWriteTempFile_CloseError(t *testing.T) {
	err := writeTempFile(&closeFailWriter{}, []byte("data"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "closing temp file")
}

func TestCache_Save_SortOrder(t *testing.T) {
	root := t.TempDir()
	c := NewCache()
	c.Put(CacheEntry{Outputs: []OutputHash{{Path: "z.txt", Hash: "h1"}}, ActionID: "id-z"})
	c.Put(CacheEntry{Outputs: []OutputHash{{Path: "a.txt", Hash: "h2"}}, ActionID: "id-a"})
	require.NoError(t, c.Save(root))

	got, err := LoadCache(root)
	require.NoError(t, err)
	require.Len(t, got.Entries, 2)
	assert.Equal(t, "a.txt", got.Entries[0].Outputs[0].Path)
	assert.Equal(t, "z.txt", got.Entries[1].Outputs[0].Path)
}

func TestCache_Save_WriteTempFileError(t *testing.T) {
	old := writeTempFileVar
	writeTempFileVar = func(_ io.WriteCloser, _ []byte) error {
		return errors.New("injected write error")
	}
	defer func() { writeTempFileVar = old }()

	root := t.TempDir()
	c := NewCache()
	err := c.Save(root)
	require.Error(t, err)
}

func TestAtomicWriteFile_CreateTempError(t *testing.T) {
	// The temp file is created in the destination's parent dir; a parent that
	// does not exist makes os.CreateTemp fail.
	final := filepath.Join(t.TempDir(), "nonexistent", "file.txt")
	err := atomicWriteFile(final, 0o644, []byte("data"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "creating temp file")
}

func TestAtomicWriteFile_ChmodError(t *testing.T) {
	old := chmodFileFn
	chmodFileFn = func(*os.File, os.FileMode) error { return errors.New("chmod failed") }
	t.Cleanup(func() { chmodFileFn = old })

	final := filepath.Join(t.TempDir(), "file.txt")
	err := atomicWriteFile(final, 0o644, []byte("data"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "setting temp file mode")
}
