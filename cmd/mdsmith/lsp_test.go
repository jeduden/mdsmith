package main_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/textproto"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLSPInitializeOverPipe spawns the shared mdsmith binary built
// by TestMain (in e2e_test.go), drives the server through a single
// initialize → didOpen → didChange → shutdown round trip over the
// process's stdio pipes, and asserts that diagnostics appear after
// didOpen. This is the acceptance gate for plan 121's "speaks LSP
// over stdio" criterion.
//
// Reusing the e2e_test.go binary skips per-test compilation (which
// otherwise consumed the per-step deadline under parallel load) and
// inherits the coverage instrumentation, so the subprocess's
// cmd/mdsmith/lsp.go execution counts toward the merged profile.
func TestLSPInitializeOverPipe(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping LSP subprocess test in -short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	pipe := startLSPSubprocess(t, ctx, binaryPath)

	pipe.assertInitializeAck(t)
	pipe.notify("initialized", map[string]any{})

	uri := "file:///tmp/lsp-e2e.md"
	pipe.openDocument(uri, "# Hi\n\ndirty line   \n")

	// Give each step its own deadline so a slow first step doesn't
	// starve the next one.
	diags := pipe.awaitDiagnostics(t, uri, time.Now().Add(30*time.Second))
	assertHasMDS006(t, diags)

	pipe.changeDocument(uri, 2, "# Hi\n\nclean line\n")
	// fetchClientSettings re-lints once the workspace/configuration
	// reply lands, so a stale dirty-buffer publish can race the
	// post-didChange clean publish. Drain frames until we see a
	// publish whose diagnostics no longer include MDS006.
	pipe.awaitClearedDiagnostics(t, uri, time.Now().Add(30*time.Second), "MDS006")

	pipe.shutdown(t)
}

// lspPipe wraps a child process's stdio so individual test steps stay
// short. It also handles the inevitable server-initiated requests
// (workspace/configuration, client/registerCapability) that arrive
// interleaved with the publishDiagnostics notifications we wait for.
type lspPipe struct {
	t        *testing.T
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	stdout   *bufio.Reader
	waitOnce sync.Once
}

func startLSPSubprocess(t *testing.T, ctx context.Context, binary string) *lspPipe {
	t.Helper()
	// Pass `--stdio` exactly like vscode-languageclient does for
	// TransportKind.stdio. Without it, the subprocess wouldn't
	// exercise the no-op flag we accept for compatibility, and a
	// regression that broke flag acceptance (as it did before
	// 4dbb669) would slip through this test.
	cmd := exec.CommandContext(ctx, binary, "lsp", "--stdio")
	cmd.Dir = repoRoot(t)
	cmd.Env = envWithCoverDir(coverDir)
	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	cmd.Stderr = nil
	// Override the default CommandContext cancel (Process.Kill →
	// SIGKILL) with a graceful stdin-close. The LSP server's read
	// loop exits on EOF, runs its own shutdown, and the Go runtime
	// flushes coverage counters to GOCOVERDIR before the process
	// exits. SIGKILL would skip every atexit hook and lose those
	// counters — which is exactly the race the previous "manually
	// call p.wait() before defer cancel()" workaround tried to
	// avoid. With this override the race is structurally
	// impossible: any path to context cancel goes through the
	// graceful close first, and WaitDelay caps how long Wait
	// blocks if the subprocess somehow hangs after EOF.
	cmd.Cancel = func() error { return stdin.Close() }
	cmd.WaitDelay = 5 * time.Second
	require.NoError(t, cmd.Start())
	p := &lspPipe{t: t, cmd: cmd, stdin: stdin, stdout: bufio.NewReader(stdout)}
	// Cleanup is the safety-net path for tests that fail before
	// shutdown() runs (e.g. a require.NoError mid-test). When the
	// happy path runs, shutdown() already waited and the once
	// guard makes this a no-op rather than a "Wait was already
	// called" error that would obscure real failures.
	t.Cleanup(func() {
		_ = stdin.Close()
		p.wait()
	})
	return p
}

// wait blocks for the subprocess to exit. Idempotent — Wait is
// inherently single-shot, so we guard with sync.Once.
func (p *lspPipe) wait() {
	p.waitOnce.Do(func() {
		_ = p.cmd.Wait()
	})
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

// fullClientCapabilities mirrors the subset of VS Code's
// capabilities the server actually consults. The test must send
// these so the server's caps-gated follow-ups (workspace/
// configuration, client/registerCapability) actually fire — the
// pipe relies on responding to them to drive run-mode-dependent
// behavior (didChange linting, watcher-triggered re-lints).
func fullClientCapabilities() map[string]any {
	return map[string]any{
		"workspace": map[string]any{
			"configuration": true,
			"didChangeWatchedFiles": map[string]any{
				"dynamicRegistration": true,
			},
		},
	}
}

func (p *lspPipe) assertInitializeAck(t *testing.T) {
	t.Helper()
	resp := p.request("initialize", 1, map[string]any{
		"capabilities": fullClientCapabilities(),
	})
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
		if method == "workspace/configuration" {
			// Reply with run=onType so didChange events trigger lint
			// passes — the test exercises that flow explicitly.
			id := m["id"]
			p.writeFrame(map[string]any{
				"jsonrpc": "2.0", "id": id,
				"result": []map[string]any{{"run": "onType"}},
			})
			continue
		}
		if method == "client/registerCapability" {
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
	// Use requestPickResult so post-rename publishDiagnostics
	// notifications interleaved before the response don't stall
	// the drain loop and consume the shutdown reply instead.
	p.requestPickResult(t, "shutdown", 99, nil)
	p.notify("exit", nil)
	// Close stdin and wait so the test asserts the subprocess
	// actually exited cleanly when reached. The cover-flush race
	// itself is handled by the cmd.Cancel override in
	// startLSPSubprocess; even on the failure path, where this
	// function never runs, context cancel triggers the same
	// graceful close. p.wait is idempotent (sync.Once), so the
	// cleanup hook in startLSPSubprocess is a no-op after this
	// returns.
	_ = p.stdin.Close()
	p.wait()
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

// awaitClearedDiagnostics keeps reading publishDiagnostics frames
// for uri until one arrives that no longer carries the named code.
// This handles the case where a stale lint pass (e.g. the re-lint
// fetchClientSettings issues after the workspace/configuration
// response lands) publishes the old diagnostics in between the
// caller's didChange and the post-change publish.
func (p *lspPipe) awaitClearedDiagnostics(t *testing.T, uri string, deadline time.Time, code string) {
	t.Helper()
	for time.Now().Before(deadline) {
		diags := p.awaitDiagnostics(t, uri, deadline)
		var stillHas bool
		for _, d := range diags.Diagnostics {
			if d.Code == code {
				stillHas = true
				break
			}
		}
		if !stillHas {
			return
		}
	}
	t.Fatalf("timed out waiting for %s to clear from %s", code, uri)
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

// TestLSPSubprocess_CoverFlushOnContextCancel pins the
// architectural invariant the cmd.Cancel override enforces:
// when the context is cancelled (the failure path in real
// tests, where defer cancel() runs before the cleanup hook),
// the subprocess MUST receive a graceful stdin-close instead
// of SIGKILL. Without that, Go's coverage runtime never
// flushes counters to GOCOVERDIR and e2e_coverage.txt
// becomes non-deterministic — which manifests as codecov's
// "indirect coverage changes" gate firing on every push.
//
// The test exercises only the lifecycle (start, cancel
// without calling shutdown(), wait, observe coverage data
// files in GOCOVERDIR), not the LSP protocol itself, so a
// regression that re-introduces the SIGKILL race fails here
// before any protocol-level test would notice.
func TestLSPSubprocess_CoverFlushOnContextCancel(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping LSP subprocess test in -short mode")
	}
	// Bound wait time so a regression in the WaitDelay
	// path doesn't hang the test suite.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	// Use a fresh GOCOVERDIR so we can count files written by
	// this one subprocess in isolation.
	dir := t.TempDir()

	cmd := exec.CommandContext(ctx, binaryPath, "lsp", "--stdio")
	cmd.Env = envWithCoverDir(dir)
	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)
	// Drain stdout so the LSP server's writes don't block; it
	// emits initialize-time work that would otherwise pile up
	// in the pipe.
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	go func() { _, _ = io.Copy(io.Discard, stdout) }()

	// Same override as the production path under test.
	cmd.Cancel = func() error { return stdin.Close() }
	cmd.WaitDelay = 5 * time.Second
	require.NoError(t, cmd.Start())

	// Give the subprocess long enough to register init code
	// paths before we cancel — without this the coverage file
	// might be written but empty of meaningful counts.
	time.Sleep(200 * time.Millisecond)

	// Trigger the graceful-close path the same way a failed
	// test's `defer cancel()` would.
	cancel()
	// cmd.Wait may return context.Canceled (the cancel reason)
	// or an exec.ExitError carrying the subprocess's exit code.
	// What matters here is that the process exited *cleanly* —
	// verified by the GOCOVERDIR file count below — not the
	// shape of the error. A SIGKILL would also exit but skip
	// the atexit hooks.
	_ = cmd.Wait()

	// The Go runtime writes per-process .meta-* / .counters-*
	// files to GOCOVERDIR on a clean exit. If the cancel had
	// sent SIGKILL, none would appear.
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.NotEmpty(t, entries,
		"GOCOVERDIR is empty — subprocess was killed before "+
			"coverage flushed; cmd.Cancel override is missing or broken")
}
