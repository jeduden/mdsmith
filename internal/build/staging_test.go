package build

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsureStagingRoot_MkdirAllError(t *testing.T) {
	old := osMkdirAllFn
	osMkdirAllFn = func(string, os.FileMode) error { return errors.New("mkdir failed") }
	t.Cleanup(func() { osMkdirAllFn = old })

	_, err := ensureStagingRoot(t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "creating staging root")
}

func TestEnsureStagingRoot_ChmodError(t *testing.T) {
	old := osChmodFn
	osChmodFn = func(string, os.FileMode) error { return errors.New("chmod failed") }
	t.Cleanup(func() { osChmodFn = old })

	_, err := ensureStagingRoot(t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "securing staging root")
}

func TestEnsureStagingRoot_CreatesWith0700(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits")
	}
	root := t.TempDir()
	dir, err := ensureStagingRoot(root)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, ".mdsmith", "build-staging"), dir)

	info, err := os.Lstat(dir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
}

func TestEnsureStagingRoot_RefusesSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics")
	}
	root := t.TempDir()
	target := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".mdsmith"), 0o700))
	require.NoError(t, os.Symlink(target, filepath.Join(root, ".mdsmith", "build-staging")))

	_, err := ensureStagingRoot(root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "symlink")
}

func TestEnsureStagingRoot_RefusesNonDir(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".mdsmith"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".mdsmith", "build-staging"), []byte("x"), 0o600))

	_, err := ensureStagingRoot(root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a directory")
}

func TestEnsureStagingRoot_RefusesGroupWritable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits")
	}
	root := t.TempDir()
	staging := filepath.Join(root, ".mdsmith", "build-staging")
	require.NoError(t, os.MkdirAll(staging, 0o700))
	require.NoError(t, os.Chmod(staging, 0o770))

	_, err := ensureStagingRoot(root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "writable")
}

func TestEnsureStagingRoot_RefusesWorldWritable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits")
	}
	root := t.TempDir()
	staging := filepath.Join(root, ".mdsmith", "build-staging")
	require.NoError(t, os.MkdirAll(staging, 0o700))
	require.NoError(t, os.Chmod(staging, 0o707))

	_, err := ensureStagingRoot(root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "writable")
}

func TestEnsureStagingRoot_AcceptsExisting0700(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits")
	}
	root := t.TempDir()
	staging := filepath.Join(root, ".mdsmith", "build-staging")
	require.NoError(t, os.MkdirAll(staging, 0o700))

	dir, err := ensureStagingRoot(root)
	require.NoError(t, err)
	assert.Equal(t, staging, dir)
}
