package lsp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/textproto"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/hexops/gotextdiff"
	"github.com/hexops/gotextdiff/span"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/bytelimit"
	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"

	// Register the production rule set so rule.All() returns what an
	// editor would actually see. The barrel keeps "what rules ship"
	// in a single place rather than duplicating the import list.
	_ "github.com/jeduden/mdsmith/internal/rules/all"
)

// testPollDeadline bounds the busy-wait loops that gate assertions on
// a goroutine's outgoing LSP frame. It is deliberately generous: the
// CI `test` job runs with -covermode=atomic coverage instrumentation
// (~2-3x slower) on a shared runner alongside ~20 other parallel
// jobs, so a 2s window occasionally elapsed before the watched
// goroutine wrote its frame, producing a flake that never reproduced
// locally. The loops still return the instant the condition is met,
// so widening the ceiling costs nothing on the happy path.
const testPollDeadline = 15 * time.Second

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
	// pipeline. Production also defaults to onType now; setting it
	// explicitly keeps the harness independent of that default.
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
	// Mirror what a fully-featured client (VS Code) advertises so
	// handleInitialized fires both fetchClientSettings and
	// registerWatchers.
	s.clientCapsMu.Lock()
	s.clientCaps = clientCapabilities{
		Workspace: &workspaceClientCapabilities{
			Configuration: true,
			DidChangeWatchedFiles: &struct {
				DynamicRegistration bool `json:"dynamicRegistration,omitempty"`
			}{DynamicRegistration: true},
		},
	}
	s.clientCapsMu.Unlock()

	s.handleInitialized(context.Background())

	// reloadConfig populated the config; registerWatchers wrote the
	// registration request synchronously.
	cfg, _, _ := s.snapshotConfig()
	assert.NotNil(t, cfg)
	assert.Contains(t, buf.String(), "client/registerCapability")

	// fetchClientSettings ran in a goroutine. Poll briefly for its
	// outgoing workspace/configuration request before asserting.
	deadline := time.Now().Add(testPollDeadline)
	for time.Now().Before(deadline) {
		if strings.Contains(buf.String(), "workspace/configuration") {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("workspace/configuration never written; buffer: %s", buf.String())
}

// Regression: clients without workspace.configuration must not see
// a workspace/configuration request, and clients without
// didChangeWatchedFiles.dynamicRegistration must not see a
// client/registerCapability request. handleInitialized should
// reload config either way and exit cleanly.
func TestHandleInitializedSkipsWhenCapabilitiesMissing(t *testing.T) {
	t.Parallel()
	var buf safeBuffer
	s := New(Options{Reader: nil, Writer: &buf, Rules: rule.All()})
	// Empty capabilities — Workspace pointer is nil.
	s.handleInitialized(context.Background())
	// Give any rogue goroutine a chance to write before asserting.
	time.Sleep(50 * time.Millisecond)

	out := buf.String()
	assert.NotContains(t, out, "workspace/configuration",
		"must skip fetchClientSettings without workspace.configuration capability")
	assert.NotContains(t, out, "client/registerCapability",
		"must skip registerWatchers without dynamic-registration capability")
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

func TestQuickFixBytesForFixerError(t *testing.T) {
	// Invalid path used to be propagated from the fixer; today it is
	// just a label so the fix typically succeeds. The point of the
	// test is that the helper does not crash on an unusual path.
	t.Parallel()
	s := New(Options{Reader: nil, Writer: io.Discard, Rules: rule.All()})
	cfg := config.Merge(config.Defaults(), nil)
	doc := &document{path: "/no/such/path/x.md", text: []byte("# Hi\n\ndirty   \n")}
	_ = s.quickFixBytesFor("no-trailing-spaces", doc, cfg, "")
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

func TestDispatchRoutesInitializedToHandler(t *testing.T) {
	t.Parallel()
	var buf safeBuffer
	s := New(Options{Reader: nil, Writer: &buf, Rules: rule.All()})
	// handleInitialized gates registerWatchers on the client's
	// dynamic-registration capability; pretend the client supports
	// it so the dispatch route is observable.
	s.clientCapsMu.Lock()
	s.clientCaps = clientCapabilities{
		Workspace: &workspaceClientCapabilities{
			DidChangeWatchedFiles: &struct {
				DynamicRegistration bool `json:"dynamicRegistration,omitempty"`
			}{DynamicRegistration: true},
		},
	}
	s.clientCapsMu.Unlock()
	s.dispatch(context.Background(), &requestMessage{Method: "initialized"})
	// handleInitialized writes the registerWatchers request
	// synchronously. fetchClientSettings runs in a goroutine and
	// may or may not have written by the time we check.
	assert.Contains(t, buf.String(), "client/registerCapability")
}

func TestDispatchRoutesDidChangeConfigurationToHandler(t *testing.T) {
	t.Parallel()
	s := New(Options{Reader: nil, Writer: io.Discard, Rules: rule.All()})
	// handleDidChangeConfiguration spawns fetchClientSettings; we
	// don't wait for the response — covering the dispatch arm is
	// enough.
	s.dispatch(context.Background(), &requestMessage{Method: "workspace/didChangeConfiguration"})
}

func TestDispatchRoutesDidChangeWatchedFilesToHandler(t *testing.T) {
	t.Parallel()
	s := New(Options{Reader: nil, Writer: io.Discard, Rules: rule.All()})
	body, _ := json.Marshal(didChangeWatchedFilesParams{
		Changes: []fileEvent{{URI: "file:///workspace/.mdsmith.yml", Type: 2}},
	})
	s.dispatch(context.Background(), &requestMessage{
		Method: "workspace/didChangeWatchedFiles",
		Params: body,
	})
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

func TestDispatchRawIDOnlyFrameIsInvalidRequest(t *testing.T) {
	t.Parallel()
	var buf safeBuffer
	s := New(Options{Reader: nil, Writer: &buf, Rules: rule.All()})
	// {jsonrpc:2.0, id:1} with no method/result/error is not a valid
	// JSON-RPC frame; the server must reply with -32600 instead of
	// silently routing it to a (non-existent) pending waiter.
	s.dispatchRaw(context.Background(), []byte(`{"jsonrpc":"2.0","id":1}`))
	out := buf.String()
	assert.Contains(t, out, `"code":-32600`)
	assert.Contains(t, out, `"id":1`)
}

func TestDispatchRawErrorOnlyResponseIsRouted(t *testing.T) {
	t.Parallel()
	// A response with `error` (and no `result`) must still be
	// classified as a response and forwarded to the waiter.
	s := New(Options{Reader: nil, Writer: io.Discard, Rules: rule.All()})
	ch := s.registerPendingResponse(`7`)
	defer s.unregisterPendingResponse(`7`)
	s.dispatchRaw(context.Background(),
		[]byte(`{"jsonrpc":"2.0","id":7,"error":{"code":-1,"message":"x"}}`))
	select {
	case resp := <-ch:
		require.NotNil(t, resp.Error)
		assert.Equal(t, -1, resp.Error.Code)
	case <-time.After(time.Second):
		t.Fatalf("error response not delivered")
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
	// Production now defaults to onType, so opt into onSave explicitly
	// to exercise the didChange-skip path this test covers.
	s.settingsMu.Lock()
	s.settings.Run = runOnSave
	s.settingsMu.Unlock()
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
	assert.Equal(t, runOnType, s.runMode())
}

func TestFetchClientSettingsHandlesEmptyArray(t *testing.T) {
	t.Parallel()
	var buf safeBuffer
	s := New(Options{Reader: nil, Writer: &buf, Rules: rule.All()})
	done := make(chan struct{})
	go func() { defer close(done); s.fetchClientSettings(context.Background()) }()
	// Drive the goroutine: deliver an empty array — VS Code returns
	// this when the section has no settings. The call should leave
	// settings unchanged.
	deliverPendingResponse(t, s, json.RawMessage(`[]`), nil)
	// fetchClientSettings is single-shot: when it returns,
	// `done` closes. Wait for that explicitly instead of an
	// unconditional sleep so the test isn't timing-dependent.
	awaitDone(t, done)
	s.settingsMu.RLock()
	defer s.settingsMu.RUnlock()
	assert.Equal(t, runOnType, s.settings.Run)
}

func TestFetchClientSettingsHandlesMalformedResult(t *testing.T) {
	t.Parallel()
	var buf safeBuffer
	s := New(Options{Reader: nil, Writer: &buf, Rules: rule.All()})
	done := make(chan struct{})
	go func() { defer close(done); s.fetchClientSettings(context.Background()) }()
	// Result is not an array — defaults must stand.
	deliverPendingResponse(t, s, json.RawMessage(`"oops"`), nil)
	awaitDone(t, done)
	s.settingsMu.RLock()
	defer s.settingsMu.RUnlock()
	assert.Equal(t, runOnType, s.settings.Run)
}

// deliverPendingResponse polls s.pendingResp for the (single)
// outstanding request id and delivers a response to it. Returns
// the response key for callers that need it.
//
// Replaces the older "sleep + assert" pattern that was flaky
// under load: instead of waiting an arbitrary 50 ms and hoping
// the goroutine got there, we hand-deliver the response and let
// the caller wait on the goroutine's done channel.
func deliverPendingResponse(
	t *testing.T, s *Server, result json.RawMessage, errResp *responseError,
) string {
	t.Helper()
	deadline := time.Now().Add(testPollDeadline)
	for time.Now().Before(deadline) {
		s.pendingRespMu.Lock()
		var key string
		for k := range s.pendingResp {
			key = k
		}
		s.pendingRespMu.Unlock()
		if key != "" {
			s.deliverResponse(key, rpcResponse{Result: result, Error: errResp})
			return key
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("no pending response slot appeared within deadline")
	return ""
}

// awaitDone blocks until the supplied done channel closes,
// failing the test if the deadline elapses first. Used to gate
// assertions on a goroutine's actual return rather than on a
// heuristic sleep.
func awaitDone(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("goroutine did not return within deadline")
	}
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
		[][]byte{[]byte("a")}, "")
	assert.Equal(t, 0, got.Range.Start.Line)
}

// TestToLSP_RelatedInformationAndCodeDescription pins plan 230's wire
// mapping: a navigable related location becomes a relatedInformation
// entry with a resolved file:// URI and 0-based coordinates, the rule
// code gets a derived codeDescription.href, and a file-less label is
// dropped.
func TestToLSP_RelatedInformationAndCodeDescription(t *testing.T) {
	t.Parallel()
	d := lint.Diagnostic{
		Line: 2, Column: 1, RuleID: "MDS020", RuleName: "required-structure",
		Severity: lint.Error, Message: `status: got "x"`,
		RelatedLocations: []lint.RelatedLocation{
			{File: "proto.md", Line: 4, Column: 3, Message: "required by schema"},
			{Message: "inline kind schema"}, // no file → dropped
		},
	}
	got := toLSP(d, [][]byte{[]byte("a"), []byte("status: x")}, "/work")
	require.NotNil(t, got.CodeDescription)
	assert.Equal(t, "https://mdsmith.dev/rules/mds020-required-structure/",
		got.CodeDescription.Href, "derived from the rule ID")
	require.Len(t, got.RelatedInformation, 1, "the file-less label is dropped")
	ri := got.RelatedInformation[0]
	assert.Equal(t, "required by schema", ri.Message)
	assert.Equal(t, 3, ri.Location.Range.Start.Line, "1-based line 4 → 0-based 3")
	assert.Equal(t, 2, ri.Location.Range.Start.Character, "1-based col 3 → 0-based 2")
	assert.True(t, strings.HasPrefix(ri.Location.URI, "file://"))
	assert.Contains(t, ri.Location.URI, "proto.md")
}

// TestToLSP_NoRelatedNoCodeDescription confirms a diagnostic with no
// related locations and an unknown rule ID gets neither field.
func TestToLSP_NoRelatedNoCodeDescription(t *testing.T) {
	t.Parallel()
	got := toLSP(lint.Diagnostic{Line: 1, Column: 1, RuleID: "MDSZZZ"},
		[][]byte{[]byte("x")}, "/w")
	assert.Nil(t, got.RelatedInformation)
	assert.Nil(t, got.CodeDescription)
}

// TestToLSP_DerivesCodeDescriptionForKnownRule confirms a real rule ID
// gets a codeDescription pointing at its website doc page.
func TestToLSP_DerivesCodeDescriptionForKnownRule(t *testing.T) {
	t.Parallel()
	got := toLSP(lint.Diagnostic{Line: 1, Column: 1, RuleID: "MDS001"},
		[][]byte{[]byte("x")}, "/w")
	require.NotNil(t, got.CodeDescription)
	assert.Equal(t, "https://mdsmith.dev/rules/mds001-line-length/",
		got.CodeDescription.Href)
}

// TestToLSP_RelatedFileOnlyAnchorsAtLineZero covers the clamp path: a
// file-only related location (Line and Column unknown / 0) anchors at
// the schema file's first line rather than underflowing to a negative
// coordinate.
func TestToLSP_RelatedFileOnlyAnchorsAtLineZero(t *testing.T) {
	t.Parallel()
	d := lint.Diagnostic{
		Line: 1, Column: 1, RuleID: "MDS020",
		RelatedLocations: []lint.RelatedLocation{
			{File: "proto.md", Message: "required by schema"},
		},
	}
	got := toLSP(d, [][]byte{[]byte("x")}, "/w")
	require.Len(t, got.RelatedInformation, 1)
	assert.Equal(t, 0, got.RelatedInformation[0].Location.Range.Start.Line)
	assert.Equal(t, 0, got.RelatedInformation[0].Location.Range.Start.Character)
}

// TestToLSP_RelatedURIGuards covers relatedURI's safety branches: a
// relative file with no workspace root, a "../" path that escapes the
// root, and an absolute path outside the root are all dropped; an
// absolute path inside the root is kept.
func TestToLSP_RelatedURIGuards(t *testing.T) {
	t.Parallel()
	mk := func(file, root string) []diagnosticRelatedInformation {
		d := lint.Diagnostic{
			Line: 1, Column: 1, RuleID: "MDS020",
			RelatedLocations: []lint.RelatedLocation{{File: file, Line: 2, Message: "x"}},
		}
		return toLSP(d, [][]byte{[]byte("x")}, root).RelatedInformation
	}
	assert.Empty(t, mk("proto.md", ""), "relative file, no root → dropped")
	assert.Empty(t, mk("/abs/proto.md", ""), "absolute file, no root → dropped")
	assert.Empty(t, mk("../../etc/passwd", "/work/project"), "escaping root → dropped")
	assert.Empty(t, mk("/etc/passwd", "/work/project"),
		"absolute path outside root → dropped")
	abs := mk("/work/project/sub/proto.md", "/work/project")
	require.Len(t, abs, 1, "absolute path inside root → kept")
	assert.Contains(t, abs[0].Location.URI, "/work/project/sub/proto.md")
}

// TestToLSPForwardsDeprecationData regresses plan 136: the
// lint.Diagnostic Deprecated / ReplacedBy fields must ride
// through to LSP clients via the diagnostic Data payload so a
// quick-fix surface can recognise a deprecation without parsing
// the message body.
func TestToLSPForwardsDeprecationData(t *testing.T) {
	t.Parallel()
	got := toLSP(lint.Diagnostic{
		Line:       1,
		Column:     1,
		RuleID:     "MDS020",
		RuleName:   "required-structure",
		Severity:   lint.Warning,
		Deprecated: true,
		ReplacedBy: "owner",
		Message:    "legacy_owner: deprecated field",
	}, [][]byte{[]byte("---")}, "")
	require.NotNil(t, got.Data)
	assert.True(t, got.Data.Deprecated)
	assert.Equal(t, "owner", got.Data.ReplacedBy)
	assert.Equal(t, "required-structure", got.Data.RuleName)
}

func TestToLSPEmptyLineProducesZeroWidthRange(t *testing.T) {
	t.Parallel()
	// Diagnostic on an empty line: the range must end at column 0,
	// not column 1 — column 1 would be past the line.
	got := toLSP(
		lint.Diagnostic{Line: 2, Column: 1, RuleID: "MDS001", Severity: lint.Error},
		[][]byte{[]byte("first"), []byte("")},
		"",
	)
	assert.Equal(t, 1, got.Range.Start.Line)
	assert.Equal(t, 0, got.Range.Start.Character)
	assert.Equal(t, 0, got.Range.End.Character,
		"empty line should produce a zero-width range, not extend past the line")
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
		// Regression: an in-root file whose name happens to start
		// with two dots must not be treated as a parent traversal.
		// A naive HasPrefix(rel, "..") would have rejected this.
		{"/repo", "/repo/..foo.md", "..foo.md"},
		{"/repo", "/repo/sub/..bar.md", "sub/..bar.md"},
		// filepath.Rel returns an error when root is relative and
		// path is absolute (it can't relativize without knowing the
		// process cwd). Pin the function's defensive path-pass-
		// through behavior in that case.
		{"rel/dir", "/abs/foo.md", "/abs/foo.md"},
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
	getEntry := func() *pendingLint {
		s.pendingMu.Lock()
		defer s.pendingMu.Unlock()
		return s.pending["file:///x.md"]
	}
	s.scheduleLint("file:///x.md", lintTriggerChange)
	first := getEntry()
	require.NotNil(t, first)
	s.scheduleLint("file:///x.md", lintTriggerChange)
	second := getEntry()
	require.NotNil(t, second)
	assert.NotSame(t, first, second, "replacement must produce a fresh pending entry")
	// stopPendingLints clears for cleanup.
	s.stopPendingLints()
}

// TestShutdownRejectsLaterRequests pins the post-shutdown request
// rejection: once `shutdown` has succeeded, any non-`exit` request
// must respond with InvalidRequest instead of running through the
// dispatch switch (LSP §3.16). Notifications are silently dropped.
func TestShutdownRejectsLaterRequests(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	_, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)
	_, errResp = h.request("shutdown", nil)
	require.Nil(t, errResp)
	// Any request after shutdown must come back with InvalidRequest.
	_, errResp = h.request("textDocument/codeAction", nil)
	require.NotNil(t, errResp)
	assert.Equal(t, codeInvalidRequest, errResp.Code)
	assert.Contains(t, errResp.Message, "shutting down")
}

// TestScheduleLintImmediateCancelsPendingDebounce pins the
// fast-path branch in scheduleLint: when an immediate trigger
// (open/save/config) arrives while a didChange-armed debounce timer
// is still pending for the same URI, the existing timer must be
// stopped and removed so it cannot fire later and republish stale
// diagnostics.
func TestScheduleLintImmediateCancelsPendingDebounce(t *testing.T) {
	t.Parallel()
	s := New(Options{Reader: nil, Writer: io.Discard, Rules: rule.All(), Debounce: 10 * time.Second})
	s.settingsMu.Lock()
	s.settings.Run = runOnType
	s.settingsMu.Unlock()
	s.docs.set("file:///x.md", &document{
		uri: "file:///x.md", path: "x.md", text: []byte("# Hi\n"),
	})
	// Arm a long debounced timer via a didChange-style trigger.
	s.scheduleLint("file:///x.md", lintTriggerChange)
	s.pendingMu.Lock()
	require.Len(t, s.pending, 1)
	originalEntry := s.pending["file:///x.md"]
	s.pendingMu.Unlock()
	// An immediate trigger (e.g. didOpen) replaces the pending
	// debounce timer with a zero-delay immediate timer instead of
	// running runLint synchronously (which would block dispatch).
	// The original timer is Stop()'d.
	s.scheduleLint("file:///x.md", lintTriggerOpen)
	s.pendingMu.Lock()
	require.Len(t, s.pending, 1, "immediate trigger should replace the pending debounce entry")
	require.NotSame(t, originalEntry, s.pending["file:///x.md"],
		"immediate trigger should install a new (zero-delay) entry, not reuse the debounce one")
	s.pendingMu.Unlock()
	// Wait briefly for the immediate timer to fire and clear
	// itself, then assert the pending map is empty.
	deadline := time.Now().Add(testPollDeadline)
	for time.Now().Before(deadline) {
		s.pendingMu.Lock()
		empty := len(s.pending) == 0
		s.pendingMu.Unlock()
		if empty {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("pending map never drained after immediate trigger fired")
}

// TestScheduleLintTimerSkipsAfterShutdown pins the in-callback
// shutdown re-check: a debounce timer that armed before shutdown
// but fires after must not call runLint or publish diagnostics.
func TestScheduleLintTimerSkipsAfterShutdown(t *testing.T) {
	t.Parallel()
	var buf safeBuffer
	s := New(Options{Reader: nil, Writer: &buf, Rules: rule.All(), Debounce: 5 * time.Millisecond})
	s.settingsMu.Lock()
	s.settings.Run = runOnType
	s.settingsMu.Unlock()
	s.docs.set("file:///x.md", &document{
		uri: "file:///x.md", path: "x.md", text: []byte("# Hi\n\ndirty   \n"),
	})
	s.scheduleLint("file:///x.md", lintTriggerChange)
	// Flip shutdown before the timer can fire so the callback's
	// re-check returns early.
	s.shutdown.Store(true)
	// Wait long enough for the timer to elapse.
	time.Sleep(50 * time.Millisecond)
	assert.NotContains(t, buf.String(), "publishDiagnostics",
		"a timer firing after shutdown must not publish diagnostics")
}

// runLintIfCurrentFixture spins up a Server configured for the
// three runLintIfCurrent unit tests below. It uses a long debounce
// so any timer minted only as a *Timer placeholder never fires on
// its own — the tests invoke runLintIfCurrent directly to exercise
// each branch deterministically.
func runLintIfCurrentFixture(t *testing.T) (*Server, *safeBuffer) {
	t.Helper()
	buf := &safeBuffer{}
	// `bytes.NewReader(nil)` is an inert reader. The Run loop never
	// starts in these tests, so the transport never reads, but other
	// LSP unit tests in this file plumb a real reader and we follow
	// the same convention so a future runLint that touches the
	// transport cannot panic with a nil-reader dereference.
	s := New(Options{Reader: bytes.NewReader(nil), Writer: buf, Rules: rule.All(), Debounce: time.Hour})
	s.settingsMu.Lock()
	s.settings.Run = runOnType
	s.settingsMu.Unlock()
	s.docs.set("file:///x.md", &document{
		uri: "file:///x.md", path: "x.md", text: []byte("# Hi\n\ndirty   \n"),
	})
	return s, buf
}

// newPendingEntry mints a fresh *pendingLint backed by a non-firing
// timer. time.AfterFunc is used (over time.NewTimer) because it
// matches what scheduleLint creates, has no Timer.C channel that
// would need draining if Stop loses the race, and only ever
// allocates a callback goroutine if the timer fires — which a 1h
// delay plus Cleanup-time Stop guarantees does not happen.
func newPendingEntry(t *testing.T) *pendingLint {
	t.Helper()
	p := &pendingLint{timer: time.AfterFunc(time.Hour, func() {})}
	t.Cleanup(func() { p.timer.Stop() })
	return p
}

// TestRunLintIfCurrentRunsWhenLive covers the happy path: the
// pendingLint passed in is still registered in pending, so the
// call clears pending[uri] and forwards to runLint, which
// publishes diagnostics for the document.
func TestRunLintIfCurrentRunsWhenLive(t *testing.T) {
	// Not t.Parallel(): runLint reaches into the toc / include rule
	// engines, which have lazy init that races under concurrent
	// callers. Running these three runLintIfCurrent tests
	// sequentially keeps `-race` clean without re-engineering the
	// rule lazy init.
	s, buf := runLintIfCurrentFixture(t)
	p := newPendingEntry(t)
	s.pendingMu.Lock()
	s.pending["file:///x.md"] = p
	s.pendingMu.Unlock()

	s.runLintIfCurrent("file:///x.md", p)

	s.pendingMu.Lock()
	_, stillPending := s.pending["file:///x.md"]
	s.pendingMu.Unlock()
	assert.False(t, stillPending, "live=true must delete pending[uri]")
	assert.Contains(t, buf.String(), "publishDiagnostics",
		"live=true must reach runLint and publish diagnostics")
}

// TestRunLintIfCurrentSkipsWhenReplaced covers the race-replaced
// path: a newer scheduleLint has already swapped pending[uri] to a
// different entry, so the stale callback must return without
// touching pending and without publishing.
func TestRunLintIfCurrentSkipsWhenReplaced(t *testing.T) {
	// See TestRunLintIfCurrentRunsWhenLive for the rationale on
	// dropping t.Parallel().
	s, buf := runLintIfCurrentFixture(t)
	stale := newPendingEntry(t)
	current := newPendingEntry(t)
	s.pendingMu.Lock()
	s.pending["file:///x.md"] = current
	s.pendingMu.Unlock()

	s.runLintIfCurrent("file:///x.md", stale)

	s.pendingMu.Lock()
	got := s.pending["file:///x.md"]
	s.pendingMu.Unlock()
	assert.Same(t, current, got,
		"stale callback must not touch the new pending entry")
	assert.NotContains(t, buf.String(), "publishDiagnostics",
		"stale callback must not reach runLint")
}

// TestRunLintIfCurrentShortCircuitsOnShutdown covers the top-of-
// callback shutdown re-check: a callback whose AfterFunc fires
// after Run's deferred cleanup must return before touching pending
// or runLint, even when pending[uri] still points at this entry.
func TestRunLintIfCurrentShortCircuitsOnShutdown(t *testing.T) {
	// See TestRunLintIfCurrentRunsWhenLive for the rationale on
	// dropping t.Parallel().
	s, buf := runLintIfCurrentFixture(t)
	p := newPendingEntry(t)
	s.pendingMu.Lock()
	s.pending["file:///x.md"] = p
	s.pendingMu.Unlock()

	s.shutdown.Store(true)
	s.runLintIfCurrent("file:///x.md", p)

	s.pendingMu.Lock()
	got := s.pending["file:///x.md"]
	s.pendingMu.Unlock()
	assert.Same(t, p, got,
		"shutdown re-check must return before mutating pending")
	assert.NotContains(t, buf.String(), "publishDiagnostics",
		"shutdown re-check must return before runLint")
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

func TestRunSurfacesTransportWriteError(t *testing.T) {
	t.Parallel()
	// Drive a single initialize request through the server with a
	// writer that always fails. The dispatch handler will try to
	// write the response, fail, and Run must return the recorded
	// transport error rather than silently looping forever.
	srvIn, clientWriter := io.Pipe()
	defer func() { _ = clientWriter.Close() }()
	s := New(Options{Reader: srvIn, Writer: failingWriter{}, Rules: rule.All()})

	done := make(chan error, 1)
	go func() { done <- s.Run(context.Background()) }()

	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`
	_, err := clientWriter.Write([]byte("Content-Length: " + strconv.Itoa(len(body)) + "\r\n\r\n" + body))
	require.NoError(t, err)

	select {
	case err := <-done:
		require.Error(t, err)
		assert.ErrorIs(t, err, io.ErrShortWrite)
	case <-time.After(2 * time.Second):
		t.Fatalf("Run did not return after a transport write failure")
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

func TestReloadConfigDiscoverEmptyFallsBack(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// No .mdsmith.yml under dir, and no .git boundary either; the
	// discover walk hits the filesystem root without finding a
	// config and returns "" — the function should fall back to the
	// defaults rather than crashing or holding stale state.
	s := New(Options{Reader: nil, Writer: io.Discard})
	s.configMu.Lock()
	s.rootDir = dir
	s.configMu.Unlock()
	s.reloadConfig()
	cfg, path, _ := s.snapshotConfig()
	require.NotNil(t, cfg, "must fall back to defaults when no config is discovered")
	assert.Empty(t, path, "configPath should be empty when discovery returns nothing")
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

// Regression: a config.Discover error path also surfaces via
// window/logMessage, not just a load error. The Discover
// implementation almost never fails in practice (only when
// filepath.Abs cannot resolve a relative path), but the branch
// must still report — otherwise an unreadable workspace silently
// falls back to defaults.
func TestReloadConfigSurfacesDiscoverFailure(t *testing.T) {
	t.Parallel()
	var buf safeBuffer
	s := New(Options{Reader: nil, Writer: &buf})
	s.discoverConfig = func(string) (string, error) {
		return "", errors.New("synthetic discover failure")
	}
	s.configMu.Lock()
	s.rootDir = "/some/root"
	s.configMu.Unlock()
	s.reloadConfig()

	out := buf.String()
	assert.Contains(t, out, `"window/logMessage"`)
	assert.Contains(t, out, "discovering")
	assert.Contains(t, out, "synthetic discover failure")
}

// Regression: reloadConfig must surface load failures via
// window/logMessage instead of silently falling back to defaults,
// so the editor user can diagnose misconfiguration.
func TestReloadConfigSurfacesLoadFailure(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, writeFile(dir+"/.mdsmith.yml", "rules: [not-a-map\n"))

	var buf safeBuffer
	s := New(Options{Reader: nil, Writer: &buf})
	s.configMu.Lock()
	s.rootDir = dir
	s.configMu.Unlock()
	s.reloadConfig()

	out := buf.String()
	assert.Contains(t, out, `"window/logMessage"`)
	assert.Contains(t, out, "loading")
	assert.Contains(t, out, ".mdsmith.yml")
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

// Regression: when the workspace folder is a subdirectory of the
// repo and `mdsmith.config` points to a config one level up, the
// effective root returned by snapshotConfig must be the config's
// directory (matching the CLI's rootDirFromConfig). Otherwise
// ignore globs / RootDir-relative rule behavior drift between LSP
// and `mdsmith check`.
func TestSnapshotConfigRootMatchesConfigDir(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	sub := repo + "/sub"
	require.NoError(t, os.MkdirAll(sub, 0o755))
	require.NoError(t, writeFile(repo+"/.mdsmith.yml", "rules: {}\n"))

	s := New(Options{Reader: nil, Writer: io.Discard})
	s.configMu.Lock()
	s.rootDir = sub
	s.configMu.Unlock()
	s.settingsMu.Lock()
	s.settings.ConfigPath = repo + "/.mdsmith.yml"
	s.settingsMu.Unlock()
	s.reloadConfig()

	_, path, root := s.snapshotConfig()
	assert.Equal(t, repo+"/.mdsmith.yml", path)
	assert.Equal(t, repo, root, "root must follow the config file, not the workspace folder")
}

// When no config is loaded, snapshotConfig falls back to the
// workspace folder root that initialize was given.
func TestSnapshotConfigRootFallsBackToWorkspace(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s := New(Options{Reader: nil, Writer: io.Discard})
	s.configMu.Lock()
	s.rootDir = dir
	s.configMu.Unlock()
	s.reloadConfig() // no config in dir → configPath stays empty

	_, path, root := s.snapshotConfig()
	assert.Equal(t, "", path)
	assert.Equal(t, dir, root)
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
	done := make(chan struct{})
	go func() { defer close(done); s.fetchClientSettings(context.Background()) }()
	deliverPendingResponse(t, s,
		json.RawMessage(`[{"run":"onType","config":"/tmp/x.yml"}]`), nil)
	awaitDone(t, done)

	s.settingsMu.RLock()
	defer s.settingsMu.RUnlock()
	assert.Equal(t, "onType", s.settings.Run)
	assert.Equal(t, "/tmp/x.yml", s.settings.ConfigPath)
}

func TestFetchClientSettingsOffClearsDiagnostics(t *testing.T) {
	t.Parallel()
	var buf safeBuffer
	s := New(Options{Reader: nil, Writer: &buf, Rules: rule.All()})
	// Seed an open document with a previously published diagnostic, as
	// if an onType/onSave lint had already run and shown a squiggle.
	uri := "file:///x.md"
	s.docs.set(uri, &document{
		uri: uri, path: "x.md", text: []byte("# Hi\n"), version: 1,
	})
	s.diagsMu.Lock()
	s.diags[uri] = []Diagnostic{{Message: "stale"}}
	s.diagsMu.Unlock()

	// Client switches mdsmith.run to off.
	done := make(chan struct{})
	go func() { defer close(done); s.fetchClientSettings(context.Background()) }()
	deliverPendingResponse(t, s, json.RawMessage(`[{"run":"off"}]`), nil)
	awaitDone(t, done)

	// off is a master switch: cached diagnostics for the open document
	// must be dropped and an empty publishDiagnostics sent so the editor
	// removes the squiggles instead of leaving them until the buffer is
	// closed.
	s.diagsMu.RLock()
	got := s.diags[uri]
	s.diagsMu.RUnlock()
	assert.Empty(t, got, "switching to run=off must clear cached diagnostics")
	assert.Contains(t, buf.String(), `"diagnostics":[]`,
		"switching to run=off must publish empty diagnostics")
}

func TestClearOpenDiagnosticsCancelsPendingLint(t *testing.T) {
	t.Parallel()
	var buf safeBuffer
	// Long debounce so the armed timer never fires during the test —
	// clearOpenDiagnostics must cancel it, not race it.
	s := New(Options{Reader: nil, Writer: &buf, Rules: rule.All(), Debounce: 10 * time.Second})
	uri := "file:///x.md"
	s.docs.set(uri, &document{
		uri: uri, path: "x.md", text: []byte("# Hi\n"), version: 2,
	})
	// Arm a debounced lint (onType) and seed a cached diagnostic.
	s.settingsMu.Lock()
	s.settings.Run = runOnType
	s.settingsMu.Unlock()
	s.scheduleLint(uri, lintTriggerChange)
	s.pendingMu.Lock()
	_, armed := s.pending[uri]
	s.pendingMu.Unlock()
	require.True(t, armed, "precondition: a debounce timer is armed")
	s.diagsMu.Lock()
	s.diags[uri] = []Diagnostic{{Message: "stale"}}
	s.diagsMu.Unlock()

	s.clearOpenDiagnostics()

	s.pendingMu.Lock()
	_, stillArmed := s.pending[uri]
	s.pendingMu.Unlock()
	assert.False(t, stillArmed, "clearOpenDiagnostics must cancel the pending lint")
	s.diagsMu.RLock()
	_, cached := s.diags[uri]
	s.diagsMu.RUnlock()
	assert.False(t, cached, "clearOpenDiagnostics must drop cached diagnostics")
	assert.Contains(t, buf.String(), `"diagnostics":[]`,
		"clearOpenDiagnostics must publish empty diagnostics")
}

func TestRunLintSkipsPublishWhenOff(t *testing.T) {
	t.Parallel()
	var buf safeBuffer
	s := New(Options{Reader: nil, Writer: &buf, Rules: rule.All()})
	// off is reachable inside runLint only via the race the guard
	// covers: a lint already in flight when the user switches to off.
	s.settingsMu.Lock()
	s.settings.Run = runOff
	s.settingsMu.Unlock()
	uri := "file:///x.md"
	s.docs.set(uri, &document{
		uri: uri, path: "x.md", text: []byte("# Hi\n\ndirty   \n"), version: 1,
	})
	s.runLint(uri)
	assert.NotContains(t, buf.String(), "publishDiagnostics",
		"runLint must not publish while run mode is off")
	s.diagsMu.RLock()
	_, cached := s.diags[uri]
	s.diagsMu.RUnlock()
	assert.False(t, cached, "runLint must not cache diagnostics while run mode is off")
}

func TestFetchClientSettingsIgnoresErrorResponse(t *testing.T) {
	t.Parallel()
	var buf safeBuffer
	s := New(Options{Reader: nil, Writer: &buf, Rules: rule.All()})
	done := make(chan struct{})
	go func() { defer close(done); s.fetchClientSettings(context.Background()) }()
	deliverPendingResponse(t, s, nil, &responseError{Code: -32000, Message: "fail"})
	awaitDone(t, done)
	// Run should remain at the default; we just verify no panic.
	s.settingsMu.RLock()
	defer s.settingsMu.RUnlock()
	assert.Equal(t, runOnType, s.settings.Run)
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
	deadline := time.Now().Add(testPollDeadline)
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
	for time.Now().Before(deadline.Add(testPollDeadline)) {
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

// TestRunOffSuppressesLint pins the contract that flipping
// mdsmith.run to "off" stops the server from publishing further
// diagnostics on subsequent didChange events. The initial open is
// linted under whatever mode was active at didOpen (here the test
// harness's onType default), so we await + drop that publish before
// flipping the setting; the assertion is that no further publish
// arrives during the brief watch window. scheduleLint's runOff
// short-circuit covers didOpen too — see the runMode docs for the
// full table.
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
	// The drive-letter strip is gated on runtime.GOOS, so the
	// expected output for `file:///C:/...` differs across platforms.
	driveURI := "file:///C:/Users/x/foo.md"
	driveWant := "/C:/Users/x/foo.md"
	if runtime.GOOS == "windows" {
		driveWant = "C:/Users/x/foo.md"
	}
	tests := []struct {
		uri  string
		want string
	}{
		{"file:///tmp/foo.md", "/tmp/foo.md"},
		{driveURI, driveWant},
		{"https://example.com", ""},
		{"untitled:Untitled-1", ""},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.want, uriToPath(tc.uri), "uri=%s", tc.uri)
	}
}

func TestUriToPathLeavesNonDriveColonPathUnchanged(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("Unix-only: drive-letter check would fire on Windows")
	}
	// On Unix, a path like `/a:/tmp/file.md` is a perfectly valid
	// absolute path. The drive-letter heuristic must not strip its
	// leading slash; otherwise the result becomes a relative path
	// that loses its anchoring.
	got := uriToPath("file:///a:/tmp/file.md")
	assert.Equal(t, "/a:/tmp/file.md", got)
}

func TestHasDriveLetterPrefix(t *testing.T) {
	t.Parallel()
	assert.True(t, hasDriveLetterPrefix("/C:/foo"))
	assert.True(t, hasDriveLetterPrefix("/c:/foo"))
	assert.True(t, hasDriveLetterPrefix("/Z:"))
	assert.False(t, hasDriveLetterPrefix("/0:/foo"))
	assert.False(t, hasDriveLetterPrefix("/abc:/foo"))
	assert.False(t, hasDriveLetterPrefix("C:/foo"))
	assert.False(t, hasDriveLetterPrefix(""))
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

// Cover the Windows branches of uriToPath from any platform by
// driving uriToPathOnOS directly with goos="windows".
func TestUriToPathOnWindowsHostBecomesUNC(t *testing.T) {
	t.Parallel()
	got := uriToPathOnOS("file://server/share/foo.md", "windows")
	// Windows UNC join produces \\server\share\foo.md (filepath.Clean
	// is platform-dependent; on Linux it returns / separators, but
	// the leading double-backslash and host segment are what matter).
	assert.Contains(t, got, "server")
	assert.Contains(t, got, "share")
	assert.Contains(t, got, "foo.md")
}

func TestUriToPathOnWindowsStripsDriveLetterSlash(t *testing.T) {
	t.Parallel()
	got := uriToPathOnOS("file:///C:/Users/me/foo.md", "windows")
	// The leading slash before "C:" is stripped on Windows so the
	// path is a real Windows path. Use ToSlash to make the assertion
	// portable across the test runner's platform.
	assert.Equal(t, "C:/Users/me/foo.md", filepath.ToSlash(got))
}

func TestUriToPathOnLinuxLeavesDriveLetterAlone(t *testing.T) {
	t.Parallel()
	// On non-Windows the drive-letter rewrite must not fire — the
	// path stays as-is, including the leading slash.
	got := uriToPathOnOS("file:///C:/Users/me/foo.md", "linux")
	assert.Equal(t, "/C:/Users/me/foo.md", got)
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
	fallback := "file:///fallback"
	legacy := "file:///legacy"
	got := pickRoot(initializeParams{
		WorkspaceFolders: []workspaceFolder{{URI: "file:///ws/root", Name: "ws"}},
		RootURI:          &fallback,
	})
	assert.Equal(t, "/ws/root", got)

	got = pickRoot(initializeParams{RootURI: &legacy})
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

// TestReloadConfigOnReloadHookFires ensures the OnConfigReload hook
// is invoked when the resolved config path changes. The CLI uses
// this to keep the include-extract projector pointing at the active
// config so `<?include extract:?>` directives produce the same
// diagnostics in the editor as `mdsmith check` does on the CLI;
// without this wiring the LSP would always emit "no extract
// projector is installed" diagnostics.
//
// A no-op reload (no path change) must NOT fire the hook — the host
// installs a closure that captures the current cfgPath, and
// re-installing the same closure on every settings refresh would
// take the projector's write lock for nothing.
func TestReloadConfigOnReloadHookFires(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, writeFile(dir+"/.mdsmith.yml", "rules:\n  line-length: false\n"))

	var gotPath string
	var calls int
	s := New(Options{
		Reader: nil, Writer: io.Discard,
		OnConfigReload: func(cfgPath string) {
			gotPath = cfgPath
			calls++
		},
	})
	s.configMu.Lock()
	s.rootDir = dir
	s.configMu.Unlock()

	s.reloadConfig()
	assert.Equal(t, 1, calls)
	assert.Equal(t, dir+"/.mdsmith.yml", gotPath)

	// A second reload with no path change must NOT re-fire the hook.
	s.reloadConfig()
	assert.Equal(t, 1, calls,
		"hook should fire only on path changes, not on every reload")

	// Move the resolved path (point at a different config). The hook
	// fires again with the new path so the host can re-install.
	otherDir := t.TempDir()
	require.NoError(t, writeFile(otherDir+"/.mdsmith.yml", "rules:\n  line-length: false\n"))
	s.configMu.Lock()
	s.rootDir = otherDir
	s.configMu.Unlock()

	s.reloadConfig()
	assert.Equal(t, 2, calls)
	assert.Equal(t, otherDir+"/.mdsmith.yml", gotPath)
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

func TestCurrentLineBytesOutOfRange(t *testing.T) {
	t.Parallel()
	lines := [][]byte{[]byte("first"), []byte("second")}
	assert.Equal(t, []byte("first"), currentLineBytes(lines, 1))
	assert.Equal(t, []byte("second"), currentLineBytes(lines, 2))
	assert.Nil(t, currentLineBytes(lines, 0))
	assert.Nil(t, currentLineBytes(lines, 99))
}

func TestSplitLines(t *testing.T) {
	t.Parallel()
	// Empty input → 1-element slice with an empty line, matching
	// bytes.Split's contract and therefore lint.File.Lines. The
	// LSP toLSP path uses currentLineBytes(lines, n) with 1-based
	// line numbers, so an empty buffer must report at least one
	// line or a diagnostic anchored at line 1 would fall through
	// the out-of-range guard and clamp to the wrong column.
	assert.Equal(t, [][]byte{nil}, splitLines(nil))
	assert.Equal(t, [][]byte{nil}, splitLines([]byte{}))
	// No trailing newline → N parts.
	assert.Equal(t, [][]byte{[]byte("a"), []byte("b")}, splitLines([]byte("a\nb")))
	// Trailing newline → N+1 parts (preserves the empty line so
	// indexing matches lint.File.Lines / bytes.Split).
	assert.Equal(t,
		[][]byte{[]byte("a"), []byte("b"), {}},
		splitLines([]byte("a\nb\n")))
	// CRLF endings strip per-line CR.
	assert.Equal(t, [][]byte{[]byte("a"), []byte("b")}, splitLines([]byte("a\r\nb")))
}

// TestToLSPMultiByteRuneBeforeColumn pins the byte-vs-rune fix for
// lint.Diagnostic.Column. The column is a 1-based UTF-8 byte
// position, so a diagnostic anchored "after the é" must report a
// UTF-16 character offset that accounts for the 2-byte encoding of
// é but only 1 UTF-16 unit. Treating Column as a rune offset would
// have produced character 2 (off by one too far) in this scenario.
func TestToLSPMultiByteRuneBeforeColumn(t *testing.T) {
	t.Parallel()
	// "é" is 0xC3 0xA9 in UTF-8 (2 bytes, 1 UTF-16 unit). The 'x'
	// starts at byte offset 2 → mdsmith Column = 3 (1-based).
	line := []byte("éx")
	got := toLSP(
		lint.Diagnostic{Line: 1, Column: 3, RuleID: "MDS001", Severity: lint.Error},
		[][]byte{line},
		"",
	)
	assert.Equal(t, 1, got.Range.Start.Character,
		"Column 3 (byte offset 2 = after é) should map to UTF-16 character 1")
}

// TestToLSPSurrogatePairBeforeColumn pins the same byte-offset
// contract for runes that take 2 UTF-16 code units (non-BMP).
func TestToLSPSurrogatePairBeforeColumn(t *testing.T) {
	t.Parallel()
	// "😀" is 4 UTF-8 bytes / 2 UTF-16 units. Column 5 = byte
	// offset 4 = after the emoji.
	line := []byte("😀x")
	got := toLSP(
		lint.Diagnostic{Line: 1, Column: 5, RuleID: "MDS001", Severity: lint.Error},
		[][]byte{line},
		"",
	)
	assert.Equal(t, 2, got.Range.Start.Character,
		"after a non-BMP rune the start character should advance by 2 UTF-16 units")
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
	assert.Equal(t, runOnType, s.runMode())
}

// Regression: catalog/toc/include used to be excluded from
// per-diagnostic Quick Fix actions on the (incorrect) theory
// that their fixes "invite partial regenerations". They produce
// whole-file fixes the same way every other rule does — and the
// action title says "Fix all <rule> with mdsmith" already — so
// there's no scope ambiguity. Excluding them left users with
// only the AI extension's "Fix" entry visible in the lightbulb
// menu when clicking a stale catalog/toc/include diagnostic.
func TestQuickFixBytesForCatalogProducesEdit(t *testing.T) {
	t.Parallel()
	s := New(Options{Reader: nil, Writer: io.Discard, Rules: rule.All()})
	cfg := config.Merge(config.Defaults(), nil)

	// Use a real on-disk directory with a matching file so the catalog
	// glob resolves to a non-empty result. The quick-fix then produces a
	// genuine regeneration edit (a stale row → the correct row) rather
	// than relying on the catalog being emptied: a populated catalog
	// whose glob matches zero files is no longer auto-emptied, so an
	// absolute path and a real match are needed to exercise the fix.
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.md"), []byte("# A\n"), 0o644))
	docPath := filepath.Join(dir, "x.md")
	stale := []byte("# Doc\n\n<?catalog\nglob: [\"a.md\"]\n?>\nold body\n<?/catalog?>\n")
	doc := &document{path: docPath, text: stale}

	// quickFixBytesFor now resolves the fix through the per-workspace
	// session, which is rooted at the server's workspace root. Set it to
	// dir so the catalog's `a.md` glob resolves against the on-disk file.
	s.configMu.Lock()
	s.rootDir = dir
	s.configMu.Unlock()

	fixed := s.quickFixBytesFor("catalog", doc, cfg, dir)
	require.NotNil(t, fixed, "catalog must surface a quick-fix action; "+
		"users expect mdsmith fix in the Quick Fix lightbulb menu")
	assert.Contains(t, string(fixed), "a.md",
		"regenerated catalog body should list the matched file")
	uri := pathToURI(docPath)
	edit := fullFileEdit(uri, doc.text, fixed)
	require.Contains(t, edit.Changes, uri)
}

func TestQuickFixBytesForUnknownRule(t *testing.T) {
	t.Parallel()
	s := New(Options{Reader: nil, Writer: io.Discard, Rules: rule.All()})
	cfg := config.Merge(config.Defaults(), nil)
	doc := &document{path: "x.md", text: []byte("# Hi\n")}
	fixed := s.quickFixBytesFor("no-such-rule", doc, cfg, "")
	assert.Nil(t, fixed)
}

func TestQuickFixBytesForNoOpReturnsNil(t *testing.T) {
	t.Parallel()
	s := New(Options{Reader: nil, Writer: io.Discard, Rules: rule.All()})
	cfg := config.Merge(config.Defaults(), nil)
	// Buffer has no trailing-spaces violations, so the fix is a no-op.
	doc := &document{path: "x.md", text: []byte("# Hi\n\nclean line\n")}
	fixed := s.quickFixBytesFor("no-trailing-spaces", doc, cfg, "")
	assert.Nil(t, fixed, "no-op fix should not surface as a code action")
}

// TestComputeCodeActionsDedupesPerRule pins the perf invariant that
// N diagnostics from the same rule trigger only one fix.SourceWithRules
// pass. Without dedup, the codeAction request would re-run the fix
// per-diagnostic and blow the latency budget on noisy files.
// TestHandleCodeActionRespectsIgnoreList pins the contract that
// VS Code's `source.fixAll.mdsmith` (which can fire on save even
// when no diagnostics were published) does not rewrite a buffer
// that the project's ignore globs would skip in `mdsmith fix`.
func TestHandleCodeActionRespectsIgnoreList(t *testing.T) {
	t.Parallel()
	var buf safeBuffer
	s := New(Options{Reader: nil, Writer: &buf, Rules: rule.All()})

	// Wire a config that ignores the doc's path; share rootDir so
	// the relative path inside computeCodeActions matches the glob.
	cfg := config.Merge(config.Defaults(), nil)
	cfg.Ignore = []string{"ignored.md"}
	s.configMu.Lock()
	s.config = cfg
	s.rootDir = "/repo"
	s.configMu.Unlock()
	s.docs.set("file:///repo/ignored.md", &document{
		uri:  "file:///repo/ignored.md",
		path: "/repo/ignored.md",
		text: []byte("# Hi\n\ndirty   \n"),
	})

	body, _ := json.Marshal(codeActionParams{
		TextDocument: textDocumentIdentifier{URI: "file:///repo/ignored.md"},
		Context: codeActionContext{
			Diagnostics: []Diagnostic{{
				Code: "MDS006",
				Data: &diagnosticData{RuleName: "no-trailing-spaces"},
			}},
		},
	})
	s.handleCodeAction(&requestMessage{ID: json.RawMessage(`1`), Params: body})
	// Empty action array — no quick fixes, no fix-all.
	assert.Contains(t, buf.String(), `"result":[]`)
}

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

// ---- previewFix / ChangeAnnotation tests ----

// helper: build a server with previewFix on and the supplied workspace
// edit capabilities.
func newPreviewServer(t *testing.T, docChanges, changeAnnotSupport bool) *Server {
	t.Helper()
	s := New(Options{Reader: nil, Writer: io.Discard, Rules: rule.All()})
	s.settingsMu.Lock()
	s.settings.PreviewFix = true
	s.settingsMu.Unlock()
	var we *workspaceEditCapabilities
	if docChanges || changeAnnotSupport {
		we = &workspaceEditCapabilities{DocumentChanges: docChanges}
		if changeAnnotSupport {
			we.ChangeAnnotationSupport = &changeAnnotationSupportCap{}
		}
	}
	s.clientCapsMu.Lock()
	s.clientCaps = clientCapabilities{
		Workspace: &workspaceClientCapabilities{WorkspaceEdit: we},
	}
	s.clientCapsMu.Unlock()
	return s
}

// TestPreviewFixLegacyFallbackWhenCapsMissing verifies that when
// previewFix is on but the client lacks capability, edits stay in the
// legacy changes form (not documentChanges).
func TestPreviewFixLegacyFallbackWhenCapsMissing(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name               string
		docChanges         bool
		changeAnnotSupport bool
	}{
		{"no caps at all", false, false},
		{"documentChanges only", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := newPreviewServer(t, tc.docChanges, tc.changeAnnotSupport)
			cfg := config.Merge(config.Defaults(), nil)
			doc := &document{path: "x.md", text: []byte("# Hi\n\ndirty   \n")}
			p := codeActionParams{
				TextDocument: textDocumentIdentifier{URI: "file:///x.md"},
				Context: codeActionContext{
					Diagnostics: []Diagnostic{{
						Code: "MDS006",
						Data: &diagnosticData{RuleName: "no-trailing-spaces"},
					}},
					Only: []string{kindQuickFix},
				},
			}
			actions := s.computeCodeActions(p, doc, cfg, "")
			require.NotEmpty(t, actions)
			edit := actions[0].Edit
			require.NotNil(t, edit)
			assert.NotEmpty(t, edit.Changes, "should use legacy changes form")
			assert.Empty(t, edit.DocumentChanges, "should not use documentChanges form")
		})
	}
}

// TestPreviewFixFallbackLogsWarningOnce verifies that useAnnotatedEdits
// emits a window/logMessage naming the missing capability exactly once
// per session when previewFix is on but capabilities are absent.
func TestPreviewFixFallbackLogsWarningOnce(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name               string
		docChanges         bool
		changeAnnotSupport bool
		wantCapName        string
	}{
		{"no caps at all", false, false, "documentChanges"},
		{"documentChanges only", true, false, "changeAnnotationSupport"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			s := New(Options{Reader: nil, Writer: &out, Rules: rule.All()})
			s.settingsMu.Lock()
			s.settings.PreviewFix = true
			s.settingsMu.Unlock()
			var we *workspaceEditCapabilities
			if tc.docChanges || tc.changeAnnotSupport {
				we = &workspaceEditCapabilities{DocumentChanges: tc.docChanges}
				if tc.changeAnnotSupport {
					we.ChangeAnnotationSupport = &changeAnnotationSupportCap{}
				}
			}
			s.clientCapsMu.Lock()
			s.clientCaps = clientCapabilities{
				Workspace: &workspaceClientCapabilities{WorkspaceEdit: we},
			}
			s.clientCapsMu.Unlock()

			cfg := config.Merge(config.Defaults(), nil)
			doc := &document{path: "x.md", text: []byte("# Hi\n\ndirty   \n")}
			p := codeActionParams{
				TextDocument: textDocumentIdentifier{URI: "file:///x.md"},
				Context: codeActionContext{
					Diagnostics: []Diagnostic{{
						Code: "MDS006",
						Data: &diagnosticData{RuleName: "no-trailing-spaces"},
					}},
					Only: []string{kindQuickFix},
				},
			}

			// First call: warning must be emitted.
			s.computeCodeActions(p, doc, cfg, "")
			first := out.String()
			assert.Contains(t, first, "window/logMessage",
				"fallback must emit a window/logMessage notification")
			assert.Contains(t, first, tc.wantCapName,
				"warning must name the missing capability")

			// Second call: warning must not repeat (once per session).
			out.Reset()
			s.computeCodeActions(p, doc, cfg, "")
			assert.Empty(t, out.String(),
				"fallback warning must fire at most once per session")
		})
	}
}

// TestPreviewFixAnnotatedWhenCapsPresent verifies that when previewFix is
// on and the client advertises both documentChanges and
// changeAnnotationSupport, source.fixAll.mdsmith uses documentChanges
// with a single mdsmith-fix-all annotation flagged needsConfirmation.
func TestPreviewFixAnnotatedFixAllAction(t *testing.T) {
	t.Parallel()
	s := newPreviewServer(t, true, true)
	cfg := config.Merge(config.Defaults(), nil)
	doc := &document{path: "x.md", text: []byte("# Hi\n\ndirty   \n")}
	p := codeActionParams{
		TextDocument: textDocumentIdentifier{URI: "file:///x.md"},
		Context: codeActionContext{
			Only: []string{kindSourceFixAll},
		},
	}
	actions := s.computeCodeActions(p, doc, cfg, "")
	require.Len(t, actions, 1)
	edit := actions[0].Edit
	require.NotNil(t, edit)
	assert.Empty(t, edit.Changes, "annotated path must not emit legacy changes")
	require.Len(t, edit.DocumentChanges, 1)
	require.Len(t, edit.DocumentChanges[0].Edits, 1)
	assert.Equal(t, "mdsmith-fix-all", edit.DocumentChanges[0].Edits[0].AnnotationID)
	ann, ok := edit.ChangeAnnotations["mdsmith-fix-all"]
	require.True(t, ok, "changeAnnotations must contain mdsmith-fix-all")
	assert.True(t, ann.NeedsConfirmation)
}

// TestPreviewFixAnnotatedPerRuleQuickFix verifies that per-rule quick
// fixes carry annotations with IDs mdsmith-fix-<rule> when the
// annotated path is active.
func TestPreviewFixAnnotatedPerRuleQuickFix(t *testing.T) {
	t.Parallel()
	s := newPreviewServer(t, true, true)
	cfg := config.Merge(config.Defaults(), nil)
	doc := &document{path: "x.md", text: []byte("# Hi\n\ndirty   \n")}
	p := codeActionParams{
		TextDocument: textDocumentIdentifier{URI: "file:///x.md"},
		Context: codeActionContext{
			Diagnostics: []Diagnostic{{
				Code: "MDS006",
				Data: &diagnosticData{RuleName: "no-trailing-spaces"},
			}},
			Only: []string{kindQuickFix},
		},
	}
	actions := s.computeCodeActions(p, doc, cfg, "")
	require.NotEmpty(t, actions)
	edit := actions[0].Edit
	require.NotNil(t, edit)
	assert.Empty(t, edit.Changes)
	require.Len(t, edit.DocumentChanges, 1)
	require.Len(t, edit.DocumentChanges[0].Edits, 1)
	wantID := "mdsmith-fix-no-trailing-spaces"
	assert.Equal(t, wantID, edit.DocumentChanges[0].Edits[0].AnnotationID)
	ann, ok := edit.ChangeAnnotations[wantID]
	require.True(t, ok)
	assert.True(t, ann.NeedsConfirmation)
}

// TestPreviewFixAnnotatedHunksNotWholeFile verifies that the annotated
// edit path emits per-hunk TextEdits sized to the actual diff, not one
// giant whole-file replacement. VS Code's Refactor Preview renders each
// TextEdit as a separate change; a single full-file edit collapses into
// "old file → new file" with no visible delta and the whole document
// flattened into the lower preview pane's single-line label.
//
// Setup: 12 unchanged lines bracketing two distant trailing-whitespace
// lines (1 and 10). The per-rule quickfix path runs only
// no-trailing-spaces, so the only diff is on those two lines and the
// hunk count check isn't muddied by other auto-fixes.
func TestPreviewFixAnnotatedHunksNotWholeFile(t *testing.T) {
	t.Parallel()
	s := newPreviewServer(t, true, true)
	cfg := config.Merge(config.Defaults(), nil)
	lines := []string{
		"# Heading",
		"intro line   ", // trailing spaces, line index 1
		"",
		"para a",
		"para b",
		"para c",
		"",
		"para d",
		"para e",
		"",
		"closing line   ", // trailing spaces, line index 10
		"",
	}
	src := []byte(strings.Join(lines, "\n") + "\n")
	doc := &document{path: "x.md", text: src}
	p := codeActionParams{
		TextDocument: textDocumentIdentifier{URI: "file:///x.md"},
		Context: codeActionContext{
			Diagnostics: []Diagnostic{{
				Code: "MDS006",
				Data: &diagnosticData{RuleName: "no-trailing-spaces"},
			}},
			Only: []string{kindQuickFix},
		},
	}
	actions := s.computeCodeActions(p, doc, cfg, "")
	require.NotEmpty(t, actions)
	edit := actions[0].Edit
	require.NotNil(t, edit)
	require.Len(t, edit.DocumentChanges, 1)
	edits := edit.DocumentChanges[0].Edits
	// Two distant changes → at least two hunks. A whole-file edit
	// would emit exactly one entry spanning the full document.
	require.GreaterOrEqual(t, len(edits), 2,
		"preview path must emit per-hunk edits, not a whole-file replacement")

	// No single edit may span the whole document.
	for i, e := range edits {
		span := e.Range.End.Line - e.Range.Start.Line
		assert.Lessf(t, span, len(lines)-1,
			"edit %d spans %d lines [%d..%d]; "+
				"preview path must emit per-hunk edits, "+
				"not a whole-file replacement",
			i, span, e.Range.Start.Line, e.Range.End.Line)
	}

	// All edits must share the same annotation id so the preview
	// pane groups them under one entry.
	for _, e := range edits {
		assert.Equal(t, "mdsmith-fix-no-trailing-spaces", e.AnnotationID)
	}

	// Applying the edits must yield the same bytes the per-rule
	// fix pipeline would have written.
	got := applyAnnotatedEdits(t, src, edits)
	want := s.quickFixBytesFor("no-trailing-spaces", doc, cfg, "")
	require.NotNil(t, want)
	assert.Equal(t, string(want), string(got))
}

// TestAnnotatedHunkEdits covers annotatedHunkEdits directly so the
// fold rule, the LSP range conversion, and round-trip correctness are
// each tractable in isolation rather than only through computeCodeActions.
//
// Each case asserts:
//   - the expected edit ranges (line-aligned, character 0), in the order
//     the function emits them;
//   - the expected NewText per edit;
//   - that applying the edits to `before` round-trips to `after`;
//   - that every edit carries the supplied annotation id.
func TestAnnotatedHunkEdits(t *testing.T) {
	t.Parallel()
	for _, tc := range hunkEditCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			checkHunkEditCase(t, tc)
		})
	}
}

type hunkEditWant struct {
	startLine, endLine int
	newText            string
}

type hunkEditCase struct {
	name   string
	before string
	after  string
	want   []hunkEditWant
}

func hunkEditCases() []hunkEditCase {
	return append(hunkEditCasesSimple(), hunkEditCasesComplex()...)
}

func hunkEditCasesSimple() []hunkEditCase {
	return []hunkEditCase{
		{
			name:   "no change",
			before: "a\nb\n",
			after:  "a\nb\n",
			want:   nil,
		},
		{
			name:   "single-line replacement folds delete+insert",
			before: "a\nold\nc\n",
			after:  "a\nnew\nc\n",
			want:   []hunkEditWant{{1, 2, "new\n"}},
		},
		{
			name:   "pure deletion",
			before: "a\nb\nc\n",
			after:  "a\nc\n",
			want:   []hunkEditWant{{1, 2, ""}},
		},
		{
			name:   "pure insertion in the middle",
			before: "a\nc\n",
			after:  "a\nb\nc\n",
			want:   []hunkEditWant{{1, 1, "b\n"}},
		},
		{
			name:   "insertion at start of file",
			before: "b\n",
			after:  "a\nb\n",
			want:   []hunkEditWant{{0, 0, "a\n"}},
		},
		{
			name:   "append at end of file",
			before: "a\n",
			after:  "a\nb\n",
			want:   []hunkEditWant{{1, 1, "b\n"}},
		},
	}
}

func hunkEditCasesComplex() []hunkEditCase {
	return []hunkEditCase{
		{
			// Edits emit bottom-up so naive clients applying them
			// in slice order don't shift later offsets — see
			// annotatedHunkEdits' doc comment and
			// sortTextEditsBottomUp in rename.go.
			name:   "two distant single-line replacements emit two hunks",
			before: "a\nold1\nc\nd\ne\nold2\ng\n",
			after:  "a\nnew1\nc\nd\ne\nnew2\ng\n",
			want: []hunkEditWant{
				{5, 6, "new2\n"},
				{1, 2, "new1\n"},
			},
		},
		{
			name:   "multi-line replacement",
			before: "a\nb1\nb2\nb3\nc\n",
			after:  "a\nX\nc\n",
			want:   []hunkEditWant{{1, 4, "X\n"}},
		},
		{
			name:   "empty before to non-empty after",
			before: "",
			after:  "a\n",
			want:   []hunkEditWant{{0, 0, "a\n"}},
		},
		{
			name:   "non-empty before to empty after",
			before: "a\n",
			after:  "",
			want:   []hunkEditWant{{0, 1, ""}},
		},
	}
}

func checkHunkEditCase(t *testing.T, tc hunkEditCase) {
	t.Helper()
	const annot = "mdsmith-fix-x"
	got := annotatedHunkEdits([]byte(tc.before), []byte(tc.after), annot)
	require.Len(t, got, len(tc.want), "edit count mismatch: got %+v", got)
	for i, w := range tc.want {
		assert.Equalf(t, w.startLine, got[i].Range.Start.Line,
			"edit %d start line", i)
		assert.Equalf(t, w.endLine, got[i].Range.End.Line,
			"edit %d end line", i)
		assert.Zerof(t, got[i].Range.Start.Character,
			"edit %d start character must be 0", i)
		assert.Zerof(t, got[i].Range.End.Character,
			"edit %d end character must be 0", i)
		assert.Equalf(t, w.newText, got[i].NewText,
			"edit %d NewText", i)
		assert.Equalf(t, annot, got[i].AnnotationID,
			"edit %d annotation id", i)
	}
	// Apply round-trip: every case must reproduce `after`.
	if len(got) > 0 {
		roundTrip := applyAnnotatedEdits(t, []byte(tc.before), got)
		assert.Equal(t, tc.after, string(roundTrip))
	}
}

// TestLineRange covers lineRange directly: the 1-indexed-line,
// column-1 gotextdiff span maps to a 0-indexed-line, character-0 LSP
// position. The conversion is one line of arithmetic but is the only
// thing keeping our LSP edits aligned with the diff hunks the algorithm
// returns; a regression here would shift every preview edit by a line.
func TestLineRange(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name               string
		spanStart, spanEnd int
		wantStart, wantEnd int
	}{
		{"first line point", 1, 1, 0, 0},
		{"first line span", 1, 2, 0, 1},
		{"interior span", 4, 7, 3, 6},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := gotextdiff.TextEdit{
				Span: span.New(span.URIFromPath(""),
					span.NewPoint(tc.spanStart, 1, 0),
					span.NewPoint(tc.spanEnd, 1, 0)),
			}
			start, end := lineRange(e)
			assert.Equal(t, tc.wantStart, start.Line)
			assert.Zero(t, start.Character)
			assert.Equal(t, tc.wantEnd, end.Line)
			assert.Zero(t, end.Character)
		})
	}
}

// applyAnnotatedEdits applies a slice of annotatedTextEdit values to
// src using the LSP edit-application rules: edits' ranges refer to
// positions in the original buffer, so we sort by descending start
// position and splice each edit's NewText into the line-indexed slice.
func applyAnnotatedEdits(t *testing.T, src []byte, edits []annotatedTextEdit) []byte {
	t.Helper()
	sorted := make([]annotatedTextEdit, len(edits))
	copy(sorted, edits)
	sort.SliceStable(sorted, func(i, j int) bool {
		a, b := sorted[i].Range.Start, sorted[j].Range.Start
		if a.Line != b.Line {
			return a.Line > b.Line
		}
		return a.Character > b.Character
	})
	// All edits in this test use character 0 (line-aligned), so we
	// can splice by line index.
	lines := strings.SplitAfter(string(src), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	for _, e := range sorted {
		require.Zero(t, e.Range.Start.Character,
			"applyAnnotatedEdits only supports line-aligned edits")
		require.Zero(t, e.Range.End.Character,
			"applyAnnotatedEdits only supports line-aligned edits")
		start := e.Range.Start.Line
		end := e.Range.End.Line
		// Validate rather than clamp: a silent clamp would mask
		// off-by-one bugs in lineRange/annotatedHunkEdits by making
		// out-of-range edits "work".
		require.GreaterOrEqualf(t, start, 0, "start line %d < 0", start)
		require.LessOrEqualf(t, start, len(lines),
			"start line %d > line count %d", start, len(lines))
		require.GreaterOrEqualf(t, end, start,
			"end line %d < start line %d", end, start)
		require.LessOrEqualf(t, end, len(lines),
			"end line %d > line count %d", end, len(lines))
		insert := strings.SplitAfter(e.NewText, "\n")
		if len(insert) > 0 && insert[len(insert)-1] == "" {
			insert = insert[:len(insert)-1]
		}
		tail := append([]string{}, lines[end:]...)
		lines = append(lines[:start], insert...)
		lines = append(lines, tail...)
	}
	return []byte(strings.Join(lines, ""))
}

// TestPreviewFixOffProducesLegacyEdits verifies that with previewFix
// off (the default) edits are byte-identical to the plain legacy shape
// for both quickfix and source.fixAll.mdsmith.
func TestPreviewFixOffProducesLegacyEdits(t *testing.T) {
	t.Parallel()
	s := New(Options{Reader: nil, Writer: io.Discard, Rules: rule.All()})
	// previewFix defaults to false
	cfg := config.Merge(config.Defaults(), nil)
	doc := &document{path: "x.md", text: []byte("# Hi\n\ndirty   \n")}

	for _, only := range [][]string{{kindQuickFix}, {kindSourceFixAll}} {
		p := codeActionParams{
			TextDocument: textDocumentIdentifier{URI: "file:///x.md"},
			Context: codeActionContext{
				Diagnostics: []Diagnostic{{
					Code: "MDS006",
					Data: &diagnosticData{RuleName: "no-trailing-spaces"},
				}},
				Only: only,
			},
		}
		actions := s.computeCodeActions(p, doc, cfg, "")
		require.NotEmpty(t, actions, "kind %v should produce actions", only)
		edit := actions[0].Edit
		require.NotNil(t, edit)
		assert.NotEmpty(t, edit.Changes, "legacy path must use changes map")
		assert.Empty(t, edit.DocumentChanges)
		assert.Empty(t, edit.ChangeAnnotations)
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

// Regression: surfaceForeignDiagnostics writes one
// window/logMessage per foreign diagnostic, with a "<file>:<line>
// <message> [<rule>]" prefix the user can use to navigate to the
// actual config-file issue.
func TestSurfaceForeignDiagnosticsEmitsLogMessage(t *testing.T) {
	t.Parallel()
	var buf safeBuffer
	s := New(Options{Reader: nil, Writer: &buf})
	s.surfaceForeignDiagnostics("file:///doc.md", []lint.Diagnostic{
		{File: "/repo/.mdsmith.yml", Line: 7, Message: "bad value", RuleName: "directory-structure"},
	})
	out := buf.String()
	assert.Contains(t, out, `"window/logMessage"`)
	assert.Contains(t, out, "/repo/.mdsmith.yml:7")
	assert.Contains(t, out, "bad value")
	assert.Contains(t, out, "[directory-structure]")
	// Default severity (zero value) is Error → MessageType 1.
	assert.Contains(t, out, `"type":1`)
}

// Regression: surfaceForeignDiagnostics must preserve severity
// in the window/logMessage Type field, otherwise warnings show
// up as errors in the editor's output channel.
func TestSurfaceForeignDiagnosticsPreservesWarningSeverity(t *testing.T) {
	t.Parallel()
	var buf safeBuffer
	s := New(Options{Reader: nil, Writer: &buf})
	s.surfaceForeignDiagnostics("file:///doc.md", []lint.Diagnostic{
		{File: "/repo/.mdsmith.yml", Line: 3, Message: "soft hint",
			RuleName: "directory-structure", Severity: lint.Warning},
	})
	out := buf.String()
	// MessageType.Warning is 2 per LSP §3.18.1.
	assert.Contains(t, out, `"type":2`)
	assert.NotContains(t, out, `"type":1`,
		"Warning severity must not surface as messageTypeError")
}

// Regression: runLint must not publish diagnostics after the
// shutdown flag is set. A lint pass already in flight when the
// client sends shutdown/exit could otherwise race the dispatch
// loop's teardown and write to a half-closed pipe. The check
// covers the publishDiagnostics, window/logMessage error
// surfacing, and surfaceForeignDiagnostics paths.
func TestRunLintSkipsPublishWhenShutdownAfterLint(t *testing.T) {
	t.Parallel()
	var buf safeBuffer
	s := New(Options{Reader: nil, Writer: &buf, Rules: rule.All()})
	s.docs.set("file:///x.md", &document{
		uri: "file:///x.md", path: "/x.md", text: []byte("# Hi\n\ndirty   \n"),
	})
	// Simulate "shutdown arrived while engine.RunSource was
	// running" by flipping the flag before runLint reaches its
	// post-lint write block.
	s.shutdown.Store(true)
	s.runLint("file:///x.md")
	assert.NotContains(t, buf.String(), "publishDiagnostics",
		"runLint must drop the publish when shutdown was requested mid-flight")
}

// resolveMaxInputBytes returns the parse error to the client via
// window/logMessage and falls back to the default cap so a typo
// in `max-input-size:` doesn't break linting.
func TestResolveMaxInputBytesSurfacesParseError(t *testing.T) {
	t.Parallel()
	var buf safeBuffer
	s := New(Options{Reader: nil, Writer: &buf})
	cfg := config.Merge(config.Defaults(), nil)
	cfg.MaxInputSize = "not-a-size"

	got := s.resolveMaxInputBytes(cfg)

	assert.Equal(t, bytelimit.DefaultMaxInputBytes, got, "parse failure must fall back to the default")
	out := buf.String()
	assert.Contains(t, out, `"window/logMessage"`)
	assert.Contains(t, out, "invalid max-input-size")
	assert.Contains(t, out, "not-a-size")
}

// resolveMaxInputBytes honors `max-input-size: 0` as unlimited so
// a workspace that opts out of the default 2 MB cap is respected
// in the LSP path.
func TestResolveMaxInputBytesUnlimited(t *testing.T) {
	t.Parallel()
	s := New(Options{Reader: nil, Writer: io.Discard})
	cfg := config.Merge(config.Defaults(), nil)
	cfg.MaxInputSize = "0"

	got := s.resolveMaxInputBytes(cfg)

	assert.Equal(t, int64(0), got, "max-input-size: 0 must propagate as 0 (unlimited)")
}

// Regression: partitionDocDiagnostics keeps document-scoped
// diagnostics (matching File, or empty File) for the squiggle
// publish path and peels off config-target findings (different
// File) for window/logMessage. Without this filter, a config-file
// finding would appear as a squiggle at its (file, line) inside
// the markdown buffer that triggered the lint — wrong file, wrong
// line.
func TestPartitionDocDiagnosticsRoutesByFile(t *testing.T) {
	t.Parallel()
	docPath := "docs/foo.md"
	configPath := "/repo/.mdsmith.yml"
	diags := []lint.Diagnostic{
		{File: docPath, Line: 1, RuleName: "line-length", Message: "too long"},
		{File: "", Line: 2, RuleName: "old-rule", Message: "no file set"},
		{File: configPath, Line: 7, RuleName: "directory-structure", Message: "config issue"},
	}

	forDoc, other := partitionDocDiagnostics(diags, docPath)

	require.Len(t, forDoc, 2, "doc-scoped + empty-File entries belong to forDoc")
	assert.Equal(t, "line-length", forDoc[0].RuleName)
	assert.Equal(t, "old-rule", forDoc[1].RuleName)

	require.Len(t, other, 1, "config-file diagnostic is routed to other")
	assert.Equal(t, configPath, other[0].File)
	assert.Equal(t, "directory-structure", other[0].RuleName)
}

// Regression: runLint must not silently swallow Runner.RunSource
// errors. Triggering the size guard yields a Runner error; the
// server should both publish empty diagnostics and emit a
// window/logMessage so the editor can show actionable feedback.
func TestRunLintSurfacesRunnerErrorsViaLogMessage(t *testing.T) {
	t.Parallel()
	var buf safeBuffer
	s := New(Options{Reader: nil, Writer: &buf, Rules: rule.All()})
	cfg := config.Merge(config.Defaults(), nil)
	cfg.MaxInputSize = "16"
	s.configMu.Lock()
	s.config = cfg
	s.configMu.Unlock()
	s.docs.set("file:///x/big.md", &document{
		uri:  "file:///x/big.md",
		path: "x/big.md",
		text: []byte("# H\n\nthis is well past sixteen bytes of body content\n"),
	})
	// Default MaxInputBytes (2 MiB) is too generous to trip the
	// guard; runLint's runner pins to bytelimit.DefaultMaxInputBytes,
	// so we exercise the path by sending a buffer larger than 2 MiB.
	bigSize := bytelimit.DefaultMaxInputBytes + 1
	s.docs.set("file:///x/huge.md", &document{
		uri:  "file:///x/huge.md",
		path: "x/huge.md",
		text: bytes.Repeat([]byte("a"), int(bigSize)),
	})
	s.runLint("file:///x/huge.md")
	out := buf.String()
	assert.Contains(t, out, `"window/logMessage"`)
	assert.Contains(t, out, "file too large")
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

func TestHandleDidChangeWatchedFiles_InvalidatesWikilinkIndex(t *testing.T) {
	t.Parallel()
	// A markdown file create/delete reported via the workspace
	// watcher must clear the wikilink index — otherwise the next
	// MDS027 Check would resolve `[[NewPage]]` against the
	// pre-create candidate set and emit a false-positive
	// not-found diagnostic.
	cases := []struct {
		name       string
		uri        string
		changeType int
		want       bool
	}{
		{"markdown create invalidates", "file:///tmp/new.md", fileChangeCreated, true},
		{"markdown delete invalidates", "file:///tmp/new.md", fileChangeDeleted, true},
		{"markdown changed does not invalidate", "file:///tmp/new.md", fileChangeChanged, false},
		// Non-markdown create/delete must also invalidate: embeds
		// like `![[image.png]]` resolve against any extension via
		// the WikilinkIndex.names map, so a binary asset rename
		// shifts the candidate set just as a markdown rename does.
		{"image create invalidates", "file:///tmp/diagram.png", fileChangeCreated, true},
		{"image delete invalidates", "file:///tmp/diagram.png", fileChangeDeleted, true},
		{"image changed does not invalidate", "file:///tmp/diagram.png", fileChangeChanged, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// The handler routes a create/delete to
			// session.InvalidateWikilinks via this decision. Testing the
			// pure decision pins which change types rebuild the wikilink
			// candidate set without a live session and its caches (the
			// session-level wikilink drop is covered in pkg/mdsmith).
			got := watchedFilesTreeChanged([]fileEvent{{URI: tc.uri, Type: tc.changeType}})
			assert.Equal(t, tc.want, got,
				"watchedFilesTreeChanged(%s) = %v, want %v", tc.name, got, tc.want)
		})
	}
}

// TestWatchedFilesTreeChangedSkipsConfig pins that a .mdsmith.yml change
// alone does not flag a tree change (it is handled by the config-reload
// path, which rebuilds the session entirely).
func TestWatchedFilesTreeChangedSkipsConfig(t *testing.T) {
	t.Parallel()
	got := watchedFilesTreeChanged([]fileEvent{
		{URI: "file:///proj/.mdsmith.yml", Type: fileChangeCreated},
	})
	assert.False(t, got, "a config-only create must not flag a wikilink tree change")
}

func TestDocumentStoreGetMissing(t *testing.T) {
	t.Parallel()
	s := newDocumentStore()
	d, ok := s.get("file:///none")
	assert.Nil(t, d)
	assert.False(t, ok)
}

// fetchClientSettings returns silently when the transport's
// writeRequest fails — the request never reached the client, so
// there's nothing to wait for. This pins the early-return branch.
func TestFetchClientSettingsWriteRequestFailureReturnsEarly(t *testing.T) {
	t.Parallel()
	s := New(Options{Reader: nil, Writer: failingWriter{}})
	s.fetchTimeout = 100 * time.Millisecond
	// Should return promptly via the writeRequest err branch — far
	// faster than the fetchTimeout deadline.
	start := time.Now()
	s.fetchClientSettings(context.Background())
	assert.Less(t, time.Since(start), 50*time.Millisecond,
		"writeRequest failure must return before the fetchTimeout fires")
}

// Regression: an `exit` notification without a prior successful
// `shutdown` request must terminate Run with an error so the CLI
// exits non-zero. LSP §3.16 marks this as an abnormal termination.
func TestRunExitWithoutShutdownReturnsError(t *testing.T) {
	t.Parallel()
	body := `{"jsonrpc":"2.0","method":"exit"}`
	frames := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)
	s := New(Options{
		Reader: strings.NewReader(frames),
		Writer: io.Discard,
		Rules:  rule.All(),
	})
	err := s.Run(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, errExitWithoutShutdown)
}

// Companion: a clean shutdown→exit pair returns nil from Run.
func TestRunShutdownThenExitReturnsNil(t *testing.T) {
	t.Parallel()
	shutdownBody := `{"jsonrpc":"2.0","id":99,"method":"shutdown"}`
	exitBody := `{"jsonrpc":"2.0","method":"exit"}`
	frames := fmt.Sprintf(
		"Content-Length: %d\r\n\r\n%sContent-Length: %d\r\n\r\n%s",
		len(shutdownBody), shutdownBody, len(exitBody), exitBody,
	)
	s := New(Options{
		Reader: strings.NewReader(frames),
		Writer: io.Discard,
		Rules:  rule.All(),
	})
	err := s.Run(context.Background())
	require.NoError(t, err)
}

// Regression: LSP clients may legitimately send `processId: null`
// (and `rootUri: null`). The previous concrete int/string types
// made json.Unmarshal fail with "cannot unmarshal null into ...",
// causing handleInitialize to reject the very first request.
func TestHandleInitializeAcceptsNullProcessIDAndRootURI(t *testing.T) {
	t.Parallel()
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize",` +
		`"params":{"processId":null,"rootUri":null,"capabilities":{}}}`
	frames := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)
	var buf safeBuffer
	s := New(Options{
		Reader: strings.NewReader(frames),
		Writer: &buf,
		Rules:  rule.All(),
	})
	// Run blocks on stdin; the test reader hits EOF after the one
	// frame, Run returns nil, and we inspect what was written.
	err := s.Run(context.Background())
	require.NoError(t, err)
	out := buf.String()
	// Successful initialize includes a result object, NOT an
	// error response (which would carry "code":-32602).
	assert.Contains(t, out, `"result":`)
	assert.NotContains(t, out, "invalid initialize params")
}

// Run's dispatch loop checks transport.WriteError() at the TOP of
// every iteration (in addition to after dispatch). Pre-record an
// error on the transport before calling Run so the very first
// iteration's top-of-loop check returns it, exercising the early
// termination branch.
func TestRunReturnsErrorRecordedBeforeFirstIteration(t *testing.T) {
	t.Parallel()
	r, w := io.Pipe()
	defer func() { _ = r.Close() }()
	defer func() { _ = w.Close() }()
	s := New(Options{Reader: r, Writer: io.Discard, Rules: rule.All()})
	// Pre-record the error directly on the transport so the next
	// loop iteration's top-of-loop WriteError check returns it
	// without needing a real write to fail first.
	s.t.recordWriteErr(errors.New("pre-recorded transport failure"))

	err := s.Run(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pre-recorded transport failure")
}

// Run's dispatch loop checks transport.WriteError() at the top of
// every iteration so a write failure (e.g. EPIPE on the client's
// stdout) terminates Run. Drive a couple of frames into a server
// whose writer always fails; the second iteration's WriteError
// check returns the recorded error.
func TestRunReturnsRecordedWriteError(t *testing.T) {
	t.Parallel()
	// Two valid frames so the loop completes one full iteration
	// (which records a write error from writeResponse) before the
	// next iteration's WriteError check returns.
	body1 := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"capabilities":{}}}`
	body2 := `{"jsonrpc":"2.0","method":"exit"}`
	frames := fmt.Sprintf("Content-Length: %d\r\n\r\n%sContent-Length: %d\r\n\r\n%s",
		len(body1), body1, len(body2), body2)

	s := New(Options{
		Reader: strings.NewReader(frames),
		Writer: failingWriter{},
		Rules:  rule.All(),
	})
	err := s.Run(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "short write")
}

// deliverResponse drops on a full channel — it must not block,
// because the dispatch loop calls it inline. Fill the buffer-1
// channel and assert a second deliver is silently dropped instead
// of deadlocking.
func TestDeliverResponseDropsWhenChannelFull(t *testing.T) {
	t.Parallel()
	s := New(Options{Reader: nil, Writer: io.Discard})
	id := `"deliver-full"`
	ch := s.registerPendingResponse(id)
	defer s.unregisterPendingResponse(id)

	s.deliverResponse(id, rpcResponse{Result: json.RawMessage(`1`)})
	// Second deliver lands on a full channel; the default branch
	// drops it. The first message is still readable.
	s.deliverResponse(id, rpcResponse{Result: json.RawMessage(`2`)})

	select {
	case got := <-ch:
		assert.Equal(t, json.RawMessage(`1`), got.Result)
	case <-time.After(time.Second):
		t.Fatal("first deliverResponse never reached the channel")
	}
	// The second deliver was dropped, so the channel is now empty.
	select {
	case <-ch:
		t.Fatal("second deliverResponse should have been dropped on a full channel")
	default:
	}
}

// fetchClientSettings has a configurable timeout. With no client
// reply, the timeout branch runs and the call returns without
// touching cached settings. We pin a 5 ms timeout so the test
// completes quickly.
func TestFetchClientSettingsTimeoutLeavesSettings(t *testing.T) {
	t.Parallel()
	var buf safeBuffer
	s := New(Options{Reader: nil, Writer: &buf})
	s.fetchTimeout = 5 * time.Millisecond
	// Capture starting settings so we can confirm the timeout branch
	// did not mutate them.
	s.settingsMu.Lock()
	s.settings = userSettings{Run: runOnSave}
	s.settingsMu.Unlock()

	s.fetchClientSettings(context.Background())

	s.settingsMu.RLock()
	got := s.settings
	s.settingsMu.RUnlock()
	assert.Equal(t, runOnSave, got.Run, "timeout branch must not change cached settings")
}

// Regression: documentStore.set must deep-copy text so a caller
// that mutates its own slice afterwards cannot race with concurrent
// get() readers observing the stored bytes.
func TestDocumentStoreSetDeepCopiesText(t *testing.T) {
	t.Parallel()
	s := newDocumentStore()
	original := []byte("hello world")
	s.set("file:///x.md", &document{uri: "file:///x.md", path: "/x.md", text: original, version: 1})

	// Mutate the caller's slice; the store must not see it.
	original[0] = 'H'

	got, ok := s.get("file:///x.md")
	assert.True(t, ok)
	assert.Equal(t, []byte("hello world"), got.text)
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

// --- Hover tests ---

func TestHoverAdvertisedInCapabilities(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	resultRaw, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)
	var res initializeResult
	require.NoError(t, json.Unmarshal(resultRaw, &res))
	assert.True(t, res.Capabilities.HoverProvider, "hoverProvider must be true in initialize response")
}

func TestHoverDiagnosticFirst(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	_, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)

	uri := "file:///workspace/hover_diag.md"
	// MDS006 fires on line 2 (0-based) due to trailing spaces.
	text := "# Title\n\ntrailing   \n"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{
			URI: uri, LanguageID: "markdown", Version: 1, Text: text,
		},
	})
	// Wait for lint pass to cache diagnostics.
	raw := h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)
	var published publishDiagnosticsParams
	require.NoError(t, json.Unmarshal(raw, &published))
	var found bool
	for _, d := range published.Diagnostics {
		if d.Code == "MDS006" {
			found = true
		}
	}
	require.True(t, found, "expected MDS006 in published diagnostics before hover")

	// Hover on the trailing-spaces line, character 9 (first trailing space,
	// which is inside the MDS006 diagnostic range starting at character 8).
	resultRaw, hoverErr := h.request("textDocument/hover", hoverParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 2, Character: 9},
	})
	require.Nil(t, hoverErr)
	require.NotEqual(t, "null", string(resultRaw), "hover over diagnostic must return content")

	var result hoverResult
	require.NoError(t, json.Unmarshal(resultRaw, &result))
	assert.Equal(t, "markdown", result.Contents.Kind)
	assert.Contains(t, result.Contents.Value, "MDS006")
	assert.NotNil(t, result.Range, "hover over diagnostic must include range")
}

func TestHoverDirectiveFallback(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	_, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)

	uri := "file:///workspace/hover_directive.md"
	// A catalog directive with no trailing-space diagnostics.
	text := "# Title\n\n<?catalog\nglob: \"*.md\"\n?>\n"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{
			URI: uri, LanguageID: "markdown", Version: 1, Text: text,
		},
	})
	// Wait for lint pass to complete so the hover handler finds a live doc.
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	// Hover on the glob line (line 3, 0-based) — inside the directive but below
	// any MDS019 diagnostic range which spans only the opening tag line.
	resultRaw, hoverErr := h.request("textDocument/hover", hoverParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 3, Character: 0},
	})
	require.Nil(t, hoverErr)
	require.NotEqual(t, "null", string(resultRaw), "hover inside catalog directive must return content")

	var result hoverResult
	require.NoError(t, json.Unmarshal(resultRaw, &result))
	assert.Equal(t, "markdown", result.Contents.Kind)
	assert.NotEmpty(t, result.Contents.Value)
	assert.NotNil(t, result.Range)
}

func TestHoverNoMatch(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	_, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)

	uri := "file:///workspace/hover_nomatch.md"
	text := "# Title\n\nPlain prose without issues.\n"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{
			URI: uri, LanguageID: "markdown", Version: 1, Text: text,
		},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	// Hover on the heading (no diagnostic, no directive).
	resultRaw, hoverErr := h.request("textDocument/hover", hoverParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 0, Character: 3},
	})
	require.Nil(t, hoverErr)
	assert.Equal(t, "null", string(resultRaw), "hover on plain prose must return null")
}

func TestRuleHoverContentAddsMaintainabilityFixWhenEnabled(t *testing.T) {
	t.Parallel()
	got := ruleHoverContent(Diagnostic{Code: "MDS033", Message: "outside allowed directories"})
	assert.Contains(t, got, "Suggested remediation:")
}

// For adoption-only rules (catalog, include, required-structure) the
// maintainability fix advertises a directive the user has not yet adopted,
// so it must not appear on a diagnostic hover. for-diagnostic defaults to
// false for those rules.
func TestRuleHoverContentOmitsAdoptionFixForCatalog(t *testing.T) {
	t.Parallel()
	got := ruleHoverContent(Diagnostic{Code: "MDS019", Message: "catalog body drifted"})
	assert.NotContains(t, got, "Suggested remediation:")
}

func TestRuleHoverContentOmitsRemediationForNullMaintainability(t *testing.T) {
	t.Parallel()
	got := ruleHoverContent(Diagnostic{Code: "MDS001", Message: "line too long"})
	assert.NotContains(t, got, "Suggested remediation:")
}

func TestRulePatternsMethodReturnsNonNullPatterns(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	_, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)

	raw, errResp := h.request("mdsmith/rulePatterns", map[string]any{})
	require.Nil(t, errResp)
	var out []map[string]any
	require.NoError(t, json.Unmarshal(raw, &out))
	require.NotEmpty(t, out)
	sawMDS019 := false
	for _, it := range out {
		require.NotEmpty(t, it["id"])
		require.NotEmpty(t, it["signal"])
		require.NotEmpty(t, it["fix"])
		// line-length is maintainability: null and must be excluded.
		assert.NotEqual(t, "MDS001", it["id"])
		if it["id"] == "MDS019" {
			sawMDS019 = true
		}
	}
	assert.True(t, sawMDS019, "expected MDS019 (catalog) in the rulePatterns payload")
}

func TestHoverUnknownDocument(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	_, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)

	// Hover on a URI that was never opened.
	resultRaw, hoverErr := h.request("textDocument/hover", hoverParams{
		TextDocument: textDocumentIdentifier{URI: "file:///not/open.md"},
		Position:     Position{Line: 0, Character: 0},
	})
	require.Nil(t, hoverErr)
	assert.Equal(t, "null", string(resultRaw), "hover on unknown document must return null")
}

func TestPosInRange(t *testing.T) {
	t.Parallel()
	r := Range{Start: Position{Line: 2, Character: 5}, End: Position{Line: 2, Character: 10}}

	// Inside the range.
	assert.True(t, posInRange(Position{2, 5}, r), "start boundary is inside")
	assert.True(t, posInRange(Position{2, 7}, r), "mid-range")
	assert.True(t, posInRange(Position{2, 9}, r), "one before end")

	// Outside: end boundary is exclusive.
	assert.False(t, posInRange(Position{2, 10}, r), "end boundary is exclusive")
	assert.False(t, posInRange(Position{2, 11}, r), "past end")
	assert.False(t, posInRange(Position{2, 4}, r), "before start")
	assert.False(t, posInRange(Position{1, 7}, r), "wrong line before")
	assert.False(t, posInRange(Position{3, 7}, r), "wrong line after")

	// Empty range (Start == End): nothing is inside.
	empty := Range{Start: Position{Line: 1, Character: 3}, End: Position{Line: 1, Character: 3}}
	assert.False(t, posInRange(Position{1, 3}, empty), "position at empty range is outside")
}

// TestDirectiveDocFor exercises the return-value contract of directiveDocFor:
// known directives return non-empty content, unknown names return false.
func TestDirectiveDocFor(t *testing.T) {
	t.Parallel()

	content, ok := directiveDocFor("catalog")
	assert.True(t, ok, "catalog should have docs")
	assert.NotEmpty(t, content, "catalog docs must be non-empty")

	_, ok = directiveDocFor("unknown-directive-xyz")
	assert.False(t, ok, "unknown directive must return false")

	for _, name := range []string{"include", "build", "allow-empty-section", "require"} {
		c, found := directiveDocFor(name)
		assert.True(t, found, "%s should have docs", name)
		assert.NotEmpty(t, c, "%s docs must be non-empty", name)
	}

	// toc and ignore have no guide page yet and must return false.
	for _, name := range []string{"toc", "ignore"} {
		_, found := directiveDocFor(name)
		assert.False(t, found, "%s must return false until a guide page exists", name)
	}
}

// TestLoadDirectiveDocs_SkipsMissingFile covers the err != nil branch in
// loadDirectiveDocs: when a referenced guide file is absent from the FS the
// entry is silently skipped so hover returns null rather than crashing.
func TestLoadDirectiveDocs_SkipsMissingFile(t *testing.T) {
	t.Parallel()
	docs := loadDirectiveDocs(fstest.MapFS{}) // empty FS — all reads fail
	assert.Empty(t, docs, "missing files must produce an empty map")
}

// TestLoadDirectiveDocs_DeduplicatesFiles covers the seen-map guard:
// multiple directives share one guide file, so the file is read only once.
func TestLoadDirectiveDocs_DeduplicatesFiles(t *testing.T) {
	t.Parallel()
	// Provide the real generating-content.md so both catalog and include
	// resolve to a non-empty entry without touching the other guide files.
	content := []byte("---\nsummary: test\n---\n# Guide\n")
	fsys := fstest.MapFS{
		"generating-content.md":  &fstest.MapFile{Data: content},
		"build.md":               &fstest.MapFile{Data: content},
		"enforcing-structure.md": &fstest.MapFile{Data: content},
	}
	docs := loadDirectiveDocs(fsys)
	// Three distinct files should be loaded once each.
	assert.Len(t, docs, 3)
}

// TestRuleHoverContentFallback covers the error branch of ruleHoverContent
// where the rule code is not registered.
func TestRuleHoverContentFallback(t *testing.T) {
	t.Parallel()
	d := Diagnostic{Code: "MDSZZZ", Message: "unknown rule test message"}
	content := ruleHoverContent(d)
	assert.Contains(t, content, "MDSZZZ")
	assert.Contains(t, content, "mdsmith help rule MDSZZZ")
}

// TestRuleHoverContent_IssueFirst pins plan 230's hover layout: the
// diagnostic message and the navigable schema link lead as the primary
// block, a separator follows, then the condensed rule identity (code,
// name, one-line description, docs link). The full README is not
// inlined.
func TestRuleHoverContent_IssueFirst(t *testing.T) {
	t.Parallel()
	d := Diagnostic{
		Code:            "MDS020",
		Message:         `status: got "draft", expected one of "open"`,
		Data:            &diagnosticData{RuleName: "required-structure"},
		CodeDescription: &codeDescription{Href: "https://mdsmith.dev/rules/MDS020"},
		RelatedInformation: []diagnosticRelatedInformation{{
			Location: location{
				URI:   "file:///w/plan/proto.md",
				Range: Range{Start: Position{Line: 3}},
			},
			Message: "required by schema",
		}},
	}
	got := ruleHoverContent(d)

	sep := strings.Index(got, "\n---\n")
	require.Positive(t, sep, "hover has an issue/rule separator")
	issue, ruleBlock := got[:sep], got[sep:]

	assert.Contains(t, issue, `status: got "draft"`, "issue leads")
	assert.Contains(t, issue,
		"[proto.md:4](file:///w/plan/proto.md#L4) — required by schema",
		"navigable related link with its message sits in the issue block")
	assert.Contains(t, ruleBlock, "`MDS020` · required-structure",
		"rule identity is secondary")
	assert.Contains(t, ruleBlock, "Document structure and front matter",
		"one-line description, not the full README")
	assert.Contains(t, ruleBlock, "https://mdsmith.dev/rules/MDS020")
}

// TestDirectiveHoverAtInRange covers the path where the cursor falls inside a
// directive block that has registered documentation.
func TestDirectiveHoverAtInRange(t *testing.T) {
	t.Parallel()
	src := []byte("# Title\n\n<?catalog\nglob: \"*.md\"\n?>\n")

	result := directiveHoverAt(src, Position{Line: 3, Character: 0})
	require.NotNil(t, result, "cursor inside documented directive must return a hover result")
	assert.Equal(t, "markdown", result.Contents.Kind)
	assert.NotEmpty(t, result.Contents.Value)
	assert.NotNil(t, result.Range)
	assert.Equal(t, 2, result.Range.Start.Line, "range must start at the opening tag line")
}

// TestDirectiveHoverAtOutOfRange covers the path where the cursor is above
// any directive block (the posInRange guard returns false for all PIs).
func TestDirectiveHoverAtOutOfRange(t *testing.T) {
	t.Parallel()
	src := []byte("# Title\n\n<?catalog\nglob: \"*.md\"\n?>\n")

	result := directiveHoverAt(src, Position{Line: 0, Character: 0})
	assert.Nil(t, result, "cursor above directive must return nil")
}

// TestDirectiveHoverAtUnknownDirective covers the WalkStop branch inside
// directiveHoverAt when the cursor is within a directive that has no docs.
func TestDirectiveHoverAtUnknownDirective(t *testing.T) {
	t.Parallel()
	src := []byte("# Title\n\n<?unknown-xyz\narg: val\n?>\n")

	result := directiveHoverAt(src, Position{Line: 2, Character: 3})
	assert.Nil(t, result, "cursor inside undocumented directive must return nil")
}

// TestDirectiveHoverAtBeforeIndent covers the startChar guard: a cursor to the
// left of the <? column (in the indentation) must not return hover content.
func TestDirectiveHoverAtBeforeIndent(t *testing.T) {
	t.Parallel()
	// Two leading spaces — <?catalog is at column 2.
	src := []byte("# Title\n\n  <?catalog\nglob: \"*.md\"\n?>\n")

	// Cursor at column 0 on the opening line — before the <? column.
	result := directiveHoverAt(src, Position{Line: 2, Character: 0})
	assert.Nil(t, result, "cursor before <? indent must return nil")

	// Cursor at column 2 — exactly at <?.
	result = directiveHoverAt(src, Position{Line: 2, Character: 2})
	require.NotNil(t, result, "cursor at <? column must return hover result")
	assert.Equal(t, 2, result.Range.Start.Character, "range start character must equal indent")
}

// TestDirectiveHoverAtSingleLinePI covers the single-line PI span check:
// hovering inside the directive returns docs; hovering after ?> returns nil.
// The returned range End.Character must end at the ?> position, not end-of-line.
func TestDirectiveHoverAtSingleLinePI(t *testing.T) {
	t.Parallel()
	// Single-line catalog directive followed by trailing prose on the same line.
	// (toc and ignore have no guide page and are not in directiveToDocFile.)
	// <?catalog glob:"*"?> is 20 UTF-16 code units (all ASCII).
	src := []byte("# Title\n\n<?catalog glob:\"*\"?> trailing text\n")

	// Cursor inside the PI span (character 3, within <?catalog glob:"*"?>).
	result := directiveHoverAt(src, Position{Line: 2, Character: 3})
	require.NotNil(t, result, "cursor inside single-line PI must return hover result")
	require.NotNil(t, result.Range, "hover result must include a range")
	assert.Equal(t, Position{Line: 2, Character: 0}, result.Range.Start)
	// End must be the ?> close (character 20), not end-of-line.
	assert.Equal(t, Position{Line: 2, Character: 20}, result.Range.End)

	// Cursor after ?> (character 22, in trailing prose).
	result = directiveHoverAt(src, Position{Line: 2, Character: 22})
	assert.Nil(t, result, "cursor after ?> in single-line PI must return nil")
}

// TestDirectiveHoverAtNoClosurePI covers the else branch of HasClosure():
// when a PI has no closing ?>, endLine is computed from the last content line.
func TestDirectiveHoverAtNoClosurePI(t *testing.T) {
	t.Parallel()
	// No closing ?> — HasClosure() is false; endLine is the last content line.
	src := []byte("# Title\n\n<?catalog\nglob: \"*.md\"\n")

	result := directiveHoverAt(src, Position{Line: 3, Character: 0})
	require.NotNil(t, result, "cursor inside unclosed catalog PI must return a hover result")
	assert.Equal(t, "markdown", result.Contents.Kind)
	assert.NotEmpty(t, result.Contents.Value)
}

// TestHoverInvalidParams covers the json.Unmarshal error branch in handleHover
// by sending a non-object params value that cannot be decoded into hoverParams.
func TestHoverInvalidParams(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	_, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)

	_, errResp = h.request("textDocument/hover", json.RawMessage("true"))
	require.NotNil(t, errResp, "invalid params must return an error response")
	assert.Equal(t, codeInvalidParams, errResp.Code)
}

// TestHoverSkipsEmptyCodeDiagnostic covers the d.Code == "" guard in
// handleHover: a diagnostic with an empty code must be skipped even when
// the cursor falls inside its range.
func TestHoverSkipsEmptyCodeDiagnostic(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	_, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)

	uri := "file:///workspace/hover_empty_code.md"
	text := "# Title\n\nsome prose\n"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{
			URI: uri, LanguageID: "markdown", Version: 1, Text: text,
		},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	// Inject a diagnostic with an empty code spanning line 2.
	h.srv.diagsMu.Lock()
	h.srv.diags[uri] = []Diagnostic{{
		Range:   Range{Start: Position{Line: 2, Character: 0}, End: Position{Line: 2, Character: 10}},
		Code:    "",
		Message: "no-code diagnostic",
	}}
	h.srv.diagsMu.Unlock()

	// Hover inside the empty-code diagnostic range; the diagnostic is skipped
	// and the result falls through to null (no directive at that position).
	resultRaw, hoverErr := h.request("textDocument/hover", hoverParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 2, Character: 5},
	})
	require.Nil(t, hoverErr)
	assert.Equal(t, "null", string(resultRaw), "empty-code diagnostic must not produce hover content")
}

// TestHandleDidClose_CancelsPendingLint verifies that handleDidClose cancels
// any armed debounce timer so a pending runLint cannot fire and re-publish
// diagnostics after the document has been removed and cleared.
func TestHandleDidClose_CancelsPendingLint(t *testing.T) {
	t.Parallel()
	// Use a long debounce so the didChange timer does not fire during the test.
	h := newDebouncedHarness(t, 10*time.Second)
	_, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)

	uri := "file:///close-race.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: "# Hello\n"},
	})
	// Drain the immediate open-triggered lint notification.
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	// Send a change — this arms a 10-second debounce timer.
	h.notify("textDocument/didChange", didChangeTextDocumentParams{
		TextDocument:   versionedTextDocumentIdentifier{URI: uri, Version: 2},
		ContentChanges: []textDocumentContentChangeEvent{{Text: "# Changed\n"}},
	})

	// Close the document — must cancel the pending timer.
	h.notify("textDocument/didClose", didCloseTextDocumentParams{
		TextDocument: textDocumentIdentifier{URI: uri},
	})

	// Drain the empty-diagnostics notification published by didClose.
	raw := h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)
	var closed publishDiagnosticsParams
	require.NoError(t, json.Unmarshal(raw, &closed))
	assert.Empty(t, closed.Diagnostics, "didClose must publish empty diagnostics")

	// The pending timer must have been cancelled by handleDidClose.
	h.srv.pendingMu.Lock()
	_, hasPending := h.srv.pending[uri]
	h.srv.pendingMu.Unlock()
	assert.False(t, hasPending, "pending lint timer must be cleared after didClose")
}

// Read-cache invalidation moved from the Server onto the per-workspace
// pkg/mdsmith.Session: the nil-guard, empty-path, and drops-entry
// branches that TestInvalidateCachedRead_* used to pin are now exercised
// by the session's own Invalidate tests in pkg/mdsmith
// (TestInvalidateClearsDependentCache, TestInvalidateDropsVersionParseCache).
// The Server's syncBuffer/dropPath wrappers that route to the session
// are covered behaviourally by the document-sync and overlay tests.

// TestIsAbsPath covers the cross-platform absolute-path classification
// relatedURI relies on: POSIX absolute, Windows drive-letter, and UNC
// paths are absolute on any host (so they are bounded against the root,
// not mis-joined to it); a relative path is not.
func TestIsAbsPath(t *testing.T) {
	t.Parallel()
	assert.True(t, isAbsPath("/abs/proto.md"), "posix absolute")
	assert.True(t, isAbsPath(`C:\work\proto.md`), "windows drive-letter")
	assert.True(t, isAbsPath(`\\server\share\proto.md`), "UNC")
	assert.False(t, isAbsPath("plan/proto.md"), "relative")
}

// TestRelatedHoverLink covers the rule-agnostic related-link renderer:
// the entry's own message trails the link, and an entry with no message
// renders the bare link.
func TestRelatedHoverLink(t *testing.T) {
	t.Parallel()
	ri := diagnosticRelatedInformation{
		Location: location{
			URI:   "file:///w/proto.md",
			Range: Range{Start: Position{Line: 4}},
		},
		Message: "required by schema",
	}
	assert.Equal(t,
		"[proto.md:5](file:///w/proto.md#L5) — required by schema",
		relatedHoverLink(ri))

	ri.Message = ""
	assert.Equal(t, "[proto.md:5](file:///w/proto.md#L5)", relatedHoverLink(ri))
}

// TestWithinRoot covers the containment check, including the
// filepath.Rel error branch (a relative root against an absolute path
// cannot be related, so it counts as outside).
func TestWithinRoot(t *testing.T) {
	t.Parallel()
	assert.True(t, withinRoot("/work", "/work/sub/x.md"), "inside root")
	assert.True(t, withinRoot("/work", "/work"), "root itself counts")
	assert.False(t, withinRoot("/work", "/etc/passwd"), "sibling escapes")
	assert.False(t, withinRoot("relative", "/abs/path"),
		"a relative root and an absolute path resolve to disjoint subtrees → outside")
}

// TestWithinRoot_SymlinkEscapeRejected pins the symlink-safe behaviour
// (Copilot review): a real file inside the root is contained, but an
// in-root symlink that resolves outside the root is rejected — a purely
// lexical check would have accepted it.
func TestWithinRoot_SymlinkEscapeRejected(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	inside := filepath.Join(root, "proto.md")
	require.NoError(t, os.WriteFile(inside, []byte("x"), 0o600))
	assert.True(t, withinRoot(root, inside), "real in-root file is contained")

	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.md")
	require.NoError(t, os.WriteFile(secret, []byte("s"), 0o600))
	link := filepath.Join(root, "evil.md")
	require.NoError(t, os.Symlink(secret, link))
	assert.False(t, withinRoot(root, link),
		"an in-root symlink resolving outside the root is rejected")
}
