package main

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"os"

	flag "github.com/spf13/pflag"

	"github.com/jeduden/mdsmith/internal/engine"
	"github.com/jeduden/mdsmith/internal/gctune"
	vlog "github.com/jeduden/mdsmith/internal/log"
	mdsmith "github.com/jeduden/mdsmith/pkg/mdsmith"
)

// checkCLIOpts bundles the runtime knobs threaded through the check
// command path. Grouped because runCheck splits between explicit-file,
// stdin, and config-discovery entry points and the same nine values
// flow to all three.
type checkCLIOpts struct {
	configPath   string
	format       string
	noColor      bool
	quiet        bool
	verbose      bool
	walk         walkCLI
	maxInputSize string
	explain      bool
	// stdout routes diagnostics and the stats line to stdout instead
	// of stderr. Runtime errors always stay on stderr.
	stdout bool
}

// stderrFormat is the format of the output check writes to stderr:
// opts.format by default, and text under --stdout, where only prose
// (runtime errors and warnings) stays on stderr.
func (o checkCLIOpts) stderrFormat() string {
	if o.stdout {
		return "text"
	}
	return o.format
}

// runCheck implements the "check" subcommand: lint files.
func runCheck(args []string) int {
	opts, fileArgs, hasStdin, code := parseCheckFlags(args)
	if code >= 0 {
		return code
	}
	gctune.ApplyBatch()
	if hasStdin {
		return checkStdin(opts)
	}
	if len(fileArgs) > 0 {
		return checkFiles(fileArgs, opts)
	}
	// No file args and no stdin: discover files from config.
	return checkDiscovered(opts)
}

// parseCheckFlags configures the `check` flag set, parses args, and
// returns the resolved opts plus positional arguments. The bool
// `hasStdin` is true when the caller passed `-` as a positional
// arg. A non-negative `code` means the caller should return that
// exit code immediately (e.g. --help or a parse error).
func parseCheckFlags(args []string) (checkCLIOpts, []string, bool, int) {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	var (
		configPath, format, maxInputSize                                      string
		noColor, quiet, verbose, noGitignore, followSymlinks, explain, stdout bool
	)

	fs.StringVarP(&configPath, "config", "c", "", "Override config file path")
	fs.StringVarP(&format, "format", "f", "text", "Output format: text, json, sarif")
	fs.BoolVar(&noColor, "no-color", false, "Disable ANSI colors")
	fs.BoolVarP(&quiet, "quiet", "q", false, "Suppress non-error output")
	fs.BoolVarP(&verbose, "verbose", "v", false, "Show config, files, and rules on stderr")
	fs.BoolVar(&noGitignore, "no-gitignore", false, "Disable .gitignore filtering when walking directories")
	fs.BoolVar(&followSymlinks, "follow-symlinks", false,
		"Follow symlinks; omitted defers to follow-symlinks config (default skip); "+
			"=false forces skip over any config opt-in")
	fs.StringVar(&maxInputSize, "max-input-size", "", "Maximum file size to process (e.g. 2MB, 500KB, 0=unlimited)")
	fs.BoolVar(&explain, "explain", false, "Attach per-leaf rule provenance to each diagnostic")

	fs.BoolVar(&stdout, "stdout", false,
		"Write diagnostics to stdout instead of stderr (errors stay on stderr)")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: mdsmith check [flags] [files...]\n\n"+
			"Lint Markdown files for style issues.\n\n"+
			"Files can be paths, directories (walked recursively for *.md), or glob patterns.\n"+
			"Pass - to read from stdin. With no file arguments, discovers files using the\n"+
			"files patterns from config (default: **/*.md, **/*.markdown).\n\n"+
			"Flags:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if code := reportFlagParseErr(err, os.Stderr, "mdsmith: check"); code >= 0 {
			return checkCLIOpts{}, nil, false, code
		}
	}

	// --quiet suppresses verbose
	if quiet {
		verbose = false
	}

	hasStdin, fileArgs := splitStdinArg(fs.Args())

	return checkCLIOpts{
		configPath: configPath,
		format:     format,
		noColor:    noColor,
		quiet:      quiet,
		verbose:    verbose,
		walk: walkCLI{
			noGitignore:    noGitignore,
			followSymlinks: followSymlinksOverride(fs, followSymlinks),
		},
		maxInputSize: maxInputSize,
		explain:      explain,
		stdout:       stdout,
	}, fileArgs, hasStdin, -1
}

// checkFiles lints the given file paths and returns the appropriate exit code.
func checkFiles(fileArgs []string, opts checkCLIOpts) int {
	cfg, cfgPath, logger, files, maxBytes, code := loadAndResolve(
		fileArgs, opts.configPath, opts.verbose, opts.walk, opts.maxInputSize,
		nonMarkdownSkipWarner(os.Stderr, opts.stderrFormat(), opts.quiet),
	)
	if code >= 0 {
		return code
	}

	sess := sessionForCLI(cfg, cfgPath)
	defer sess.Dispose()
	result := sess.CheckPaths(files, checkBatchOptions(opts, logger, maxBytes))
	return reportCheckResult(result, opts, logger)
}

// checkBatchOptions maps the check CLI flags, the resolved logger, and
// the resolved byte cap onto the session's BatchOptions.
func checkBatchOptions(opts checkCLIOpts, logger *vlog.Logger, maxBytes int64) mdsmith.BatchOptions {
	return mdsmith.BatchOptions{
		Explain:       opts.explain,
		MaxInputBytes: batchMaxBytes(maxBytes),
		Logger:        logger,
	}
}

// batchMaxBytes maps the CLI's fully-resolved max-input-size (config
// merged with the --max-input-size flag) onto BatchOptions.MaxInputBytes,
// which treats 0 as "use the session default". The CLI value is always
// authoritative, and resolveMaxInputBytes returns 0 for an explicit
// `max-input-size: 0` (unlimited) — so map that to math.MaxInt64, the
// engine's explicit-unlimited sentinel, to keep it authoritative and
// non-zero rather than silently falling back to the 2 MB default.
func batchMaxBytes(resolved int64) int64 {
	if resolved <= 0 {
		return math.MaxInt64
	}
	return resolved
}

// checkStdin reads from stdin, lints the content, and returns the appropriate
// exit code. Uses runner.RunSource to ensure Configurable settings are applied.
func checkStdin(opts checkCLIOpts) int {
	logger := &vlog.Logger{Enabled: opts.verbose, W: os.Stderr}

	cfg, cfgPath, err := loadConfig(opts.configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}
	if cfgPath != "" {
		logger.Printf("config: %s", cfgPath)
	}

	maxBytes, err := resolveMaxInputBytes(cfg, opts.maxInputSize)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}

	source, err := readStdinLimited(maxBytes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}

	sess := sessionForCLI(cfg, cfgPath)
	defer sess.Dispose()
	result := sess.CheckSource("<stdin>", source, checkBatchOptions(opts, logger, maxBytes))
	return reportCheckResult(result, opts, logger)
}

// checkDiscovered loads config, discovers files from config patterns,
// and lints them. Returns the appropriate exit code.
func checkDiscovered(opts checkCLIOpts) int {
	cfg, cfgPath, logger, files, code := discoverFiles(opts.configPath, opts.verbose, opts.walk)
	if code >= 0 {
		return code
	}

	maxBytes, err := resolveMaxInputBytes(cfg, opts.maxInputSize)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}

	sess := sessionForCLI(cfg, cfgPath)
	defer sess.Dispose()
	result := sess.CheckPaths(files, checkBatchOptions(opts, logger, maxBytes))
	return reportCheckResult(result, opts, logger)
}

// reportCheckResult writes diagnostics + the run-stats line to the
// process streams and computes the exit code shared by checkFiles,
// checkStdin, and checkDiscovered.
func reportCheckResult(result *engine.Result, opts checkCLIOpts, logger *vlog.Logger) int {
	return reportCheckResultTo(result, opts, logger, os.Stdout, os.Stderr)
}

// reportCheckResultTo is the injectable form of reportCheckResult.
// Tests pass alternate writers to exercise the write-error branches
// without leaking to the real streams.
//
// Runtime errors always go to stderrW. Diagnostics and the stats line
// go to stdoutW under --stdout and to stderrW otherwise. A failed
// write of those is itself a runtime error: its message goes to
// stderrW and the exit code is 2.
//
// Each stream gets one buffered writer: the text formatter emits
// several small writes per diagnostic, and issuing each as its own
// syscall dominated wall time on diagnostic-heavy runs. Without
// --stdout both share one buffer. With it, the error buffer is
// flushed before any diagnostic is written, so errors print first on
// a shared terminal. Every buffer is flushed before the verbose
// logger line so output ordering on a shared fd is preserved.
func reportCheckResultTo(
	result *engine.Result, opts checkCLIOpts, logger *vlog.Logger, stdoutW, stderrW io.Writer,
) int {
	ew := bufio.NewWriterSize(stderrW, stderrBufSize)
	printErrorsTo(ew, result.Errors)
	out := ew
	if opts.stdout {
		// A failed flush is ignored, as printErrorsTo ignores its
		// writes: a closed stderr must not keep diagnostics off stdout.
		_ = ew.Flush()
		out = bufio.NewWriterSize(stdoutW, stderrBufSize)
	}

	if err := writeCheckReport(out, result, opts); err != nil {
		// A write error is a runtime error, so it goes to stderr and
		// never into the redirected diagnostics.
		printWriteErrorTo(ew, err)
		_ = ew.Flush()
		return 2
	}
	logger.Printf("checked %d files, %d issues found", result.FilesChecked, len(result.Diagnostics))

	if len(result.Errors) > 0 && len(result.Diagnostics) == 0 {
		return 2
	}
	if len(result.Diagnostics) > 0 {
		return 1
	}
	return 0
}

// writeCheckReport writes the diagnostics and the run-stats line to
// out, flushes it, and returns the first write error.
func writeCheckReport(out *bufio.Writer, result *engine.Result, opts checkCLIOpts) error {
	// SARIF must be emitted even with zero diagnostics so the file is valid
	// SARIF 2.1.0 (not an empty byte stream) when uploaded to Code Scanning.
	// JSON on stdout emits `[]` for the same reason: `--stdout -f json >
	// out.json` must leave valid JSON, not an empty file.
	emptyDoc := opts.format == "sarif" || (opts.stdout && opts.format == "json")
	if !opts.quiet && (len(result.Diagnostics) > 0 || emptyDoc) {
		if err := writeDiagnostics(out, result.Diagnostics, opts.format, opts.noColor); err != nil {
			return err
		}
	}
	printRunStatsTo(out, opts.format, opts.quiet, runStats{
		Checked:  result.FilesChecked,
		Fixed:    0,
		Failures: len(result.Diagnostics),
		Unfixed:  len(result.Diagnostics),
	})
	return out.Flush()
}

// readStdinLimited reads stdin with an optional size limit.
// When maxBytes <= 0 no limit is applied.
func readStdinLimited(maxBytes int64) ([]byte, error) {
	// Treat MaxInt64 as unlimited to avoid overflow in the +1 sentinel.
	if maxBytes > 0 && maxBytes < math.MaxInt64 {
		data, err := io.ReadAll(io.LimitReader(os.Stdin, maxBytes+1))
		if err != nil {
			return nil, err
		}
		if int64(len(data)) > maxBytes {
			return nil, fmt.Errorf(
				"reading \"<stdin>\": file too large "+
					"(%d bytes, max %d)", int64(len(data)), maxBytes)
		}
		return data, nil
	}
	return io.ReadAll(os.Stdin)
}
