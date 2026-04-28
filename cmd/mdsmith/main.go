package main

import (
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime/debug"

	flag "github.com/spf13/pflag"
	"gopkg.in/yaml.v3"

	"github.com/jeduden/mdsmith/internal/concepts"
	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/discovery"
	"github.com/jeduden/mdsmith/internal/engine"
	fixpkg "github.com/jeduden/mdsmith/internal/fix"
	"github.com/jeduden/mdsmith/internal/lint"
	vlog "github.com/jeduden/mdsmith/internal/log"
	"github.com/jeduden/mdsmith/internal/output"
	"github.com/jeduden/mdsmith/internal/query"
	"github.com/jeduden/mdsmith/internal/rule"
	ruledocs "github.com/jeduden/mdsmith/internal/rules"

	// Import all rule packages so their init() functions register rules.
	_ "github.com/jeduden/mdsmith/internal/rules/blanklinearoundfencedcode"
	_ "github.com/jeduden/mdsmith/internal/rules/blanklinearoundheadings"
	_ "github.com/jeduden/mdsmith/internal/rules/blanklinearoundlists"
	_ "github.com/jeduden/mdsmith/internal/rules/catalog"
	_ "github.com/jeduden/mdsmith/internal/rules/concisenessscoring"
	_ "github.com/jeduden/mdsmith/internal/rules/crossfilereferenceintegrity"
	_ "github.com/jeduden/mdsmith/internal/rules/directorystructure"
	_ "github.com/jeduden/mdsmith/internal/rules/duplicatedcontent"
	_ "github.com/jeduden/mdsmith/internal/rules/emptysectionbody"
	_ "github.com/jeduden/mdsmith/internal/rules/fencedcodelanguage"
	_ "github.com/jeduden/mdsmith/internal/rules/fencedcodestyle"
	_ "github.com/jeduden/mdsmith/internal/rules/firstlineheading"
	_ "github.com/jeduden/mdsmith/internal/rules/headingincrement"
	_ "github.com/jeduden/mdsmith/internal/rules/headingstyle"
	_ "github.com/jeduden/mdsmith/internal/rules/include"
	_ "github.com/jeduden/mdsmith/internal/rules/linelength"
	_ "github.com/jeduden/mdsmith/internal/rules/listindent"
	_ "github.com/jeduden/mdsmith/internal/rules/listmarkerstyle"
	_ "github.com/jeduden/mdsmith/internal/rules/markdownflavor"
	_ "github.com/jeduden/mdsmith/internal/rules/maxfilelength"
	_ "github.com/jeduden/mdsmith/internal/rules/maxsectionlength"
	_ "github.com/jeduden/mdsmith/internal/rules/nobareurls"
	_ "github.com/jeduden/mdsmith/internal/rules/noduplicateheadings"
	_ "github.com/jeduden/mdsmith/internal/rules/noemphasisasheading"
	_ "github.com/jeduden/mdsmith/internal/rules/noemptyalttext"
	_ "github.com/jeduden/mdsmith/internal/rules/nohardtabs"
	_ "github.com/jeduden/mdsmith/internal/rules/nomultipleblanks"
	_ "github.com/jeduden/mdsmith/internal/rules/notrailingpunctuation"
	_ "github.com/jeduden/mdsmith/internal/rules/notrailingspaces"
	_ "github.com/jeduden/mdsmith/internal/rules/paragraphreadability"
	_ "github.com/jeduden/mdsmith/internal/rules/paragraphstructure"
	_ "github.com/jeduden/mdsmith/internal/rules/requiredstructure"
	_ "github.com/jeduden/mdsmith/internal/rules/singletrailingnewline"
	_ "github.com/jeduden/mdsmith/internal/rules/tableformat"
	_ "github.com/jeduden/mdsmith/internal/rules/tablereadability"
	_ "github.com/jeduden/mdsmith/internal/rules/toc"
	_ "github.com/jeduden/mdsmith/internal/rules/tocdirective"
	_ "github.com/jeduden/mdsmith/internal/rules/tokenbudget"
	_ "github.com/jeduden/mdsmith/internal/rules/unclosedcodeblock"
)

func main() {
	os.Exit(run())
}

const usageText = `Usage: mdsmith <command> [flags] [files...]

Commands:
  check          Lint Markdown files (default when given file arguments)
  fix            Auto-fix lint issues in place
  query          Select files by CUE expression on front matter
  help           Show help for rules and topics
  metrics        Show and rank shared Markdown metrics
  merge-driver   Git merge driver for regenerable sections
  archetypes     Discover, show, and locate archetype schemas
  kinds          Inspect declared kinds and resolve effective config per file
  init           Generate a default .mdsmith.yml config file
  version        Print version and exit

Global flags:
  -h, --help      Show this help

Run 'mdsmith <command> --help' for more information on a command.
`

func run() int {
	// Set a process-level memory limit to bound CUE evaluation and
	// other potentially unbounded operations. The Go runtime will
	// aggressively GC before hitting this limit and OOM-panic beyond
	// it. Respect any externally set GOMEMLIMIT environment variable.
	if os.Getenv("GOMEMLIMIT") == "" {
		debug.SetMemoryLimit(512 * 1024 * 1024)
	}

	// Handle no arguments: print usage, exit 0.
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usageText)
		return 0
	}

	// Handle global flags before subcommand dispatch.
	first := os.Args[1]

	switch first {
	case "--help", "-h":
		fmt.Fprint(os.Stderr, usageText)
		return 0
	}

	// Dispatch to subcommand.
	switch first {
	case "check":
		return runCheck(os.Args[2:])
	case "fix":
		return runFix(os.Args[2:])
	case "query":
		return runQuery(os.Args[2:])
	case "help":
		return runHelp(os.Args[2:])
	case "metrics":
		return runMetrics(os.Args[2:])
	case "merge-driver":
		return runMergeDriver(os.Args[2:])
	case "archetypes":
		return runArchetypes(os.Args[2:])
	case "kinds":
		return runKinds(os.Args[2:])
	case "init":
		return runInit(os.Args[2:])
	case "version":
		printVersion()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "mdsmith: unknown command %q\n\n%s", first, usageText)
		return 2
	}
}

// version is set via ldflags at build time (e.g. -X main.version=v1.0.0).
var version string

func printVersion() {
	v := version
	if v == "" {
		v = "(devel)"
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
			v = info.Main.Version
		}
	}
	fmt.Printf("mdsmith %s\n", v)
}

// runCheck implements the "check" subcommand: lint files.
func runCheck(args []string) int {
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
		return 2
	}

	// --quiet suppresses verbose
	if quiet {
		verbose = false
	}

	walk := walkCLI{
		noGitignore:    noGitignore,
		followSymlinks: followSymlinksOverride(fs, followSymlinks),
	}

	allArgs := fs.Args()

	// Check for explicit stdin argument "-".
	hasStdin, fileArgs := splitStdinArg(allArgs)

	if hasStdin {
		return checkStdin(format, noColor, quiet, verbose, configPath, maxInputSize, explain)
	}

	if len(fileArgs) > 0 {
		return checkFiles(fileArgs, configPath, format, noColor, quiet, verbose, walk, maxInputSize, explain)
	}

	// No file args and no stdin: discover files from config.
	return checkDiscovered(configPath, format, noColor, quiet, verbose, walk, maxInputSize, explain)
}

// runFix implements the "fix" subcommand: auto-fix lint issues in place.
func runFix(args []string) int {
	fs := flag.NewFlagSet("fix", flag.ContinueOnError)
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
	fs.BoolVar(&explain, "explain", false, "Attach per-leaf rule provenance to each remaining diagnostic")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: mdsmith fix [flags] [files...]\n\n"+
			"Auto-fix lint issues in Markdown files.\n\n"+
			"Files can be paths, directories (walked recursively for *.md), or glob patterns.\n"+
			"Pass - to read from stdin (rejected: files must be writable).\n"+
			"With no file arguments, discovers files using config patterns.\n\n"+
			"Flags:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return 2
	}

	// --quiet suppresses verbose
	if quiet {
		verbose = false
	}

	walk := walkCLI{
		noGitignore:    noGitignore,
		followSymlinks: followSymlinksOverride(fs, followSymlinks),
	}

	allArgs := fs.Args()

	// Check for explicit stdin argument "-".
	hasStdin, fileArgs := splitStdinArg(allArgs)

	if hasStdin {
		fmt.Fprintf(os.Stderr, "mdsmith: cannot fix stdin in place\n")
		return 2
	}

	if len(fileArgs) > 0 {
		return fixFiles(fileArgs, configPath, format, noColor, quiet, verbose, walk, maxInputSize, explain)
	}

	// No file args: discover files from config.
	return fixDiscovered(configPath, format, noColor, quiet, verbose, walk, maxInputSize, explain)
}

// runQuery implements the "query" subcommand: select files by CUE
// expression on front matter.
type queryOptions struct {
	nul          bool
	verbose      bool
	configPath   string
	maxInputSize string
}

func parseQueryFlags(args []string) (queryOptions, []string, error) {
	fs := flag.NewFlagSet("query", flag.ContinueOnError)
	var opts queryOptions

	fs.BoolVarP(&opts.nul, "null", "0", false, "NUL-delimit output (for xargs -0)")
	fs.BoolVarP(&opts.verbose, "verbose", "v", false, "Print skipped files and reasons on stderr")
	fs.StringVarP(&opts.configPath, "config", "c", "", "Override config file path")
	fs.StringVar(&opts.maxInputSize, "max-input-size", "",
		"Maximum file size to process (e.g. 2MB, 500KB, 0=unlimited)")

	fs.Usage = func() {
		fmt.Fprint(os.Stderr, "Usage: mdsmith query [flags] <cue-expr> [files...]\n\n"+
			"Print paths of Markdown files whose front matter satisfies a CUE expression.\n"+
			"With no file arguments, searches the current directory recursively.\n\n"+
			"Exit codes: 0 match, 1 no match, 2 error\n\nFlags:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return opts, nil, err
	}
	return opts, fs.Args(), nil
}

func runQuery(args []string) int {
	opts, posArgs, err := parseQueryFlags(args)
	if err != nil {
		return 2
	}

	if len(posArgs) == 0 {
		fmt.Fprintf(os.Stderr, "mdsmith: query requires a CUE expression argument\n")
		return 2
	}

	expr := posArgs[0]
	fileArgs := posArgs[1:]

	matcher, err := query.Compile(expr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}

	if len(fileArgs) == 0 {
		fileArgs = []string{"."}
	}

	files, err := lint.ResolveFilesWithOpts(fileArgs, lint.ResolveOpts{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}

	cfg, _, err := loadConfig(opts.configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}
	maxBytes, err := resolveMaxInputBytes(cfg, opts.maxInputSize)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}

	delim := "\n"
	if opts.nul {
		delim = "\x00"
	}

	matched := queryFiles(matcher, files, delim, opts.verbose, maxBytes)
	if matched > 0 {
		return 0
	}
	return 1
}

// queryFiles tests each file against matcher and writes matching paths
// to stdout. Returns the number of matches.
func queryFiles(matcher *query.Matcher, files []string, delim string, verbose bool, maxBytes int64) int {
	matched := 0
	for _, f := range files {
		fm, err := readFrontMatterRaw(f, maxBytes)
		if err != nil {
			if verbose {
				fmt.Fprintf(os.Stderr, "skip %s: %v\n", f, err)
			}
			continue
		}
		if fm == nil {
			if verbose {
				fmt.Fprintf(os.Stderr, "skip %s: no front matter\n", f)
			}
			continue
		}
		if matcher.Match(fm) {
			_, _ = fmt.Fprintf(os.Stdout, "%s%s", f, delim)
			matched++
		} else if verbose {
			fmt.Fprintf(os.Stderr, "skip %s: expression not satisfied\n", f)
		}
	}
	return matched
}

// readFrontMatterRaw reads a file, strips front matter, and
// unmarshals YAML into map[string]any (preserving numeric types).
func readFrontMatterRaw(path string, maxBytes int64) (map[string]any, error) {
	data, err := lint.ReadFileLimited(path, maxBytes)
	if err != nil {
		return nil, err
	}
	prefix, _ := lint.StripFrontMatter(data)
	if prefix == nil {
		return nil, nil
	}
	// Strip the --- delimiters to get the YAML body.
	delim := []byte("---\n")
	yamlBytes := prefix[len(delim) : len(prefix)-len(delim)]

	if err := lint.RejectYAMLAliases(yamlBytes); err != nil {
		return nil, fmt.Errorf("parsing front matter: %w", err)
	}
	var raw map[string]any
	if err := yaml.Unmarshal(yamlBytes, &raw); err != nil {
		return nil, fmt.Errorf("parsing front matter: %w", err)
	}
	// Distinguish empty front matter (---\n---\n) from absent front matter.
	// An empty YAML document unmarshals to nil; normalize to an empty map
	// so the caller only sees nil when no front matter block exists.
	if raw == nil {
		raw = make(map[string]any)
	}
	return raw, nil
}

// runInit implements the "init" subcommand: generate .mdsmith.yml.
func runInit(args []string) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: mdsmith init\n\n"+
			"Generate a default .mdsmith.yml config file in the current directory.\n")
	}

	if err := fs.Parse(args); err != nil {
		return 2
	}

	if fs.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "mdsmith: init takes no arguments\n")
		return 2
	}

	const configFile = ".mdsmith.yml"

	// Check if config file already exists.
	if _, err := os.Stat(configFile); err == nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %s already exists\n", configFile)
		return 2
	}

	cfg := config.DumpDefaults()

	// Set front-matter: true as default.
	fm := true
	cfg.FrontMatter = &fm

	data, err := yaml.Marshal(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: marshalling config: %v\n", err)
		return 2
	}

	if err := os.WriteFile(configFile, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: writing %s: %v\n", configFile, err)
		return 2
	}

	fmt.Fprintf(os.Stderr, "mdsmith: created %s\n", configFile)
	return 0
}

// formatDiagnosticsTo writes diagnostics to w using the specified format.
// Returns a non-zero exit code on write error, or 0 on success.
func formatDiagnosticsTo(w io.Writer, diags []lint.Diagnostic, format string, noColor bool) int {
	var formatter output.Formatter
	switch format {
	case "json":
		formatter = &output.JSONFormatter{}
	default:
		formatter = &output.TextFormatter{Color: !noColor}
	}
	if err := formatter.Format(w, diags); err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: error writing output: %v\n", err)
		return 2
	}
	return 0
}

// formatDiagnostics writes diagnostics to stderr using the specified format.
func formatDiagnostics(diags []lint.Diagnostic, format string, noColor bool) int {
	return formatDiagnosticsTo(os.Stderr, diags, format, noColor)
}

// printErrors writes runtime errors to stderr.
func printErrors(errs []error) {
	for _, e := range errs {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", e)
	}
}

type runStats struct {
	Checked  int
	Fixed    int
	Failures int
	Unfixed  int
}

func printRunStats(format string, quiet bool, stats runStats) {
	if quiet || format == "json" {
		return
	}
	fmt.Fprintf(
		os.Stderr,
		"stats: checked=%d fixed=%d failures=%d unfixed=%d\n",
		stats.Checked,
		stats.Fixed,
		stats.Failures,
		stats.Unfixed,
	)
}

// checkFiles lints the given file paths and returns the appropriate exit code.
func checkFiles(
	fileArgs []string, configPath, format string,
	noColor, quiet, verbose bool, walk walkCLI,
	maxInputSize string, explain bool,
) int {
	cfg, cfgPath, logger, files, maxBytes, code := loadAndResolve(
		fileArgs, configPath, verbose, walk, maxInputSize,
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
		Explain:          explain,
	}
	result := runner.Run(files)
	printErrors(result.Errors)

	if !quiet && len(result.Diagnostics) > 0 {
		if code := formatDiagnostics(result.Diagnostics, format, noColor); code != 0 {
			return code
		}
	}
	printRunStats(format, quiet, runStats{
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

// fixFiles fixes lint issues in the given file paths.
func fixFiles(
	fileArgs []string, configPath, format string,
	noColor, quiet, verbose bool, walk walkCLI,
	maxInputSize string, explain bool,
) int {
	cfg, cfgPath, logger, files, maxBytes, code := loadAndResolve(
		fileArgs, configPath, verbose, walk, maxInputSize,
	)
	if code >= 0 {
		return code
	}

	fixer := &fixpkg.Fixer{
		Config:           cfg,
		Rules:            rule.All(),
		StripFrontMatter: frontMatterEnabled(cfg),
		Logger:           logger,
		RootDir:          rootDirFromConfig(cfgPath),
		MaxInputBytes:    maxBytes,
		Explain:          explain,
	}
	fixResult := fixer.Fix(files)
	printErrors(fixResult.Errors)

	if !quiet && len(fixResult.Diagnostics) > 0 {
		if code := formatDiagnostics(fixResult.Diagnostics, format, noColor); code != 0 {
			return code
		}
	}
	printRunStats(format, quiet, runStats{
		Checked:  fixResult.FilesChecked,
		Fixed:    len(fixResult.Modified),
		Failures: fixResult.Failures,
		Unfixed:  len(fixResult.Diagnostics),
	})
	logger.Printf("checked %d files, %d issues found", fixResult.FilesChecked, len(fixResult.Diagnostics))

	if len(fixResult.Errors) > 0 && len(fixResult.Diagnostics) == 0 {
		return 2
	}
	if len(fixResult.Diagnostics) > 0 {
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

// checkStdin reads from stdin, lints the content, and returns the appropriate
// exit code. Uses runner.RunSource to ensure Configurable settings are applied.
func checkStdin(format string, noColor, quiet, verbose bool, configPath, maxInputSize string, explain bool) int {
	logger := &vlog.Logger{Enabled: verbose, W: os.Stderr}

	cfg, cfgPath, err := loadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}
	if cfgPath != "" {
		logger.Printf("config: %s", cfgPath)
	}

	maxBytes, err := resolveMaxInputBytes(cfg, maxInputSize)
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
		Explain:          explain,
	}
	result := runner.RunSource("<stdin>", source)
	printErrors(result.Errors)

	if !quiet && len(result.Diagnostics) > 0 {
		if code := formatDiagnostics(result.Diagnostics, format, noColor); code != 0 {
			return code
		}
	}
	printRunStats(format, quiet, runStats{
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

// loadAndResolve loads config, resolves file paths, and parses the max
// input size. Returns exit code >= 0 on error (caller should return it)
// or -1 on success.
func loadAndResolve(
	fileArgs []string, configPath string,
	verbose bool, walk walkCLI,
	maxInputSize string,
) (*config.Config, string, *vlog.Logger, []string, int64, int) {
	logger := &vlog.Logger{Enabled: verbose, W: os.Stderr}

	cfg, cfgPath, err := loadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return nil, "", nil, nil, 0, 2
	}
	if cfgPath != "" {
		logger.Printf("config: %s", cfgPath)
	}

	opts := resolveOpts(cfg, walk)
	files, err := lint.ResolveFilesWithOpts(fileArgs, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return nil, "", nil, nil, 0, 2
	}
	if len(files) == 0 {
		return nil, "", nil, nil, 0, 0
	}

	maxBytes, err := resolveMaxInputBytes(cfg, maxInputSize)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return nil, "", nil, nil, 0, 2
	}

	return cfg, cfgPath, logger, files, maxBytes, -1
}

// splitStdinArg separates a "-" argument (stdin) from file arguments.
// Returns true if "-" was found and the remaining file arguments.
func splitStdinArg(args []string) (hasStdin bool, fileArgs []string) {
	for _, a := range args {
		if a == "-" {
			hasStdin = true
		} else {
			fileArgs = append(fileArgs, a)
		}
	}
	return hasStdin, fileArgs
}

// discoverFiles loads config, discovers files from config patterns, and
// returns the config, config path, logger, and discovered file list. On
// error or empty results it prints a message and returns a non-negative
// exit code; the caller should return it directly. A negative code means
// "continue with the returned values".
func discoverFiles(
	configPath string, verbose bool, walk walkCLI,
) (*config.Config, string, *vlog.Logger, []string, int) {
	logger := &vlog.Logger{Enabled: verbose, W: os.Stderr}

	cfg, cfgPath, err := loadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return nil, "", nil, nil, 2
	}
	if cfgPath != "" {
		logger.Printf("config: %s", cfgPath)
	}
	if len(cfg.Files) == 0 {
		return nil, "", nil, nil, 0
	}

	files, err := discovery.Discover(discovery.Options{
		Patterns:       cfg.Files,
		UseGitignore:   !walk.noGitignore,
		FollowSymlinks: resolveOpts(cfg, walk).FollowSymlinks,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: discovering files: %v\n", err)
		return nil, "", nil, nil, 2
	}
	if len(files) == 0 {
		return nil, "", nil, nil, 0
	}
	return cfg, cfgPath, logger, files, -1
}

// checkDiscovered loads config, discovers files from config patterns,
// and lints them. Returns the appropriate exit code.
func checkDiscovered(
	configPath, format string,
	noColor, quiet, verbose bool, walk walkCLI,
	maxInputSize string, explain bool,
) int {
	cfg, cfgPath, logger, files, code := discoverFiles(configPath, verbose, walk)
	if code >= 0 {
		return code
	}

	maxBytes, err := resolveMaxInputBytes(cfg, maxInputSize)
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
		Explain:          explain,
	}
	result := runner.Run(files)
	printErrors(result.Errors)

	if !quiet && len(result.Diagnostics) > 0 {
		if code := formatDiagnostics(result.Diagnostics, format, noColor); code != 0 {
			return code
		}
	}
	printRunStats(format, quiet, runStats{
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

// fixDiscovered loads config, discovers files from config patterns,
// and fixes them. Returns the appropriate exit code.
func fixDiscovered(
	configPath, format string,
	noColor, quiet, verbose bool, walk walkCLI,
	maxInputSize string, explain bool,
) int {
	cfg, cfgPath, logger, files, code := discoverFiles(configPath, verbose, walk)
	if code >= 0 {
		return code
	}

	maxBytes, err := resolveMaxInputBytes(cfg, maxInputSize)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}

	fixer := &fixpkg.Fixer{
		Config:           cfg,
		Rules:            rule.All(),
		StripFrontMatter: frontMatterEnabled(cfg),
		Logger:           logger,
		RootDir:          rootDirFromConfig(cfgPath),
		MaxInputBytes:    maxBytes,
		Explain:          explain,
	}
	fixResult := fixer.Fix(files)
	printErrors(fixResult.Errors)

	if !quiet && len(fixResult.Diagnostics) > 0 {
		if code := formatDiagnostics(fixResult.Diagnostics, format, noColor); code != 0 {
			return code
		}
	}
	printRunStats(format, quiet, runStats{
		Checked:  fixResult.FilesChecked,
		Fixed:    len(fixResult.Modified),
		Failures: fixResult.Failures,
		Unfixed:  len(fixResult.Diagnostics),
	})
	logger.Printf("checked %d files, %d issues found", fixResult.FilesChecked, len(fixResult.Diagnostics))

	if len(fixResult.Errors) > 0 && len(fixResult.Diagnostics) == 0 {
		return 2
	}
	if len(fixResult.Diagnostics) > 0 {
		return 1
	}
	return 0
}

// walkCLI bundles the CLI flags that affect how files are
// discovered and resolved, so helpers can thread one value
// instead of several (and the next addition isn't a parameter
// explosion). followSymlinks is tri-state: nil means "fall back
// to cfg.FollowSymlinks"; non-nil overrides config either way,
// so users can write `--follow-symlinks=false` to force the
// secure default for a one-off run against a config that opts
// in.
type walkCLI struct {
	noGitignore    bool
	followSymlinks *bool
}

// frontMatterEnabled returns whether front matter stripping is enabled.
// Defaults to true if not set in config.
// resolveOpts builds ResolveOpts from config and CLI flags.
// The `--follow-symlinks` CLI flag overrides `follow-symlinks:`
// from config when explicitly set; otherwise the config value
// stands.
func resolveOpts(cfg *config.Config, walk walkCLI) lint.ResolveOpts {
	useGitignore := !walk.noGitignore
	follow := cfg.FollowSymlinks
	if walk.followSymlinks != nil {
		follow = *walk.followSymlinks
	}
	return lint.ResolveOpts{
		UseGitignore:   &useGitignore,
		FollowSymlinks: follow,
	}
}

// followSymlinksOverride returns a *bool override for the
// `--follow-symlinks` flag if it was explicitly set on the
// command line, or nil to defer to config.
func followSymlinksOverride(fs *flag.FlagSet, value bool) *bool {
	if fs.Changed("follow-symlinks") {
		v := value
		return &v
	}
	return nil
}

// resolveMaxInputBytes returns the effective max-input-size in bytes.
// CLI flag overrides config; if neither is set, the default (2 MB) is used.
func resolveMaxInputBytes(cfg *config.Config, cliFlag string) (int64, error) {
	raw := cliFlag
	if raw == "" {
		raw = cfg.MaxInputSize
	}
	if raw == "" {
		return lint.DefaultMaxInputBytes, nil
	}
	n, err := config.ParseSize(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid max-input-size %q: %w", raw, err)
	}
	return n, nil
}

func frontMatterEnabled(cfg *config.Config) bool {
	if cfg.FrontMatter != nil {
		return *cfg.FrontMatter
	}
	return true
}

// rootDirFromConfig returns the project root directory derived from the
// config file path. If cfgPath is empty, it falls back to the current
// working directory so that includes with ".." paths still resolve.
func rootDirFromConfig(cfgPath string) string {
	if cfgPath != "" {
		return filepath.Dir(cfgPath)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return cwd
}

// loadConfig loads configuration by either using the specified path or
// discovering a config file from the current directory. It returns the
// merged config, the path that was loaded (empty if defaults only), and
// any error.
func loadConfig(configPath string) (*config.Config, string, error) {
	cfg, path, err := loadConfigRaw(configPath)
	if err != nil {
		return nil, "", err
	}
	config.InjectArchetypeRoots(cfg)
	return cfg, path, nil
}

func loadConfigRaw(configPath string) (*config.Config, string, error) {
	defaults := config.Defaults()

	if configPath != "" {
		loaded, err := config.Load(configPath)
		if err != nil {
			return nil, "", err
		}
		merged := config.Merge(defaults, loaded)
		printDeprecations(merged)
		return merged, configPath, nil
	}

	// Try to discover a config file.
	cwd, err := os.Getwd()
	if err != nil {
		return config.Merge(defaults, nil), "", nil
	}

	discovered, err := config.Discover(cwd)
	if err != nil {
		return config.Merge(defaults, nil), "", nil
	}

	if discovered == "" {
		return config.Merge(defaults, nil), "", nil
	}

	loaded, err := config.Load(discovered)
	if err != nil {
		return nil, "", err
	}

	merged := config.Merge(defaults, loaded)
	printDeprecations(merged)
	return merged, discovered, nil
}

// printDeprecations writes config deprecation warnings to stderr. It is
// safe to call multiple times; the warnings are consumed so the second
// call is a no-op.
func printDeprecations(cfg *config.Config) {
	if cfg == nil {
		return
	}
	for _, msg := range cfg.Deprecations {
		fmt.Fprintf(os.Stderr, "mdsmith: deprecated: %s\n", msg)
	}
	cfg.Deprecations = nil
}

const helpUsageText = `Usage: mdsmith help <topic>

Topics:
  rule [id|name]        Show rule documentation
  metrics [id|name]     Show metric documentation
  kinds                 Show concept page for file kinds
  kinds-cli             Summarize the 'kinds' subcommand surface
  placeholder-grammar   Show placeholder vocabulary reference
`

// runHelp implements the "help" subcommand.
func runHelp(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, helpUsageText)
		return 0
	}

	switch args[0] {
	case "rule":
		return runHelpRule(args[1:])
	case "metrics":
		return runHelpMetrics(args[1:])
	case "kinds":
		return runHelpKinds()
	case "kinds-cli":
		return runHelpKindsCLI()
	case "placeholder-grammar":
		return runHelpConcept("placeholder-grammar")
	default:
		fmt.Fprintf(os.Stderr, "mdsmith: help: unknown topic %q\n", args[0])
		return 2
	}
}

const helpKindsText = `File Kinds

A kind is a named bundle of rule settings that can be applied to a set of
files. Kinds let you share per-rule tuning across files that serve the same
purpose (schema, template, fragment, prompt, …) without repeating overrides.

DECLARATION

Declare kinds under the kinds: key. The body has the same shape as an
override entry (rules:, categories:) — minus files:, since files are bound
to kinds separately:

  kinds:
    plan:
      rules:
        required-structure:
          schema: plan/proto.md
        paragraph-readability: false
    proto:
      rules:
        paragraph-readability: false
        first-line-heading: false

Kind names are project-chosen. mdsmith ships no built-in kinds.

ASSIGNMENT

A file's effective kind list is built from two sources, concatenated in
this order:

  1. Front-matter kinds: field (YAML list).
  2. Matching entries in kind-assignment: (config order; each entry's kinds
     in the order listed).

Duplicate names are dropped after their first occurrence. Referencing an
undeclared kind is a config error.

  kind-assignment:
    - files: ["plan/[0-9]*_*.md"]
      kinds: [plan]
    - files: ["**/proto.md"]
      kinds: [proto]

MERGE ORDER

Kinds apply after top-level rules and before glob overrides:

  top-level rules → kinds (effective-list order) → glob overrides

Within kinds, the later kind in the effective list replaces the earlier
kind's entire rule config for that rule — no deep-merge, same as overrides.
A file's own glob overrides apply last and take highest precedence.

COMPOSABILITY

Rules never reference kind names. New kinds cannot regress existing behavior.
`

// runHelpKinds prints the kinds concept page.
func runHelpKinds() int {
	fmt.Print(helpKindsText)
	return 0
}

const helpKindsCLIText = `Kinds Subcommand

mdsmith kinds <subcommand> [args]

Subcommands:
  list                  Print declared kinds with their merged bodies.
  show <name>           Print one kind's merged body.
  path <name>           Print the resolved schema path of the kind's
                        required-structure rule, if any.
  resolve <file>        Print the resolved kind list and merged rule
                        config for a file, with per-leaf provenance.
  why <file> <rule>     Print the full merge chain for one rule on
                        one file: every applicable layer, including
                        no-ops, with the value at each step.

Each subcommand accepts --json for stable structured output. The
schema is documented in docs/reference/cli.md.

Provenance layers are: 'default' (top-level rules: + built-ins),
'kinds.<name>' (one per kind in the effective list), and
'overrides[<i>]' (one per matching glob override entry).

See also: 'mdsmith check --explain' / 'mdsmith fix --explain' to
attach the same provenance trailer to each diagnostic.
`

// runHelpKindsCLI prints the kinds-cli help topic.
func runHelpKindsCLI() int {
	fmt.Print(helpKindsCLIText)
	return 0
}

// runHelpConcept prints the named concept page.
func runHelpConcept(name string) int {
	content, err := concepts.Lookup(name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}
	fmt.Print(content)
	return 0
}

// runHelpRule implements "help rule [id|name]".
func runHelpRule(args []string) int {
	if len(args) == 0 {
		return listAllRules()
	}
	return showRule(args[0])
}

func listAllRules() int {
	rules, err := ruledocs.ListRules()
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}

	for _, r := range rules {
		fmt.Printf("%-6s %-40s %-10s %s\n", r.ID, r.Name, r.Status, r.Description)
	}
	return 0
}

func showRule(query string) int {
	content, err := ruledocs.LookupRule(query)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}
	fmt.Print(content)
	return 0
}
