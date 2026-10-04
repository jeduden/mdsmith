package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/linkgraph"
	mdsmith "github.com/jeduden/mdsmith/pkg/mdsmith"
)

// writeWikilinkTree writes files (workspace-relative path → body) under
// a fresh temp root and returns it.
func writeWikilinkTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, body := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	}
	return root
}

// countWikilinkWalks swaps s's fresh-walk seam for one that counts its
// calls before walking, and returns the counter.
func countWikilinkWalks(s *Server) *atomic.Int32 {
	var n atomic.Int32
	s.walkWikilinks = func(root string) *linkgraph.WikilinkIndex {
		n.Add(1)
		return linkgraph.WikilinkIndexAtDir(root)
	}
	return &n
}

// moveStem plans a willRenameFiles batch moving rel to dst under root,
// through the workspace the handler builds, and returns its edits'
// texts keyed by workspace-relative path.
func moveStem(t *testing.T, s *Server, root, rel, dst string) map[string][]string {
	t.Helper()
	batch := planRenameBatch(s.moveWorkspace(root), root, []fileRename{{
		OldURI: pathToURI(filepath.Join(root, rel)),
		NewURI: pathToURI(filepath.Join(root, dst)),
	}})
	out := map[string][]string{}
	for key, edits := range batch.Edits {
		out[workspaceRelative(root, uriToPath(key))] = editTexts(edits)
	}
	return out
}

// TestWillRenameReusesCachedWikilinkIndexWhenWatching locks that a move
// batch with a `[[stem]]` edge reads the session's warm wikilink index
// once the server watches files, rather than walking the root on the
// request goroutine. Registration and a received watched-file event
// each count as watching.
func TestWillRenameReusesCachedWikilinkIndexWhenWatching(t *testing.T) {
	t.Parallel()
	for name, watch := range map[string]func(*Server){
		"watcher registration": func(s *Server) { s.registerWatchers() },
		"watched-file event": func(s *Server) {
			raw, err := json.Marshal(didChangeWatchedFilesParams{Changes: []fileEvent{
				{URI: "file:///elsewhere/x.md", Type: fileChangeChanged},
			}})
			require.NoError(t, err)
			s.handleDidChangeWatchedFiles(context.Background(), raw)
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := writeWikilinkTree(t, map[string]string{
				"api.md": "# API\n", "guide.md": "See [[api]].\n",
			})
			s := New(Options{Writer: io.Discard})
			s.rootDir = root
			walks := countWikilinkWalks(s)
			watch(s)
			sess, _ := s.currentSession()
			require.NotNil(t, sess)
			require.NotNil(t, sess.WikilinkIndex(), "warm the session's index")

			got := moveStem(t, s, root, "api.md", "service.md")
			assert.Equal(t, []string{"service"}, got["guide.md"])
			assert.Zero(t, walks.Load(), "a watched move reads the cached index")
		})
	}
}

// TestWillRenameWalksFreshWithoutWatching locks the fallback: with no
// file-watch registration the cached index may be stale, so the move
// walks the root fresh, once, and counts a gitignored same-stem file
// (archive/guide.md sorts before docs/guide.md), leaving `[[guide]]` as
// written.
func TestWillRenameWalksFreshWithoutWatching(t *testing.T) {
	t.Parallel()
	root := writeWikilinkTree(t, map[string]string{
		".gitignore":       "archive/\n",
		"archive/guide.md": "# Old\n",
		"docs/guide.md":    "# Guide\n",
		"c.md":             "See [[guide]] here.\n",
	})
	s := New(Options{Writer: io.Discard})
	s.rootDir = root
	walks := countWikilinkWalks(s)

	got := moveStem(t, s, root, "docs/guide.md", "docs/manual.md")
	assert.NotContains(t, got, "c.md")
	assert.Equal(t, int32(1), walks.Load())
}

// TestMoveWikilinkIndexWalksWhenSessionMissing locks that a watching
// server with no session (its constructor failed) still walks root for
// the move's index rather than reading through a nil session.
func TestMoveWikilinkIndexWalksWhenSessionMissing(t *testing.T) {
	t.Parallel()
	root := writeWikilinkTree(t, map[string]string{"guide.md": "# G\n"})
	s := New(Options{Writer: io.Discard})
	s.rootDir = root
	s.newSession = func(mdsmith.SessionOptions) (*mdsmith.Session, error) {
		return nil, errors.New("boom")
	}
	walks := countWikilinkWalks(s)
	s.registerWatchers()

	idx := s.moveWikilinkIndex(root)
	require.NotNil(t, idx)
	assert.Equal(t, []string{"guide.md"}, idx.StemPaths("guide"))
	assert.Equal(t, int32(1), walks.Load())
}

// TestMoveWikilinkIndexWalksWhenSessionRootDiffers locks that a watching
// server reads the session's cached index only for the root the session
// was built at. A move spelled against another root (a config reload
// moved it between the request's snapshot and the read) walks that root
// fresh rather than reading an index keyed to the old directory.
func TestMoveWikilinkIndexWalksWhenSessionRootDiffers(t *testing.T) {
	t.Parallel()
	oldRoot := writeWikilinkTree(t, map[string]string{"old.md": "# Old\n"})
	newRoot := writeWikilinkTree(t, map[string]string{"guide.md": "# G\n"})
	s := New(Options{Writer: io.Discard})
	s.rootDir = oldRoot
	walks := countWikilinkWalks(s)
	s.registerWatchers()
	sess, _ := s.currentSession()
	require.NotNil(t, sess)
	require.NotNil(t, sess.WikilinkIndex(), "warm the old root's index")

	idx := s.moveWikilinkIndex(newRoot)
	require.NotNil(t, idx)
	assert.Equal(t, []string{"guide.md"}, idx.StemPaths("guide"))
	assert.Empty(t, idx.StemPaths("old"))
	assert.Equal(t, int32(1), walks.Load())
}

// TestDidRenameFilesDropsCachedWikilinkIndex locks that an editor rename
// drops the session's cached wikilink index at once, so a move planned
// before the watcher's delete and create events arrive does not read the
// pre-rename file set.
func TestDidRenameFilesDropsCachedWikilinkIndex(t *testing.T) {
	t.Parallel()
	root := writeWikilinkTree(t, map[string]string{"api.md": "# API\n"})
	s := New(Options{Writer: io.Discard})
	s.rootDir = root
	s.registerWatchers()
	sess, _ := s.currentSession()
	require.NotNil(t, sess)
	require.Equal(t, []string{"api.md"}, sess.WikilinkIndex().StemPaths("api"))

	oldPath, newPath := filepath.Join(root, "api.md"), filepath.Join(root, "service.md")
	require.NoError(t, os.Rename(oldPath, newPath))
	raw, err := json.Marshal(renameFilesParams{Files: []fileRename{{
		OldURI: pathToURI(oldPath), NewURI: pathToURI(newPath),
	}}})
	require.NoError(t, err)
	s.handleDidRenameFiles(raw)

	idx := s.moveWikilinkIndex(root)
	require.NotNil(t, idx)
	assert.Empty(t, idx.StemPaths("api"))
	assert.Equal(t, []string{"service.md"}, idx.StemPaths("service"))
}

// TestDidRenameFilesWithoutSession locks that a rename notification on a
// server whose session constructor failed skips the wikilink drop rather
// than reading through a nil session.
func TestDidRenameFilesWithoutSession(t *testing.T) {
	t.Parallel()
	s := New(Options{Writer: io.Discard})
	s.rootDir = t.TempDir()
	s.newSession = func(mdsmith.SessionOptions) (*mdsmith.Session, error) {
		return nil, errors.New("boom")
	}
	raw, err := json.Marshal(renameFilesParams{})
	require.NoError(t, err)
	assert.NotPanics(t, func() { s.handleDidRenameFiles(raw) })
}

// TestConfigOnlyWatchedEventKeepsFreshWalk locks that a watched-file
// batch reporting only `.mdsmith.yml` does not make a move trust the
// cached index: a client that syncs just the config file (as the VS
// Code extension's static watcher does) never reports a Markdown create
// or delete, so the move keeps walking fresh.
func TestConfigOnlyWatchedEventKeepsFreshWalk(t *testing.T) {
	t.Parallel()
	root := writeWikilinkTree(t, map[string]string{"guide.md": "# G\n"})
	s := New(Options{Writer: io.Discard})
	s.rootDir = root
	walks := countWikilinkWalks(s)
	raw, err := json.Marshal(didChangeWatchedFilesParams{Changes: []fileEvent{
		{URI: pathToURI(filepath.Join(root, ".mdsmith.yml")), Type: fileChangeChanged},
	}})
	require.NoError(t, err)
	s.handleDidChangeWatchedFiles(context.Background(), raw)

	require.NotNil(t, s.moveWikilinkIndex(root))
	assert.Equal(t, int32(1), walks.Load(), "a config-only batch proves no Markdown watch")
}
