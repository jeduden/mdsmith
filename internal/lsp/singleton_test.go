package lsp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/rule"
)

func TestWatchSingletonFiresWhenOwnerChanges(t *testing.T) {
	t.Parallel()
	var fired atomic.Bool
	watchSingleton(context.Background(), "k", "me", time.Millisecond,
		func(string) string { return "newer-instance" },
		func() { fired.Store(true) })
	assert.True(t, fired.Load(), "must step aside once a different owner claims the workspace")
}

func TestWatchSingletonStaysWhenOwnerMatches(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	var fired atomic.Bool
	watchSingleton(ctx, "k", "me", time.Millisecond,
		func(string) string { return "me" },
		func() { fired.Store(true) })
	assert.False(t, fired.Load(), "must not step aside while it is still the owner")
}

func TestWatchSingletonStaysWhenOwnerEmpty(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	var fired atomic.Bool
	watchSingleton(ctx, "k", "me", time.Millisecond,
		func(string) string { return "" },
		func() { fired.Store(true) })
	assert.False(t, fired.Load(), "an empty owner (unreadable registry) must not reap the last server")
}

func TestWatchSingletonStopsOnContextCancel(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var fired atomic.Bool
	watchSingleton(ctx, "k", "me", time.Hour,
		func(string) string { return "newer-instance" },
		func() { fired.Store(true) })
	assert.False(t, fired.Load(), "a canceled watcher must not fire onSuperseded")
}

func TestStartSingletonWatchNoopWithoutRoot(t *testing.T) {
	t.Parallel()
	s := New(Options{Reader: nil, Writer: io.Discard, Rules: rule.All()})
	s.instanceID = "me"
	s.singletonInterval = time.Millisecond
	s.singletonClaim = func(string, string) error {
		t.Error("must not claim a workspace when no root was provided")
		return nil
	}
	s.startSingletonWatch("", "scope")
	time.Sleep(20 * time.Millisecond)
}

// The empty-scope gate is distinct from the empty-root guard above: here
// the root, instance id, and registry seams are all present, so only the
// missing client opt-in keeps the server from claiming and watching.
func TestStartSingletonWatchNoopWithoutScope(t *testing.T) {
	t.Parallel()
	s := New(Options{Reader: nil, Writer: io.Discard, Rules: rule.All()})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s.runCtx = ctx
	s.instanceID = "me"
	s.singletonInterval = time.Millisecond
	s.singletonClaim = func(string, string) error {
		t.Error("must not claim a workspace when the client sent no singletonScope")
		return nil
	}
	s.singletonCurrent = func(string) string {
		t.Error("must not watch the registry when the client sent no singletonScope")
		return "newer-instance"
	}
	var exited atomic.Bool
	s.onSupersededExit = func() { exited.Store(true) }

	s.startSingletonWatch("/work/space", "")
	time.Sleep(20 * time.Millisecond)
	assert.False(t, exited.Load(), "a scope-less server must never be superseded")
}

// A root carrying a NUL byte (rootUri "file:///w%00scope" decodes to
// one) would make its legacy key sha256("/w\x00scope") equal the scoped
// key of root "/w" with scope "scope", so its legacy write would
// supersede that other server. Such a root opts out, like a NUL scope.
func TestStartSingletonWatchNoopWithNULRoot(t *testing.T) {
	t.Parallel()
	require.Equal(t, workspaceKey("/w", "scope"), workspaceKey("/w\x00scope", ""),
		"precondition: the NUL root's legacy key aliases another scoped key")
	s := New(Options{Reader: nil, Writer: io.Discard, Rules: rule.All()})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s.runCtx = ctx
	s.instanceID = "me"
	s.singletonInterval = time.Millisecond
	s.singletonClaim = func(key, _ string) error {
		t.Errorf("must not claim for a root containing a NUL byte (key %s)", key)
		return nil
	}
	s.singletonCurrent = func(string) string { return "me" }

	s.startSingletonWatch("/w\x00scope", "x")
	time.Sleep(20 * time.Millisecond)
}

// The first initialize decides singleton participation for the whole
// session: a scope-less (or root-less) first call must use up the Once,
// so a stray later initialize carrying a scope cannot start claiming
// mid-session.
func TestStartSingletonWatchFirstCallDecides(t *testing.T) {
	t.Parallel()
	for _, first := range []struct{ name, root, scope string }{
		{"no scope", "/work/space", ""},
		{"no root", "", "scope"},
	} {
		t.Run(first.name, func(t *testing.T) {
			t.Parallel()
			s := New(Options{Reader: nil, Writer: io.Discard, Rules: rule.All()})
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			s.runCtx = ctx
			s.instanceID = "me"
			s.singletonInterval = time.Millisecond
			var claimed atomic.Bool
			s.singletonClaim = func(string, string) error {
				claimed.Store(true)
				return nil
			}
			s.singletonCurrent = func(string) string { return "me" }
			s.startSingletonWatch(first.root, first.scope)
			s.startSingletonWatch("/work/space", "scope")
			time.Sleep(20 * time.Millisecond)
			assert.False(t, claimed.Load(), "a second initialize must not claim after the first opted out")
		})
	}
}

func TestStartSingletonWatchNoopWithoutInstanceID(t *testing.T) {
	t.Parallel()
	s := New(Options{Reader: nil, Writer: io.Discard, Rules: rule.All()})
	s.instanceID = "" // feature off
	s.singletonClaim = func(string, string) error {
		t.Error("must not claim a workspace when the feature is off")
		return nil
	}
	s.startSingletonWatch("/work/space", "scope")
	time.Sleep(20 * time.Millisecond)
}

func TestStartSingletonWatchStaysWhileOwner(t *testing.T) {
	t.Parallel()
	s := New(Options{Reader: nil, Writer: io.Discard, Rules: rule.All()})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s.runCtx = ctx
	s.instanceID = "me"
	s.singletonInterval = time.Millisecond
	s.singletonClaim = func(string, string) error { return nil }
	s.singletonCurrent = func(string) string { return "me" }
	var exited atomic.Bool
	s.onSupersededExit = func() { exited.Store(true) }

	s.startSingletonWatch("/work/space", "scope")
	time.Sleep(30 * time.Millisecond)
	assert.False(t, exited.Load(), "must not step aside while it is still the registered owner")
}

func TestStartSingletonWatchSupersedesAndNotifies(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	s := New(Options{Reader: nil, Writer: &buf, Rules: rule.All()})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s.runCtx = ctx
	s.instanceID = "me"
	s.singletonInterval = time.Millisecond
	var claimedKey, claimedID string
	s.singletonClaim = func(key, id string) error {
		if claimedKey == "" {
			claimedKey, claimedID = key, id
		}
		return nil
	}
	s.singletonCurrent = func(string) string { return "newer-instance" }
	exited := make(chan struct{})
	s.onSupersededExit = func() { close(exited) }

	s.startSingletonWatch("/work/space", "scope")

	select {
	case <-exited:
	case <-time.After(2 * time.Second):
		t.Fatal("did not step aside when a newer server claimed the workspace")
	}
	assert.Equal(t, "me", claimedID, "must claim the workspace under its own instance id")
	assert.Equal(t, workspaceKey("/work/space", "scope"), claimedKey,
		"must claim under the workspace key for this root and scope")
	assert.Contains(t, buf.String(), "mdsmith/superseded",
		"must notify the editor before exiting so its client does not restart us")
	assert.Contains(t, buf.String(), `"reason":"superseded"`,
		"must serialize the superseded reason payload the supersededParams struct declares")
}

// A failed legacy write is logged and ignored: the scoped claim
// already succeeded, so the watcher still runs on the scoped key.
func TestStartSingletonWatchIgnoresLegacyClaimFailure(t *testing.T) {
	t.Parallel()
	s := New(Options{Reader: nil, Writer: io.Discard, Rules: rule.All()})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s.runCtx = ctx
	s.instanceID = "me"
	s.singletonInterval = time.Millisecond
	scoped := workspaceKey("/work/space", "scope")
	s.singletonClaim = func(key, _ string) error {
		if key != scoped {
			return io.ErrClosedPipe
		}
		return nil
	}
	watched := make(chan string, 1)
	s.singletonCurrent = func(key string) string {
		select {
		case watched <- key:
		default:
		}
		return "me"
	}
	s.startSingletonWatch("/work/space", "scope")
	select {
	case key := <-watched:
		assert.Equal(t, scoped, key, "the watcher polls the scoped key only")
	case <-time.After(2 * time.Second):
		t.Fatal("a failed legacy write must not stop the scoped watcher")
	}
}

func TestStartSingletonWatchNoopWithoutClaimSeam(t *testing.T) {
	t.Parallel()
	s := New(Options{Reader: nil, Writer: io.Discard, Rules: rule.All()})
	s.instanceID = "me" // set, but the registry seam is not wired
	s.singletonClaim = nil
	s.singletonCurrent = func(string) string {
		t.Error("must not start a watcher when the claim seam is nil")
		return ""
	}
	s.startSingletonWatch("/work/space", "scope") // must not panic on the nil seam
	time.Sleep(20 * time.Millisecond)
}

func TestStartSingletonWatchClaimsOnlyOnce(t *testing.T) {
	t.Parallel()
	s := New(Options{Reader: nil, Writer: io.Discard, Rules: rule.All()})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s.runCtx = ctx
	s.instanceID = "me"
	s.singletonInterval = time.Hour // keep the watcher idle
	var claims []string
	s.singletonClaim = func(key, _ string) error {
		claims = append(claims, key)
		return nil
	}
	s.singletonCurrent = func(string) string { return "me" }

	s.startSingletonWatch("/work/space", "scope")
	s.startSingletonWatch("/work/space", "scope") // a stray re-initialize must not re-claim
	s.startSingletonWatch("/other", "scope")      // nor one with a different root
	assert.Equal(t, []string{workspaceKey("/work/space", "scope"), workspaceKey("/work/space", "")}, claims,
		"claim is guarded by the watch Once: the scoped key, then the legacy key, exactly once")
}

func TestNewDefaultOnSupersededExitCallsOsExit(t *testing.T) {
	orig := osExit
	t.Cleanup(func() { osExit = orig })
	var called bool
	var code int
	osExit = func(c int) { called, code = true, c }
	s := New(Options{Reader: nil, Writer: io.Discard, Rules: rule.All()})
	s.onSupersededExit() // run the real default closure New() installed
	assert.True(t, called, "the default onSupersededExit must exit the process")
	assert.Equal(t, 0, code, "must exit cleanly so the editor does not treat it as a crash")
}

func TestStartSingletonWatchKeepsRunningWhenClaimFails(t *testing.T) {
	t.Parallel()
	s := New(Options{Reader: nil, Writer: io.Discard, Rules: rule.All()})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s.runCtx = ctx
	s.instanceID = "me"
	s.singletonInterval = time.Millisecond
	s.singletonClaim = func(string, string) error { return io.ErrClosedPipe }
	s.singletonCurrent = func(string) string {
		t.Error("must not watch (and risk self-supersession) when the claim failed")
		return "newer-instance"
	}
	var exited atomic.Bool
	s.onSupersededExit = func() { exited.Store(true) }

	s.startSingletonWatch("/work/space", "scope")
	time.Sleep(30 * time.Millisecond)
	assert.False(t, exited.Load(), "a failed claim must leave the server running, not reap it")
}

func TestHandleInitializeClaimsWorkspaceSingleton(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	s := New(Options{Reader: nil, Writer: &buf, Rules: rule.All()})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel) // stop the watcher goroutine started by handleInitialize
	s.runCtx = ctx
	s.instanceID = "me"
	s.singletonInterval = time.Hour // keep the watcher idle for the test
	var claimedKey, claimedID string
	s.singletonClaim = func(key, id string) error {
		if claimedKey == "" {
			claimedKey, claimedID = key, id
		}
		return nil
	}
	s.singletonCurrent = func(string) string { return "me" }

	msg := &requestMessage{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "initialize",
		Params: json.RawMessage(`{"processId":null,"rootUri":"file:///work/space",` +
			`"initializationOptions":{"mdsmith":{"singletonScope":"ws-uuid"}}}`),
	}
	s.handleInitialize(msg)

	assert.Equal(t, "me", claimedID, "initialize must claim the workspace singleton")
	assert.Equal(t, workspaceKey("/work/space", "ws-uuid"), claimedKey,
		"initialize must claim under the rootUri's workspace key for the client's scope")
}

func TestHandleInitializeWithoutScopeNeverClaims(t *testing.T) {
	t.Parallel()
	for _, params := range []string{
		`{"processId":null,"rootUri":"file:///work/space"}`,
		`{"processId":null,"rootUri":"file:///work/space","initializationOptions":null}`,
		`{"processId":null,"rootUri":"file:///work/space","initializationOptions":{"mdsmith":{"singletonScope":""}}}`,
	} {
		var buf bytes.Buffer
		s := New(Options{Reader: nil, Writer: &buf, Rules: rule.All()})
		ctx, cancel := context.WithCancel(context.Background())
		s.runCtx = ctx
		s.instanceID = "me"
		s.singletonInterval = time.Millisecond
		s.singletonClaim = func(string, string) error {
			t.Errorf("must not claim the registry without a singletonScope: %s", params)
			return nil
		}
		s.singletonCurrent = func(string) string { return "newer-instance" }
		var exited atomic.Bool
		s.onSupersededExit = func() { exited.Store(true) }

		s.handleInitialize(&requestMessage{
			JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: "initialize",
			Params: json.RawMessage(params),
		})
		time.Sleep(20 * time.Millisecond)
		cancel()
		assert.False(t, exited.Load(), "a scope-less server must never be superseded: %s", params)
		assert.NotContains(t, buf.String(), "mdsmith/superseded")
	}
}

func TestNewEnablesWorkspaceSingleton(t *testing.T) {
	t.Parallel()
	on := New(Options{Reader: nil, Writer: io.Discard, Rules: rule.All(), EnableWorkspaceSingleton: true})
	assert.NotEmpty(t, on.instanceID, "an enabled server gets a real instance id")
	require.NotNil(t, on.singletonClaim, "an enabled server wires the registry claim seam")
	require.NotNil(t, on.singletonCurrent, "an enabled server wires the registry read seam")

	off := New(Options{Reader: nil, Writer: io.Discard, Rules: rule.All()})
	assert.Empty(t, off.instanceID, "a disabled server has no instance id, so the watch is a no-op")
	assert.Nil(t, off.singletonClaim, "a disabled server wires no registry seam")
}

func TestFileRegistryClaimCurrentRoundTrip(t *testing.T) {
	t.Parallel()
	r := fileRegistry{dir: t.TempDir()}
	assert.Equal(t, "", r.current("k"), "no claim yet reads as no owner")

	require.NoError(t, r.claim("k", "id-1"))
	assert.Equal(t, "id-1", r.current("k"), "current reads back the claimed id")

	require.NoError(t, r.claim("k", "id-2"))
	assert.Equal(t, "id-2", r.current("k"), "a newer claim overwrites the older owner")

	assert.Equal(t, "", r.current("other"), "an unrelated key has no owner")
}

func TestWorkspaceKeyStableAndDistinct(t *testing.T) {
	t.Parallel()
	assert.Equal(t, workspaceKey("/a/b", "s"), workspaceKey("/a/b/", "s"),
		"a trailing slash must not change the key")
	assert.Equal(t, workspaceKey("/a/b", "s"), workspaceKey("/a/./b", "s"),
		"a redundant path element must not change the key")
	assert.NotEqual(t, workspaceKey("/a/b", "s"), workspaceKey("/a/c", "s"),
		"distinct workspaces get distinct keys")
}

func TestWorkspaceKeyScopeSameRootDifferentScopeDiffers(t *testing.T) {
	t.Parallel()
	assert.NotEqual(t, workspaceKey("/a/b", "vscode-1"), workspaceKey("/a/b", "vscode-2"),
		"two scopes on one workspace must contend for different owner records")
	assert.NotEqual(t, workspaceKey("/a/b", "vscode-1"), workspaceKey("/a/b", ""),
		"a scoped key must not collide with the legacy root-only key")
}

func TestWorkspaceKeyScopeSameRootSameScopeMatches(t *testing.T) {
	t.Parallel()
	assert.Equal(t, workspaceKey("/a/b", "vscode-1"), workspaceKey("/a/b/", "vscode-1"),
		"an orphan and its respawn sharing one scope must share one owner record")
}

func TestWorkspaceKeyScopeEmptyIsLegacyRootOnlyKey(t *testing.T) {
	t.Parallel()
	// The pre-scope derivation: sha256 over the cleaned root alone. An
	// empty scope must reproduce it byte for byte, so the one key
	// function keeps the legacy format rather than growing a sibling.
	legacy := sha256.Sum256([]byte("/a/b"))
	assert.Equal(t, hex.EncodeToString(legacy[:]), workspaceKey("/a/b/", ""))
}

func TestWorkspaceKeyScopeSeparatorPreventsAmbiguity(t *testing.T) {
	t.Parallel()
	// Without the NUL separator, root "/a" with scope "b" and root
	// "/ab" with no scope would hash the same bytes, "/ab".
	assert.NotEqual(t, workspaceKey("/a", "b"), workspaceKey("/ab", ""),
		"root and scope must be framed so their concatenation is unambiguous")
}

func TestNewInstanceIDUniqueAndNonEmpty(t *testing.T) {
	t.Parallel()
	a := newInstanceID()
	b := newInstanceID()
	assert.NotEmpty(t, a)
	assert.NotEqual(t, a, b, "each instance id must be distinct")
}

// The following tests drive the registry's failure branches and the OS
// seams. They override package-level seams, so they do NOT run in
// parallel; Go schedules every t.Parallel() test after the sequential
// ones finish, so the override-and-restore never races a concurrent
// reader.

func TestNewInstanceIDEmptyOnRandFailure(t *testing.T) {
	orig := randRead
	t.Cleanup(func() { randRead = orig })
	randRead = func([]byte) (int, error) { return 0, errors.New("rng down") }
	assert.Empty(t, newInstanceID(), "a failed RNG yields no id, disabling the singleton")
}

func TestDefaultRegistryUsesCacheDir(t *testing.T) {
	orig := userCacheDir
	t.Cleanup(func() { userCacheDir = orig })
	userCacheDir = func() (string, error) { return "/cache", nil }
	assert.Equal(t, filepath.Join("/cache", "mdsmith", "lsp-singleton"), defaultRegistry().dir)
}

func TestDefaultRegistryFallsBackToTempDirWhenCacheUnavailable(t *testing.T) {
	orig := userCacheDir
	t.Cleanup(func() { userCacheDir = orig })
	userCacheDir = func() (string, error) { return "", errors.New("no cache dir") }
	assert.Equal(t, filepath.Join(os.TempDir(), "mdsmith", "lsp-singleton"), defaultRegistry().dir)
}

func TestFileRegistryClaimErrorsWhenDirUncreatable(t *testing.T) {
	t.Parallel()
	blocker := filepath.Join(t.TempDir(), "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o600))
	// r.dir sits under a regular file, so MkdirAll cannot create it.
	r := fileRegistry{dir: filepath.Join(blocker, "sub")}
	assert.Error(t, r.claim("k", "id"))
}

func TestFileRegistryClaimErrorsWhenTempPathIsDir(t *testing.T) {
	t.Parallel()
	r := fileRegistry{dir: t.TempDir()}
	// claim writes to "<owner>.<id>.tmp"; a directory there makes the
	// WriteFile step fail.
	require.NoError(t, os.Mkdir(r.path("k")+"."+"id"+".tmp", 0o755))
	assert.Error(t, r.claim("k", "id"))
}

func TestFileRegistryClaimRemovesTempOnRenameFailure(t *testing.T) {
	t.Parallel()
	r := fileRegistry{dir: t.TempDir()}
	// A directory at the owner path makes os.Rename(tmp, owner) fail
	// (can't rename a file onto a directory) after WriteFile succeeds.
	require.NoError(t, os.Mkdir(r.path("k"), 0o755))
	assert.Error(t, r.claim("k", "id"))
	_, statErr := os.Stat(r.path("k") + "." + "id" + ".tmp")
	assert.True(t, os.IsNotExist(statErr), "a failed claim must not leave its temp file behind")
}

func TestFileRegistryClaimPrunesStaleRecords(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r := fileRegistry{dir: dir}
	old := time.Now().Add(-2 * singletonRecordMaxAge)
	stale := filepath.Join(dir, "stale.owner")
	require.NoError(t, os.WriteFile(stale, []byte("x"), 0o600))
	require.NoError(t, os.Chtimes(stale, old, old))
	fresh := filepath.Join(dir, "fresh.owner")
	require.NoError(t, os.WriteFile(fresh, []byte("y"), 0o600))

	require.NoError(t, r.claim("mine", "me"))

	assert.NoFileExists(t, stale, "a record untouched past the max age must be pruned on claim")
	assert.FileExists(t, fresh, "a recent record belongs to a live server and must stay")
	assert.Equal(t, "me", r.current("mine"))
}

func TestPruneStaleRecords(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	now := time.Now()
	old := now.Add(-2 * time.Hour)
	write := func(name string, mtime time.Time) string {
		p := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(p, []byte("x"), 0o600))
		require.NoError(t, os.Chtimes(p, mtime, mtime))
		return p
	}
	staleOwner := write("a.owner", old)
	staleTmp := write("a.owner.id.tmp", old)
	keep := write("k.owner", old)
	freshOwner := write("b.owner", now)
	foreign := write("notes.txt", old)
	require.NoError(t, os.Mkdir(filepath.Join(dir, "d.owner"), 0o755))

	pruneStaleRecords(dir, keep, now.Add(-time.Hour))

	assert.NoFileExists(t, staleOwner)
	assert.NoFileExists(t, staleTmp)
	assert.FileExists(t, keep, "the record just claimed is never pruned")
	assert.FileExists(t, freshOwner)
	assert.FileExists(t, foreign, "only registry records are pruned")
	assert.DirExists(t, filepath.Join(dir, "d.owner"), "directories are left alone")
	pruneStaleRecords(filepath.Join(dir, "missing"), "", now) // an unreadable dir is a no-op
}

func TestFileRegistryCurrentEmptyWhenNotReadable(t *testing.T) {
	t.Parallel()
	r := fileRegistry{dir: t.TempDir()}
	// A directory at the owner path: os.Open succeeds but io.ReadAll
	// fails, so current reports no owner rather than a bogus one.
	require.NoError(t, os.Mkdir(r.path("k"), 0o755))
	assert.Equal(t, "", r.current("k"))
}
