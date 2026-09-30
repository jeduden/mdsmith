package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/engine"
	vlog "github.com/jeduden/mdsmith/internal/log"
)

// fileIO is testIO with a create hook that hands out f and records
// the path it was asked for in *created.
func fileIO(t *testing.T, stdout, stderr io.Writer, f *fakeReportFile, created *string) reportIO {
	t.Helper()
	rio := testIO(t, stdout, stderr)
	rio.create = func(path string) (io.WriteCloser, error) {
		*created = path
		return f, nil
	}
	return rio
}

// The report (diagnostics and the stats line) goes where -o points;
// runtime errors stay on stderr on every route.
func TestReportCheckResultTo_RoutesReport(t *testing.T) {
	result := &engine.Result{
		FilesChecked: 1,
		Diagnostics:  manyDiagnostics(1),
		Errors:       []error{errors.New("boom")},
	}
	opts := checkCLIOpts{reportFlags: reportFlags{format: "json"}}

	t.Run("default is stderr", func(t *testing.T) {
		var out, errOut bytes.Buffer
		code := reportCheckResultTo(result, opts, &vlog.Logger{}, testIO(t, &out, &errOut))
		assert.Equal(t, 1, code)
		assert.Empty(t, out.String())
		assert.Contains(t, errOut.String(), "mdsmith: boom")
		assert.Contains(t, errOut.String(), "line too long")
	})
	t.Run("dash is stdout", func(t *testing.T) {
		var out, errOut bytes.Buffer
		o := opts
		o.output = "-"
		code := reportCheckResultTo(result, o, &vlog.Logger{}, testIO(t, &out, &errOut))
		assert.Equal(t, 1, code)
		assert.Equal(t, 1, len(decodeJSONDiags(t, out.Bytes())))
		assert.Equal(t, "mdsmith: boom\n", errOut.String())
	})
	t.Run("path is a file", func(t *testing.T) {
		var out, errOut bytes.Buffer
		f := &fakeReportFile{}
		var created string
		o := opts
		o.output = "report.json"
		code := reportCheckResultTo(result, o, &vlog.Logger{}, fileIO(t, &out, &errOut, f, &created))
		assert.Equal(t, 1, code)
		assert.Equal(t, "report.json", created)
		assert.Equal(t, 1, len(decodeJSONDiags(t, f.Bytes())))
		assert.True(t, f.closed)
		assert.Empty(t, out.String())
		assert.Equal(t, "mdsmith: boom\n", errOut.String())
	})
}

func decodeJSONDiags(t *testing.T, b []byte) []map[string]any {
	t.Helper()
	var diags []map[string]any
	require.NoError(t, json.Unmarshal(b, &diags), "report=%q", b)
	return diags
}

func TestReportCheckResultTo_TextStatsFollowDiagnostics(t *testing.T) {
	opts := checkCLIOpts{reportFlags: reportFlags{format: "text", output: "-"}}
	result := &engine.Result{FilesChecked: 1, Diagnostics: manyDiagnostics(1)}
	var out, errOut bytes.Buffer
	code := reportCheckResultTo(result, opts, &vlog.Logger{}, testIO(t, &out, &errOut))
	assert.Equal(t, 1, code)
	assert.Regexp(t, `(?s)^f\.md:1:1 MDS001 line too long\n.*\nstats: checked=1 fixed=0 failures=1 unfixed=1\n$`,
		out.String())
	assert.Empty(t, errOut.String())
}

// Off the default route the report is batched through its own
// buffer, so a diagnostic-heavy run does not pay one write per line.
func TestReportCheckResultTo_BuffersRoutedWrites(t *testing.T) {
	opts := checkCLIOpts{reportFlags: reportFlags{format: "text", output: "-"}}
	result := &engine.Result{FilesChecked: 1, Diagnostics: manyDiagnostics(100)}
	w := &countingWriter{}
	code := reportCheckResultTo(result, opts, &vlog.Logger{}, testIO(t, w, io.Discard))
	assert.Equal(t, 1, code)
	assert.Contains(t, w.buf.String(), "line too long")
	assert.LessOrEqual(t, w.calls, 4)
}

// A failed report write is a runtime error: its message goes to
// stderr, never into the report, and the exit code is 2. 2000
// diagnostics overflow the 64 KiB buffer, so the formatter itself
// sees the failure.
func TestReportCheckResultTo_WriteErrorGoesToStderr(t *testing.T) {
	opts := checkCLIOpts{reportFlags: reportFlags{format: "text", output: "-"}}
	result := &engine.Result{FilesChecked: 1, Diagnostics: manyDiagnostics(2000)}
	var errOut bytes.Buffer
	code := reportCheckResultTo(result, opts, &vlog.Logger{}, testIO(t, &alwaysErrorWriter{}, &errOut))
	assert.Equal(t, 2, code)
	assert.Equal(t, "mdsmith: error writing output: write failed\n", errOut.String())
}

// A stderr that fails writes must not keep the report off stdout, and
// the exit code must still be the lint result (1), not 2.
func TestReportCheckResultTo_BrokenStderrStillWritesReport(t *testing.T) {
	opts := checkCLIOpts{reportFlags: reportFlags{format: "json", output: "-"}}
	result := &engine.Result{
		FilesChecked: 1,
		Diagnostics:  manyDiagnostics(1),
		Errors:       []error{errors.New("boom")},
	}
	var out bytes.Buffer
	code := reportCheckResultTo(result, opts, &vlog.Logger{}, testIO(t, &out, &alwaysErrorWriter{}))
	assert.Equal(t, 1, code)
	assert.Len(t, decodeJSONDiags(t, out.Bytes()), 1)
}

// A clean run writes a valid document on every route: `[]` for json
// and a SARIF log with one run and no results for sarif. Text writes
// only the stats line. -q writes nothing.
func TestReportCheckResultTo_CleanRunOutput(t *testing.T) {
	for _, output := range []string{"", "-"} {
		for _, tc := range []struct {
			name  string
			opts  checkCLIOpts
			want  string
			sarif bool
		}{
			{name: "json", opts: checkCLIOpts{reportFlags: reportFlags{format: "json"}}, want: "[]\n"},
			{name: "sarif", opts: checkCLIOpts{reportFlags: reportFlags{format: "sarif"}}, sarif: true},
			{name: "text", opts: checkCLIOpts{reportFlags: reportFlags{format: "text"}}, want: "stats: checked=1 fixed=0 failures=0 unfixed=0\n"},
			{name: "json quiet", opts: checkCLIOpts{reportFlags: reportFlags{format: "json", quiet: true}}},
			{name: "sarif quiet", opts: checkCLIOpts{reportFlags: reportFlags{format: "sarif", quiet: true}}},
			{name: "text quiet", opts: checkCLIOpts{reportFlags: reportFlags{format: "text", quiet: true}}},
		} {
			t.Run("-o "+output+" "+tc.name, func(t *testing.T) {
				var out, errOut bytes.Buffer
				tc.opts.output = output
				code := reportCheckResultTo(&engine.Result{FilesChecked: 1}, tc.opts,
					&vlog.Logger{}, testIO(t, &out, &errOut))
				assert.Equal(t, 0, code)
				report, other := &errOut, &out
				if output == "-" {
					report, other = &out, &errOut
				}
				if tc.sarif {
					assertEmptySARIF(t, report.Bytes())
				} else {
					assert.Equal(t, tc.want, report.String())
				}
				assert.Empty(t, other.String())
			})
		}
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

// Text output carries ANSI color only when the report's destination
// is a terminal and neither --no-color nor NO_COLOR turns it off.
func TestReportCheckResultTo_Color(t *testing.T) {
	result := &engine.Result{FilesChecked: 1, Diagnostics: manyDiagnostics(1)}
	for _, tc := range []struct {
		name    string
		tty     bool
		noColor bool
		env     string
		want    bool
	}{
		{name: "terminal", tty: true, want: true},
		{name: "not a terminal"},
		{name: "terminal with --no-color", tty: true, noColor: true},
		{name: "terminal with NO_COLOR", tty: true, env: "1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			rio := testIO(t, &out, io.Discard)
			rio.isTerminal = func(w io.Writer) bool { return tc.tty && w == &out }
			rio.getenv = func(string) string { return tc.env }
			opts := checkCLIOpts{reportFlags: reportFlags{format: "text", output: "-", noColor: tc.noColor}}
			assert.Equal(t, 1, reportCheckResultTo(result, opts, &vlog.Logger{}, rio))
			assert.Equal(t, tc.want, bytes.Contains(out.Bytes(), []byte("\033[")), "report=%q", out.String())
		})
	}
}

func TestWriteCheckReport(t *testing.T) {
	result := &engine.Result{FilesChecked: 1, Diagnostics: manyDiagnostics(1)}
	t.Run("diagnostics then stats", func(t *testing.T) {
		var buf bytes.Buffer
		require.NoError(t, writeCheckReport(&buf, result, checkCLIOpts{reportFlags: reportFlags{format: "text"}}, false))
		assert.Regexp(t, `(?s)line too long.*stats: checked=1 fixed=0 failures=1 unfixed=1\n$`, buf.String())
	})
	t.Run("quiet writes nothing", func(t *testing.T) {
		for _, output := range []string{"", "-"} {
			var buf bytes.Buffer
			opts := checkCLIOpts{reportFlags: reportFlags{format: "json", output: output, quiet: true}}
			require.NoError(t, writeCheckReport(&buf, result, opts, false))
			assert.Empty(t, buf.String(), "-o %q", output)
		}
	})
	t.Run("quiet still fills an -o file", func(t *testing.T) {
		var buf bytes.Buffer
		opts := checkCLIOpts{reportFlags: reportFlags{format: "text", output: "r.txt", quiet: true}}
		require.NoError(t, writeCheckReport(&buf, result, opts, false))
		assert.Regexp(t, `(?s)line too long.*stats: checked=1 fixed=0 failures=1 unfixed=1\n$`, buf.String())
	})
	t.Run("returns formatter error", func(t *testing.T) {
		assert.EqualError(t,
			writeCheckReport(&alwaysErrorWriter{}, result, checkCLIOpts{reportFlags: reportFlags{format: "text"}}, false),
			"write failed")
	})
}

func TestParseCheckFlags_Output(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{args: []string{"a.md"}, want: ""},
		{args: []string{"-o", "out.json", "a.md"}, want: "out.json"},
		{args: []string{"--output", "-", "a.md"}, want: "-"},
		{args: []string{"--output=-"}, want: "-"},
	} {
		opts, _, _, code := parseCheckFlags(tc.args)
		require.Equal(t, -1, code, "%v", tc.args)
		assert.Equal(t, tc.want, opts.output, "%v", tc.args)
	}
}

// An empty -o value is a usage error rather than a silent fall-back
// to stderr: `-o "$REPORT"` with REPORT unset must not look like it
// worked.
func TestParseCheckFlags_EmptyOutputIsUsageError(t *testing.T) {
	stderr := captureStderr(func() {
		_, _, _, code := parseCheckFlags([]string{"-o", "", "a.md"})
		assert.Equal(t, 2, code)
	})
	assert.Equal(t, "mdsmith: check: --output needs a path, or - for stdout\n", stderr)
}

// --stdout was never released; -o - replaces it.
func TestParseCheckFlags_StdoutFlagIsGone(t *testing.T) {
	stderr := captureStderr(func() {
		_, _, _, code := parseCheckFlags([]string{"--stdout"})
		assert.Equal(t, 2, code)
	})
	assert.Contains(t, stderr, "unknown flag: --stdout")
}

// A run that resolved no Markdown file is clean. json and sarif still
// write their empty document on every route; text writes nothing, as
// it always has; -q writes nothing.
func TestReportNoFiles(t *testing.T) {
	for _, output := range []string{"", "-"} {
		for _, tc := range []struct {
			name  string
			flags reportFlags
			want  string
			sarif bool
		}{
			{name: "json", flags: reportFlags{format: "json"}, want: "[]\n"},
			{name: "sarif", flags: reportFlags{format: "sarif"}, sarif: true},
			{name: "text", flags: reportFlags{format: "text"}},
			{name: "json quiet", flags: reportFlags{format: "json", quiet: true}},
		} {
			t.Run("-o "+output+" "+tc.name, func(t *testing.T) {
				var out, errOut bytes.Buffer
				tc.flags.output = output
				assert.Equal(t, 0, reportNoFiles(tc.flags, testIO(t, &out, &errOut)))
				report, other := &errOut, &out
				if output == "-" {
					report, other = &out, &errOut
				}
				if tc.sarif {
					assertEmptySARIF(t, report.Bytes())
				} else {
					assert.Equal(t, tc.want, report.String())
				}
				assert.Empty(t, other.String())
			})
		}
	}
}

// -q silences the terminal only: an -o file still gets the report,
// here the empty json document, and nothing reaches the streams.
func TestReportNoFiles_QuietStillFillsFile(t *testing.T) {
	f := &fakeReportFile{}
	var created string
	var out, errOut bytes.Buffer
	code := reportNoFiles(reportFlags{format: "json", output: "r.json", quiet: true},
		fileIO(t, &out, &errOut, f, &created))
	assert.Equal(t, 0, code)
	assert.Equal(t, "[]\n", f.String())
	assert.Empty(t, out.String())
	assert.Empty(t, errOut.String())
}

// With -o <path> a no-files run still creates the file, even when
// there is nothing to write into it, so a stale report never survives
// a run that reached the report.
func TestReportNoFiles_CreatesFile(t *testing.T) {
	f := &fakeReportFile{}
	var created string
	code := reportNoFiles(reportFlags{format: "text", output: "r.txt"},
		fileIO(t, io.Discard, io.Discard, f, &created))
	assert.Equal(t, 0, code)
	assert.Equal(t, "r.txt", created)
	assert.Empty(t, f.String())
	assert.True(t, f.closed)
}

// The empty document is still a report write, so its failure is a
// runtime error reported on stderr with exit 2.
func TestReportNoFiles_WriteErrorGoesToStderr(t *testing.T) {
	var errOut bytes.Buffer
	code := reportNoFiles(reportFlags{format: "json", output: "-"}, testIO(t, &alwaysErrorWriter{}, &errOut))
	assert.Equal(t, 2, code)
	assert.Equal(t, "mdsmith: error writing output: write failed\n", errOut.String())
}
