package main

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"

	"github.com/stretchr/testify/assert"
)

// errWriter always returns an error on Write so we can test the failure path.
type errWriter struct{ err error }

func (e *errWriter) Write(_ []byte) (int, error) { return 0, e.err }

func TestFormatDiagnosticsTo_TextSuccess(t *testing.T) {
	diags := []lint.Diagnostic{{
		File: "foo.md", Line: 1, Column: 1,
		RuleID: "MDS001", RuleName: "test-rule",
		Severity: lint.Warning, Message: "test message",
	}}
	var buf strings.Builder
	code := formatDiagnosticsTo(&buf, diags, "text", true)
	assert.Equal(t, 0, code)
	assert.Contains(t, buf.String(), "foo.md")
}

func TestFormatDiagnosticsTo_JSONSuccess(t *testing.T) {
	diags := []lint.Diagnostic{{
		File: "bar.md", Line: 2, Column: 3,
		RuleID: "MDS002", RuleName: "other-rule",
		Severity: lint.Warning, Message: "json test",
	}}
	var buf strings.Builder
	code := formatDiagnosticsTo(&buf, diags, "json", true)
	assert.Equal(t, 0, code)
	assert.Contains(t, buf.String(), "bar.md")
}

func TestFormatDiagnosticsTo_WriteError(t *testing.T) {
	diags := []lint.Diagnostic{{
		File: "z.md", Line: 1, Column: 1,
		RuleID: "MDS001", RuleName: "test-rule",
		Severity: lint.Warning, Message: "will fail",
	}}
	w := &errWriter{err: errors.New("disk full")}
	code := formatDiagnosticsTo(w, diags, "text", true)
	assert.Equal(t, 2, code)
}

func TestFormatDiagnosticsTo_Empty(t *testing.T) {
	code := formatDiagnosticsTo(io.Discard, nil, "text", true)
	assert.Equal(t, 0, code)
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
			assert.NoError(t, writeDiagnostics(&buf, diags, format, true), format)
			assert.Contains(t, buf.String(), want, format)
		}
	})
	t.Run("returns write error unreported", func(t *testing.T) {
		w := &errWriter{err: errors.New("disk full")}
		assert.EqualError(t, writeDiagnostics(w, diags, "text", true), "disk full")
	})
}

func TestPrintWriteErrorTo(t *testing.T) {
	var buf strings.Builder
	printWriteErrorTo(&buf, errors.New("disk full"))
	assert.Equal(t, "mdsmith: error writing output: disk full\n", buf.String())
}
