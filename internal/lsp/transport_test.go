package lsp

import (
	"bytes"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReadRawMissingContentLength locks the rejection path: the LSP
// transport requires a Content-Length header on every frame.
func TestReadRawMissingContentLength(t *testing.T) {
	t.Parallel()
	r := strings.NewReader("Other-Header: 1\r\n\r\nbody")
	tr := newTransport(r, io.Discard)
	_, err := tr.readRaw()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing Content-Length")
}

func TestReadRawInvalidContentLength(t *testing.T) {
	t.Parallel()
	r := strings.NewReader("Content-Length: not-a-number\r\n\r\nbody")
	tr := newTransport(r, io.Discard)
	_, err := tr.readRaw()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid Content-Length")
}

func TestReadRawNegativeContentLength(t *testing.T) {
	t.Parallel()
	r := strings.NewReader("Content-Length: -5\r\n\r\nbody")
	tr := newTransport(r, io.Discard)
	_, err := tr.readRaw()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "out of bounds")
}

func TestReadRawHugeContentLength(t *testing.T) {
	t.Parallel()
	// 64 MiB + 1 — the cap is 64*1024*1024 inclusive.
	r := strings.NewReader("Content-Length: 67108865\r\n\r\nbody")
	tr := newTransport(r, io.Discard)
	_, err := tr.readRaw()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "out of bounds")
}

func TestReadRawTruncatedBody(t *testing.T) {
	t.Parallel()
	// Says 100 bytes but only 4 are present.
	r := strings.NewReader("Content-Length: 100\r\n\r\nshort")
	tr := newTransport(r, io.Discard)
	_, err := tr.readRaw()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading body")
}

func TestReadRawValidFrame(t *testing.T) {
	t.Parallel()
	body := `{"jsonrpc":"2.0","method":"x"}`
	frame := "Content-Length: " + itoaTransport(len(body)) + "\r\n\r\n" + body
	r := strings.NewReader(frame)
	tr := newTransport(r, io.Discard)
	out, err := tr.readRaw()
	require.NoError(t, err)
	assert.Equal(t, body, string(out))
}

func TestWriteJSONFraming(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	tr := newTransport(strings.NewReader(""), &buf)
	require.NoError(t, tr.writeJSON(map[string]any{"hello": "world"}))
	out := buf.String()
	assert.Contains(t, out, "Content-Length: ")
	assert.Contains(t, out, `{"hello":"world"}`)
}

func TestWriteJSONUnencodableInput(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	tr := newTransport(strings.NewReader(""), &buf)
	// json cannot encode a channel; expect a wrapped error.
	err := tr.writeJSON(make(chan int))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "encoding JSON")
}

func TestWriteJSONWriterFails(t *testing.T) {
	t.Parallel()
	tr := newTransport(strings.NewReader(""), failingWriter{})
	err := tr.writeJSON(map[string]any{"x": 1})
	require.Error(t, err)
}

func TestWriteResponseEmitsNullForNilResult(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	tr := newTransport(strings.NewReader(""), &buf)
	require.NoError(t, tr.writeResponse(json.RawMessage(`5`), nil))
	out := buf.String()
	assert.Contains(t, out, `"result":null`)
}

func TestWriteResponseUnencodableResult(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	tr := newTransport(strings.NewReader(""), &buf)
	err := tr.writeResponse(json.RawMessage(`5`), make(chan int))
	require.Error(t, err)
}

func TestWriteErrorEmitsErrorBody(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	tr := newTransport(strings.NewReader(""), &buf)
	require.NoError(t, tr.writeError(json.RawMessage(`5`), -32601, "no method"))
	out := buf.String()
	assert.Contains(t, out, `"code":-32601`)
	assert.Contains(t, out, `"message":"no method"`)
}

func TestWriteNotificationFraming(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	tr := newTransport(strings.NewReader(""), &buf)
	require.NoError(t, tr.writeNotification("textDocument/publishDiagnostics", map[string]any{"k": 1}))
	out := buf.String()
	assert.Contains(t, out, `"method":"textDocument/publishDiagnostics"`)
}

func TestWriteRequestFraming(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	tr := newTransport(strings.NewReader(""), &buf)
	require.NoError(t, tr.writeRequest(json.RawMessage(`7`), "client/registerCapability", nil))
	out := buf.String()
	assert.Contains(t, out, `"id":7`)
	assert.Contains(t, out, `"method":"client/registerCapability"`)
}

// failingWriter always returns an error so transport-level writes can
// be exercised on their failure path.
type failingWriter struct{}

func (failingWriter) Write(p []byte) (int, error) {
	return 0, io.ErrShortWrite
}

// secondCallFailsWriter accepts the first Write (the header) and
// errors on every subsequent call. Used to exercise the body-write
// failure branch in transport.writeJSON without short-circuiting on
// the header write first.
type secondCallFailsWriter struct {
	calls int
	buf   bytes.Buffer
}

func (w *secondCallFailsWriter) Write(p []byte) (int, error) {
	w.calls++
	if w.calls == 1 {
		return w.buf.Write(p)
	}
	return 0, io.ErrShortWrite
}

func TestWriteJSONBodyWriteFails(t *testing.T) {
	t.Parallel()
	w := &secondCallFailsWriter{}
	tr := newTransport(strings.NewReader(""), w)
	err := tr.writeJSON(map[string]any{"x": 1})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "writing body")
}

func TestWriteJSONRecordsFirstError(t *testing.T) {
	t.Parallel()
	tr := newTransport(strings.NewReader(""), failingWriter{})
	require.NoError(t, tr.WriteError(), "no writes yet, no error")
	err := tr.writeJSON(map[string]any{"x": 1})
	require.Error(t, err)
	assert.ErrorIs(t, tr.WriteError(), io.ErrShortWrite,
		"transport must remember the first write failure for Run() to surface")
}

func TestWriteJSONPreservesFirstError(t *testing.T) {
	t.Parallel()
	tr := newTransport(strings.NewReader(""), failingWriter{})
	_ = tr.writeJSON(map[string]any{"x": 1})
	first := tr.WriteError()
	// A second write also fails, but WriteError must still return the
	// original wrapped error so the caller sees the first cause.
	_ = tr.writeJSON(map[string]any{"x": 2})
	assert.Same(t, errPtr(first), errPtr(tr.WriteError()))
}

// TestRecordWriteErrIgnoresNil pins the early-return guard:
// recordWriteErr(nil) must not stash a sentinel-typed nil under
// the atomic pointer (which would make WriteError() return a
// non-nil error from a nil error value via interface wrapping).
func TestRecordWriteErrIgnoresNil(t *testing.T) {
	t.Parallel()
	tr := newTransport(strings.NewReader(""), io.Discard)
	tr.recordWriteErr(nil)
	assert.NoError(t, tr.WriteError())
}

// errPtr lets the test assert pointer-identity on error values.
func errPtr(e error) any { return e }

func itoaTransport(i int) string {
	return strconv.Itoa(i)
}
