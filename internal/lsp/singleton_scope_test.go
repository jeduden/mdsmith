package lsp

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/rule"
)

// scopedServer is one mdsmith lsp instance wired to a shared on-disk
// registry, standing in for a separate process on the same workspace.
// out is a safeBuffer so the test can read it while the watcher
// goroutine writes the superseded notification.
type scopedServer struct {
	srv    *Server
	out    *safeBuffer
	exited chan struct{}
}

func newScopedServer(t *testing.T, reg fileRegistry) *scopedServer {
	t.Helper()
	out := &safeBuffer{}
	s := New(Options{Reader: nil, Writer: out, Rules: rule.All()})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s.runCtx = ctx
	s.instanceID = newInstanceID()
	require.NotEmpty(t, s.instanceID)
	s.singletonInterval = time.Millisecond
	s.singletonClaim = reg.claim
	s.singletonCurrent = reg.current
	exited := make(chan struct{})
	var once sync.Once
	s.onSupersededExit = func() { once.Do(func() { close(exited) }) }
	return &scopedServer{srv: s, out: out, exited: exited}
}

// initialize sends an initialize request with the given scope; an
// empty scope omits initializationOptions entirely.
func (ss *scopedServer) initialize(scope string) {
	params := map[string]any{"processId": nil, "rootUri": "file:///work/space"}
	if scope != "" {
		params["initializationOptions"] = map[string]any{
			"mdsmith": map[string]any{"singletonScope": scope},
		}
	}
	raw, _ := json.Marshal(params)
	ss.srv.handleInitialize(&requestMessage{
		JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: "initialize", Params: raw,
	})
}

func (ss *scopedServer) superseded(within time.Duration) bool {
	select {
	case <-ss.exited:
		return true
	case <-time.After(within):
		return false
	}
}

// The VS Code upgrade hand-off: the leaked host's server and the
// freshly spawned one read the same persisted scope, so the newest
// claim wins and the older steps aside with mdsmith/superseded.
func TestSingletonSameScopeNewestWins(t *testing.T) {
	t.Parallel()
	reg := fileRegistry{dir: t.TempDir()}
	older := newScopedServer(t, reg)
	newer := newScopedServer(t, reg)

	older.initialize("ws-uuid")
	newer.initialize("ws-uuid")

	require.True(t, older.superseded(2*time.Second), "the older server must step aside")
	assert.Contains(t, older.out.String(), "mdsmith/superseded",
		"the older server must tell its client the exit is intentional")
	assert.False(t, newer.superseded(30*time.Millisecond), "the newest server must stay")
	assert.NotContains(t, newer.out.String(), "mdsmith/superseded")
}

// Two clients with their own scopes on one workspace (e.g. two VS
// Code-style clients with distinct stored ids) never contend.
func TestSingletonDifferentScopesCoexist(t *testing.T) {
	t.Parallel()
	reg := fileRegistry{dir: t.TempDir()}
	a := newScopedServer(t, reg)
	b := newScopedServer(t, reg)

	a.initialize("scope-a")
	b.initialize("scope-b")

	assert.False(t, a.superseded(50*time.Millisecond), "a must not be superseded by another scope")
	assert.False(t, b.superseded(time.Millisecond), "b must not be superseded by another scope")
	assert.NotContains(t, a.out.String(), "mdsmith/superseded")
	assert.NotContains(t, b.out.String(), "mdsmith/superseded")
}

// A scope-less server (Claude Code plugin, a second terminal, Neovim)
// never writes the registry, so it neither displaces a scoped server
// nor is displaced by one started after it.
func TestSingletonNoScopeNeverClaims(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	reg := fileRegistry{dir: dir}
	vscode := newScopedServer(t, reg)
	plugin := newScopedServer(t, reg)
	laterVSCode := newScopedServer(t, reg)

	vscode.initialize("ws-uuid")
	plugin.initialize("")

	assert.False(t, vscode.superseded(50*time.Millisecond),
		"a scope-less server starting later must not supersede the scoped one")
	assert.Equal(t, vscode.srv.instanceID, reg.current(workspaceKey("/work/space", "ws-uuid")),
		"the scoped server must remain the recorded owner")

	laterVSCode.initialize("ws-uuid")
	require.True(t, vscode.superseded(2*time.Second), "same-scope hand-off still works")
	assert.False(t, plugin.superseded(30*time.Millisecond),
		"a scope-less server must never be superseded")
	assert.NotContains(t, plugin.out.String(), "mdsmith/superseded")

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "only the scoped key has an owner record")
	assert.Equal(t, workspaceKey("/work/space", "ws-uuid")+".owner", entries[0].Name())
	assert.Empty(t, reg.current(workspaceKey("/work/space", "")),
		"no owner is ever written under the scope-less (legacy) key")
}
