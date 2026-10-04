package lsp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/linkgraph"
	"github.com/jeduden/mdsmith/internal/testsymlink"
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

// ackWatchers registers s's file watchers and answers the registration
// request as a client that accepted it, then waits for the server to
// record the acknowledgement.
func ackWatchers(t *testing.T, s *Server) {
	t.Helper()
	s.registerWatchers(context.Background())
	id, err := json.Marshal(s.nextReqID.Load())
	require.NoError(t, err)
	s.deliverResponse(string(id), rpcResponse{Result: json.RawMessage("null")})
	require.Eventually(t, s.watchingFiles.Load, testPollDeadline, time.Millisecond)
}

// TestWillRenameReusesCachedWikilinkIndexWhenWatching locks that a move
// batch with a `[[stem]]` edge reads the session's warm wikilink index
// once the client has accepted the watcher registration, rather than
// walking the root on the request goroutine.
func TestWillRenameReusesCachedWikilinkIndexWhenWatching(t *testing.T) {
	t.Parallel()
	root := writeWikilinkTree(t, map[string]string{
		"api.md": "# API\n", "guide.md": "See [[api]].\n",
	})
	s := New(Options{Writer: io.Discard})
	s.rootDir = root
	walks := countWikilinkWalks(s)
	ackWatchers(t, s)
	sess, _ := s.currentSession()
	require.NotNil(t, sess)
	require.NotNil(t, sess.WikilinkIndex(), "warm the session's index")

	got := moveStem(t, s, root, "api.md", "service.md")
	assert.Equal(t, []string{"service"}, got["guide.md"])
	assert.Zero(t, walks.Load(), "a watched move reads the cached index")
}

// TestWatcherAckDropsIndexBuiltBeforeWatch locks that the session's
// cached wikilink index is dropped when the client accepts the watcher
// registration: an index a lint built before the watch began may miss a
// file created in between, and no event will ever report that create,
// so a move must not trust it.
func TestWatcherAckDropsIndexBuiltBeforeWatch(t *testing.T) {
	t.Parallel()
	root := writeWikilinkTree(t, map[string]string{"guide.md": "# G\n"})
	s := New(Options{Writer: io.Discard})
	s.rootDir = root
	sess, _ := s.currentSession()
	require.NotNil(t, sess)
	require.Empty(t, sess.WikilinkIndex().StemPaths("api"), "warm the index before the watch")
	require.NoError(t, os.WriteFile(filepath.Join(root, "api.md"), []byte("# API\n"), 0o644))

	ackWatchers(t, s)
	assert.Equal(t, []string{"api.md"}, s.moveWikilinkIndex(root).StemPaths("api"))
}

// TestDropPreWatchWikilinksWithoutSession locks that an accepted watch
// on a server with no session yet builds none and drops nothing.
func TestDropPreWatchWikilinksWithoutSession(t *testing.T) {
	t.Parallel()
	s := New(Options{Writer: io.Discard})
	assert.NotPanics(t, s.dropPreWatchWikilinks)
	s.sessionMu.RLock()
	defer s.sessionMu.RUnlock()
	assert.Nil(t, s.session, "dropping builds no session")
}

// TestRegisterWatchersTrustsOnlyAcceptedRegistration locks that sending
// the registration request alone does not make a move trust the cached
// index: a client may reject it, or never answer. Only a success reply
// does; an error reply leaves the move walking fresh.
func TestRegisterWatchersTrustsOnlyAcceptedRegistration(t *testing.T) {
	t.Parallel()
	s := New(Options{Writer: io.Discard})
	s.registerWatchers(context.Background())
	assert.False(t, s.watchingFiles.Load(), "an unanswered registration is not a watch")

	id, err := json.Marshal(s.nextReqID.Load())
	require.NoError(t, err)
	s.deliverResponse(string(id), rpcResponse{Error: &responseError{Code: -32601, Message: "no"}})
	assert.Never(t, s.watchingFiles.Load, 50*time.Millisecond, time.Millisecond,
		"a rejected registration is not a watch")
}

// TestWatchedEventAloneKeepsFreshWalk locks that a watched-file event
// does not make a move trust the cached index. A client may sync a
// narrower glob statically (`**/*.md`, case-sensitive) that never
// reports an image or a `.MD` create, so only an accepted `**/*`
// registration proves every file-set change reaches the server.
func TestWatchedEventAloneKeepsFreshWalk(t *testing.T) {
	t.Parallel()
	root := writeWikilinkTree(t, map[string]string{"guide.md": "# G\n"})
	s := New(Options{Writer: io.Discard})
	s.rootDir = root
	walks := countWikilinkWalks(s)
	for _, typ := range []int{fileChangeChanged, fileChangeCreated} {
		raw, err := json.Marshal(didChangeWatchedFilesParams{Changes: []fileEvent{
			{URI: pathToURI(filepath.Join(root, "x.md")), Type: typ},
		}})
		require.NoError(t, err)
		s.handleDidChangeWatchedFiles(context.Background(), raw)
	}

	require.NotNil(t, s.moveWikilinkIndex(root))
	assert.Equal(t, int32(1), walks.Load(), "an event alone proves no full watch")
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
	ackWatchers(t, s)

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
	ackWatchers(t, s)
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
	ackWatchers(t, s)
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

// TestRegisterWatchersReportsEveryCreateAndDelete locks that the watch
// covers the wikilink index's whole file set: the index keys every file
// (images and `.MD` spellings included), so a create or delete of any
// file must reach InvalidateWikilinks, not only a `*.md` one.
func TestRegisterWatchersReportsEveryCreateAndDelete(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	s := New(Options{Writer: &buf})
	s.registerWatchers(context.Background())
	out := buf.String()
	body := out[strings.Index(out, "{"):]
	var msg struct {
		Params registrationParams `json:"params"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &msg))
	require.Len(t, msg.Params.Registrations, 1)
	var opts struct {
		Watchers []fileSystemWatcher `json:"watchers"`
	}
	raw, err := json.Marshal(msg.Params.Registrations[0].RegisterOptions)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &opts))
	assert.Contains(t, opts.Watchers, fileSystemWatcher{
		GlobPattern: "**/*", Kind: watchKindCreate | watchKindDelete,
	})
}

// TestMoveWikilinkIndexWalksWhenRootOutsideWorkspace locks that a move
// at a session root outside the watched workspace folder (mdsmith.config
// pointing elsewhere) walks fresh: the client reports changes only under
// the folder it watches, so the cached index there may be stale.
func TestMoveWikilinkIndexWalksWhenRootOutsideWorkspace(t *testing.T) {
	t.Parallel()
	folder := t.TempDir()
	outside := writeWikilinkTree(t, map[string]string{"guide.md": "# G\n"})
	s := New(Options{Writer: io.Discard})
	s.rootDir = folder
	walks := countWikilinkWalks(s)
	ackWatchers(t, s)
	cfg, _, _ := s.resolveConfig("")
	s.rebuildSession(cfg, filepath.Join(outside, ".mdsmith.yml"))
	require.NotNil(t, s.sessionAt(outside))

	require.NotNil(t, s.moveWikilinkIndex(outside))
	assert.Equal(t, int32(1), walks.Load())
}

// TestMoveWikilinkIndexReadsCacheUnderWorkspace locks that a session
// root nested inside the watched folder still reads the cached index.
func TestMoveWikilinkIndexReadsCacheUnderWorkspace(t *testing.T) {
	t.Parallel()
	folder := writeWikilinkTree(t, map[string]string{"docs/guide.md": "# G\n"})
	nested := filepath.Join(folder, "docs")
	s := New(Options{Writer: io.Discard})
	s.rootDir = folder
	walks := countWikilinkWalks(s)
	ackWatchers(t, s)
	cfg, _, _ := s.resolveConfig("")
	s.rebuildSession(cfg, filepath.Join(nested, ".mdsmith.yml"))

	require.NotNil(t, s.moveWikilinkIndex(nested))
	assert.Zero(t, walks.Load())
}

// TestWatchesRoot locks which move roots lie inside the watched folder.
func TestWatchesRoot(t *testing.T) {
	t.Parallel()
	folder := filepath.Join(t.TempDir(), "ws")
	cases := map[string]struct {
		folder, root string
		want         bool
	}{
		"the folder itself":    {folder, folder, true},
		"nested under it":      {folder, filepath.Join(folder, "docs"), true},
		"its parent":           {folder, filepath.Dir(folder), false},
		"a sibling":            {folder, folder + "2", false},
		"a dotdot-named child": {folder, filepath.Join(folder, "..x"), true},
		"no workspace folder":  {"", folder, false},
		"no root":              {folder, "", false},
		"relative against abs": {folder, "rel", false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := New(Options{Writer: io.Discard})
			s.rootDir = tc.folder
			assert.Equal(t, tc.want, s.watchesRoot(tc.root))
		})
	}
}

// TestWatchesRootResolvesSymlinks locks that a move root spelled under
// the watched folder through a symlink to a directory outside it is not
// watched: the client's recursive watcher does not follow the link, so
// a create or delete behind it never reaches the server.
func TestWatchesRootResolvesSymlinks(t *testing.T) {
	t.Parallel()
	testsymlink.SkipIfSymlinkUnsupported(t)
	folder := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(folder, "docs")
	require.NoError(t, os.Symlink(outside, link))
	s := New(Options{Writer: io.Discard})
	s.rootDir = folder
	assert.False(t, s.watchesRoot(link))
}

// TestRegisterWatchersWriteFailureLeavesNoPendingReply locks that a
// registration the transport failed to send drops its pending-reply
// slot at once rather than leaving it, and a waiter, behind.
func TestRegisterWatchersWriteFailureLeavesNoPendingReply(t *testing.T) {
	t.Parallel()
	s := New(Options{Writer: failingWriter{}})
	s.registerWatchers(context.Background())
	s.pendingRespMu.Lock()
	n := len(s.pendingResp)
	s.pendingRespMu.Unlock()
	assert.Zero(t, n)
	assert.False(t, s.watchingFiles.Load())
}

// pendingReplies returns how many server requests still wait on a reply.
func pendingReplies(s *Server) int {
	s.pendingRespMu.Lock()
	defer s.pendingRespMu.Unlock()
	return len(s.pendingResp)
}

// TestRegisterWatchersUnansweredTimesOut locks that a client which
// never answers the registration leaves the server not watching, and
// the waiter gives up after fetchTimeout and drops its pending slot.
func TestRegisterWatchersUnansweredTimesOut(t *testing.T) {
	t.Parallel()
	s := New(Options{Writer: io.Discard})
	s.fetchTimeout = time.Millisecond
	s.registerWatchers(context.Background())
	require.Eventually(t, func() bool { return pendingReplies(s) == 0 },
		testPollDeadline, time.Millisecond)
	assert.False(t, s.watchingFiles.Load())
}

// TestRegisterWatchersStopsOnContextDone locks that the waiter exits
// and drops its pending slot when the server's context ends before the
// client answers, without marking the server as watching.
func TestRegisterWatchersStopsOnContextDone(t *testing.T) {
	t.Parallel()
	s := New(Options{Writer: io.Discard})
	s.fetchTimeout = time.Hour
	ctx, cancel := context.WithCancel(context.Background())
	s.registerWatchers(ctx)
	cancel()
	require.Eventually(t, func() bool { return pendingReplies(s) == 0 },
		testPollDeadline, time.Millisecond)
	assert.False(t, s.watchingFiles.Load())
}
