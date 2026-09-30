package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeReportFile is the io.WriteCloser a test reportIO.create hands
// out in place of a real file.
type fakeReportFile struct {
	bytes.Buffer
	closed   bool
	writeErr error
	closeErr error
}

func (f *fakeReportFile) Write(p []byte) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	return f.Buffer.Write(p)
}

func (f *fakeReportFile) Close() error {
	f.closed = true
	return f.closeErr
}

// failFirstWriter fails its first n writes, then accepts the rest. It
// stands in for a stderr whose first flush fails and whose later
// writes succeed.
type failFirstWriter struct {
	n     int
	calls int
	buf   bytes.Buffer
}

func (w *failFirstWriter) Write(p []byte) (int, error) {
	w.calls++
	if w.calls <= w.n {
		return 0, errors.New("stderr write failed")
	}
	return w.buf.Write(p)
}

// testIO returns a reportIO over the given streams with no terminal,
// no NO_COLOR, and a create hook that fails the test if called.
func testIO(t *testing.T, stdout, stderr io.Writer) reportIO {
	t.Helper()
	return reportIO{
		stdout: stdout,
		stderr: stderr,
		create: func(path string) (io.WriteCloser, error) {
			t.Errorf("unexpected create(%q)", path)
			return nil, errors.New("unexpected create")
		},
		isTerminal: func(io.Writer) bool { return false },
		getenv:     func(string) string { return "" },
	}
}

// writeBody returns a report body that writes s and records the color
// decision it was handed.
func writeBody(s string, color *bool) reportBody {
	return func(w io.Writer, c bool) error {
		if color != nil {
			*color = c
		}
		_, err := io.WriteString(w, s)
		return err
	}
}

func TestDeliverReport_DefaultRouteSharesStderr(t *testing.T) {
	var out bytes.Buffer
	errOut := &countingWriter{}
	rio := testIO(t, &out, errOut)
	code := rio.deliverReport("", colorUnset, []error{errors.New("boom")}, writeBody("REPORT\n", nil))
	assert.Equal(t, 0, code)
	assert.Equal(t, "mdsmith: boom\nREPORT\n", errOut.buf.String())
	assert.Equal(t, 1, errOut.calls, "errors and report share one buffer on stderr")
	assert.Empty(t, out.String())
}

func TestDeliverReport_DashRouteWritesStdout(t *testing.T) {
	var out, errOut bytes.Buffer
	rio := testIO(t, &out, &errOut)
	code := rio.deliverReport("-", colorUnset, []error{errors.New("boom")}, writeBody("REPORT\n", nil))
	assert.Equal(t, 0, code)
	assert.Equal(t, "REPORT\n", out.String())
	assert.Equal(t, "mdsmith: boom\n", errOut.String())
}

// On a shared terminal (or `>file 2>&1`) the runtime errors print
// before the report: the error buffer is flushed first.
func TestDeliverReport_DashRouteErrorsPrintFirst(t *testing.T) {
	var shared bytes.Buffer
	rio := testIO(t, &shared, &shared)
	code := rio.deliverReport("-", colorUnset, []error{errors.New("boom")}, writeBody("REPORT\n", nil))
	assert.Equal(t, 0, code)
	assert.Equal(t, "mdsmith: boom\nREPORT\n", shared.String())
}

func TestDeliverReport_FileRouteWritesFile(t *testing.T) {
	var out, errOut bytes.Buffer
	rio := testIO(t, &out, &errOut)
	f := &fakeReportFile{}
	var created string
	rio.create = func(path string) (io.WriteCloser, error) {
		created = path
		return f, nil
	}
	code := rio.deliverReport("out.json", colorUnset, []error{errors.New("boom")}, writeBody("REPORT\n", nil))
	assert.Equal(t, 0, code)
	assert.Equal(t, "out.json", created)
	assert.Equal(t, "REPORT\n", f.String())
	assert.True(t, f.closed, "the report file must be closed")
	assert.Equal(t, "mdsmith: boom\n", errOut.String())
	assert.Empty(t, out.String())
}

// Opening the file is part of writing the report: a failure is a
// runtime error on stderr with exit 2, and the body never runs.
func TestDeliverReport_CreateErrorGoesToStderr(t *testing.T) {
	var out, errOut bytes.Buffer
	rio := testIO(t, &out, &errOut)
	rio.create = func(string) (io.WriteCloser, error) {
		return nil, errors.New("open out.json: permission denied")
	}
	called := false
	code := rio.deliverReport("out.json", colorUnset, nil, func(io.Writer, bool) error {
		called = true
		return nil
	})
	assert.Equal(t, 2, code)
	assert.False(t, called)
	assert.Equal(t, "mdsmith: error writing output: open out.json: permission denied\n", errOut.String())
	assert.Empty(t, out.String())
}

func TestDeliverReport_WriteErrors(t *testing.T) {
	const msg = "mdsmith: error writing output: write failed\n"
	t.Run("body error on stdout", func(t *testing.T) {
		var errOut bytes.Buffer
		rio := testIO(t, &alwaysErrorWriter{}, &errOut)
		code := rio.deliverReport("-", colorUnset, nil, func(io.Writer, bool) error {
			return errors.New("write failed")
		})
		assert.Equal(t, 2, code)
		assert.Equal(t, msg, errOut.String())
	})
	t.Run("flush error on stdout", func(t *testing.T) {
		var errOut bytes.Buffer
		rio := testIO(t, &failAfterWriter{n: 0}, &errOut)
		code := rio.deliverReport("-", colorUnset, nil, writeBody("REPORT\n", nil))
		assert.Equal(t, 2, code)
		assert.Equal(t, msg, errOut.String())
	})
	t.Run("write error on the file", func(t *testing.T) {
		var errOut bytes.Buffer
		rio := testIO(t, io.Discard, &errOut)
		f := &fakeReportFile{writeErr: errors.New("write failed")}
		rio.create = func(string) (io.WriteCloser, error) { return f, nil }
		code := rio.deliverReport("out.json", colorUnset, nil, writeBody("REPORT\n", nil))
		assert.Equal(t, 2, code)
		assert.Equal(t, msg, errOut.String())
		assert.True(t, f.closed, "the report file is closed even after a failed write")
	})
	t.Run("close error on the file", func(t *testing.T) {
		var errOut bytes.Buffer
		rio := testIO(t, io.Discard, &errOut)
		f := &fakeReportFile{closeErr: errors.New("write failed")}
		rio.create = func(string) (io.WriteCloser, error) { return f, nil }
		code := rio.deliverReport("out.json", colorUnset, nil, writeBody("REPORT\n", nil))
		assert.Equal(t, 2, code)
		assert.Equal(t, msg, errOut.String())
	})
	t.Run("flush error on the default route", func(t *testing.T) {
		code := testIO(t, io.Discard, &failAfterWriter{n: 0}).
			deliverReport("", colorUnset, nil, writeBody("REPORT\n", nil))
		assert.Equal(t, 2, code)
	})
}

// A stderr that fails writes (a full disk, say) must not keep the
// report off its destination, and must not turn a delivered report
// into exit 2. A broken stderr pipe is different: the Go runtime ends
// the process on SIGPIPE.
func TestDeliverReport_BrokenStderrStillWritesReport(t *testing.T) {
	var out bytes.Buffer
	rio := testIO(t, &out, &alwaysErrorWriter{})
	code := rio.deliverReport("-", colorUnset, []error{errors.New("boom")}, writeBody("REPORT\n", nil))
	assert.Equal(t, 0, code)
	assert.Equal(t, "REPORT\n", out.String())
}

// The error flush's failure must not swallow a later write-error
// message: that message goes straight to stderr, not through the
// failed buffer's sticky error.
func TestDeliverReport_WriteErrorAfterFailedErrorFlush(t *testing.T) {
	errOut := &failFirstWriter{n: 1}
	rio := testIO(t, &alwaysErrorWriter{}, errOut)
	code := rio.deliverReport("-", colorUnset, []error{errors.New("boom")}, writeBody("REPORT\n", nil))
	assert.Equal(t, 2, code)
	assert.Equal(t, "mdsmith: error writing output: write failed\n", errOut.buf.String())
}

// deliverReport decides color for the report's own destination: the
// stream or file the report goes to, not some other stream.
func TestDeliverReport_Color(t *testing.T) {
	for _, tc := range []struct {
		name, output  string
		color         colorMode
		noColorEnv    string
		forceColorEnv string
		ttyStream     string
		want          bool
	}{
		{name: "stderr tty", output: "", ttyStream: "stderr", want: true},
		{name: "stderr not tty", output: ""},
		{name: "stderr tty, stdout report", output: "-", ttyStream: "stderr"},
		{name: "stdout tty", output: "-", ttyStream: "stdout", want: true},
		{name: "file tty", output: "/dev/tty", ttyStream: "file", want: true},
		{name: "file not tty", output: "out.txt", ttyStream: "stderr"},
		{name: "file, --color=always", output: "out.txt", color: colorAlways, want: true},
		{name: "no-color flag", output: "", ttyStream: "stderr", color: colorNever},
		{name: "NO_COLOR set", output: "", ttyStream: "stderr", noColorEnv: "1"},
		{name: "stderr, FORCE_COLOR", output: "", forceColorEnv: "1", want: true},
		{name: "stdout, FORCE_COLOR", output: "-", forceColorEnv: "1", want: true},
		{name: "file, FORCE_COLOR", output: "out.txt", forceColorEnv: "1"},
		{name: "file tty, FORCE_COLOR", output: "/dev/tty", ttyStream: "file", forceColorEnv: "1", want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			f := &fakeReportFile{}
			streams := map[string]io.Writer{"stdout": &out, "stderr": &errOut, "file": f}
			rio := testIO(t, &out, &errOut)
			rio.create = func(string) (io.WriteCloser, error) { return f, nil }
			rio.isTerminal = func(w io.Writer) bool { return w == streams[tc.ttyStream] }
			rio.getenv = func(key string) string {
				return map[string]string{"NO_COLOR": tc.noColorEnv, "FORCE_COLOR": tc.forceColorEnv}[key]
			}
			var got bool
			require.Equal(t, 0, rio.deliverReport(tc.output, tc.color, nil, writeBody("", &got)))
			assert.Equal(t, tc.want, got)
		})
	}
}

// wantColor is the documented color precedence, written out as a
// spec for TestColorFor_Matrix: an explicit --color=always or never
// decides; --color=auto asks the terminal alone; without a flag a
// non-empty NO_COLOR turns color off, then a FORCE_COLOR that is
// neither empty nor 0 turns it on except in an -o <path> file, and
// the terminal decides the rest.
func wantColor(mode colorMode, noColor, forceColor string, tty, toFile bool) bool {
	switch mode {
	case colorAlways:
		return true
	case colorNever:
		return false
	case colorAuto:
		return tty
	}
	if noColor != "" {
		return false
	}
	if forceColor != "" && forceColor != "0" && !toFile {
		return true
	}
	return tty
}

// TestColorFor_Matrix runs every combination of the color flag,
// NO_COLOR, FORCE_COLOR, a terminal destination, and an -o <path>
// file destination through the injectable isTerminal/getenv seam.
func TestColorFor_Matrix(t *testing.T) {
	for _, mode := range []colorMode{colorUnset, colorAuto, colorAlways, colorNever} {
		for _, noColor := range []string{"", "1"} {
			for _, force := range []string{"", "0", "1", "true"} {
				for _, tty := range []bool{false, true} {
					for _, toFile := range []bool{false, true} {
						dst := &bytes.Buffer{}
						rio := testIO(t, io.Discard, io.Discard)
						rio.isTerminal = func(w io.Writer) bool { return tty && w == dst }
						rio.getenv = func(key string) string {
							return map[string]string{"NO_COLOR": noColor, "FORCE_COLOR": force}[key]
						}
						assert.Equal(t, wantColor(mode, noColor, force, tty, toFile), rio.colorFor(dst, mode, toFile),
							"--color=%q NO_COLOR=%q FORCE_COLOR=%q tty=%v file=%v", mode, noColor, force, tty, toFile)
					}
				}
			}
		}
	}
}

// The cases the precedence exists for, spelled out.
func TestColorFor_Precedence(t *testing.T) {
	dst := &bytes.Buffer{}
	noColor := map[string]string{"NO_COLOR": "1"}
	force := map[string]string{"FORCE_COLOR": "1"}
	forceOff := map[string]string{"FORCE_COLOR": "0"}
	for _, tc := range []struct {
		name   string
		mode   colorMode
		env    map[string]string
		tty    bool
		toFile bool
		want   bool
	}{
		{name: "NO_COLOR beats FORCE_COLOR", env: map[string]string{"NO_COLOR": "1", "FORCE_COLOR": "1"}, tty: true},
		{name: "FORCE_COLOR colors a pipe", env: force, want: true},
		{name: "FORCE_COLOR=0 forces nothing", env: forceOff},
		{name: "FORCE_COLOR=0 keeps a terminal colored", env: forceOff, tty: true, want: true},
		{name: "--color=always beats NO_COLOR", mode: colorAlways, env: noColor, want: true},
		{name: "--color=never beats FORCE_COLOR", mode: colorNever, env: force, tty: true},
		{name: "--color=auto ignores FORCE_COLOR", mode: colorAuto, env: force},
		{name: "--color=auto ignores NO_COLOR", mode: colorAuto, env: noColor, tty: true, want: true},
		{name: "FORCE_COLOR leaves an -o file plain", env: force, toFile: true},
		{name: "--color=always colors an -o file", mode: colorAlways, toFile: true, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rio := testIO(t, io.Discard, io.Discard)
			rio.isTerminal = func(io.Writer) bool { return tc.tty }
			rio.getenv = func(key string) string { return tc.env[key] }
			assert.Equal(t, tc.want, rio.colorFor(dst, tc.mode, tc.toFile))
		})
	}
}

func TestIsTerminal(t *testing.T) {
	assert.False(t, isTerminal(&bytes.Buffer{}), "a non-file writer")

	r, w, err := os.Pipe()
	require.NoError(t, err)
	defer r.Close() //nolint:errcheck // test cleanup
	defer w.Close() //nolint:errcheck // test cleanup
	assert.False(t, isTerminal(w), "a pipe")

	f, err := os.Create(filepath.Join(t.TempDir(), "report.txt"))
	require.NoError(t, err)
	defer f.Close() //nolint:errcheck // test cleanup
	assert.False(t, isTerminal(f), "a regular file")

	closed, err := os.Create(filepath.Join(t.TempDir(), "closed.txt"))
	require.NoError(t, err)
	require.NoError(t, closed.Close())
	assert.False(t, isTerminal(closed), "a file whose Stat fails")

	// A terminal is a character device. os.DevNull is one too; the
	// report never reaches a reader there, so its color is harmless.
	dev, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	require.NoError(t, err)
	defer dev.Close() //nolint:errcheck // test cleanup
	assert.True(t, isTerminal(dev), "a character device")
}

func TestCreateReportFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.json")
	require.NoError(t, os.WriteFile(path, []byte("stale report, longer than the new one\n"), 0o600))
	before, err := os.Stat(path)
	require.NoError(t, err)

	f, err := createReportFile(path)
	require.NoError(t, err)
	_, err = io.WriteString(f, "[]\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "[]\n", string(got), "an existing file is truncated")
	after, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, before.Mode().Perm(), after.Mode().Perm(), "an existing file keeps its mode")

	fresh := filepath.Join(dir, "fresh.json")
	f, err = createReportFile(fresh)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	created, err := os.Stat(fresh)
	require.NoError(t, err, "a missing file is created")
	// Mode 0644 before the umask: compare with a file created with
	// 0644 under the same umask.
	ref := filepath.Join(dir, "ref.json")
	require.NoError(t, os.WriteFile(ref, nil, 0o644))
	want, err := os.Stat(ref)
	require.NoError(t, err)
	assert.Equal(t, want.Mode().Perm(), created.Mode().Perm(), "a missing file is created with mode 0644")

	_, err = createReportFile(filepath.Join(dir, "missing", "out.json"))
	assert.Error(t, err, "a missing parent directory is an error")
}

func TestProcessIO(t *testing.T) {
	rio := processIO()
	assert.Same(t, os.Stdout, rio.stdout)
	assert.Same(t, os.Stderr, rio.stderr)
	t.Setenv("MDSMITH_PROCESS_IO_PROBE", "x")
	assert.Equal(t, "x", rio.getenv("MDSMITH_PROCESS_IO_PROBE"))
	assert.False(t, rio.isTerminal(&bytes.Buffer{}))
	f, err := rio.create(filepath.Join(t.TempDir(), "r.txt"))
	require.NoError(t, err)
	require.NoError(t, f.Close())
}

func TestReportFlags_StructuredOnStderr(t *testing.T) {
	for _, tc := range []struct {
		format, output string
		want           bool
	}{
		{"text", "", false},
		{"json", "", true},
		{"sarif", "", true},
		{"json", "-", false},
		{"sarif", "out.sarif", false},
		// writeDiagnostics renders an unknown -f value as text, so
		// a prose line cannot corrupt it.
		{"xml", "", false},
	} {
		got := reportFlags{format: tc.format, output: tc.output}.structuredOnStderr()
		assert.Equal(t, tc.want, got, fmt.Sprintf("%s -o %q", tc.format, tc.output))
	}
}

// -q silences the terminal routes only; an -o file keeps its report.
func TestReportFlags_Silenced(t *testing.T) {
	for _, tc := range []struct {
		output string
		quiet  bool
		want   bool
	}{
		{"", true, true},
		{"-", true, true},
		{"r.json", true, false},
		{"", false, false},
		{"r.json", false, false},
	} {
		got := reportFlags{output: tc.output, quiet: tc.quiet}.silenced()
		assert.Equal(t, tc.want, got, "-q=%v -o %q", tc.quiet, tc.output)
	}
}
