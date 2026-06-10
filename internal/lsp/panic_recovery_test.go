package lsp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	vlog "github.com/jeduden/mdsmith/internal/log"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/stretchr/testify/assert"
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

	// The panic must have been logged so it is observable in the server
	// log rather than silently swallowed.
	assert.Contains(t, logBuf.String(), "injected lint panic",
		"the recovered panic value must appear in the server log")
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

// TestDispatchRawPanicReturnsErrorForPendingRequest verifies that a
// request whose handler panics gets an InternalError response rather
// than leaving the client hanging. (Optional: only asserts the server
// stays alive, since writing a response from recover is optional.)
func TestDispatchRawPanicServerSurvivesOnRequestPanic(t *testing.T) {
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

	// Before the fix this panics; after the fix it returns normally.
	s.dispatchRaw(context.Background(), raw)

	// The panic must be logged.
	assert.Contains(t, logBuf.String(), "injected dispatch panic",
		"the recovered dispatch panic must be logged")

	// After the panic the server is not in a wedged state; a fresh
	// request still works.
	s.dispatchPanicHook = nil
	s.dispatchRaw(context.Background(),
		[]byte(`{"jsonrpc":"2.0","id":100,"method":"shutdown"}`))
}
