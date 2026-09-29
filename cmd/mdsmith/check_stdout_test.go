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

func TestReportCheckResultTo_StdoutFlagRoutesDiagnostics(t *testing.T) {
	opts := checkCLIOpts{format: "json", stdout: true}
	result := &engine.Result{
		FilesChecked: 1,
		Diagnostics:  manyDiagnostics(1),
		Errors:       []error{errors.New("boom")},
	}
	var out, errOut bytes.Buffer
	code := reportCheckResultTo(result, opts, &vlog.Logger{}, &out, &errOut)
	assert.Equal(t, 1, code)
	assert.Contains(t, out.String(), "line too long")
	assert.NotContains(t, errOut.String(), "line too long")
	assert.Contains(t, errOut.String(), "mdsmith: boom")
	assert.NotContains(t, out.String(), "boom")
}

func TestReportCheckResultTo_TextStatsFollowDiagnostics(t *testing.T) {
	opts := checkCLIOpts{format: "text", noColor: true, stdout: true}
	result := &engine.Result{FilesChecked: 1, Diagnostics: manyDiagnostics(1)}
	var out, errOut bytes.Buffer
	code := reportCheckResultTo(result, opts, &vlog.Logger{}, &out, &errOut)
	assert.Equal(t, 1, code)
	assert.Contains(t, out.String(), "stats: checked=1")
	assert.Empty(t, errOut.String())
}

func TestReportCheckResultTo_DefaultKeepsStderr(t *testing.T) {
	opts := checkCLIOpts{format: "json"}
	result := &engine.Result{FilesChecked: 1, Diagnostics: manyDiagnostics(1)}
	var out, errOut bytes.Buffer
	code := reportCheckResultTo(result, opts, &vlog.Logger{}, &out, &errOut)
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
func TestReportCheckResultTo_BrokenStderrStillWritesStdout(t *testing.T) {
	opts := checkCLIOpts{format: "json", stdout: true}
	result := &engine.Result{
		FilesChecked: 1,
		Diagnostics:  manyDiagnostics(1),
		Errors:       []error{errors.New("boom")},
	}
	var out bytes.Buffer
	code := reportCheckResultTo(result, opts, &vlog.Logger{}, &out, &alwaysErrorWriter{})
	assert.Equal(t, 1, code)
	var diags []map[string]any
	require.NoError(t, json.Unmarshal(out.Bytes(), &diags), "stdout=%q", out.String())
	assert.Len(t, diags, 1)
}

// A failed write to stdout is a runtime error, so its message goes to
// stderr and never into the redirected diagnostics file. 2000
// diagnostics overflow the 64 KiB buffer, so the formatter itself
// sees the failure.
func TestReportCheckResultTo_FormatterErrorGoesToStderr(t *testing.T) {
	opts := checkCLIOpts{format: "text", noColor: true, stdout: true}
	result := &engine.Result{FilesChecked: 1, Diagnostics: manyDiagnostics(2000)}
	var errOut bytes.Buffer
	code := reportCheckResultTo(result, opts, &vlog.Logger{}, &alwaysErrorWriter{}, &errOut)
	assert.Equal(t, 2, code)
	assert.Equal(t, "mdsmith: error writing output: write failed\n", errOut.String())
}

// The final flush is the first stdout write when the output fits the
// buffer; its failure is reported on stderr too.
func TestReportCheckResultTo_StdoutFlushErrorGoesToStderr(t *testing.T) {
	opts := checkCLIOpts{format: "text", stdout: true}
	var errOut bytes.Buffer
	code := reportCheckResultTo(&engine.Result{FilesChecked: 1}, opts,
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

// A clean run must still leave a valid document in a redirected file:
// `check --stdout -f json > out.json` on a clean tree writes `[]`,
// matching the JSON formatter's shape, and sarif writes an empty SARIF
// log as it always has. --quiet still suppresses all of it, and the
// default stderr route keeps its existing empty-json behavior.
func TestReportCheckResultTo_CleanRunOutput(t *testing.T) {
	tests := []struct {
		name            string
		opts            checkCLIOpts
		wantOut, errOut string
		sarifOn         string
	}{
		{name: "stdout json", opts: checkCLIOpts{format: "json", stdout: true}, wantOut: "[]\n"},
		{name: "stdout sarif", opts: checkCLIOpts{format: "sarif", stdout: true}, sarifOn: "stdout"},
		{
			name:    "stdout text",
			opts:    checkCLIOpts{format: "text", stdout: true},
			wantOut: "stats: checked=1 fixed=0 failures=0 unfixed=0\n",
		},
		{name: "stdout json quiet", opts: checkCLIOpts{format: "json", stdout: true, quiet: true}},
		{name: "stdout sarif quiet", opts: checkCLIOpts{format: "sarif", stdout: true, quiet: true}},
		{name: "stderr json", opts: checkCLIOpts{format: "json"}},
		{name: "stderr sarif", opts: checkCLIOpts{format: "sarif"}, sarifOn: "stderr"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			code := reportCheckResultTo(&engine.Result{FilesChecked: 1}, tt.opts,
				&vlog.Logger{}, &out, &errOut)
			assert.Equal(t, 0, code)
			switch tt.sarifOn {
			case "stdout":
				assertEmptySARIF(t, out.Bytes())
				assert.Empty(t, errOut.String())
			case "stderr":
				assertEmptySARIF(t, errOut.Bytes())
				assert.Empty(t, out.String())
			default:
				assert.Equal(t, tt.wantOut, out.String())
				assert.Equal(t, tt.errOut, errOut.String())
			}
		})
	}
}

func assertEmptySARIF(t *testing.T, b []byte) {
	t.Helper()
	var log struct {
		Version string `json:"version"`
		Runs    []struct {
			Results []any `json:"results"`
		} `json:"runs"`
	}
	require.NoError(t, json.Unmarshal(b, &log), "sarif=%q", b)
	assert.Equal(t, "2.1.0", log.Version)
	require.Len(t, log.Runs, 1)
	assert.Empty(t, log.Runs[0].Results)
}

func TestCheckCLIOpts_StderrFormat(t *testing.T) {
	for _, format := range []string{"text", "json", "sarif"} {
		assert.Equal(t, format, checkCLIOpts{format: format}.stderrFormat(), format)
		assert.Equal(t, "text", checkCLIOpts{format: format, stdout: true}.stderrFormat(), format)
	}
}

// A run that resolved no Markdown file is clean. Under --stdout a
// json or sarif run still writes its empty document; everything else
// writes nothing, as the default route always has.
func TestReportNoFilesTo(t *testing.T) {
	tests := []struct {
		name    string
		opts    checkCLIOpts
		wantOut string
		sarif   bool
	}{
		{name: "stdout json", opts: checkCLIOpts{format: "json", stdout: true}, wantOut: "[]\n"},
		{name: "stdout sarif", opts: checkCLIOpts{format: "sarif", stdout: true}, sarif: true},
		{name: "stdout text", opts: checkCLIOpts{format: "text", stdout: true}},
		{name: "stdout json quiet", opts: checkCLIOpts{format: "json", stdout: true, quiet: true}},
		{name: "stderr json", opts: checkCLIOpts{format: "json"}},
		{name: "stderr sarif", opts: checkCLIOpts{format: "sarif"}},
		{name: "stderr text", opts: checkCLIOpts{format: "text"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			assert.Equal(t, 0, reportNoFilesTo(tt.opts, &out, &errOut))
			if tt.sarif {
				assertEmptySARIF(t, out.Bytes())
			} else {
				assert.Equal(t, tt.wantOut, out.String())
			}
			assert.Empty(t, errOut.String())
		})
	}
}

// The empty document is still a stdout write, so its failure is a
// runtime error reported on stderr with exit 2.
func TestReportNoFilesTo_WriteErrorGoesToStderr(t *testing.T) {
	var errOut bytes.Buffer
	code := reportNoFilesTo(checkCLIOpts{format: "json", stdout: true}, &alwaysErrorWriter{}, &errOut)
	assert.Equal(t, 2, code)
	assert.Equal(t, "mdsmith: error writing output: write failed\n", errOut.String())
}
