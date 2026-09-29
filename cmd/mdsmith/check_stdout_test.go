package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/engine"
	vlog "github.com/jeduden/mdsmith/internal/log"
)

func TestReportCheckResultStreams_StdoutFlagRoutesDiagnostics(t *testing.T) {
	opts := checkCLIOpts{format: "json", stdout: true}
	result := &engine.Result{
		FilesChecked: 1,
		Diagnostics:  manyDiagnostics(1),
		Errors:       []error{errors.New("boom")},
	}
	var out, errOut bytes.Buffer
	code := reportCheckResultStreams(result, opts, &vlog.Logger{}, &out, &errOut)
	assert.Equal(t, 1, code)
	assert.Contains(t, out.String(), "line too long")
	assert.NotContains(t, errOut.String(), "line too long")
	assert.Contains(t, errOut.String(), "mdsmith: boom")
	assert.NotContains(t, out.String(), "boom")
}

func TestReportCheckResultStreams_TextStatsFollowDiagnostics(t *testing.T) {
	opts := checkCLIOpts{format: "text", noColor: true, stdout: true}
	result := &engine.Result{FilesChecked: 1, Diagnostics: manyDiagnostics(1)}
	var out, errOut bytes.Buffer
	code := reportCheckResultStreams(result, opts, &vlog.Logger{}, &out, &errOut)
	assert.Equal(t, 1, code)
	assert.Contains(t, out.String(), "stats: checked=1")
	assert.Empty(t, errOut.String())
}

func TestReportCheckResultStreams_DefaultKeepsStderr(t *testing.T) {
	opts := checkCLIOpts{format: "json"}
	result := &engine.Result{FilesChecked: 1, Diagnostics: manyDiagnostics(1)}
	var out, errOut bytes.Buffer
	code := reportCheckResultStreams(result, opts, &vlog.Logger{}, &out, &errOut)
	assert.Equal(t, 1, code)
	assert.Empty(t, out.String())
	assert.Contains(t, errOut.String(), "line too long")
}

func TestParseCheckFlags_Stdout(t *testing.T) {
	opts, _, _, code := parseCheckFlags([]string{"--stdout", "a.md"})
	require.Equal(t, -1, code)
	assert.True(t, opts.stdout)

	opts, _, _, code = parseCheckFlags([]string{"a.md"})
	require.Equal(t, -1, code)
	assert.False(t, opts.stdout)
}

// A closed or broken stderr must not keep diagnostics off stdout, and
// the exit code must still report the lint result (1), not a write
// error (2).
func TestReportCheckResultStreams_BrokenStderrStillWritesStdout(t *testing.T) {
	opts := checkCLIOpts{format: "json", stdout: true}
	result := &engine.Result{
		FilesChecked: 1,
		Diagnostics:  manyDiagnostics(1),
		Errors:       []error{errors.New("boom")},
	}
	var out bytes.Buffer
	code := reportCheckResultStreams(result, opts, &vlog.Logger{}, &out, &alwaysErrorWriter{})
	assert.Equal(t, 1, code)
	var diags []map[string]any
	require.NoError(t, json.Unmarshal(out.Bytes(), &diags), "stdout=%q", out.String())
	assert.Len(t, diags, 1)
}
