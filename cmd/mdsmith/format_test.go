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
