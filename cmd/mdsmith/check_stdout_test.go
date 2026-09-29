package main

import (
	"bufio"
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

// A failed write to stdout is a runtime error, so its message goes to
// stderr and never into the redirected diagnostics file. 2000
// diagnostics overflow the 64 KiB buffer, so the formatter itself
// sees the failure.
func TestReportCheckResultStreams_FormatterErrorGoesToStderr(t *testing.T) {
	opts := checkCLIOpts{format: "text", noColor: true, stdout: true}
	result := &engine.Result{FilesChecked: 1, Diagnostics: manyDiagnostics(2000)}
	var errOut bytes.Buffer
	code := reportCheckResultStreams(result, opts, &vlog.Logger{}, &alwaysErrorWriter{}, &errOut)
	assert.Equal(t, 2, code)
	assert.Equal(t, "mdsmith: error writing output: write failed\n", errOut.String())
}

// The final flush is the first stdout write when the output fits the
// buffer; its failure is reported on stderr too.
func TestReportCheckResultStreams_StdoutFlushErrorGoesToStderr(t *testing.T) {
	opts := checkCLIOpts{format: "text", stdout: true}
	var errOut bytes.Buffer
	code := reportCheckResultStreams(&engine.Result{FilesChecked: 1}, opts,
		&vlog.Logger{}, &failAfterWriter{n: 0}, &errOut)
	assert.Equal(t, 2, code)
	assert.Equal(t, "mdsmith: error writing output: write failed\n", errOut.String())
}

func TestWriteCheckReport(t *testing.T) {
	t.Run("diagnostics then stats, flushed", func(t *testing.T) {
		var buf bytes.Buffer
		out := bufio.NewWriter(&buf)
		result := &engine.Result{FilesChecked: 1, Diagnostics: manyDiagnostics(1)}
		require.NoError(t, writeCheckReport(out, result, checkCLIOpts{format: "text", noColor: true}))
		assert.Equal(t, 0, out.Buffered())
		assert.Regexp(t, `(?s)line too long.*stats: checked=1 fixed=0 failures=1 unfixed=1\n$`, buf.String())
	})
	t.Run("returns formatter error", func(t *testing.T) {
		out := bufio.NewWriterSize(&alwaysErrorWriter{}, 16)
		result := &engine.Result{FilesChecked: 1, Diagnostics: manyDiagnostics(1)}
		assert.EqualError(t, writeCheckReport(out, result, checkCLIOpts{format: "text"}), "write failed")
	})
	t.Run("returns flush error", func(t *testing.T) {
		out := bufio.NewWriter(&alwaysErrorWriter{})
		assert.EqualError(t,
			writeCheckReport(out, &engine.Result{FilesChecked: 1}, checkCLIOpts{format: "text"}),
			"write failed")
	})
}
