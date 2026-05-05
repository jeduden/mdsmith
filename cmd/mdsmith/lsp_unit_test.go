package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/textproto"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunLSPRejectsPositionalArgs locks the contract that
// `mdsmith lsp` takes no positional arguments and fails fast (exit
// code 2) when it gets any.
func TestRunLSPRejectsPositionalArgs(t *testing.T) {
	t.Parallel()
	var out, errBuf bytes.Buffer
	code := runLSPWith([]string{"unexpected"}, strings.NewReader(""), &out, &errBuf)
	assert.Equal(t, 2, code)
	assert.Contains(t, errBuf.String(), "takes no positional arguments")
}

func TestRunLSPRejectsUnknownFlag(t *testing.T) {
	t.Parallel()
	var out, errBuf bytes.Buffer
	code := runLSPWith([]string{"--no-such-flag"}, strings.NewReader(""), &out, &errBuf)
	assert.Equal(t, 2, code)
	// The error should be visible on stderr — silently exiting on
	// fs.Parse failure (the previous behavior) made VS Code's
	// "spawn ENOENT" / "exit code 2" trace impossible to debug.
	assert.Contains(t, errBuf.String(), "mdsmith: lsp:")
}

// Regression: vscode-languageclient appends `--stdio` to the
// command's args whenever TransportKind.stdio is selected. Without
// the no-op flag, fs.Parse would return "unknown flag: --stdio"
// and the server would exit 2 on every VS Code launch.
func TestRunLSPAcceptsStdioFlag(t *testing.T) {
	t.Parallel()
	// EOF on stdin → Run returns nil → exit 0. The point is just
	// to get past fs.Parse with the --stdio arg present.
	var out, errBuf bytes.Buffer
	code := runLSPWith([]string{"--stdio"}, strings.NewReader(""), &out, &errBuf)
	assert.Equal(t, 0, code, "stderr=%q", errBuf.String())
}

func TestRunLSPHelpFlag(t *testing.T) {
	t.Parallel()
	// pflag's auto-help prints usage and returns ErrHelp from Parse;
	// runLSPWith maps that to exit 0 to match the rest of the CLI.
	var out, errBuf bytes.Buffer
	code := runLSPWith([]string{"-h"}, strings.NewReader(""), &out, &errBuf)
	assert.Equal(t, 0, code)
	assert.Contains(t, errBuf.String(), "Usage: mdsmith lsp")
}

// failingReader returns an error on every Read so the LSP server's
// transport.readRaw surfaces a non-EOF error and runLSPWith takes
// the "print to stderr, exit 2" branch.
type failingReader struct{}

func (failingReader) Read(p []byte) (int, error) {
	return 0, fmt.Errorf("synthetic read error")
}

func TestRunLSPRunFailurePrintsStderr(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	code := runLSPWith(nil, failingReader{}, &stdout, &stderr)
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr.String(), "mdsmith: lsp:")
}

// TestRunLSPRoundTrip drives the server end to end through
// runLSPWith using in-memory pipes. Exercises the CLI entry point
// and the full Run loop including a clean shutdown.
func TestRunLSPRoundTrip(t *testing.T) {
	t.Parallel()
	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	t.Cleanup(func() {
		_ = stdinW.Close()
		_ = stdoutR.Close()
	})

	done := make(chan int, 1)
	go func() {
		var stderr bytes.Buffer
		done <- runLSPWith(nil, stdinR, stdoutW, &stderr)
		_ = stdoutW.Close()
	}()

	br := bufio.NewReader(stdoutR)

	// initialize
	writeUnitFrame(t, stdinW, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{"capabilities": map[string]any{}},
	})
	resp := readUnitFrame(t, br)
	require.Equal(t, float64(1), resp["id"])

	// shutdown + exit
	writeUnitFrame(t, stdinW, map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "shutdown",
	})
	_ = readUnitFrame(t, br)
	writeUnitFrame(t, stdinW, map[string]any{"jsonrpc": "2.0", "method": "exit"})
	_ = stdinW.Close()

	select {
	case code := <-done:
		assert.Equal(t, 0, code)
	case <-time.After(5 * time.Second):
		t.Fatalf("server did not exit")
	}
}

func writeUnitFrame(t *testing.T, w io.Writer, v any) {
	t.Helper()
	body, err := json.Marshal(v)
	require.NoError(t, err)
	_, err = fmt.Fprintf(w, "Content-Length: %d\r\n\r\n", len(body))
	require.NoError(t, err)
	_, err = w.Write(body)
	require.NoError(t, err)
}

func readUnitFrame(t *testing.T, br *bufio.Reader) map[string]any {
	t.Helper()
	tp := textproto.NewReader(br)
	hdr, err := tp.ReadMIMEHeader()
	require.NoError(t, err)
	cl := hdr.Get("Content-Length")
	require.NotEmpty(t, cl)
	n, err := strconv.Atoi(cl)
	require.NoError(t, err)
	body := make([]byte, n)
	_, err = io.ReadFull(br, body)
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(body, &m))
	return m
}
