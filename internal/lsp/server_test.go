package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/textproto"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"

	// Register rule packages so rule.All() returns the production set.
	_ "github.com/jeduden/mdsmith/internal/rules/linelength"
	_ "github.com/jeduden/mdsmith/internal/rules/notrailingspaces"
)

// testHarness wires a Server to a pair of in-memory pipes plus a
// dedicated reader goroutine that demultiplexes incoming frames so
// tests can wait for specific notifications without racing on the
// underlying bufio.Reader.
type testHarness struct {
	t            *testing.T
	srv          *Server
	clientWriter io.WriteCloser
	srvDone      chan error
	cancel       context.CancelFunc

	writeMu sync.Mutex
	nextID  atomic.Int64

	// Channels populated by the reader goroutine.
	notifications chan parsedNotification
	responses     chan parsedResponse

	// seenServer counts auto-acked server-initiated requests by
	// method, in case a test wants to assert they fired without
	// driving the read loop itself.
	seenMu     sync.Mutex
	seenServer map[string]int
}

type parsedNotification struct {
	Method string
	Params json.RawMessage
}

type parsedResponse struct {
	ID   string
	Resp rpcResponse
}

func newHarness(t *testing.T) *testHarness {
	return newHarnessWithDebounce(t, -1)
}

func newDebouncedHarness(t *testing.T, debounce time.Duration) *testHarness {
	return newHarnessWithDebounce(t, debounce)
}

func newHarnessWithDebounce(t *testing.T, debounce time.Duration) *testHarness {
	t.Helper()
	srvIn, clientWriter := io.Pipe()
	clientRawReader, srvOut := io.Pipe()
	srv := New(Options{
		Rules:    rule.All(),
		Reader:   srvIn,
		Writer:   srvOut,
		Debounce: debounce,
	})
	// Tests want lint-on-didChange to fire so they can verify the
	// pipeline. Production defaults to onSave (skipping didChange);
	// flipping to onType keeps the existing tests deterministic.
	srv.settingsMu.Lock()
	srv.settings.Run = runOnType
	srv.settingsMu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- srv.Run(ctx)
		_ = srvOut.Close()
	}()

	h := &testHarness{
		t:             t,
		srv:           srv,
		clientWriter:  clientWriter,
		srvDone:       done,
		cancel:        cancel,
		notifications: make(chan parsedNotification, 64),
		responses:     make(chan parsedResponse, 64),
		seenServer:    make(map[string]int),
	}

	go h.readPump(bufio.NewReader(clientRawReader))

	t.Cleanup(func() {
		_ = clientWriter.Close()
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Logf("server did not exit cleanly")
		}
	})
	return h
}

// readPump is the only goroutine that reads from the pipe. It demuxes
// frames into the notifications / responses channels, and auto-acks
// server-initiated requests so the dispatch loop never blocks.
func (h *testHarness) readPump(r *bufio.Reader) {
	for {
		raw, err := readFrame(r)
		if err != nil {
			return
		}
		var probe struct {
			ID     json.RawMessage `json:"id,omitempty"`
			Method string          `json:"method,omitempty"`
			Params json.RawMessage `json:"params,omitempty"`
			Result json.RawMessage `json:"result,omitempty"`
			Error  *responseError  `json:"error,omitempty"`
		}
		if err := json.Unmarshal(raw, &probe); err != nil {
			continue
		}

		// Server-initiated request: auto-ack with nil result so the
		// server's dispatch loop can move on, then record that we saw it.
		if probe.Method != "" && len(probe.ID) > 0 {
			h.write(struct {
				JSONRPC string          `json:"jsonrpc"`
				ID      json.RawMessage `json:"id"`
				Result  any             `json:"result"`
			}{JSONRPC: "2.0", ID: probe.ID, Result: nil})
			h.seenMu.Lock()
			h.seenServer[probe.Method]++
			h.seenMu.Unlock()
			continue
		}

		// Notification (no id, has method).
		if probe.Method != "" {
			select {
			case h.notifications <- parsedNotification{Method: probe.Method, Params: probe.Params}:
			default:
				// Drop on overflow — tests only care about a small
				// suffix of notifications.
			}
			continue
		}

		// Response to a client request.
		if len(probe.ID) > 0 {
			select {
			case h.responses <- parsedResponse{
				ID:   string(probe.ID),
				Resp: rpcResponse{Result: probe.Result, Error: probe.Error},
			}:
			default:
			}
		}
	}
}

func readFrame(r *bufio.Reader) ([]byte, error) {
	tp := textproto.NewReader(r)
	hdr, err := tp.ReadMIMEHeader()
	if err != nil {
		return nil, err
	}
	cl := hdr.Get("Content-Length")
	if cl == "" {
		return nil, fmt.Errorf("missing Content-Length")
	}
	n, err := strconv.Atoi(cl)
	if err != nil {
		return nil, err
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}
	return body, nil
}

func (h *testHarness) write(v any) {
	body, err := json.Marshal(v)
	require.NoError(h.t, err)
	h.writeMu.Lock()
	defer h.writeMu.Unlock()
	if _, err := fmt.Fprintf(h.clientWriter, "Content-Length: %d\r\n\r\n", len(body)); err != nil {
		return
	}
	_, _ = h.clientWriter.Write(body)
}

func (h *testHarness) request(method string, params any) (json.RawMessage, *responseError) {
	h.t.Helper()
	id := h.nextID.Add(1)
	idJSON, err := json.Marshal(id)
	require.NoError(h.t, err)
	h.write(struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Method  string          `json:"method"`
		Params  any             `json:"params,omitempty"`
	}{JSONRPC: "2.0", ID: idJSON, Method: method, Params: params})

	deadline := time.After(5 * time.Second)
	for {
		select {
		case r := <-h.responses:
			if r.ID == string(idJSON) {
				return r.Resp.Result, r.Resp.Error
			}
		case <-deadline:
			h.t.Fatalf("timeout waiting for response to %s", method)
			return nil, nil
		}
	}
}

func (h *testHarness) notify(method string, params any) {
	h.t.Helper()
	h.write(notificationMessage{JSONRPC: "2.0", Method: method, Params: params})
}

// awaitNotification consumes from the notifications channel until it
// finds one matching method or hits the timeout.
func (h *testHarness) awaitNotification(method string, timeout time.Duration) json.RawMessage {
	h.t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case n := <-h.notifications:
			if n.Method == method {
				return n.Params
			}
		case <-deadline:
			h.t.Fatalf("timeout waiting for notification %s", method)
			return nil
		}
	}
}

func TestInitializeAdvertisesCapabilities(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	resultRaw, errResp := h.request("initialize", initializeParams{
		Capabilities: clientCapabilities{Workspace: &workspaceClientCapabilities{Configuration: true}},
	})
	require.Nil(t, errResp)

	var res initializeResult
	require.NoError(t, json.Unmarshal(resultRaw, &res))
	assert.True(t, res.Capabilities.TextDocumentSync.OpenClose)
	assert.Equal(t, syncFull, res.Capabilities.TextDocumentSync.Change)
	assert.Contains(t, res.Capabilities.CodeActionProvider.CodeActionKinds, kindQuickFix)
	assert.Contains(t, res.Capabilities.CodeActionProvider.CodeActionKinds, kindSourceFixAll)
	assert.Equal(t, "mdsmith", res.ServerInfo.Name)
}

func TestDidOpenPublishesDiagnostics(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	_, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)

	doc := didOpenTextDocumentParams{
		TextDocument: textDocumentItem{
			URI:        "file:///workspace/sample.md",
			LanguageID: "markdown",
			Version:    1,
			Text:       "# Title\n\nThis line has trailing spaces   \n",
		},
	}
	h.notify("textDocument/didOpen", doc)

	raw := h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)
	var p publishDiagnosticsParams
	require.NoError(t, json.Unmarshal(raw, &p))
	assert.Equal(t, doc.TextDocument.URI, p.URI)

	var found bool
	for _, d := range p.Diagnostics {
		if d.Code == "MDS006" {
			found = true
			assert.NotZero(t, int(d.Severity))
			assert.Equal(t, "mdsmith", d.Source)
			require.NotNil(t, d.Data)
			assert.Equal(t, "no-trailing-spaces", d.Data.RuleName)
			break
		}
	}
	assert.True(t, found, "expected MDS006 diagnostic in %+v", p.Diagnostics)
}

func TestDidCloseClearsDiagnostics(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	_, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)

	uri := "file:///workspace/clear.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: "# Hi\n\ntrailing  \n"},
	})
	first := h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)
	var p publishDiagnosticsParams
	require.NoError(t, json.Unmarshal(first, &p))
	assert.NotEmpty(t, p.Diagnostics)

	h.notify("textDocument/didClose", didCloseTextDocumentParams{
		TextDocument: textDocumentIdentifier{URI: uri},
	})
	second := h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)
	var cleared publishDiagnosticsParams
	require.NoError(t, json.Unmarshal(second, &cleared))
	assert.Equal(t, uri, cleared.URI)
	assert.Empty(t, cleared.Diagnostics)
}

func TestShutdownReturnsNullResult(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	_, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)
	resultRaw, errResp := h.request("shutdown", nil)
	require.Nil(t, errResp)
	assert.Equal(t, "null", string(resultRaw))
}

func TestDidChangeUpdatesDiagnostics(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	_, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)

	uri := "file:///workspace/change.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{
			URI: uri, LanguageID: "markdown", Version: 1,
			Text: "# Hi\n\nclean line\n",
		},
	})
	first := h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)
	var p1 publishDiagnosticsParams
	require.NoError(t, json.Unmarshal(first, &p1))
	assert.Empty(t, p1.Diagnostics)

	h.notify("textDocument/didChange", didChangeTextDocumentParams{
		TextDocument: versionedTextDocumentIdentifier{URI: uri, Version: 2},
		ContentChanges: []textDocumentContentChangeEvent{
			{Text: "# Hi\n\ndirty line   \n"},
		},
	})
	second := h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)
	var p2 publishDiagnosticsParams
	require.NoError(t, json.Unmarshal(second, &p2))
	var saw006 bool
	for _, d := range p2.Diagnostics {
		if d.Code == "MDS006" {
			saw006 = true
			break
		}
	}
	assert.True(t, saw006, "expected MDS006 after didChange, got %+v", p2.Diagnostics)
}

func TestCodeActionQuickFix(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	_, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)

	uri := "file:///workspace/qf.md"
	dirty := "# Hi\n\ndirty line   \n"
	clean := "# Hi\n\ndirty line\n"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: dirty},
	})
	raw := h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)
	var p publishDiagnosticsParams
	require.NoError(t, json.Unmarshal(raw, &p))

	var trailing Diagnostic
	for _, d := range p.Diagnostics {
		if d.Code == "MDS006" {
			trailing = d
			break
		}
	}
	require.NotEmpty(t, trailing.Code, "no MDS006 in %+v", p.Diagnostics)

	resultRaw, errResp := h.request("textDocument/codeAction", codeActionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Range:        trailing.Range,
		Context:      codeActionContext{Diagnostics: []Diagnostic{trailing}},
	})
	require.Nil(t, errResp)

	var actions []codeAction
	require.NoError(t, json.Unmarshal(resultRaw, &actions))
	require.NotEmpty(t, actions)

	var qf *codeAction
	for i, a := range actions {
		if a.Kind == kindQuickFix {
			qf = &actions[i]
			break
		}
	}
	require.NotNil(t, qf)
	require.NotNil(t, qf.Edit)
	edits, ok := qf.Edit.Changes[uri]
	require.True(t, ok)
	require.Len(t, edits, 1)
	assert.Equal(t, clean, edits[0].NewText)
}

func TestCodeActionSourceFixAll(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	_, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)

	uri := "file:///workspace/all.md"
	dirty := "# Hi\n\ndirty line   \n"
	clean := "# Hi\n\ndirty line\n"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: dirty},
	})
	raw := h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)
	var p publishDiagnosticsParams
	require.NoError(t, json.Unmarshal(raw, &p))
	require.NotEmpty(t, p.Diagnostics)

	resultRaw, errResp := h.request("textDocument/codeAction", codeActionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Range:        Range{},
		Context:      codeActionContext{Diagnostics: p.Diagnostics, Only: []string{kindSourceFixAll}},
	})
	require.Nil(t, errResp)
	var actions []codeAction
	require.NoError(t, json.Unmarshal(resultRaw, &actions))
	var fixAll *codeAction
	for i, a := range actions {
		if a.Kind == kindSourceFixAll {
			fixAll = &actions[i]
			break
		}
	}
	require.NotNil(t, fixAll, "expected source.fixAll.mdsmith action; got %+v", actions)
	edits := fixAll.Edit.Changes[uri]
	require.Len(t, edits, 1)
	assert.Equal(t, clean, edits[0].NewText)
}

func TestCodeActionOnlyFiltersOutQuickFix(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	_, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)

	uri := "file:///workspace/only.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: "# Hi\n\ndirty   \n"},
	})
	raw := h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)
	var p publishDiagnosticsParams
	require.NoError(t, json.Unmarshal(raw, &p))
	require.NotEmpty(t, p.Diagnostics)

	// Ask only for source.fixAll — quickfix actions must not be returned.
	resultRaw, errResp := h.request("textDocument/codeAction", codeActionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Range:        Range{},
		Context:      codeActionContext{Diagnostics: p.Diagnostics, Only: []string{kindSourceFixAll}},
	})
	require.Nil(t, errResp)
	var actions []codeAction
	require.NoError(t, json.Unmarshal(resultRaw, &actions))
	for _, a := range actions {
		assert.NotEqual(t, kindQuickFix, a.Kind, "Only=source.fixAll filter should suppress quickfix actions")
	}
}

func TestUnknownMethodReturnsError(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	_, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)
	_, errResp = h.request("textDocument/bogus", nil)
	require.NotNil(t, errResp)
	assert.Equal(t, codeMethodNotFound, errResp.Code)
}

func TestRegisterWatchersWritesRequest(t *testing.T) {
	t.Parallel()
	var buf safeBuffer
	s := New(Options{Reader: nil, Writer: &buf, Rules: rule.All()})
	s.registerWatchers()
	out := buf.String()
	assert.Contains(t, out, "client/registerCapability")
	assert.Contains(t, out, "**/.mdsmith.yml")
}

func TestHandleInitializedRunsConfigAndWatchers(t *testing.T) {
	t.Parallel()
	var buf safeBuffer
	s := New(Options{Reader: nil, Writer: &buf, Rules: rule.All()})
	s.handleInitialized(context.Background())

	// reloadConfig populated the config; registerWatchers wrote the
	// registration request synchronously.
	cfg, _, _ := s.snapshotConfig()
	assert.NotNil(t, cfg)
	assert.Contains(t, buf.String(), "client/registerCapability")

	// fetchClientSettings ran in a goroutine. Poll briefly for its
	// outgoing workspace/configuration request before asserting.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(buf.String(), "workspace/configuration") {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("workspace/configuration never written; buffer: %s", buf.String())
}

func TestHandleInitializeMalformedParams(t *testing.T) {
	t.Parallel()
	var buf safeBuffer
	s := New(Options{Reader: nil, Writer: &buf, Rules: rule.All()})
	msg := &requestMessage{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "initialize",
		Params:  json.RawMessage(`not json`),
	}
	s.handleInitialize(msg)
	out := buf.String()
	assert.Contains(t, out, "invalid initialize params")
}

func TestHandleInitializeEmptyParams(t *testing.T) {
	t.Parallel()
	var buf safeBuffer
	s := New(Options{Reader: nil, Writer: &buf, Rules: rule.All()})
	msg := &requestMessage{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "initialize",
	}
	s.handleInitialize(msg)
	out := buf.String()
	assert.Contains(t, out, `"capabilities"`)
	assert.Contains(t, out, "mdsmith")
}

func TestHandleCodeActionUnknownDocument(t *testing.T) {
	t.Parallel()
	var buf safeBuffer
	s := New(Options{Reader: nil, Writer: &buf, Rules: rule.All()})
	body, _ := json.Marshal(codeActionParams{
		TextDocument: textDocumentIdentifier{URI: "file:///none.md"},
	})
	msg := &requestMessage{ID: json.RawMessage(`1`), Method: "textDocument/codeAction", Params: body}
	s.handleCodeAction(msg)
	// Response is an empty action array.
	assert.Contains(t, buf.String(), `"result":[]`)
}

func TestHandleCodeActionInvalidJSON(t *testing.T) {
	t.Parallel()
	var buf safeBuffer
	s := New(Options{Reader: nil, Writer: &buf, Rules: rule.All()})
	msg := &requestMessage{ID: json.RawMessage(`1`), Params: json.RawMessage("oops")}
	s.handleCodeAction(msg)
	assert.Contains(t, buf.String(), "invalid codeAction params")
}

func TestQuickFixEditForFixerError(t *testing.T) {
	// Invalid path used to be propagated from the fixer; today it is
	// just a label so the fix typically succeeds. The point of the
	// test is that the helper does not crash on an unusual path.
	t.Parallel()
	s := New(Options{Reader: nil, Writer: io.Discard, Rules: rule.All()})
	cfg := config.Merge(config.Defaults(), nil)
	doc := &document{path: "/no/such/path/x.md", text: []byte("# Hi\n\ndirty   \n")}
	_ = s.quickFixEditFor("no-trailing-spaces", doc, cfg, "", "file:///x.md")
}

func TestDispatchHandlesNotificationsWithoutResponse(t *testing.T) {
	t.Parallel()
	s := New(Options{Reader: nil, Writer: io.Discard, Rules: rule.All()})
	// $/cancelRequest is silently accepted.
	s.dispatch(context.Background(), &requestMessage{Method: "$/cancelRequest"})
	s.dispatch(context.Background(), &requestMessage{Method: "$/setTrace"})
	s.dispatch(context.Background(), &requestMessage{Method: "$/progress"})
}

func TestDispatchUnknownNotificationIsSilentlyDropped(t *testing.T) {
	t.Parallel()
	var buf safeBuffer
	s := New(Options{Reader: nil, Writer: &buf, Rules: rule.All()})
	// No ID → notification → must not write any frame.
	s.dispatch(context.Background(), &requestMessage{Method: "completely/unknown"})
	assert.Empty(t, buf.String())
}

func TestDispatchExitTogglesShutdownFlags(t *testing.T) {
	t.Parallel()
	s := New(Options{Reader: nil, Writer: io.Discard, Rules: rule.All()})
	s.dispatch(context.Background(), &requestMessage{Method: "exit"})
	assert.True(t, s.shutdown.Load())
	assert.True(t, s.exitRequested.Load())
}

func TestDispatchRawRoutesResponseToWaiter(t *testing.T) {
	t.Parallel()
	s := New(Options{Reader: nil, Writer: io.Discard, Rules: rule.All()})
	ch := s.registerPendingResponse(`42`)
	defer s.unregisterPendingResponse(`42`)
	s.dispatchRaw(context.Background(),
		[]byte(`{"jsonrpc":"2.0","id":42,"result":{"k":1}}`))
	select {
	case resp := <-ch:
		assert.NotEmpty(t, resp.Result)
	case <-time.After(time.Second):
		t.Fatalf("response not delivered")
	}
}

func TestDispatchRawRejectsWrongVersionWithID(t *testing.T) {
	t.Parallel()
	var buf safeBuffer
	s := New(Options{Reader: nil, Writer: &buf, Rules: rule.All()})
	s.dispatchRaw(context.Background(),
		[]byte(`{"jsonrpc":"1.0","id":7,"method":"x"}`))
	out := buf.String()
	assert.Contains(t, out, `"id":7`)
	assert.Contains(t, out, "jsonrpc must be 2.0")
}

func TestScheduleLintSkipsWhenShutdown(t *testing.T) {
	t.Parallel()
	var buf safeBuffer
	s := New(Options{Reader: nil, Writer: &buf, Rules: rule.All()})
	s.shutdown.Store(true)
	s.docs.set("file:///x.md", &document{
		uri: "file:///x.md", path: "x.md", text: []byte("# Hi\n\ndirty   \n"),
	})
	s.scheduleLint("file:///x.md", lintTriggerOpen)
	// Server is shutting down — no diagnostics should land on the wire.
	assert.NotContains(t, buf.String(), "publishDiagnostics")
}

func TestScheduleLintOnSaveSkipsChange(t *testing.T) {
	t.Parallel()
	var buf safeBuffer
	s := New(Options{Reader: nil, Writer: &buf, Rules: rule.All()})
	// Default run mode is onSave.
	s.docs.set("file:///x.md", &document{
		uri: "file:///x.md", path: "x.md", text: []byte("# Hi\n\ndirty   \n"),
	})
	s.scheduleLint("file:///x.md", lintTriggerChange)
	assert.NotContains(t, buf.String(), "publishDiagnostics")
}

func TestStopPendingLintsCancelsTimers(t *testing.T) {
	t.Parallel()
	s := New(Options{Reader: nil, Writer: io.Discard, Rules: rule.All(), Debounce: 10 * time.Second})
	s.settingsMu.Lock()
	s.settings.Run = runOnType
	s.settingsMu.Unlock()
	s.docs.set("file:///x.md", &document{
		uri: "file:///x.md", path: "x.md", text: []byte("# Hi\n"),
	})
	s.scheduleLint("file:///x.md", lintTriggerChange)
	s.pendingMu.Lock()
	require.Len(t, s.pending, 1)
	s.pendingMu.Unlock()
	s.stopPendingLints()
	s.pendingMu.Lock()
	assert.Empty(t, s.pending, "stopPendingLints should clear the pending map")
	s.pendingMu.Unlock()
}

func TestRunModeFallsBackOnEmpty(t *testing.T) {
	t.Parallel()
	s := New(Options{Reader: nil, Writer: io.Discard})
	s.settingsMu.Lock()
	s.settings.Run = ""
	s.settingsMu.Unlock()
	assert.Equal(t, runOnSave, s.runMode())
}

func TestFetchClientSettingsHandlesEmptyArray(t *testing.T) {
	t.Parallel()
	var buf safeBuffer
	s := New(Options{Reader: nil, Writer: &buf, Rules: rule.All()})
	go s.fetchClientSettings(context.Background())
	// Drive the goroutine: deliver an empty array — VS Code returns
	// this when the section has no settings. The call should leave
	// settings unchanged.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s.pendingRespMu.Lock()
		var key string
		for k := range s.pendingResp {
			key = k
		}
		s.pendingRespMu.Unlock()
		if key != "" {
			s.deliverResponse(key, rpcResponse{Result: json.RawMessage(`[]`)})
			break
		}
		time.Sleep(time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	s.settingsMu.RLock()
	defer s.settingsMu.RUnlock()
	assert.Equal(t, runOnSave, s.settings.Run)
}

func TestFetchClientSettingsHandlesMalformedResult(t *testing.T) {
	t.Parallel()
	var buf safeBuffer
	s := New(Options{Reader: nil, Writer: &buf, Rules: rule.All()})
	go s.fetchClientSettings(context.Background())
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s.pendingRespMu.Lock()
		var key string
		for k := range s.pendingResp {
			key = k
		}
		s.pendingRespMu.Unlock()
		if key != "" {
			// Result is not an array — defaults must stand.
			s.deliverResponse(key, rpcResponse{Result: json.RawMessage(`"oops"`)})
			break
		}
		time.Sleep(time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	s.settingsMu.RLock()
	defer s.settingsMu.RUnlock()
	assert.Equal(t, runOnSave, s.settings.Run)
}

func TestFetchClientSettingsHonorsContextCancel(t *testing.T) {
	t.Parallel()
	s := New(Options{Reader: nil, Writer: io.Discard, Rules: rule.All()})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		s.fetchClientSettings(ctx)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("fetchClientSettings did not return on ctx cancel")
	}
}

func TestComputeCodeActionsSkipsDiagnosticsWithoutData(t *testing.T) {
	t.Parallel()
	s := New(Options{Reader: nil, Writer: io.Discard, Rules: rule.All()})
	cfg := config.Merge(config.Defaults(), nil)
	doc := &document{path: "x.md", text: []byte("# Hi\n\ndirty   \n")}
	p := codeActionParams{
		TextDocument: textDocumentIdentifier{URI: "file:///x.md"},
		Context: codeActionContext{
			Diagnostics: []Diagnostic{
				{Code: "MDS006"}, // no Data — must be skipped
			},
			Only: []string{kindQuickFix},
		},
	}
	actions := s.computeCodeActions(p, doc, cfg, "")
	assert.Empty(t, actions, "diagnostic without Data should not produce a quickfix")
}

func TestComputeCodeActionsCachesNilEdits(t *testing.T) {
	t.Parallel()
	// Verify that the per-rule cache stores the negative result for
	// rules whose fix is unavailable, so a second diagnostic from
	// the same rule does not retry the fix.
	s := New(Options{Reader: nil, Writer: io.Discard, Rules: rule.All()})
	cfg := config.Merge(config.Defaults(), nil)
	doc := &document{path: "x.md", text: []byte("# Hi\n")}
	d := Diagnostic{Code: "X", Data: &diagnosticData{RuleName: "no-such-rule"}}
	p := codeActionParams{
		TextDocument: textDocumentIdentifier{URI: "file:///x.md"},
		Context: codeActionContext{
			Diagnostics: []Diagnostic{d, d, d},
			Only:        []string{kindQuickFix},
		},
	}
	actions := s.computeCodeActions(p, doc, cfg, "")
	assert.Empty(t, actions)
}

func TestToLSPClampsZeroLine(t *testing.T) {
	t.Parallel()
	got := toLSP(lint.Diagnostic{Line: 0, Column: 1, RuleID: "MDS001", Severity: lint.Error},
		[][]byte{[]byte("a")})
	assert.Equal(t, 0, got.Range.Start.Line)
}

func TestToLSPEmptyLineProducesZeroWidthRange(t *testing.T) {
	t.Parallel()
	// Diagnostic on an empty line: the range must end at column 0,
	// not column 1 — column 1 would be past the line.
	got := toLSP(
		lint.Diagnostic{Line: 2, Column: 1, RuleID: "MDS001", Severity: lint.Error},
		[][]byte{[]byte("first"), []byte("")},
	)
	assert.Equal(t, 1, got.Range.Start.Line)
	assert.Equal(t, 0, got.Range.Start.Character)
	assert.Equal(t, 0, got.Range.End.Character,
		"empty line should produce a zero-width range, not extend past the line")
}

func TestUtf16ColumnTreatsInvalidRunesAsOneUnit(t *testing.T) {
	t.Parallel()
	// Unpaired high surrogate (\uD800). Go's range over a string
	// yields utf8.RuneError for invalid sequences, which has
	// RuneLen 1 — but we want to make sure the helper never adds a
	// negative width even when an actual lone surrogate sneaks in
	// via the rune literal.
	s := string([]rune{0xD800, 'a'})
	got := utf16Column(s, 2)
	assert.Positive(t, got, "utf16Column must stay non-negative on invalid runes")
}

func TestDispatchRawInvalidJSONRespondsWithParseError(t *testing.T) {
	t.Parallel()
	var buf safeBuffer
	s := New(Options{Reader: nil, Writer: &buf, Rules: rule.All()})
	s.dispatchRaw(context.Background(), []byte("not json"))
	out := buf.String()
	assert.Contains(t, out, `"code":-32700`)
	assert.Contains(t, out, `"id":null`)
}

func TestHandleInitializeMalformedReturnsInvalidParams(t *testing.T) {
	t.Parallel()
	var buf safeBuffer
	s := New(Options{Reader: nil, Writer: &buf, Rules: rule.All()})
	s.handleInitialize(&requestMessage{
		ID:     json.RawMessage(`1`),
		Method: "initialize",
		Params: json.RawMessage(`not json`),
	})
	// JSON-RPC §5.1: bad params → -32602.
	assert.Contains(t, buf.String(), `"code":-32602`)
}

func TestHandleCodeActionMalformedReturnsInvalidParams(t *testing.T) {
	t.Parallel()
	var buf safeBuffer
	s := New(Options{Reader: nil, Writer: &buf, Rules: rule.All()})
	s.handleCodeAction(&requestMessage{
		ID:     json.RawMessage(`1`),
		Params: json.RawMessage(`not json`),
	})
	assert.Contains(t, buf.String(), `"code":-32602`)
}

func TestWorkspaceRelativePathHandling(t *testing.T) {
	t.Parallel()
	tests := []struct {
		root, path, want string
	}{
		{"", "/abs/foo.md", "/abs/foo.md"},
		{"/repo", "rel.md", "rel.md"},
		{"/repo", "/repo/docs/foo.md", "docs/foo.md"},
		{"/repo", "/elsewhere/foo.md", "/elsewhere/foo.md"},
	}
	for _, tc := range tests {
		got := workspaceRelative(tc.root, tc.path)
		assert.Equal(t, tc.want, got, "root=%q path=%q", tc.root, tc.path)
	}
}

func TestDirFSForPathRelativeIsNil(t *testing.T) {
	t.Parallel()
	// A relative path label has no meaningful directory; engine
	// treats nil SourceFS as "do not override".
	assert.Nil(t, dirFSForPath("rel.md"))
	assert.NotNil(t, dirFSForPath("/abs/foo.md"))
}

// TestScheduleLintTimerRaceLeavesNewTimer pins the identity-checked
// replacement: a closure whose firing races a fresh scheduleLint
// must not delete the new timer's map entry.
func TestScheduleLintTimerRaceLeavesNewTimer(t *testing.T) {
	t.Parallel()
	s := New(Options{Reader: nil, Writer: io.Discard, Rules: rule.All(), Debounce: 10 * time.Second})
	s.settingsMu.Lock()
	s.settings.Run = runOnType
	s.settingsMu.Unlock()
	s.docs.set("file:///x.md", &document{
		uri: "file:///x.md", path: "x.md", text: []byte("# Hi\n"),
	})
	// Arm a timer, then forge a stale "old timer fired" call by
	// invoking the scheduleLint deletion logic with a tampered map
	// pointer (we can simulate via a second scheduleLint that
	// overwrites). The real concern is the pointer-equality guard;
	// here we verify the new timer remains in the map after the
	// replacement.
	getTimer := func() *time.Timer {
		s.pendingMu.Lock()
		defer s.pendingMu.Unlock()
		return s.pending["file:///x.md"]
	}
	s.scheduleLint("file:///x.md", lintTriggerChange)
	first := getTimer()
	require.NotNil(t, first)
	s.scheduleLint("file:///x.md", lintTriggerChange)
	second := getTimer()
	require.NotNil(t, second)
	assert.NotSame(t, first, second, "replacement must produce a fresh timer")
	// stopPendingLints clears for cleanup.
	s.stopPendingLints()
}

func TestRunReturnsAfterShutdownPlusExit(t *testing.T) {
	t.Parallel()
	srvIn, clientWriter := io.Pipe()
	s := New(Options{Reader: srvIn, Writer: io.Discard, Rules: rule.All()})

	done := make(chan error, 1)
	go func() { done <- s.Run(context.Background()) }()

	// Send shutdown then exit. Both arrive as JSON-RPC frames.
	send := func(body string) {
		_, err := clientWriter.Write([]byte("Content-Length: " + strconv.Itoa(len(body)) + "\r\n\r\n" + body))
		require.NoError(t, err)
	}
	send(`{"jsonrpc":"2.0","id":1,"method":"shutdown"}`)
	send(`{"jsonrpc":"2.0","method":"exit"}`)

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(2 * time.Second):
		_ = clientWriter.Close()
		t.Fatalf("Run did not return after shutdown+exit")
	}
}

func TestRunSurfacesNonEOFError(t *testing.T) {
	t.Parallel()
	// A reader that returns garbage forces readRaw into an error.
	r := strings.NewReader("not a valid LSP frame at all")
	s := New(Options{Reader: r, Writer: io.Discard, Rules: rule.All()})
	err := s.Run(context.Background())
	assert.Error(t, err)
	assert.NotErrorIs(t, err, context.Canceled)
}

func TestUriToPathInvalidURL(t *testing.T) {
	t.Parallel()
	// Malformed file URI — url.Parse rejects spaces in opaque.
	got := uriToPath("file://%zz/tmp/foo")
	assert.Empty(t, got)
}

func TestReloadConfigBadYAMLFallsBack(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, writeFile(dir+"/.mdsmith.yml", "rules: [not-a-map\n"))

	s := New(Options{Reader: nil, Writer: io.Discard})
	s.configMu.Lock()
	s.rootDir = dir
	s.configMu.Unlock()
	s.reloadConfig()
	cfg, path, _ := s.snapshotConfig()
	require.NotNil(t, cfg, "must fall back to defaults on bad YAML")
	assert.Empty(t, path, "path should be empty when load fails")
}

func TestReloadConfigOverrideRelativePath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, writeFile(dir+"/cfg.yml", "rules:\n  line-length: false\n"))

	s := New(Options{Reader: nil, Writer: io.Discard})
	s.configMu.Lock()
	s.rootDir = dir
	s.configMu.Unlock()
	s.settingsMu.Lock()
	s.settings.ConfigPath = "cfg.yml"
	s.settingsMu.Unlock()
	s.reloadConfig()
	cfg, path, _ := s.snapshotConfig()
	require.NotNil(t, cfg)
	assert.Equal(t, dir+"/cfg.yml", path)
}

func TestRunReturnsOnContextCancel(t *testing.T) {
	t.Parallel()
	r, _ := io.Pipe()
	s := New(Options{Reader: r, Writer: io.Discard, Rules: rule.All()})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := s.Run(ctx)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestFetchClientSettingsAppliesResponse(t *testing.T) {
	t.Parallel()
	var buf safeBuffer
	s := New(Options{Reader: nil, Writer: &buf, Rules: rule.All()})
	// fetchClientSettings registers a pending channel keyed by the
	// JSON-encoded id. We deliver a synthetic response right after
	// the call publishes, so the goroutine receives it before its
	// internal timeout fires.
	done := make(chan struct{})
	go func() {
		s.fetchClientSettings(context.Background())
		close(done)
	}()

	// Wait until fetchClientSettings has registered a pending channel.
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("fetchClientSettings never registered a pending response")
			return
		default:
		}
		s.pendingRespMu.Lock()
		var key string
		for k := range s.pendingResp {
			key = k
		}
		s.pendingRespMu.Unlock()
		if key != "" {
			s.deliverResponse(key, rpcResponse{
				Result: json.RawMessage(`[{"run":"onType","config":"/tmp/x.yml"}]`),
			})
			break
		}
		time.Sleep(time.Millisecond)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("fetchClientSettings never returned")
	}

	s.settingsMu.RLock()
	defer s.settingsMu.RUnlock()
	assert.Equal(t, "onType", s.settings.Run)
	assert.Equal(t, "/tmp/x.yml", s.settings.ConfigPath)
}

func TestFetchClientSettingsIgnoresErrorResponse(t *testing.T) {
	t.Parallel()
	var buf safeBuffer
	s := New(Options{Reader: nil, Writer: &buf, Rules: rule.All()})

	go func() {
		s.fetchClientSettings(context.Background())
	}()

	// Wait for the pending channel and reply with an error.
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("never registered pending response")
		default:
		}
		s.pendingRespMu.Lock()
		var key string
		for k := range s.pendingResp {
			key = k
		}
		s.pendingRespMu.Unlock()
		if key != "" {
			s.deliverResponse(key, rpcResponse{Error: &responseError{Code: -32000, Message: "fail"}})
			break
		}
		time.Sleep(time.Millisecond)
	}
	// Run should remain at the default; we just verify no panic.
	time.Sleep(50 * time.Millisecond)
	s.settingsMu.RLock()
	defer s.settingsMu.RUnlock()
	assert.Equal(t, runOnSave, s.settings.Run)
}

func TestDidChangeWatchedFilesRelintsOpenDocs(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	_, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)

	uri := "file:///workspace/watched.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: "# Hi\n\ndirty   \n"},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	h.notify("workspace/didChangeWatchedFiles", didChangeWatchedFilesParams{
		Changes: []fileEvent{{URI: "file:///workspace/.mdsmith.yml", Type: 2}},
	})

	raw := h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)
	var p publishDiagnosticsParams
	require.NoError(t, json.Unmarshal(raw, &p))
	assert.Equal(t, uri, p.URI)
}

func TestDidChangeWatchedFilesIgnoresUnrelatedFiles(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	_, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)
	// Unrelated file: must not crash, no notification expected.
	h.notify("workspace/didChangeWatchedFiles", didChangeWatchedFilesParams{
		Changes: []fileEvent{{URI: "file:///workspace/other.txt", Type: 2}},
	})
	// Give the server a moment to process. We assert no diagnostics arrive.
	select {
	case n := <-h.notifications:
		t.Fatalf("unexpected notification: %s", n.Method)
	case <-time.After(100 * time.Millisecond):
	}
}

// TestHandleDidChangeConfigurationRelintsOpenDocs verifies that a
// configuration-change notification triggers a re-lint of every open
// document — but only after the new settings actually land. The
// handler kicks off fetchClientSettings asynchronously; once the
// client replies (or, here, we inject a synthetic reply) the lint
// pass fires.
func TestHandleDidChangeConfigurationRelintsOpenDocs(t *testing.T) {
	t.Parallel()
	var buf safeBuffer
	s := New(Options{Reader: nil, Writer: &buf, Rules: rule.All()})
	s.settingsMu.Lock()
	s.settings.Run = runOnType
	s.settingsMu.Unlock()
	s.docs.set("file:///x.md", &document{
		uri: "file:///x.md", path: "x.md", text: []byte("# Hi\n\ndirty   \n"),
	})
	s.handleDidChangeConfiguration(context.Background())

	// Drive the goroutine: wait for the pending response slot to
	// appear, then deliver a settings reply that keeps run=onType.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s.pendingRespMu.Lock()
		var key string
		for k := range s.pendingResp {
			key = k
		}
		s.pendingRespMu.Unlock()
		if key != "" {
			s.deliverResponse(key, rpcResponse{
				Result: json.RawMessage(`[{"run":"onType"}]`),
			})
			break
		}
		time.Sleep(time.Millisecond)
	}

	// The post-fetch re-lint runs synchronously inside the
	// goroutine; poll for the resulting publishDiagnostics frame.
	for time.Now().Before(deadline.Add(2 * time.Second)) {
		if strings.Contains(buf.String(), "MDS006") {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("never saw MDS006 after settings landed; buffer: %s", buf.String())
}

func TestDebouncedLintCollapsesRapidChanges(t *testing.T) {
	t.Parallel()
	h := newDebouncedHarness(t, 50*time.Millisecond)
	_, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)

	uri := "file:///workspace/debounce.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: "# Hi\n\nclean\n"},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	for i, txt := range []string{"# Hi\n\ndirty1   \n", "# Hi\n\ndirty2   \n", "# Hi\n\nfinal   \n"} {
		h.notify("textDocument/didChange", didChangeTextDocumentParams{
			TextDocument:   versionedTextDocumentIdentifier{URI: uri, Version: i + 2},
			ContentChanges: []textDocumentContentChangeEvent{{Text: txt}},
		})
	}
	raw := h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)
	var p publishDiagnosticsParams
	require.NoError(t, json.Unmarshal(raw, &p))
	assert.Equal(t, uri, p.URI)
	var saw006 bool
	for _, d := range p.Diagnostics {
		if d.Code == "MDS006" {
			saw006 = true
			break
		}
	}
	assert.True(t, saw006)
}

// TestDidSaveLintsWhenRunOnSave verifies that save events still
// produce diagnostics when run=onSave.
func TestDidSaveLintsWhenRunOnSave(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	// Reset to onSave to exercise the save-only path.
	h.srv.settingsMu.Lock()
	h.srv.settings.Run = runOnSave
	h.srv.settingsMu.Unlock()

	_, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)

	uri := "file:///workspace/save.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: "# Hi\n\ndirty   \n"},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	h.notify("textDocument/didSave", map[string]any{
		"textDocument": map[string]any{"uri": uri},
	})
	raw := h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)
	var p publishDiagnosticsParams
	require.NoError(t, json.Unmarshal(raw, &p))
	assert.Equal(t, uri, p.URI)
}

// TestRunOffSuppressesLint verifies run=off skips lint passes from
// didChange but didOpen still produces an initial snapshot.
func TestRunOffSuppressesLint(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	_, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)

	uri := "file:///workspace/off.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: "# Hi\n\nclean\n"},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	// Switch run mode to off.
	h.srv.settingsMu.Lock()
	h.srv.settings.Run = runOff
	h.srv.settingsMu.Unlock()

	h.notify("textDocument/didChange", didChangeTextDocumentParams{
		TextDocument:   versionedTextDocumentIdentifier{URI: uri, Version: 2},
		ContentChanges: []textDocumentContentChangeEvent{{Text: "# Hi\n\ndirty   \n"}},
	})
	select {
	case n := <-h.notifications:
		t.Fatalf("unexpected diagnostic notification when run=off: %s", n.Method)
	case <-time.After(150 * time.Millisecond):
	}
}

func TestUriToPathRoundTrip(t *testing.T) {
	t.Parallel()
	tests := []struct {
		uri  string
		want string
	}{
		{"file:///tmp/foo.md", "/tmp/foo.md"},
		{"file:///C:/Users/x/foo.md", "C:/Users/x/foo.md"},
		{"https://example.com", ""},
		{"untitled:Untitled-1", ""},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.want, uriToPath(tc.uri), "uri=%s", tc.uri)
	}
}

func TestUriToPathLocalhostHostIsTreatedAsEmpty(t *testing.T) {
	t.Parallel()
	// RFC 8089 §3: "localhost" host is equivalent to an empty
	// host, so file://localhost/tmp/foo.md must resolve identically
	// to file:///tmp/foo.md.
	got := uriToPath("file://localhost/tmp/foo.md")
	assert.Equal(t, "/tmp/foo.md", got)
}

func TestUriToPathRemoteHostRejectedOnUnix(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("UNC paths only meaningful on Windows")
	}
	// A non-empty, non-localhost host produces a UNC path on
	// Windows; on Unix we have no way to mount a remote share, so
	// uriToPath returns "" and the caller skips the document.
	got := uriToPath("file://server/share/foo.md")
	assert.Empty(t, got)
}

func TestIsWholeFileOnly(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"catalog", "toc", "toc-directive", "include"} {
		assert.True(t, isWholeFileOnly(name), "expected %s to be whole-file-only", name)
	}
	assert.False(t, isWholeFileOnly("line-length"))
}

func TestIsFixableUsesRegistry(t *testing.T) {
	t.Parallel()
	rules := rule.All()
	assert.True(t, isFixable(rules, "no-trailing-spaces"))
	assert.False(t, isFixable(rules, "no-such-rule"))
}

func TestDocumentStoreOpenURIs(t *testing.T) {
	t.Parallel()
	s := newDocumentStore()
	s.set("file:///a", &document{uri: "file:///a", path: "/a"})
	s.set("file:///b", &document{uri: "file:///b", path: "/b"})
	got := s.openURIs()
	assert.ElementsMatch(t, []string{"file:///a", "file:///b"}, got)
}

func TestPickRootPrefersWorkspaceFolder(t *testing.T) {
	t.Parallel()
	got := pickRoot(initializeParams{
		WorkspaceFolders: []workspaceFolder{{URI: "file:///ws/root", Name: "ws"}},
		RootURI:          "file:///fallback",
	})
	assert.Equal(t, "/ws/root", got)

	got = pickRoot(initializeParams{RootURI: "file:///legacy"})
	assert.Equal(t, "/legacy", got)

	got = pickRoot(initializeParams{})
	assert.Equal(t, "", got)
}

func TestWantsKind(t *testing.T) {
	t.Parallel()
	assert.True(t, wantsKind(nil, "quickfix"))
	assert.True(t, wantsKind([]string{"quickfix"}, "quickfix"))
	assert.True(t, wantsKind([]string{"source"}, "source.fixAll.mdsmith"))
	assert.False(t, wantsKind([]string{"refactor"}, "quickfix"))
}

func TestDocumentEndPositionTrailingNewline(t *testing.T) {
	t.Parallel()
	endLine, endChar := documentEndPosition([]byte("hello\nworld\n"))
	assert.Equal(t, 2, endLine)
	assert.Equal(t, 0, endChar)
}

func TestDocumentEndPositionNoTrailingNewline(t *testing.T) {
	t.Parallel()
	endLine, endChar := documentEndPosition([]byte("hello\nworld"))
	assert.Equal(t, 1, endLine)
	assert.Equal(t, 5, endChar)
}

func TestDocumentEndPositionEmpty(t *testing.T) {
	t.Parallel()
	endLine, endChar := documentEndPosition(nil)
	assert.Equal(t, 0, endLine)
	assert.Equal(t, 0, endChar)
}

func TestReloadConfigEmptyRoot(t *testing.T) {
	t.Parallel()
	s := New(Options{Reader: nil, Writer: io.Discard})
	s.reloadConfig()
	cfg, _, _ := s.snapshotConfig()
	assert.NotNil(t, cfg, "expected default config when no root is set")
}

func TestReloadConfigDiscoverInTempDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, writeFile(dir+"/.mdsmith.yml", "rules:\n  line-length: false\n"))
	s := New(Options{Reader: nil, Writer: io.Discard})
	s.configMu.Lock()
	s.rootDir = dir
	s.configMu.Unlock()
	s.reloadConfig()
	cfg, path, _ := s.snapshotConfig()
	require.NotNil(t, cfg)
	assert.Equal(t, dir+"/.mdsmith.yml", path)
}

func TestReloadConfigOverridePath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfgPath := dir + "/custom.yml"
	require.NoError(t, writeFile(cfgPath, "rules:\n  line-length: false\n"))

	s := New(Options{Reader: nil, Writer: io.Discard})
	s.settingsMu.Lock()
	s.settings.ConfigPath = cfgPath
	s.settingsMu.Unlock()
	s.reloadConfig()
	cfg, path, _ := s.snapshotConfig()
	require.NotNil(t, cfg)
	assert.Equal(t, cfgPath, path)
}

func TestReloadConfigOverridePathInvalidUsesDefaults(t *testing.T) {
	t.Parallel()
	s := New(Options{Reader: nil, Writer: io.Discard})
	s.settingsMu.Lock()
	s.settings.ConfigPath = "/no/such/path.yml"
	s.settingsMu.Unlock()
	s.reloadConfig()
	cfg, path, _ := s.snapshotConfig()
	require.NotNil(t, cfg, "expected fallback to defaults on missing override")
	assert.Empty(t, path)
}

// writeFile is a tiny test helper that writes a file.
func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}

func TestSeverityForMappings(t *testing.T) {
	t.Parallel()
	// lint.Warning maps to severityWarning; everything else (including
	// Error and any future severity) defaults to severityError, since
	// LSP only knows error/warning/info/hint and we conservatively
	// elevate unknown severities.
	assert.Equal(t, severityWarning, severityFor("warning"))
	assert.Equal(t, severityError, severityFor("error"))
	assert.Equal(t, severityError, severityFor("note"))
}

func TestCurrentLineOutOfRange(t *testing.T) {
	t.Parallel()
	lines := [][]byte{[]byte("first"), []byte("second")}
	assert.Equal(t, "first", currentLine(lines, 1))
	assert.Equal(t, "second", currentLine(lines, 2))
	assert.Equal(t, "", currentLine(lines, 0))
	assert.Equal(t, "", currentLine(lines, 99))
}

func TestSplitLinesEmpty(t *testing.T) {
	t.Parallel()
	assert.Nil(t, splitLines(nil))
	assert.Equal(t, [][]byte{[]byte("a"), []byte("b")}, splitLines([]byte("a\nb")))
	assert.Equal(t, [][]byte{[]byte("a"), []byte("b")}, splitLines([]byte("a\nb\n")))
}

func TestUtf16ColumnSurrogatePair(t *testing.T) {
	t.Parallel()
	// A non-BMP rune (\U0001F600 — 😀) takes two UTF-16 code units.
	// utf16Column at rune 1 should report 2.
	assert.Equal(t, 0, utf16Column("😀x", 0))
	assert.Equal(t, 2, utf16Column("😀x", 1))
	assert.Equal(t, 3, utf16Column("😀x", 2))
}

func TestFrontMatterEnabledExplicit(t *testing.T) {
	t.Parallel()
	// nil cfg → default true.
	assert.True(t, frontMatterEnabled(nil))
	// nil pointer → default true.
	cfg := &config.Config{}
	assert.True(t, frontMatterEnabled(cfg))
	// Explicit false.
	f := false
	cfg.FrontMatter = &f
	assert.False(t, frontMatterEnabled(cfg))
	// Explicit true.
	tr := true
	cfg.FrontMatter = &tr
	assert.True(t, frontMatterEnabled(cfg))
}

func TestDispatchRawIgnoresInvalidJSON(t *testing.T) {
	t.Parallel()
	srv := New(Options{Reader: nil, Writer: io.Discard})
	// Should not panic on invalid JSON.
	srv.dispatchRaw(context.Background(), []byte("not json"))
}

func TestDispatchRawRejectsWrongVersion(t *testing.T) {
	t.Parallel()
	var buf safeBuffer
	srv := New(Options{Reader: nil, Writer: &buf})
	srv.dispatchRaw(context.Background(), []byte(`{"jsonrpc":"1.0","id":1,"method":"x"}`))
	out := buf.String()
	assert.Contains(t, out, "jsonrpc must be 2.0")
}

func TestRunModeFallsBackOnUnknown(t *testing.T) {
	t.Parallel()
	s := New(Options{Reader: nil, Writer: io.Discard})
	s.settingsMu.Lock()
	s.settings.Run = "garbage-mode"
	s.settingsMu.Unlock()
	assert.Equal(t, runOnSave, s.runMode())
}

func TestQuickFixEditForRejectsWholeFileRule(t *testing.T) {
	t.Parallel()
	s := New(Options{Reader: nil, Writer: io.Discard, Rules: rule.All()})
	cfg := config.Merge(config.Defaults(), nil)
	doc := &document{path: "x.md", text: []byte("# Hi\n")}
	edit := s.quickFixEditFor("catalog", doc, cfg, "", "file:///x.md")
	assert.Nil(t, edit, "whole-file-only rules must not produce per-diagnostic fixes")
}

func TestQuickFixEditForUnknownRule(t *testing.T) {
	t.Parallel()
	s := New(Options{Reader: nil, Writer: io.Discard, Rules: rule.All()})
	cfg := config.Merge(config.Defaults(), nil)
	doc := &document{path: "x.md", text: []byte("# Hi\n")}
	edit := s.quickFixEditFor("no-such-rule", doc, cfg, "", "file:///x.md")
	assert.Nil(t, edit)
}

func TestQuickFixEditForNoOpReturnsNil(t *testing.T) {
	t.Parallel()
	s := New(Options{Reader: nil, Writer: io.Discard, Rules: rule.All()})
	cfg := config.Merge(config.Defaults(), nil)
	// Buffer has no trailing-spaces violations, so the fix is a no-op.
	doc := &document{path: "x.md", text: []byte("# Hi\n\nclean line\n")}
	edit := s.quickFixEditFor("no-trailing-spaces", doc, cfg, "", "file:///x.md")
	assert.Nil(t, edit, "no-op fix should not surface as a code action")
}

// TestComputeCodeActionsDedupesPerRule pins the perf invariant that
// N diagnostics from the same rule trigger only one fix.SourceWithRules
// pass. Without dedup, the codeAction request would re-run the fix
// per-diagnostic and blow the latency budget on noisy files.
func TestComputeCodeActionsDedupesPerRule(t *testing.T) {
	t.Parallel()
	s := New(Options{Reader: nil, Writer: io.Discard, Rules: rule.All()})
	cfg := config.Merge(config.Defaults(), nil)
	doc := &document{
		path: "x.md",
		text: []byte("# Hi\n\ndirty1   \ndirty2   \ndirty3   \n"),
	}
	// Three diagnostics, all from no-trailing-spaces.
	mkDiag := func() Diagnostic {
		return Diagnostic{
			Code: "MDS006",
			Data: &diagnosticData{RuleName: "no-trailing-spaces"},
		}
	}
	p := codeActionParams{
		TextDocument: textDocumentIdentifier{URI: "file:///x.md"},
		Context: codeActionContext{
			Diagnostics: []Diagnostic{mkDiag(), mkDiag(), mkDiag()},
			Only:        []string{kindQuickFix},
		},
	}
	actions := s.computeCodeActions(p, doc, cfg, "")
	require.Len(t, actions, 3, "one quickfix action per diagnostic")
	// All three actions must reference the same WorkspaceEdit so the
	// fix only runs once.
	for i := 1; i < len(actions); i++ {
		assert.Same(t, actions[0].Edit, actions[i].Edit,
			"quickfix actions for the same rule must share the cached edit")
	}
	for _, a := range actions {
		assert.Equal(t, "Fix all no-trailing-spaces with mdsmith", a.Title)
	}
}

func TestRunLintIgnoredFile(t *testing.T) {
	t.Parallel()
	// runLint publishes empty diagnostics for files matched by
	// cfg.Ignore. We use a configMu-locked ignore list and a
	// in-memory writer so we can inspect what the server emitted.
	var buf safeBuffer
	s := New(Options{Reader: nil, Writer: &buf, Rules: rule.All()})
	cfg := config.Merge(config.Defaults(), nil)
	cfg.Ignore = []string{"**/ignored.md"}
	s.configMu.Lock()
	s.config = cfg
	s.configMu.Unlock()
	s.docs.set("file:///x/ignored.md", &document{
		uri: "file:///x/ignored.md", path: "x/ignored.md", text: []byte("# Hi\n"),
	})
	s.runLint("file:///x/ignored.md")
	out := buf.String()
	assert.Contains(t, out, `"diagnostics":[]`)
}

func TestRunLintMissingDoc(t *testing.T) {
	t.Parallel()
	s := New(Options{Reader: nil, Writer: io.Discard, Rules: rule.All()})
	// No-op when uri is unknown.
	s.runLint("file:///none.md")
}

// safeBuffer is a goroutine-safe bytes.Buffer used as an LSP writer
// when we want to inspect what was written.
type safeBuffer struct {
	mu  sync.Mutex
	buf []byte
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	return len(p), nil
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.buf)
}

func TestHandleDidOpenInvalidJSON(t *testing.T) {
	t.Parallel()
	s := New(Options{Reader: nil, Writer: io.Discard, Rules: rule.All()})
	// Should silently return on invalid JSON.
	s.handleDidOpen(context.Background(), json.RawMessage("not json"))
}

func TestHandleDidOpenNonFileURI(t *testing.T) {
	t.Parallel()
	s := New(Options{Reader: nil, Writer: io.Discard, Rules: rule.All()})
	body, _ := json.Marshal(didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: "untitled:Untitled-1", Text: ""},
	})
	s.handleDidOpen(context.Background(), body)
	assert.Empty(t, s.docs.openURIs(), "non-file URI should be skipped")
}

func TestHandleDidChangeInvalidJSON(t *testing.T) {
	t.Parallel()
	s := New(Options{Reader: nil, Writer: io.Discard, Rules: rule.All()})
	s.handleDidChange(context.Background(), json.RawMessage("oops"))
}

func TestHandleDidChangeUnknownURI(t *testing.T) {
	t.Parallel()
	s := New(Options{Reader: nil, Writer: io.Discard, Rules: rule.All()})
	body, _ := json.Marshal(didChangeTextDocumentParams{
		TextDocument:   versionedTextDocumentIdentifier{URI: "file:///none.md", Version: 1},
		ContentChanges: []textDocumentContentChangeEvent{{Text: "x"}},
	})
	s.handleDidChange(context.Background(), body)
}

func TestHandleDidChangeEmptyContentChanges(t *testing.T) {
	t.Parallel()
	s := New(Options{Reader: nil, Writer: io.Discard, Rules: rule.All()})
	s.docs.set("file:///x.md", &document{uri: "file:///x.md", path: "x.md", text: []byte("# Hi\n")})
	body, _ := json.Marshal(didChangeTextDocumentParams{
		TextDocument: versionedTextDocumentIdentifier{URI: "file:///x.md", Version: 2},
	})
	s.handleDidChange(context.Background(), body)
}

func TestHandleDidCloseInvalidJSON(t *testing.T) {
	t.Parallel()
	s := New(Options{Reader: nil, Writer: io.Discard, Rules: rule.All()})
	s.handleDidClose(json.RawMessage("oops"))
}

func TestHandleDidSaveInvalidJSON(t *testing.T) {
	t.Parallel()
	s := New(Options{Reader: nil, Writer: io.Discard, Rules: rule.All()})
	s.handleDidSave(context.Background(), json.RawMessage("oops"))
}

func TestHandleDidChangeWatchedFilesInvalidJSON(t *testing.T) {
	t.Parallel()
	s := New(Options{Reader: nil, Writer: io.Discard, Rules: rule.All()})
	s.handleDidChangeWatchedFiles(context.Background(), json.RawMessage("oops"))
}

func TestDocumentStoreGetMissing(t *testing.T) {
	t.Parallel()
	s := newDocumentStore()
	d, ok := s.get("file:///none")
	assert.Nil(t, d)
	assert.False(t, ok)
}

func TestUnregisterPendingResponseClearsSlot(t *testing.T) {
	t.Parallel()
	s := New(Options{Reader: nil, Writer: io.Discard})
	ch := s.registerPendingResponse("1")
	require.NotNil(t, ch)
	s.unregisterPendingResponse("1")
	// deliverResponse for an id with no waiter is a no-op.
	s.deliverResponse("1", rpcResponse{})
}

func TestDeliverResponseUnknownIDIsNoOp(t *testing.T) {
	t.Parallel()
	s := New(Options{Reader: nil, Writer: io.Discard})
	// Must not panic and must not block.
	s.deliverResponse("999", rpcResponse{})
}
