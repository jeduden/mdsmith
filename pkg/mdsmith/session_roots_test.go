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
// an OSWorkspace opens its disk roots once, lends them to every Check
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
	assert.Len(t, got, 2, "one lent project root and one source view")
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
