package lsp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
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
	assert.Equal(t, workspaceKey("/work/space"), claimedKey, "must claim under the workspace key")
	assert.Contains(t, buf.String(), "mdsmith/superseded",
		"must notify the editor before exiting so its client does not restart us")
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
	assert.Equal(t, workspaceKey("/work/space"), claimedKey,
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
	assert.Equal(t, workspaceKey("/a/b"), workspaceKey("/a/b/"),
		"a trailing slash must not change the key")
	assert.Equal(t, workspaceKey("/a/b"), workspaceKey("/a/./b"),
		"a redundant path element must not change the key")
	assert.NotEqual(t, workspaceKey("/a/b"), workspaceKey("/a/c"),
		"distinct workspaces get distinct keys")
}

func TestNewInstanceIDUniqueAndNonEmpty(t *testing.T) {
	t.Parallel()
	a := newInstanceID()
	b := newInstanceID()
	assert.NotEmpty(t, a)
	assert.NotEqual(t, a, b, "each instance id must be distinct")
}
