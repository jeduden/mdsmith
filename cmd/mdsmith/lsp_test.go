package main_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/textproto"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLSPInitializeOverPipe spawns `go run ./cmd/mdsmith lsp`, drives
// the server through a single initialize → didOpen → didChange →
// shutdown round trip over the process's stdio pipes, and asserts
// that diagnostics appear after didOpen. This is the acceptance gate
// for plan 121's "speaks LSP over stdio" criterion.
func TestLSPInitializeOverPipe(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping LSP subprocess test in -short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pipe := startLSPSubprocess(t, ctx)
	deadline := time.Now().Add(20 * time.Second)

	pipe.assertInitializeAck(t)
	pipe.notify("initialized", map[string]any{})

	uri := "file:///tmp/lsp-e2e.md"
	pipe.openDocument(uri, "# Hi\n\ndirty line   \n")

	diags := pipe.awaitDiagnostics(t, uri, deadline)
	assertHasMDS006(t, diags)

	pipe.changeDocument(uri, 2, "# Hi\n\nclean line\n")
	cleared := pipe.awaitDiagnostics(t, uri, deadline)
	assertNoMDS006(t, cleared)

	pipe.shutdown(t)
}

// lspPipe wraps a child process's stdio so individual test steps stay
// short. It also handles the inevitable server-initiated requests
// (workspace/configuration, client/registerCapability) that arrive
// interleaved with the publishDiagnostics notifications we wait for.
type lspPipe struct {
	t      *testing.T
	stdin  io.WriteCloser
	stdout *bufio.Reader
}

func startLSPSubprocess(t *testing.T, ctx context.Context) *lspPipe {
	t.Helper()
	cmd := exec.CommandContext(ctx, "go", "run", "./cmd/mdsmith", "lsp")
	cmd.Dir = repoRoot(t)
	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	cmd.Stderr = nil
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = stdin.Close()
		_ = cmd.Wait()
	})
	return &lspPipe{t: t, stdin: stdin, stdout: bufio.NewReader(stdout)}
}

func (p *lspPipe) writeFrame(v any) {
	p.t.Helper()
	body, err := json.Marshal(v)
	require.NoError(p.t, err)
	_, err = fmt.Fprintf(p.stdin, "Content-Length: %d\r\n\r\n", len(body))
	require.NoError(p.t, err)
	_, err = p.stdin.Write(body)
	require.NoError(p.t, err)
}

func (p *lspPipe) readFrame() map[string]any {
	p.t.Helper()
	tp := textproto.NewReader(p.stdout)
	hdr, err := tp.ReadMIMEHeader()
	require.NoError(p.t, err)
	cl := hdr.Get("Content-Length")
	require.NotEmpty(p.t, cl)
	n, err := strconv.Atoi(cl)
	require.NoError(p.t, err)
	body := make([]byte, n)
	_, err = io.ReadFull(p.stdout, body)
	require.NoError(p.t, err)
	var m map[string]any
	require.NoError(p.t, json.Unmarshal(body, &m))
	return m
}

func (p *lspPipe) request(method string, id int, params any) map[string]any {
	p.writeFrame(map[string]any{
		"jsonrpc": "2.0", "id": id, "method": method, "params": params,
	})
	return p.readFrame()
}

func (p *lspPipe) notify(method string, params any) {
	p.writeFrame(map[string]any{
		"jsonrpc": "2.0", "method": method, "params": params,
	})
}

func (p *lspPipe) assertInitializeAck(t *testing.T) {
	t.Helper()
	resp := p.request("initialize", 1, map[string]any{"capabilities": map[string]any{}})
	require.Equal(t, float64(1), resp["id"])
	res, ok := resp["result"].(map[string]any)
	require.True(t, ok, "expected result object, got %T", resp["result"])
	caps, ok := res["capabilities"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, caps, "textDocumentSync")
	assert.Contains(t, caps, "codeActionProvider")
}

func (p *lspPipe) openDocument(uri, text string) {
	p.notify("textDocument/didOpen", map[string]any{
		"textDocument": map[string]any{
			"uri": uri, "languageId": "markdown", "version": 1, "text": text,
		},
	})
}

func (p *lspPipe) changeDocument(uri string, version int, text string) {
	p.notify("textDocument/didChange", map[string]any{
		"textDocument":   map[string]any{"uri": uri, "version": version},
		"contentChanges": []map[string]any{{"text": text}},
	})
}

// awaitDiagnostics reads frames until it sees publishDiagnostics for
// the given URI, answering server-initiated requests along the way so
// the dispatch loop on the other end stays unblocked.
func (p *lspPipe) awaitDiagnostics(t *testing.T, uri string, deadline time.Time) publishDiagnosticsParams {
	t.Helper()
	for time.Now().Before(deadline) {
		m := p.readFrame()
		method, _ := m["method"].(string)
		if method == "" {
			continue
		}
		if method == "workspace/configuration" || method == "client/registerCapability" {
			id := m["id"]
			p.writeFrame(map[string]any{"jsonrpc": "2.0", "id": id, "result": nil})
			continue
		}
		if method != "textDocument/publishDiagnostics" {
			continue
		}
		raw, _ := json.Marshal(m["params"])
		var diags publishDiagnosticsParams
		require.NoError(t, json.Unmarshal(raw, &diags))
		if diags.URI == uri {
			return diags
		}
	}
	t.Fatalf("timed out waiting for publishDiagnostics on %s", uri)
	return publishDiagnosticsParams{}
}

func (p *lspPipe) shutdown(t *testing.T) {
	t.Helper()
	resp := p.request("shutdown", 99, nil)
	require.Equal(t, float64(99), resp["id"])
	p.notify("exit", nil)
}

func assertHasMDS006(t *testing.T, diags publishDiagnosticsParams) {
	t.Helper()
	for _, d := range diags.Diagnostics {
		if d.Code == "MDS006" {
			return
		}
	}
	t.Fatalf("expected MDS006 in published diagnostics, got %+v", diags.Diagnostics)
}

func assertNoMDS006(t *testing.T, diags publishDiagnosticsParams) {
	t.Helper()
	for _, d := range diags.Diagnostics {
		assert.NotEqual(t, "MDS006", d.Code, "MDS006 should be cleared after fix")
	}
}

// repoRoot returns the repository root via `git rev-parse`.
func repoRoot(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	require.NoError(t, err)
	return strings.TrimSpace(string(out))
}

// publishDiagnosticsParams mirrors the LSP wire shape; redeclared
// here so the test does not depend on internal/lsp's unexported types.
type publishDiagnosticsParams struct {
	URI         string              `json:"uri"`
	Diagnostics []publishedDiagItem `json:"diagnostics"`
}

type publishedDiagItem struct {
	Code    string `json:"code"`
	Source  string `json:"source"`
	Message string `json:"message"`
}
