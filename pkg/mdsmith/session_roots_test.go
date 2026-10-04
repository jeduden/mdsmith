//go:build !tinygo

package mdsmith

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/lint/rootfstest"
)

// writeRootsTree writes a.md linking to b.md under a fresh root.
func writeRootsTree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.md"), []byte("# B\n"), 0o644))
	return dir
}

const rootsSrc = "# A\n\nSee [b](b.md).\n"

// TestSessionLendsOneRootAndClosesItOnDispose locks that a Session over
// an OSWorkspace opens one disk root, used as both its source view and
// its lent RootFS, lends it to every Check
// and CheckVersion (whose parse cache keeps each File, and its root,
// past the call), and closes them in Dispose. Not parallel: it records
// lint.OpenRootFS.
func TestSessionLendsOneRootAndClosesItOnDispose(t *testing.T) {
	dir := writeRootsTree(t)
	opened := rootfstest.Record(t)
	s, err := NewSession(SessionOptions{Workspace: OSWorkspace{Root: dir}, Config: ConfigYAML("")})
	require.NoError(t, err)

	for v := 1; v <= 3; v++ {
		res := s.CheckVersion("a.md", []byte(rootsSrc), v)
		require.Empty(t, res.Errors)
	}
	_, err = s.Check("c.md", []byte(rootsSrc))
	require.NoError(t, err)

	got := opened()
	assert.Len(t, got, 1, "one handle serves as both the lent root and the source view")
	for _, r := range got {
		_, err := fs.Stat(r, "b.md")
		assert.NoError(t, err, "a lent root stays open while the session lives")
	}
	s.Dispose()
	s.Dispose()
	for _, r := range got {
		_, err := fs.Stat(r, "b.md")
		assert.Error(t, err, "Dispose closes every root the session lent")
	}
}

// TestSessionCheckVersionAfterDisposeReadsDisk locks that Dispose drops
// the parse-cache entries that hold the roots it closes, so a
// CheckVersion at an already-cached version still reads [b](b.md) from
// disk, and that every root a call after Dispose opens (its parse, its
// source view, a Fix) is closed when that call ends. Not parallel: it
// records lint.OpenRootFS.
func TestSessionCheckVersionAfterDisposeReadsDisk(t *testing.T) {
	dir := writeRootsTree(t)
	s, err := NewSession(SessionOptions{Workspace: OSWorkspace{Root: dir}, Config: ConfigYAML("")})
	require.NoError(t, err)
	require.Empty(t, s.CheckVersion("a.md", []byte(rootsSrc), 1).Diagnostics)
	s.Dispose()

	opened := rootfstest.Record(t)
	for v := 1; v <= 2; v++ {
		res := s.CheckVersion("a.md", []byte(rootsSrc), v)
		require.Empty(t, res.Errors)
		assert.Empty(t, res.Diagnostics, "version %d reads b.md from disk", v)
	}
	_, err = s.Fix("a.md", []byte(rootsSrc))
	require.NoError(t, err)
	got := opened()
	require.NotEmpty(t, got)
	for i, r := range got {
		_, err := fs.Stat(r, ".")
		assert.Error(t, err, "root %d a call after Dispose opens is closed when it ends", i)
	}
}

// TestSessionRetriesARootThatFailedToOpen locks that a disk root the
// session cannot open is not kept for its lifetime: once the directory
// exists, the next call opens it and resolves [b](b.md).
func TestSessionRetriesARootThatFailedToOpen(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ws")
	s, err := NewSession(SessionOptions{Workspace: OSWorkspace{Root: dir}, Config: ConfigYAML("")})
	require.NoError(t, err)
	t.Cleanup(s.Dispose)
	require.NotEmpty(t, s.CheckVersion("a.md", []byte(rootsSrc), 1).Diagnostics,
		"b.md is unreadable while the root is missing")

	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.md"), []byte("# B\n"), 0o644))
	assert.Empty(t, s.CheckVersion("a.md", []byte(rootsSrc), 2).Diagnostics)
}

// TestSessionCheckAfterDisposeReadsDisk locks that a Check racing or
// following Dispose still reads cross-file targets from disk rather
// than through the roots Dispose closed: [b](b.md) resolves, so no
// broken-link diagnostic appears.
func TestSessionCheckAfterDisposeReadsDisk(t *testing.T) {
	dir := writeRootsTree(t)
	s, err := NewSession(SessionOptions{Workspace: OSWorkspace{Root: dir}, Config: ConfigYAML("")})
	require.NoError(t, err)
	_, err = s.Check("a.md", []byte(rootsSrc))
	require.NoError(t, err)
	s.Dispose()

	diags, err := s.Check("c.md", []byte(rootsSrc))
	require.NoError(t, err)
	assert.Empty(t, diags)
}
