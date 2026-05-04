package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/textproto"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/rule"

	// Register rule packages so rule.All() returns the production set.
	_ "github.com/jeduden/mdsmith/internal/rules/linelength"
	_ "github.com/jeduden/mdsmith/internal/rules/notrailingspaces"
)

// testHarness wires a Server to a pair of in-memory pipes plus a
// helper for the test side to read framed messages without racing the
// server.
type testHarness struct {
	t            *testing.T
	srv          *Server
	clientWriter io.WriteCloser
	clientReader *bufio.Reader
	srvDone      chan error
	cancel       context.CancelFunc
	mu           sync.Mutex
	nextID       int
}

func newHarness(t *testing.T) *testHarness {
	t.Helper()
	srvIn, clientWriter := io.Pipe()
	clientRawReader, srvOut := io.Pipe()
	srv := New(Options{
		Rules:    rule.All(),
		Reader:   srvIn,
		Writer:   srvOut,
		Debounce: -1,
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- srv.Run(ctx)
		_ = srvOut.Close()
	}()
	h := &testHarness{
		t:            t,
		srv:          srv,
		clientWriter: clientWriter,
		clientReader: bufio.NewReader(clientRawReader),
		srvDone:      done,
		cancel:       cancel,
	}
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

func (h *testHarness) request(method string, params any) (json.RawMessage, *responseError) {
	h.t.Helper()
	h.mu.Lock()
	h.nextID++
	id := h.nextID
	h.mu.Unlock()
	idJSON, err := json.Marshal(id)
	require.NoError(h.t, err)
	body := struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Method  string          `json:"method"`
		Params  any             `json:"params,omitempty"`
	}{JSONRPC: "2.0", ID: idJSON, Method: method, Params: params}
	h.write(body)
	for {
		raw := h.read()
		var probe struct {
			ID     json.RawMessage `json:"id,omitempty"`
			Method string          `json:"method,omitempty"`
		}
		require.NoError(h.t, json.Unmarshal(raw, &probe))
		if probe.Method != "" && len(probe.ID) == 0 {
			continue // server-side notification; skip while waiting for our reply
		}
		var resp responseMessage
		require.NoError(h.t, json.Unmarshal(raw, &resp))
		// Skip server-to-client requests (they have a Method).
		if probe.Method != "" {
			// Acknowledge with an empty result so the server can proceed.
			h.write(struct {
				JSONRPC string          `json:"jsonrpc"`
				ID      json.RawMessage `json:"id"`
				Result  any             `json:"result"`
			}{JSONRPC: "2.0", ID: probe.ID, Result: nil})
			continue
		}
		if string(resp.ID) != string(idJSON) {
			continue
		}
		var resultRaw json.RawMessage
		if resp.Result != nil {
			resultRaw, _ = json.Marshal(resp.Result)
		}
		return resultRaw, resp.Error
	}
}

func (h *testHarness) notify(method string, params any) {
	h.t.Helper()
	body := notificationMessage{JSONRPC: "2.0", Method: method, Params: params}
	h.write(body)
}

func (h *testHarness) write(v any) {
	h.t.Helper()
	body, err := json.Marshal(v)
	require.NoError(h.t, err)
	_, err = fmt.Fprintf(h.clientWriter, "Content-Length: %d\r\n\r\n", len(body))
	require.NoError(h.t, err)
	_, err = h.clientWriter.Write(body)
	require.NoError(h.t, err)
}

func (h *testHarness) read() []byte {
	h.t.Helper()
	tp := textproto.NewReader(h.clientReader)
	hdr, err := tp.ReadMIMEHeader()
	require.NoError(h.t, err)
	cl := hdr.Get("Content-Length")
	require.NotEmpty(h.t, cl)
	n, err := strconv.Atoi(cl)
	require.NoError(h.t, err)
	body := make([]byte, n)
	_, err = io.ReadFull(h.clientReader, body)
	require.NoError(h.t, err)
	return body
}

// awaitNotification reads frames until one matches the requested
// method. Server-to-client requests are answered with a nil result so
// the dispatch loop keeps running. The deadline guards against a
// silent server.
func (h *testHarness) awaitNotification(method string, timeout time.Duration) json.RawMessage {
	h.t.Helper()
	deadline := time.Now().Add(timeout)
	type frame struct {
		raw []byte
		err error
	}
	ch := make(chan frame, 1)
	go func() {
		raw := h.read()
		ch <- frame{raw: raw}
	}()
	for {
		select {
		case f := <-ch:
			if f.err != nil {
				h.t.Fatalf("read error: %v", f.err)
			}
			var probe struct {
				ID     json.RawMessage `json:"id,omitempty"`
				Method string          `json:"method,omitempty"`
				Params json.RawMessage `json:"params,omitempty"`
			}
			require.NoError(h.t, json.Unmarshal(f.raw, &probe))
			if probe.Method == method && len(probe.ID) == 0 {
				return probe.Params
			}
			if probe.Method != "" && len(probe.ID) > 0 {
				h.write(struct {
					JSONRPC string          `json:"jsonrpc"`
					ID      json.RawMessage `json:"id"`
					Result  any             `json:"result"`
				}{JSONRPC: "2.0", ID: probe.ID, Result: nil})
			}
			// Spin up another reader.
			go func() {
				raw := h.read()
				ch <- frame{raw: raw}
			}()
		case <-time.After(time.Until(deadline)):
			h.t.Fatalf("timeout waiting for %s", method)
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

	// The body has trailing whitespace on line 3, which MDS006 catches.
	var found bool
	for _, d := range p.Diagnostics {
		if d.Code == "MDS006" {
			found = true
			// MDS006 is a warning. The exact severity is not what the
			// LSP transport guarantees; what matters here is that the
			// mapping populated severity, code, source, and the
			// rule-name data field.
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

	// Inject a trailing-spaces violation.
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
	require.NotEmpty(t, actions, "expected at least one code action")

	var qf *codeAction
	for i, a := range actions {
		if a.Kind == kindQuickFix {
			qf = &actions[i]
			break
		}
	}
	require.NotNil(t, qf, "expected quickfix action; got %+v", actions)
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

func TestUnknownMethodReturnsError(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	_, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)
	_, errResp = h.request("textDocument/bogus", nil)
	require.NotNil(t, errResp)
	assert.Equal(t, codeMethodNotFound, errResp.Code)
}
