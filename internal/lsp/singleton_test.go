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
	s.startSingletonWatch("")
	time.Sleep(20 * time.Millisecond)
}

func TestStartSingletonWatchNoopWithoutInstanceID(t *testing.T) {
	t.Parallel()
	s := New(Options{Reader: nil, Writer: io.Discard, Rules: rule.All()})
	s.instanceID = "" // feature off
	s.singletonClaim = func(string, string) error {
		t.Error("must not claim a workspace when the feature is off")
		return nil
	}
	s.startSingletonWatch("/work/space")
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

	s.startSingletonWatch("/work/space")
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
		claimedKey, claimedID = key, id
		return nil
	}
	s.singletonCurrent = func(string) string { return "newer-instance" }
	exited := make(chan struct{})
	s.onSupersededExit = func() { close(exited) }

	s.startSingletonWatch("/work/space")

	select {
	case <-exited:
	case <-time.After(2 * time.Second):
		t.Fatal("did not step aside when a newer server claimed the workspace")
	}
	assert.Equal(t, "me", claimedID, "must claim the workspace under its own instance id")
	assert.Equal(t, workspaceKey("/work/space", ""), claimedKey, "must claim under the workspace key")
	assert.Contains(t, buf.String(), "mdsmith/superseded",
		"must notify the editor before exiting so its client does not restart us")
	assert.Contains(t, buf.String(), `"reason":"superseded"`,
		"must serialize the superseded reason payload the supersededParams struct declares")
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
	s.startSingletonWatch("/work/space") // must not panic on the nil seam
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
	var claims int
	s.singletonClaim = func(string, string) error {
		claims++
		return nil
	}
	s.singletonCurrent = func(string) string { return "me" }

	s.startSingletonWatch("/work/space")
	s.startSingletonWatch("/work/space") // a stray re-initialize must not re-claim
	s.startSingletonWatch("/other")      // nor one with a different root
	assert.Equal(t, 1, claims, "claim is guarded by the watch Once, so it runs exactly once")
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

	s.startSingletonWatch("/work/space")
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
		claimedKey, claimedID = key, id
		return nil
	}
	s.singletonCurrent = func(string) string { return "me" }

	msg := &requestMessage{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "initialize",
		Params:  json.RawMessage(`{"processId":null,"rootUri":"file:///work/space"}`),
	}
	s.handleInitialize(msg)

	assert.Equal(t, "me", claimedID, "initialize must claim the workspace singleton")
	assert.Equal(t, workspaceKey("/work/space", ""), claimedKey,
		"initialize must claim under the rootUri's workspace key")
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
	// empty scope must reproduce it byte for byte so a no-token client
	// and an older root-only binary still agree on one key.
	legacy := sha256.Sum256([]byte("/a/b"))
	assert.Equal(t, hex.EncodeToString(legacy[:]), workspaceKey("/a/b/", ""))
}

func TestWorkspaceKeyScopeSeparatorPreventsAmbiguity(t *testing.T) {
	t.Parallel()
	// Without the NUL separator, root "/a" + scope "b" and root "/a"
	// + scope "" over a root spelled "/ab" could hash the same bytes.
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

func TestFileRegistryCurrentEmptyWhenNotReadable(t *testing.T) {
	t.Parallel()
	r := fileRegistry{dir: t.TempDir()}
	// A directory at the owner path: os.Open succeeds but io.ReadAll
	// fails, so current reports no owner rather than a bogus one.
	require.NoError(t, os.Mkdir(r.path("k"), 0o755))
	assert.Equal(t, "", r.current("k"))
}
