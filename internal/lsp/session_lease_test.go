package lsp

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/lint/rootfstest"
)

// leaseSrc links to b.md, so a lint reads disk through both the
// session's lent root and its overlay's disk root.
const leaseSrc = "# A\n\nSee [b](b.md).\n"

// warmSessionRoots writes b.md under s's root and lints a buffer that
// links to it through the current session, so the session opens its
// disk roots. It returns the roots opened so far.
func warmSessionRoots(t *testing.T, s *Server, opened func() []lint.RootFS) []lint.RootFS {
	t.Helper()
	_, _, root := s.snapshotConfig()
	require.NoError(t, os.WriteFile(filepath.Join(root, "b.md"), []byte("# B\n"), 0o644))
	sess, release := s.currentSession()
	defer release()
	require.NotNil(t, sess)
	require.Empty(t, sess.CheckVersion("a.md", []byte(leaseSrc), 1).Diagnostics)
	_, err := sess.Check("c.md", []byte(leaseSrc))
	require.NoError(t, err)
	got := opened()
	require.NotEmpty(t, got)
	return got
}

// TestRebuildSessionRetiresSupersededSessionOnceReleased locks that a
// rebuild leaves a superseded session's disk roots (its lent root and
// its overlay's) open while a caller still holds it, and closes them
// once that caller releases it. Not parallel: it records
// lint.OpenRootFS.
func TestRebuildSessionRetiresSupersededSessionOnceReleased(t *testing.T) {
	opened := rootfstest.Record(t)
	s := serverWithSession(t)
	old := warmSessionRoots(t, s, opened)

	held, release := s.currentSession()
	cfg, cfgPath, _ := s.snapshotConfig()
	s.rebuildSession(cfg, cfgPath)
	for i, r := range old {
		_, err := fs.Stat(r, ".")
		assert.NoError(t, err, "root %d stays open while a caller holds the session", i)
	}
	require.Empty(t, held.CheckVersion("a.md", []byte(leaseSrc), 2).Diagnostics,
		"a held superseded session still reads disk")

	release()
	release() // a second release is a no-op
	for i, r := range old {
		_, err := fs.Stat(r, ".")
		assert.Error(t, err, "root %d is closed once the last holder releases", i)
	}
	cur, done := s.currentSession()
	defer done()
	require.NotSame(t, held, cur)
	assert.Empty(t, cur.CheckVersion("a.md", []byte(leaseSrc), 1).Diagnostics,
		"the new session reads disk through roots of its own")
}

// TestRebuildSessionRetiresIdleSessionAtOnce locks that a superseded
// session no caller holds is retired by the rebuild itself. Not
// parallel: it records lint.OpenRootFS.
func TestRebuildSessionRetiresIdleSessionAtOnce(t *testing.T) {
	opened := rootfstest.Record(t)
	s := serverWithSession(t)
	old := warmSessionRoots(t, s, opened)

	cfg, cfgPath, _ := s.snapshotConfig()
	s.rebuildSession(cfg, cfgPath)
	for i, r := range old {
		_, err := fs.Stat(r, ".")
		assert.Error(t, err, "root %d of the idle superseded session is closed", i)
	}
}

// TestSessionAtLeasesTheSession locks that sessionAt hands out a lease
// like currentSession: the session it returns survives a rebuild until
// released, and a root mismatch returns no session and a no-op release.
func TestSessionAtLeasesTheSession(t *testing.T) {
	opened := rootfstest.Record(t)
	s := serverWithSession(t)
	old := warmSessionRoots(t, s, opened)
	_, _, root := s.snapshotConfig()

	sess, release := s.sessionAt(root)
	require.NotNil(t, sess)
	cfg, cfgPath, _ := s.snapshotConfig()
	s.rebuildSession(cfg, cfgPath)
	_, err := fs.Stat(old[0], ".")
	assert.NoError(t, err, "a sessionAt lease keeps the superseded session open")
	release()
	_, err = fs.Stat(old[0], ".")
	assert.Error(t, err)

	none, noop := s.sessionAt(t.TempDir())
	assert.Nil(t, none)
	assert.NotPanics(t, noop)
}
