package main

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fixpkg "github.com/jeduden/mdsmith/internal/fix"
	vlog "github.com/jeduden/mdsmith/internal/log"
)

func TestParseFixFlags_Output(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{args: []string{"a.md"}, want: ""},
		{args: []string{"-o", "out.json", "a.md"}, want: "out.json"},
		{args: []string{"--output", "-", "a.md"}, want: "-"},
	} {
		opts, _, _, code := parseFixFlags(tc.args)
		require.Equal(t, -1, code, "%v", tc.args)
		assert.Equal(t, tc.want, opts.output, "%v", tc.args)
	}
}

func TestParseFixFlags_Color(t *testing.T) {
	opts, _, _, code := parseFixFlags([]string{"--color=always", "a.md"})
	require.Equal(t, -1, code)
	assert.Equal(t, colorAlways, opts.color)
	opts, _, _, code = parseFixFlags([]string{"--no-color", "a.md"})
	require.Equal(t, -1, code)
	assert.Equal(t, colorNever, opts.color)
}

func TestParseFixFlags_EmptyOutputIsUsageError(t *testing.T) {
	stderr := captureStderr(func() {
		_, _, _, code := parseFixFlags([]string{"--output=", "a.md"})
		assert.Equal(t, 2, code)
	})
	assert.Equal(t, "mdsmith: fix: --output needs a path, or - for stdout\n", stderr)
}

// fix routes its report (remaining diagnostics, the dry-run preview,
// and the stats line) exactly as check does; runtime errors stay on
// stderr.
func TestReportFixResultTo_RoutesReport(t *testing.T) {
	result := &fixpkg.Result{
		FilesChecked: 1,
		Failures:     1,
		Diagnostics:  manyDiagnostics(1),
		Errors:       []error{errors.New("boom")},
	}
	t.Run("default is stderr", func(t *testing.T) {
		var out, errOut bytes.Buffer
		opts := fixCLIOpts{reportFlags: reportFlags{format: "json"}}
		code := reportFixResultTo(opts, result, &vlog.Logger{}, testIO(t, &out, &errOut))
		assert.Equal(t, 1, code)
		assert.Empty(t, out.String())
		assert.Contains(t, errOut.String(), "mdsmith: boom")
		assert.Contains(t, errOut.String(), "line too long")
	})
	t.Run("dash is stdout", func(t *testing.T) {
		var out, errOut bytes.Buffer
		opts := fixCLIOpts{reportFlags: reportFlags{format: "json", output: "-"}}
		code := reportFixResultTo(opts, result, &vlog.Logger{}, testIO(t, &out, &errOut))
		assert.Equal(t, 1, code)
		assert.Len(t, decodeJSONDiags(t, out.Bytes()), 1)
		assert.Equal(t, "mdsmith: boom\n", errOut.String())
	})
	t.Run("path is a file", func(t *testing.T) {
		var out, errOut bytes.Buffer
		f := &fakeReportFile{}
		var created string
		opts := fixCLIOpts{reportFlags: reportFlags{format: "text", output: "fix.txt"}}
		code := reportFixResultTo(opts, result, &vlog.Logger{}, fileIO(t, &out, &errOut, f, &created))
		assert.Equal(t, 1, code)
		assert.Equal(t, "fix.txt", created)
		assert.Regexp(t, `(?s)^f\.md:1:1 MDS001 line too long\n.*stats: checked=1 fixed=0 failures=1 unfixed=1\n$`,
			f.String())
		assert.Empty(t, out.String())
		assert.Equal(t, "mdsmith: boom\n", errOut.String())
	})
}

func TestReportFixResultTo_DryRunPreviewIsPartOfReport(t *testing.T) {
	result := &fixpkg.Result{
		FilesChecked: 1,
		WouldFix:     1,
		WouldFixFiles: []fixpkg.WouldFixFile{{
			Path: "a.md", Count: 1,
			Rules: []fixpkg.RuleFixCount{{RuleID: "MDS009", Count: 1}},
		}},
	}
	var out, errOut bytes.Buffer
	opts := fixCLIOpts{reportFlags: reportFlags{format: "text", output: "-"}, dryRun: true}
	code := reportFixResultTo(opts, result, &vlog.Logger{}, testIO(t, &out, &errOut))
	assert.Equal(t, 0, code)
	assert.Equal(t, "a.md: would fix 1 violation (MDS009)\n"+
		"stats: checked=1 fixed=0 failures=0 unfixed=0 would-fix=1\n", out.String())
	assert.Empty(t, errOut.String())
}

// A clean json run writes `[]` and a clean sarif run an empty log on
// every route, as check does. -q writes nothing.
func TestReportFixResultTo_CleanRunOutput(t *testing.T) {
	for _, output := range []string{"", "-"} {
		for _, tc := range []struct {
			name   string
			flags  reportFlags
			dryRun bool
			want   string
			sarif  bool
		}{
			{name: "json", flags: reportFlags{format: "json"}, want: "[]\n"},
			{name: "dry-run json", flags: reportFlags{format: "json"}, dryRun: true, want: "[]\n"},
			{name: "sarif", flags: reportFlags{format: "sarif"}, sarif: true},
			{name: "dry-run sarif", flags: reportFlags{format: "sarif"}, dryRun: true, sarif: true},
			{name: "text", flags: reportFlags{format: "text"}, want: "stats: checked=1 fixed=0 failures=0 unfixed=0\n"},
			{name: "json quiet", flags: reportFlags{format: "json", quiet: true}},
			{name: "dry-run json quiet", flags: reportFlags{format: "json", quiet: true}, dryRun: true},
		} {
			t.Run("-o "+output+" "+tc.name, func(t *testing.T) {
				var out, errOut bytes.Buffer
				tc.flags.output = output
				opts := fixCLIOpts{reportFlags: tc.flags, dryRun: tc.dryRun}
				code := reportFixResultTo(opts, &fixpkg.Result{FilesChecked: 1},
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

func TestReportFixResultTo_Color(t *testing.T) {
	result := &fixpkg.Result{FilesChecked: 1, Diagnostics: manyDiagnostics(1)}
	for _, tty := range []bool{true, false} {
		var out bytes.Buffer
		rio := testIO(t, &out, io.Discard)
		rio.isTerminal = func(w io.Writer) bool { return tty && w == &out }
		opts := fixCLIOpts{reportFlags: reportFlags{format: "text", output: "-"}}
		assert.Equal(t, 1, reportFixResultTo(opts, result, &vlog.Logger{}, rio))
		assert.Equal(t, tty, bytes.Contains(out.Bytes(), []byte("\033[")), "tty=%v report=%q", tty, out.String())
	}
	var out bytes.Buffer
	opts := fixCLIOpts{reportFlags: reportFlags{format: "text", output: "-", color: colorAlways}}
	assert.Equal(t, 1, reportFixResultTo(opts, result, &vlog.Logger{}, testIO(t, &out, io.Discard)))
	assert.Contains(t, out.String(), "\033[", "--color=always colors a pipe")
}

// A failed write of any report part is a runtime error on stderr with
// exit 2, never text inside the report.
func TestReportFixResultTo_WriteErrorGoesToStderr(t *testing.T) {
	result := &fixpkg.Result{
		FilesChecked:  1,
		WouldFixFiles: []fixpkg.WouldFixFile{{Path: "f.md", Count: 1}},
		Diagnostics:   manyDiagnostics(2000),
	}
	for _, opts := range []fixCLIOpts{
		{reportFlags: reportFlags{format: "json", output: "-"}, dryRun: true},
		{reportFlags: reportFlags{format: "sarif", output: "-"}, dryRun: true},
		{reportFlags: reportFlags{format: "text", output: "-"}},
	} {
		var errOut bytes.Buffer
		code := reportFixResultTo(opts, result, &vlog.Logger{}, testIO(t, &alwaysErrorWriter{}, &errOut))
		assert.Equal(t, 2, code, "%+v", opts)
		assert.Equal(t, "mdsmith: error writing output: write failed\n", errOut.String(), "%+v", opts)
	}
}

func TestWriteFixReport_Quiet(t *testing.T) {
	result := &fixpkg.Result{FilesChecked: 1, Diagnostics: manyDiagnostics(1)}
	for _, output := range []string{"", "-"} {
		var buf bytes.Buffer
		opts := fixCLIOpts{reportFlags: reportFlags{format: "text", output: output, quiet: true}, dryRun: true}
		require.NoError(t, writeFixReport(&buf, result, opts, false))
		assert.Empty(t, buf.String(), "-o %q", output)
	}
}

// -q silences the terminal only: an explicit -o file still gets the
// full report, here the remaining diagnostics and the stats line.
func TestWriteFixReport_QuietStillFillsFile(t *testing.T) {
	result := &fixpkg.Result{FilesChecked: 1, Failures: 1, Diagnostics: manyDiagnostics(1)}
	var buf bytes.Buffer
	opts := fixCLIOpts{reportFlags: reportFlags{format: "text", output: "fix.txt", quiet: true}}
	require.NoError(t, writeFixReport(&buf, result, opts, false))
	assert.Regexp(t, `(?s)^f\.md:1:1 MDS001 line too long\n.*stats: checked=1 fixed=0 failures=1 unfixed=1\n$`,
		buf.String())
}

// --build-only has no lint report, so a run that resolved no Markdown
// file writes none either: no -o file is created (testIO fails the
// test on create) and nothing reaches the streams.
func TestReportFixNoFiles_BuildOnlyWritesNoReport(t *testing.T) {
	var out, errOut bytes.Buffer
	opts := fixCLIOpts{reportFlags: reportFlags{format: "json", output: "r.json"}}
	opts.build.buildOnly = true
	assert.Equal(t, 0, reportFixNoFiles(opts, testIO(t, &out, &errOut)))
	assert.Empty(t, out.String())
	assert.Empty(t, errOut.String())
}

// Without --build-only the no-files run writes its empty document.
func TestReportFixNoFiles_WritesEmptyDocument(t *testing.T) {
	var out bytes.Buffer
	opts := fixCLIOpts{reportFlags: reportFlags{format: "json", output: "-"}}
	assert.Equal(t, 0, reportFixNoFiles(opts, testIO(t, &out, io.Discard)))
	assert.Equal(t, "[]\n", out.String())
}
