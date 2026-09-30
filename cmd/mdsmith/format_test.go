package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errWriter always returns an error on Write so we can test the failure path.
type errWriter struct{ err error }

func (e *errWriter) Write(_ []byte) (int, error) { return 0, e.err }

// formatDiagnostics (export's stale bodies, extract's conformance
// failures) follows the same color rule as check: stderr here is a
// pipe, not a terminal, so the text carries no ANSI color.
func TestFormatDiagnostics_NoColorOffTerminal(t *testing.T) {
	diags := []lint.Diagnostic{{
		File: "foo.md", Line: 1, Column: 1,
		RuleID: "MDS001", RuleName: "test-rule",
		Severity: lint.Warning, Message: "test message",
	}}
	var code int
	stderr := captureStderr(func() { code = formatDiagnostics(diags, "text", false) })
	assert.Equal(t, 0, code)
	assert.Contains(t, stderr, "foo.md:1:1 MDS001 test message")
	assert.NotContains(t, stderr, "\033[")
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
