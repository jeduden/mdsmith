//go:build !tinygo

package oscompat_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/oscompat"
)

func TestChmod_SetsPermissions(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "chmod-test")
	require.NoError(t, err)
	f.Close()
	require.NoError(t, oscompat.Chmod(f.Name(), 0o600))
	fi, err := os.Stat(f.Name())
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), fi.Mode().Perm())
}

func TestChmod_NonexistentFile_ReturnsError(t *testing.T) {
	err := oscompat.Chmod(filepath.Join(t.TempDir(), "no-such-file"), 0o600)
	assert.Error(t, err)
}

func TestEvalSymlinks_ResolvesSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	require.NoError(t, os.WriteFile(target, nil, 0o600))
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}
	got, err := oscompat.EvalSymlinks(link)
	require.NoError(t, err)
	// Resolve both sides: on macOS t.TempDir() may return a path with a /var
	// symlink component that EvalSymlinks resolves to /private/var.
	wantResolved, err := filepath.EvalSymlinks(target)
	require.NoError(t, err)
	assert.Equal(t, wantResolved, got)
}

func TestEvalSymlinks_NonexistentPath_ReturnsError(t *testing.T) {
	_, err := oscompat.EvalSymlinks(filepath.Join(t.TempDir(), "missing"))
	assert.Error(t, err)
}

func TestSameFile_SameFile_ReturnsTrue(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "samefile")
	require.NoError(t, err)
	f.Close()
	fi1, err := os.Stat(f.Name())
	require.NoError(t, err)
	fi2, err := os.Stat(f.Name())
	require.NoError(t, err)
	assert.True(t, oscompat.SameFile(fi1, fi2), "SameFile on same path returned false")
}

func TestSameFile_DifferentFiles_ReturnsFalse(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a")
	b := filepath.Join(dir, "b")
	require.NoError(t, os.WriteFile(a, nil, 0o600))
	require.NoError(t, os.WriteFile(b, nil, 0o600))
	fi1, err := os.Stat(a)
	require.NoError(t, err)
	fi2, err := os.Stat(b)
	require.NoError(t, err)
	assert.False(t, oscompat.SameFile(fi1, fi2), "SameFile on different files returned true")
}
