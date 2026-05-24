package main

import (
	"fmt"
	"io"
	"math"
	"os"

	flag "github.com/spf13/pflag"

	"github.com/jeduden/mdsmith/internal/engine"
	vlog "github.com/jeduden/mdsmith/internal/log"
	"github.com/jeduden/mdsmith/internal/rule"
)

// checkCLIOpts bundles the runtime knobs threaded through the check
// command path. Grouped because runCheck splits between explicit-file,
// stdin, and config-discovery entry points and the same eight values
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
}

// runCheck implements the "check" subcommand: lint files.
func runCheck(args []string) int {
	opts, fileArgs, hasStdin, code := parseCheckFlags(args)
	if code >= 0 {
		return code
	}
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
		configPath, format, maxInputSize                              string
		noColor, quiet, verbose, noGitignore, followSymlinks, explain bool
	)

	fs.StringVarP(&configPath, "config", "c", "", "Override config file path")
	fs.StringVarP(&format, "format", "f", "text", "Output format: text, json")
	fs.BoolVar(&noColor, "no-color", false, "Disable ANSI colors")
	fs.BoolVarP(&quiet, "quiet", "q", false, "Suppress non-error output")
	fs.BoolVarP(&verbose, "verbose", "v", false, "Show config, files, and rules on stderr")
	fs.BoolVar(&noGitignore, "no-gitignore", false, "Disable .gitignore filtering when walking directories")
	fs.BoolVar(&followSymlinks, "follow-symlinks", false,
		"Follow symlinks; omitted defers to follow-symlinks config (default skip); "+
			"=false forces skip over any config opt-in")
	fs.StringVar(&maxInputSize, "max-input-size", "", "Maximum file size to process (e.g. 2MB, 500KB, 0=unlimited)")
	fs.BoolVar(&explain, "explain", false, "Attach per-leaf rule provenance to each diagnostic")

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
	}, fileArgs, hasStdin, -1
}

// checkFiles lints the given file paths and returns the appropriate exit code.
func checkFiles(fileArgs []string, opts checkCLIOpts) int {
	cfg, cfgPath, logger, files, maxBytes, code := loadAndResolve(
		fileArgs, opts.configPath, opts.verbose, opts.walk, opts.maxInputSize,
	)
	if code >= 0 {
		return code
	}

	runner := &engine.Runner{
		Config:           cfg,
		Rules:            rule.All(),
		StripFrontMatter: frontMatterEnabled(cfg),
		Logger:           logger,
		RootDir:          rootDirFromConfig(cfgPath),
		MaxInputBytes:    maxBytes,
		Explain:          opts.explain,
		ConfigPath:       cfgPath,
	}
	return reportCheckResult(runner.Run(files), opts, logger)
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

	runner := &engine.Runner{
		Config:           cfg,
		Rules:            rule.All(),
		StripFrontMatter: frontMatterEnabled(cfg),
		Logger:           logger,
		RootDir:          rootDirFromConfig(cfgPath),
		MaxInputBytes:    maxBytes,
		Explain:          opts.explain,
		ConfigPath:       cfgPath,
	}
	return reportCheckResult(runner.RunSource("<stdin>", source), opts, logger)
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

	runner := &engine.Runner{
		Config:           cfg,
		Rules:            rule.All(),
		StripFrontMatter: frontMatterEnabled(cfg),
		Logger:           logger,
		RootDir:          rootDirFromConfig(cfgPath),
		MaxInputBytes:    maxBytes,
		Explain:          opts.explain,
		ConfigPath:       cfgPath,
	}
	return reportCheckResult(runner.Run(files), opts, logger)
}

// reportCheckResult writes diagnostics + the run-stats line and
// computes the exit code shared by checkFiles, checkStdin, and
// checkDiscovered.
func reportCheckResult(result *engine.Result, opts checkCLIOpts, logger *vlog.Logger) int {
	return reportCheckResultTo(result, opts, logger, os.Stderr)
}

// reportCheckResultTo is the injectable form of reportCheckResult. Tests
// pass an alternate stderr writer to exercise the write-error branches
// without leaking to the real stderr.
func reportCheckResultTo(result *engine.Result, opts checkCLIOpts, logger *vlog.Logger, stderrW io.Writer) int {
	printErrorsTo(stderrW, result.Errors)

	if !opts.quiet && len(result.Diagnostics) > 0 {
		if code := formatDiagnosticsTo(stderrW, result.Diagnostics, opts.format, opts.noColor); code != 0 {
			return code
		}
	}
	printRunStatsTo(stderrW, opts.format, opts.quiet, runStats{
		Checked:  result.FilesChecked,
		Fixed:    0,
		Failures: len(result.Diagnostics),
		Unfixed:  len(result.Diagnostics),
	})
	logger.Printf("checked %d files, %d issues found", result.FilesChecked, len(result.Diagnostics))

	if len(result.Errors) > 0 && len(result.Diagnostics) == 0 {
		return 2
	}
	if len(result.Diagnostics) > 0 {
		return 1
	}
	return 0
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
