package lsp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	vlog "github.com/jeduden/mdsmith/internal/log"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunLintPanicIsRecovered verifies that a panic inside the lint
// pipeline (simulated via the lintPanicHook seam) does not terminate
// the LSP server. The panic is caught by the deferred recover in
// runLint; the server logs the panic and publishes no diagnostics for
// that file.
func TestRunLintPanicIsRecovered(t *testing.T) {
	t.Parallel()
	var logBuf strings.Builder
	var out safeBuffer
	s := New(Options{
		Reader: nil,
		Writer: &out,
		Rules:  rule.All(),
		Logger: &vlog.Logger{Enabled: true, W: &logBuf},
	})

	const uri = "file:///workspace/hostile.md"
	s.docs.set(uri, &document{
		uri:  uri,
		path: "workspace/hostile.md",
		text: []byte("# Title\n\nsome content\n"),
	})

	// lintPanicHook runs inside runLint, replacing the real CheckVersion
	// call. Set it to panic; the deferred recover must catch it.
	s.lintPanicHook = func() {
		panic("injected lint panic")
	}

	// Direct call: before the fix this causes the test process to crash.
	// After the fix runLint returns normally, having logged the panic.
	s.runLint(uri)

	// No publishDiagnostics must have been written (prior diagnostics
	// are left untouched, which for a fresh document means none).
	assert.NotContains(t, out.String(), "publishDiagnostics",
		"a panic in the lint pipeline must not publish diagnostics")

	// The panic must have been logged with its stack so the underlying
	// bug is diagnosable from the only artifact recovery leaves.
	assert.Contains(t, logBuf.String(), "injected lint panic",
		"the recovered panic value must appear in the server log")
	assert.Contains(t, logBuf.String(), "goroutine",
		"the recovery log must include the stack trace")

	// Production runs with logging disabled, so the recovery must also
	// reach the editor's output channel via window/logMessage.
	assert.Contains(t, out.String(), "window/logMessage",
		"the recovered panic must surface to the editor")
}

// TestRunLintIfCurrentPanicIsRecovered verifies the same through the
// runLintIfCurrent entrypoint (the real AfterFunc callback path).
func TestRunLintIfCurrentPanicIsRecovered(t *testing.T) {
	t.Parallel()
	var logBuf strings.Builder
	var out safeBuffer
	s := New(Options{
		Reader: nil,
		Writer: &out,
		Rules:  rule.All(),
		Logger: &vlog.Logger{Enabled: true, W: &logBuf},
	})

	const uri = "file:///workspace/hostile2.md"
	s.docs.set(uri, &document{
		uri:  uri,
		path: "workspace/hostile2.md",
		text: []byte("# Title\n\nsome content\n"),
	})

	s.lintPanicHook = func() {
		panic("injected lint panic via runLintIfCurrent")
	}

	// Register a live pendingLint and call runLintIfCurrent directly
	// so we test the actual AfterFunc path without a real timer.
	p := &pendingLint{}
	s.pendingMu.Lock()
	s.pending[uri] = p
	s.pendingMu.Unlock()

	// Must not crash (panic must be recovered).
	s.runLintIfCurrent(uri, p)

	assert.NotContains(t, out.String(), "publishDiagnostics",
		"a panic in the lint pipeline must not publish diagnostics")
	assert.Contains(t, logBuf.String(), "injected lint panic",
		"the recovered panic value must appear in the server log")
}

// TestDispatchRawPanicAllowsNextRequest verifies that a panic during
// the handling of one LSP message does not kill the dispatch loop.
// The second request must be served normally after the first panics.
func TestDispatchRawPanicAllowsNextRequest(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	// Initialize so the server is ready.
	_, errResp := h.request("initialize", initializeParams{})
	assert.Nil(t, errResp)

	// Install a one-shot dispatch panic hook that panics on the first
	// call and disables itself afterward.
	h.srv.dispatchPanicHook = func() {
		h.srv.dispatchPanicHook = nil // fire only once
		panic("injected dispatch panic")
	}

	// Send a notification that will hit the panic hook; since it is a
	// notification (no ID) the client does not expect a response.
	h.notify("$/cancelRequest", map[string]any{"id": 1})

	// Give the dispatch goroutine a moment to process the panic.
	time.Sleep(20 * time.Millisecond)

	// Now send a real request; the dispatch loop must still be alive.
	resultRaw, errResp2 := h.request("shutdown", nil)
	assert.Nil(t, errResp2)
	assert.Equal(t, "null", string(resultRaw),
		"dispatch loop must still serve requests after a recovered panic")
}

// TestDispatchRawPanicAnswersPendingRequest verifies that a request
// whose handler panics gets an InternalError response, so the
// client's promise for that ID settles instead of pending forever.
func TestDispatchRawPanicAnswersPendingRequest(t *testing.T) {
	t.Parallel()
	var out safeBuffer
	var logBuf strings.Builder
	s := New(Options{
		Reader: nil,
		Writer: &out,
		Rules:  rule.All(),
		Logger: &vlog.Logger{Enabled: true, W: &logBuf},
	})

	// Install a one-shot panic hook.
	s.dispatchPanicHook = func() {
		s.dispatchPanicHook = nil
		panic("injected dispatch panic")
	}

	// Dispatch a request with an ID so the client expects a response.
	raw, _ := json.Marshal(struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Method  string          `json:"method"`
	}{JSONRPC: "2.0", ID: json.RawMessage(`99`), Method: "shutdown"})

	s.dispatchRaw(context.Background(), raw)

	// The panic must be logged with its stack.
	assert.Contains(t, logBuf.String(), "injected dispatch panic",
		"the recovered dispatch panic must be logged")
	assert.Contains(t, logBuf.String(), "goroutine",
		"the recovery log must include the stack trace")

	// The in-flight request must be answered with InternalError.
	assert.Contains(t, out.String(), `"id":99`,
		"the panicked request's ID must be answered")
	assert.Contains(t, out.String(), `-32603`,
		"the answer must be an InternalError response")

	// After the panic the server is not in a wedged state; a fresh
	// request is served with a real response.
	s.dispatchPanicHook = nil
	s.dispatchRaw(context.Background(),
		[]byte(`{"jsonrpc":"2.0","id":100,"method":"shutdown"}`))
	assert.Contains(t, out.String(), `"id":100`,
		"the follow-up request must receive its response")
}

// TestFetchClientSettingsPanicIsRecovered verifies that a panic on
// the client-settings goroutine — here injected via the host's
// OnConfigReload callback, which runs inside the response path's
// reloadConfig — is contained instead of killing the server. The
// same reload triggered from the dispatch loop was already covered
// by dispatchRaw's recover; this pins the asymmetric goroutine path.
func TestFetchClientSettingsPanicIsRecovered(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	require.NoError(t, os.WriteFile(cfgPath,
		[]byte("rules:\n  line-length:\n    max: 120\n"), 0o600))

	var out safeBuffer
	var logBuf strings.Builder
	s := New(Options{
		Reader: nil,
		Writer: &out,
		Rules:  rule.All(),
		Logger: &vlog.Logger{Enabled: true, W: &logBuf},
		OnConfigReload: func(string) {
			panic("injected config-reload panic")
		},
	})
	s.fetchTimeout = 5 * time.Second

	// Deliver the workspace/configuration response (pointing
	// mdsmith.config at the temp file, so the resolved path changes
	// and OnConfigReload fires) once the request has been written.
	respond := func() {
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			if strings.Contains(out.String(), "workspace/configuration") {
				break
			}
			time.Sleep(time.Millisecond)
		}
		result, err := json.Marshal([]clientSettings{{ConfigPath: &cfgPath}})
		require.NoError(t, err)
		s.deliverResponse("1", rpcResponse{Result: result})
	}
	go respond()

	// Direct call: without the deferred recover the injected panic
	// propagates out of fetchClientSettings and fails this test.
	s.fetchClientSettings(context.Background())

	assert.Contains(t, logBuf.String(), "injected config-reload panic",
		"the recovered settings-path panic must be logged")
	assert.Contains(t, logBuf.String(), "goroutine",
		"the recovery log must include the stack trace")
}
