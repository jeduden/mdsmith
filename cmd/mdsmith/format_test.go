package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errWriter always returns an error on Write so we can test the failure path.
type errWriter struct{ err error }

func (e *errWriter) Write(_ []byte) (int, error) { return 0, e.err }

// oneDiag is a single text-formattable diagnostic.
var oneDiag = []lint.Diagnostic{{
	File: "foo.md", Line: 1, Column: 1,
	RuleID: "MDS001", RuleName: "test-rule",
	Severity: lint.Warning, Message: "test message",
}}

// withBrokenStderr runs f with os.Stderr open read-only, so every
// write to it fails.
func withBrokenStderr(t *testing.T, f func()) {
	t.Helper()
	ro, err := os.Open(os.DevNull)
	require.NoError(t, err)
	defer ro.Close() //nolint:errcheck // test cleanup
	old := os.Stderr
	os.Stderr = ro
	defer func() { os.Stderr = old }()
	f()
}

// failWithDiagnostics (export's stale bodies, extract's conformance
// failures) follows the same color rule as check: stderr here is a
// pipe, not a terminal, so the text carries no ANSI color. Printed
// diagnostics are exit 1.
func TestFailWithDiagnostics_NoColorOffTerminal(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("FORCE_COLOR", "")
	var code int
	stderr := captureStderr(func() { code = failWithDiagnostics(oneDiag) })
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "foo.md:1:1 MDS001 test message")
	assert.NotContains(t, stderr, "\033[")
}

// A stderr that cannot take the diagnostics is a write error, exit 2,
// not a silent exit 1.
func TestFailWithDiagnostics_WriteErrorIsExit2(t *testing.T) {
	withBrokenStderr(t, func() {
		assert.Equal(t, 2, failWithDiagnostics(oneDiag))
	})
}

func TestWriteStderrDiagnostics(t *testing.T) {
	t.Run("plain on a pipe", func(t *testing.T) {
		var errOut bytes.Buffer
		assert.Equal(t, 0, testIO(t, io.Discard, &errOut).writeStderrDiagnostics(oneDiag))
		assert.Equal(t, "foo.md:1:1 MDS001 test message\n", errOut.String())
	})
	t.Run("colored on a terminal", func(t *testing.T) {
		var errOut bytes.Buffer
		rio := testIO(t, io.Discard, &errOut)
		rio.isTerminal = func(w io.Writer) bool { return w == &errOut }
		assert.Equal(t, 0, rio.writeStderrDiagnostics(oneDiag))
		assert.Contains(t, errOut.String(), "\033[")
	})
	t.Run("FORCE_COLOR colors a pipe", func(t *testing.T) {
		var errOut bytes.Buffer
		rio := testIO(t, io.Discard, &errOut)
		rio.getenv = func(key string) string { return map[string]string{"FORCE_COLOR": "1"}[key] }
		assert.Equal(t, 0, rio.writeStderrDiagnostics(oneDiag))
		assert.Contains(t, errOut.String(), "\033[")
	})
	t.Run("write error", func(t *testing.T) {
		errOut := &failAfterWriter{n: 0}
		assert.Equal(t, 2, testIO(t, io.Discard, errOut).writeStderrDiagnostics(oneDiag))
	})
	t.Run("batches writes", func(t *testing.T) {
		errOut := &countingWriter{}
		diags := manyDiagnostics(20)
		assert.Equal(t, 0, testIO(t, io.Discard, errOut).writeStderrDiagnostics(diags))
		assert.LessOrEqual(t, errOut.calls, 2)
	})
}

func TestWriteDiagnostics(t *testing.T) {
	diags := []lint.Diagnostic{{
		File: "w.md", Line: 1, Column: 1,
		RuleID: "MDS001", RuleName: "test-rule",
		Severity: lint.Warning, Message: "write me",
	}}
	t.Run("formats", func(t *testing.T) {
		for format, want := range map[string]string{
			"text":  "w.md:1:1 MDS001 write me",
			"json":  `"message": "write me"`,
			"sarif": `"version": "2.1.0"`,
		} {
			var buf strings.Builder
			assert.NoError(t, writeDiagnostics(&buf, diags, format, false), format)
			assert.Contains(t, buf.String(), want, format)
		}
	})
	t.Run("color", func(t *testing.T) {
		var plain, colored strings.Builder
		require.NoError(t, writeDiagnostics(&plain, diags, "text", false))
		require.NoError(t, writeDiagnostics(&colored, diags, "text", true))
		assert.NotContains(t, plain.String(), "\033[")
		assert.Contains(t, colored.String(), "\033[")
	})
	t.Run("empty", func(t *testing.T) {
		for format, want := range map[string]string{"text": "", "json": "[]\n"} {
			var buf strings.Builder
			require.NoError(t, writeDiagnostics(&buf, nil, format, false), format)
			assert.Equal(t, want, buf.String(), format)
		}
	})
	t.Run("returns write error unreported", func(t *testing.T) {
		w := &errWriter{err: errors.New("disk full")}
		assert.EqualError(t, writeDiagnostics(w, diags, "text", false), "disk full")
	})
}

func TestPrintWriteErrorTo(t *testing.T) {
	var buf strings.Builder
	printWriteErrorTo(&buf, errors.New("disk full"))
	assert.Equal(t, "mdsmith: error writing output: disk full\n", buf.String())
}
