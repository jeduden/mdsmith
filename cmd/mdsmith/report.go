package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"

	flag "github.com/spf13/pflag"

	"github.com/jeduden/mdsmith/internal/lint"
)

// reportRoutingHelp is the usage paragraph check and fix print on how
// -o routes the report and when text output is colored.
const reportRoutingHelp = "The report (diagnostics and the stats line) goes to stderr unless -o names\n" +
	"a file, or - for stdout. Runtime errors always go to stderr. -q silences the\n" +
	"terminal; an -o file still gets the report. An -o path that is, or would be,\n" +
	"one of the inputs is refused.\n\n" +
	"Text output is colored only when the report goes to a terminal. --color=always\n" +
	"and --color=never (or --no-color) force it on or off, and --color=auto asks the\n" +
	"terminal alone; the last color flag given wins. With no color flag, a non-empty\n" +
	"NO_COLOR turns color off, or else a FORCE_COLOR other than empty or 0 turns it on,\n" +
	"except in an -o file, which only --color=always colors.\n\n"

// colorMode is the color setting --color and --no-color write.
// colorUnset means neither flag was given, so NO_COLOR, FORCE_COLOR,
// and the terminal decide (see reportIO.colorFor).
type colorMode string

const (
	colorUnset  colorMode = ""
	colorAuto   colorMode = "auto"
	colorAlways colorMode = "always"
	colorNever  colorMode = "never"
)

// colorFlag is the pflag.Value behind --color.
type colorFlag struct{ mode *colorMode }

func (c colorFlag) String() string { return string(*c.mode) }

// Type names the value in --help.
func (c colorFlag) Type() string { return "when" }

// Set accepts auto, always, or never.
func (c colorFlag) Set(s string) error {
	switch m := colorMode(s); m {
	case colorAuto, colorAlways, colorNever:
		*c.mode = m
		return nil
	}
	return errors.New("must be auto, always, or never")
}

// noColorFlag is the pflag.Value behind --no-color, an alias for
// --color=never. It writes the same setting as --color, so the last
// of the two on the command line wins. --no-color=false changes
// nothing.
type noColorFlag struct{ mode *colorMode }

func (c noColorFlag) String() string { return "false" }

// Type is "bool" so --help shows --no-color as a plain switch.
func (c noColorFlag) Type() string { return "bool" }

// Set takes a boolean; true selects --color=never.
func (c noColorFlag) Set(s string) error {
	on, err := strconv.ParseBool(s)
	if err != nil {
		return err
	}
	if on {
		*c.mode = colorNever
	}
	return nil
}

// registerColorFlags adds --color and --no-color to the check or fix
// flag set. Both write mode.
func registerColorFlags(fs *flag.FlagSet, mode *colorMode) {
	fs.Var(colorFlag{mode}, "color",
		"Color text output: `when` is auto, always, or never "+
			"(default: NO_COLOR, then FORCE_COLOR, then auto)")
	fs.VarPF(noColorFlag{mode}, "no-color", "", "Same as --color=never").NoOptDefVal = "true"
}

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
	output string
	// color is the --color / --no-color setting.
	color colorMode
	quiet bool
}

// silenced reports whether -q drops the report. -q silences the
// terminal routes only: stderr, and stdout under -o -. An explicit
// -o <path> file still gets the full report.
func (f reportFlags) silenced() bool {
	return f.quiet && (f.output == "" || f.output == "-")
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

// colorFor reports whether text written to w carries ANSI color.
// toFile says w is the file an explicit -o <path> names, not a stream.
// The first rule that applies decides:
//
//  1. --color=always is on and --color=never (or --no-color) is off.
//     A flag beats both environment variables.
//  2. --color=auto is on when w is a terminal, whatever the
//     environment says.
//  3. With no color flag, a non-empty NO_COLOR is off
//     (https://no-color.org).
//  4. Then a FORCE_COLOR that is neither empty nor 0 is on
//     (https://force-color.org), except in an -o <path> file: a
//     variable set for a whole CI job must not put escape codes in a
//     report file, which only --color=always colors. stderr and stdout
//     stay forced even when redirected, as in other tools.
//  5. Otherwise color is on when w is a terminal.
func (r reportIO) colorFor(w io.Writer, mode colorMode, toFile bool) bool {
	switch mode {
	case colorAlways:
		return true
	case colorNever:
		return false
	case colorAuto:
		return r.isTerminal(w)
	}
	if r.getenv("NO_COLOR") != "" {
		return false
	}
	if v := r.getenv("FORCE_COLOR"); v != "" && v != "0" && !toFile {
		return true
	}
	return r.isTerminal(w)
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
func (r reportIO) deliverReport(output string, color colorMode, errs []error, body reportBody) int {
	dst := r.stderr
	closeDst := func() error { return nil }
	toFile := false
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
			dst, closeDst, toFile = f, f.Close, true
		}
		bw = bufio.NewWriterSize(dst, reportBufSize)
	}

	err := body(bw, r.colorFor(dst, color, toFile))
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

// writeStderrDiagnostics writes diags as text on stderr and returns 0,
// or 2 when the write fails. Color follows colorFor with no color
// flag, as export and extract have none. These runs print a handful of
// diagnostics, so a default-size buffer batches them without the
// 64 KiB report buffer.
func (r reportIO) writeStderrDiagnostics(diags []lint.Diagnostic) int {
	bw := bufio.NewWriter(r.stderr)
	err := writeDiagnostics(bw, diags, "text", r.colorFor(r.stderr, colorUnset, false))
	if err == nil {
		err = bw.Flush()
	}
	if err != nil {
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
// writes nothing to the terminal (see reportFlags.silenced).
func reportNoFiles(f reportFlags, rio reportIO) int {
	return rio.deliverReport(f.output, f.color, nil, func(w io.Writer, color bool) error {
		if f.silenced() {
			return nil
		}
		return writeDiagnostics(w, nil, f.format, color)
	})
}
