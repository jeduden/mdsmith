package lint_test

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadFileLimited_Normal(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "small.md")
	require.NoError(t, os.WriteFile(p, []byte("hello"), 0o644))

	data, err := lint.ReadFileLimited(p, 100)
	require.NoError(t, err)
	assert.Equal(t, []byte("hello"), data)
}

func TestReadFileLimited_AtLimit(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "exact.md")
	content := make([]byte, 50)
	for i := range content {
		content[i] = 'x'
	}
	require.NoError(t, os.WriteFile(p, content, 0o644))

	data, err := lint.ReadFileLimited(p, 50)
	require.NoError(t, err)
	assert.Equal(t, content, data)
}

func TestReadFileLimited_OverLimit(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "huge.md")
	content := make([]byte, 100)
	require.NoError(t, os.WriteFile(p, content, 0o644))

	_, err := lint.ReadFileLimited(p, 50)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "file too large")
	assert.Contains(t, err.Error(), "100 bytes")
	assert.Contains(t, err.Error(), "max 50")
}

func TestReadFileLimited_ZeroUnlimited(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "big.md")
	content := make([]byte, 10000)
	require.NoError(t, os.WriteFile(p, content, 0o644))

	data, err := lint.ReadFileLimited(p, 0)
	require.NoError(t, err)
	assert.Len(t, data, 10000)
}

func TestReadFileLimited_NegativeUnlimited(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "big.md")
	content := make([]byte, 10000)
	require.NoError(t, os.WriteFile(p, content, 0o644))

	data, err := lint.ReadFileLimited(p, -1)
	require.NoError(t, err)
	assert.Len(t, data, 10000)
}

func TestReadFileLimited_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "empty.md")
	require.NoError(t, os.WriteFile(p, nil, 0o644))

	data, err := lint.ReadFileLimited(p, 100)
	require.NoError(t, err)
	assert.Empty(t, data)
}

func TestReadFileLimited_NotFound(t *testing.T) {
	_, err := lint.ReadFileLimited("/nonexistent/file.md", 100)
	require.Error(t, err)
}

func TestReadFileLimited_MaxInt64Unlimited(t *testing.T) {
	// MaxInt64 must be treated as unlimited to avoid overflow in max+1.
	dir := t.TempDir()
	p := filepath.Join(dir, "file.md")
	require.NoError(t, os.WriteFile(p, []byte("hello"), 0o644))

	data, err := lint.ReadFileLimited(p, math.MaxInt64)
	require.NoError(t, err)
	assert.Equal(t, []byte("hello"), data)
}

func TestReadFSFileLimited_MaxInt64Unlimited(t *testing.T) {
	fsys := fstest.MapFS{
		"test.md": &fstest.MapFile{Data: []byte("hello")},
	}
	data, err := lint.ReadFSFileLimited(fsys, "test.md", math.MaxInt64)
	require.NoError(t, err)
	assert.Equal(t, []byte("hello"), data)
}

func TestReadFSFileLimited_Normal(t *testing.T) {
	fsys := fstest.MapFS{
		"test.md": &fstest.MapFile{Data: []byte("hello")},
	}
	data, err := lint.ReadFSFileLimited(fsys, "test.md", 100)
	require.NoError(t, err)
	assert.Equal(t, []byte("hello"), data)
}

func TestReadFSFileLimited_OverLimit(t *testing.T) {
	content := make([]byte, 100)
	fsys := fstest.MapFS{
		"big.md": &fstest.MapFile{Data: content},
	}
	_, err := lint.ReadFSFileLimited(fsys, "big.md", 50)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "file too large")
}

func TestReadFSFileLimited_ZeroUnlimited(t *testing.T) {
	content := make([]byte, 10000)
	fsys := fstest.MapFS{
		"big.md": &fstest.MapFile{Data: content},
	}
	data, err := lint.ReadFSFileLimited(fsys, "big.md", 0)
	require.NoError(t, err)
	assert.Len(t, data, 10000)
}

func TestReadFSFileLimited_AtLimit(t *testing.T) {
	content := make([]byte, 50)
	for i := range content {
		content[i] = 'a'
	}
	fsys := fstest.MapFS{
		"exact.md": &fstest.MapFile{Data: content},
	}
	data, err := lint.ReadFSFileLimited(fsys, "exact.md", 50)
	require.NoError(t, err)
	assert.Equal(t, content, data)
}
