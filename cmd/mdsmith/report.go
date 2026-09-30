package main

import (
	"bufio"
	"fmt"
	"io"
	"os"

	flag "github.com/spf13/pflag"
)

// reportRoutingHelp is the usage paragraph check and fix print on how
// -o routes the report and when text output is colored.
const reportRoutingHelp = "The report (diagnostics and the stats line) goes to stderr unless -o names\n" +
	"a file, or - for stdout. Runtime errors always go to stderr. Text output is\n" +
	"colored only on a terminal; --no-color or a non-empty NO_COLOR turns it off.\n\n"

// registerOutputFlag adds -o/--output to the check or fix flag set.
func registerOutputFlag(fs *flag.FlagSet, output *string) {
	fs.StringVarP(output, "output", "o", "",
		"Write the report to `path`; - writes it to stdout (default stderr)")
}

// checkOutputFlag rejects an explicitly empty -o value with exit 2.
// Taking it as "not given" would send the report to stderr, so
// `-o "$REPORT"` with REPORT unset would look like it worked. It
// returns -1 when the value is usable.
func checkOutputFlag(fs *flag.FlagSet, output, cmd string) int {
	if fs.Changed("output") && output == "" {
		fmt.Fprintf(os.Stderr, "mdsmith: %s: --output needs a path, or - for stdout\n", cmd)
		return 2
	}
	return -1
}

// reportFlags are the report flags check and fix share.
type reportFlags struct {
	// format is the -f value: text, json, or sarif.
	format string
	// output is the -o value: "" (not given) sends the report to
	// stderr, "-" to stdout, and anything else names a file.
	output  string
	noColor bool
	quiet   bool
}

// structuredOnStderr reports whether a json or sarif report shares
// stderr with the run's prose lines, where one of those lines would
// corrupt the document. Any other -f value renders as text (see
// writeDiagnostics), which a prose line cannot corrupt.
func (f reportFlags) structuredOnStderr() bool {
	return f.output == "" && (f.format == "json" || f.format == "sarif")
}

// reportBody writes a report into w. color says whether text output
// may carry ANSI color.
type reportBody func(w io.Writer, color bool) error

// reportIO is the process surface the report paths write through:
// the two streams, the report-file opener, the terminal check, and
// the environment. processIO returns the real one; tests pass fakes.
type reportIO struct {
	stdout io.Writer
	stderr io.Writer
	// create opens the file an -o path names.
	create func(path string) (io.WriteCloser, error)
	// isTerminal reports whether w is an interactive terminal.
	isTerminal func(w io.Writer) bool
	getenv     func(key string) string
}

// processIO returns the reportIO of the running process. It reads
// os.Stdout and os.Stderr when called, so a test that swaps them in
// first is honored.
func processIO() reportIO {
	return reportIO{
		stdout:     os.Stdout,
		stderr:     os.Stderr,
		create:     createReportFile,
		isTerminal: isTerminal,
		getenv:     os.Getenv,
	}
}

// createReportFile opens path for writing a report. A missing file is
// created with mode 0644 (before the umask), the same mode `export -o`
// uses; an existing file is truncated and keeps its mode.
func createReportFile(path string) (io.WriteCloser, error) {
	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
}

// isTerminal reports whether w is a character device, the file type
// of a terminal on Unix and of a console on Windows. It needs no
// dependency beyond os. os.DevNull is a character device too, but a
// report sent there has no reader, so the color it gets is harmless.
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// colorFor reports whether text written to w carries ANSI color: only
// when w is a terminal, --no-color is off, and the NO_COLOR
// environment variable is empty or unset (https://no-color.org).
func (r reportIO) colorFor(w io.Writer, noColor bool) bool {
	return !noColor && r.getenv("NO_COLOR") == "" && r.isTerminal(w)
}

// deliverReport prints the run's runtime errors on stderr, then opens
// the report destination named by output ("" stderr, "-" stdout, any
// other value a file) and has body write the report into it. It
// returns 0 once the report is delivered, and 2 when the destination
// cannot be opened, written, flushed, or closed. That failure is a
// runtime error, so its message goes to stderr, never into the report.
//
// The report goes through one 64 KiB buffer: the text formatter emits
// several small writes per diagnostic, and issuing each as its own
// syscall dominated wall time on diagnostic-heavy runs. On the default
// route the errors share that buffer, which keeps their order on the
// one stream. On the other routes they are flushed to stderr first, so
// they still print before the report on a shared terminal.
func (r reportIO) deliverReport(output string, noColor bool, errs []error, body reportBody) int {
	dst := r.stderr
	closeDst := func() error { return nil }
	var bw *bufio.Writer
	if output == "" {
		bw = bufio.NewWriterSize(r.stderr, reportBufSize)
		printErrorsTo(bw, errs)
	} else {
		if len(errs) > 0 {
			ew := bufio.NewWriter(r.stderr)
			printErrorsTo(ew, errs)
			// Ignored, as printErrorsTo ignores its own writes: a
			// stderr that fails writes must not keep the report from
			// its destination.
			_ = ew.Flush()
		}
		if output == "-" {
			dst = r.stdout
		} else {
			f, err := r.create(output)
			if err != nil {
				printWriteErrorTo(r.stderr, err)
				return 2
			}
			dst, closeDst = f, f.Close
		}
		bw = bufio.NewWriterSize(dst, reportBufSize)
	}

	err := body(bw, r.colorFor(dst, noColor))
	if err == nil {
		err = bw.Flush()
	}
	if cerr := closeDst(); err == nil {
		err = cerr
	}
	if err != nil {
		// Straight to stderr, not through a buffer a failed flush has
		// left with a sticky error.
		printWriteErrorTo(r.stderr, err)
		return 2
	}
	return 0
}

// reportNoFiles ends a check or fix run that resolved no Markdown
// file, which loadAndResolve and discoverFiles signal with code 0. The
// run is clean and exits 0, but a json or sarif report still gets its
// empty document, so every clean run leaves a valid one. Text writes
// no stats line here, as before. An -o file is still created. -q
// writes nothing.
func reportNoFiles(f reportFlags, rio reportIO) int {
	return rio.deliverReport(f.output, f.noColor, nil, func(w io.Writer, color bool) error {
		if f.quiet {
			return nil
		}
		return writeDiagnostics(w, nil, f.format, color)
	})
}
