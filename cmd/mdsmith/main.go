package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"

	flag "github.com/spf13/pflag"

	"github.com/jeduden/mdsmith/internal/bytelimit"
	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/discovery"
	"github.com/jeduden/mdsmith/internal/lint"
	vlog "github.com/jeduden/mdsmith/internal/log"
	"github.com/jeduden/mdsmith/internal/markdownlint"
	"github.com/jeduden/mdsmith/internal/output"
	"github.com/jeduden/mdsmith/internal/profiling"
	"github.com/jeduden/mdsmith/internal/query"
	"github.com/jeduden/mdsmith/internal/yamlutil"
	mdsmith "github.com/jeduden/mdsmith/pkg/mdsmith"

	// Import every production rule package via the shared barrel so
	// the registry is populated by init(). Tests that need the same
	// set (e.g. internal/lsp/bench_test.go) blank-import the same
	// path; the barrel is the single source of truth for "what
	// rules ship in mdsmith".
	_ "github.com/jeduden/mdsmith/internal/rules/all"
)

func main() {
	os.Exit(run())
}

const usageText = `Usage: mdsmith <command> [flags] [files...]

Commands:
  check             Lint Markdown files (default when given file arguments)
  fix               Auto-fix lint issues in place
  export            Write a portable, directive-free copy of a Markdown file
  extract           Emit a kind-conformant file as a JSON/YAML/msgpack data tree
  list              Walk the workspace and emit matches (files or link records)
  deps              Show a file's dependency-graph edges (includes, links, …)
  rename            Rename a heading or link-ref label and rewrite dependents
  help              Show help for rules and topics
  metrics           Show and rank shared Markdown metrics
  merge-driver      Git merge driver for regenerable sections
  pre-merge-commit  Install/manage pre-merge-commit hook
  kinds             Inspect declared kinds and resolve effective config per file
  init              Generate .mdsmith.yml (--from-markdownlint converts an existing config)
  lsp               Run the Language Server Protocol server on stdio
  version           Print version and exit

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

	// Env-gated profiler (no CLI flag): when MDSMITH_CPUPROFILE /
	// MDSMITH_MEMPROFILE are set, profile the real end-to-end path
	// so a tripped performance gate can be traced to a function.
	defer profiling.Start()()

	// No arguments or a global help flag: print usage and exit 0.
	if len(os.Args) < 2 || os.Args[1] == "--help" || os.Args[1] == "-h" {
		fmt.Fprint(os.Stderr, usageText)
		return 0
	}

	// Dispatch to subcommand.
	return dispatch(os.Args[1], os.Args[2:])
}

// dispatch routes a subcommand name to its handler. Split out of run
// so run stays under the statement-count limit as commands are added.
func dispatch(first string, args []string) int {
	switch first {
	case "check":
		return runCheck(args)
	case "fix":
		return runFix(args)
	case "export":
		return runExport(args)
	case "extract":
		return runExtract(args)
	case "list":
		return runList(args)
	case "deps":
		return runDeps(args)
	case "rename":
		return runRename(args)
	case "help":
		return runHelp(args)
	case "metrics":
		return runMetrics(args)
	case "merge-driver":
		return runMergeDriver(args)
	case "pre-merge-commit":
		return runPreMergeCommit(args)
	case "kinds":
		return runKinds(args)
	case "init":
		return runInit(args)
	case "lsp":
		return runLSP(args)
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
		fmt.Fprint(os.Stderr, "Usage: mdsmith list query [flags] <cue-expr> [files...]\n\n"+
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
		if code := reportFlagParseErr(err, os.Stderr, "mdsmith: list query"); code >= 0 {
			return code
		}
	}

	if len(posArgs) == 0 {
		fmt.Fprintf(os.Stderr, "mdsmith: list query requires a CUE expression argument\n")
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
	data, err := bytelimit.ReadFileLimited(path, maxBytes)
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

	var raw map[string]any
	if err := yamlutil.UnmarshalSafe(yamlBytes, &raw); err != nil {
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

// runInit implements the "init" subcommand: generate .mdsmith.yml,
// either from the built-in defaults or converted from an existing
// markdownlint config (--from-markdownlint).
func runInit(args []string) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	var fromMarkdownlint string
	fs.StringVar(&fromMarkdownlint, "from-markdownlint", "",
		"Convert a markdownlint config instead of writing defaults (optionally --from-markdownlint=<path>)")
	fs.Lookup("from-markdownlint").NoOptDefVal = "auto"

	fs.Usage = func() {
		fmt.Fprint(os.Stderr, "Usage: mdsmith init [--from-markdownlint[=path]]\n\n"+
			"Generate a default .mdsmith.yml config file in the current directory.\n\n"+
			"With --from-markdownlint, convert an existing markdownlint config\n"+
			"(.markdownlint.jsonc/.json/.yaml/.yml or .markdownlintrc) instead of\n"+
			"writing the defaults. Without =path the config is auto-discovered in\n"+
			"the current directory.\n\nFlags:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if code := reportFlagParseErr(err, os.Stderr, "mdsmith: init"); code >= 0 {
			return code
		}
	}

	if fs.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "mdsmith: init takes no arguments (use --from-markdownlint=<path> to name a config)\n")
		return 2
	}

	const configFile = ".mdsmith.yml"

	// Check if config file already exists.
	if _, err := os.Stat(configFile); err == nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %s already exists\n", configFile)
		return 2
	}

	data, source, err := initConfigBytes(fromMarkdownlint, os.Stderr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}

	if err := os.WriteFile(configFile, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: writing %s: %v\n", configFile, err)
		return 2
	}

	if source != "" {
		fmt.Fprintf(os.Stderr, "mdsmith: created %s from %s\n", configFile, source)
	} else {
		fmt.Fprintf(os.Stderr, "mdsmith: created %s\n", configFile)
	}
	return 0
}

// initConfigBytes produces the .mdsmith.yml contents for init. An empty
// fromMarkdownlint yields the full defaults dump; otherwise the named
// markdownlint config ("auto" = discover in the current directory) is
// converted, its notes echoed to w, and source reports which file fed
// the conversion.
func initConfigBytes(fromMarkdownlint string, w io.Writer) (data []byte, source string, err error) {
	if fromMarkdownlint == "" {
		data, err = defaultConfigBytes()
		return data, "", err
	}
	return convertedConfigBytes(fromMarkdownlint, w)
}

// defaultConfigBytes marshals the built-in defaults, the plain
// `mdsmith init` output.
func defaultConfigBytes() ([]byte, error) {
	cfg := config.DumpDefaults()

	// Set front-matter: true as default.
	fm := true
	cfg.FrontMatter = &fm

	return yamlutil.Marshal(cfg)
}

// convertedConfigBytes resolves the markdownlint config path ("auto" =
// discover in the current directory), converts it via
// markdownlint.ConvertFile, and echoes the conversion notes to w.
func convertedConfigBytes(path string, w io.Writer) (data []byte, source string, err error) {
	source = path
	if path == "auto" {
		source, err = markdownlint.Discover(".")
		if err != nil {
			return nil, "", err
		}
	}
	data, notes, err := markdownlint.ConvertFile(source)
	if err != nil {
		return nil, "", err
	}
	for _, note := range notes {
		_, _ = fmt.Fprintf(w, "mdsmith: note: %s\n", note)
	}
	return data, source, nil
}

// formatDiagnosticsTo writes diagnostics to w using the specified format.
// Returns a non-zero exit code on write error, or 0 on success. The
// write-error message is best-effort routed to the same w so callers
// that pass an alternate writer (production: os.Stderr; tests: a
// fault-injecting writer or a buffer) keep all formatter output
// confined to one destination.
func formatDiagnosticsTo(w io.Writer, diags []lint.Diagnostic, format string, noColor bool) int {
	var formatter output.Formatter
	switch format {
	case "json":
		formatter = &output.JSONFormatter{}
	default:
		formatter = &output.TextFormatter{Color: !noColor}
	}
	if err := formatter.Format(w, diags); err != nil {
		_, _ = fmt.Fprintf(w, "mdsmith: error writing output: %v\n", err)
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
	printErrorsTo(os.Stderr, errs)
}

// printErrorsTo writes runtime errors to the supplied writer.
// Write errors are intentionally swallowed; the run itself has
// already produced its diagnostic content and a partial stderr
// notice is not worth stopping the process for.
func printErrorsTo(w io.Writer, errs []error) {
	for _, e := range errs {
		_, _ = fmt.Fprintf(w, "mdsmith: %v\n", e)
	}
}

type runStats struct {
	Checked  int
	Fixed    int
	Failures int
	Unfixed  int
	// WouldFix is the total diagnostics a dry-run preview would
	// have fixed. Rendered only when DryRun is true.
	WouldFix int
	// DryRun signals that the run was a `fix --dry-run`. When true,
	// the stats line appends a `would-fix=N` field; the existing
	// `fixed=` field reads zero because nothing was written.
	DryRun bool
}

func printRunStats(format string, quiet bool, stats runStats) {
	printRunStatsTo(os.Stderr, format, quiet, stats)
}

// printRunStatsTo writes the stats line to the supplied writer.
func printRunStatsTo(w io.Writer, format string, quiet bool, stats runStats) {
	if quiet || format == "json" {
		return
	}
	if stats.DryRun {
		// Write errors swallowed: see printErrorsTo rationale.
		_, _ = fmt.Fprintf(
			w,
			"stats: checked=%d fixed=%d failures=%d unfixed=%d would-fix=%d\n",
			stats.Checked,
			stats.Fixed,
			stats.Failures,
			stats.Unfixed,
			stats.WouldFix,
		)
		return
	}
	_, _ = fmt.Fprintf(
		w,
		"stats: checked=%d fixed=%d failures=%d unfixed=%d\n",
		stats.Checked,
		stats.Fixed,
		stats.Failures,
		stats.Unfixed,
	)
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

// reportFlagParseErr converts an fs.Parse error into the
// canonical CLI exit code while making sure the user sees WHY
// parsing failed. pflag with ContinueOnError silently returns
// the error from Parse — it does not write to fs.Output() — so
// every subcommand that just `return 2`s on a parse error left
// the user staring at a non-zero exit with nothing on stderr.
//
//   - nil           → -1 (caller continues)
//   - flag.ErrHelp  →  0 (Usage was already printed by pflag)
//   - any other err →  2 with `<prefix>: <err>` on stderr
func reportFlagParseErr(err error, stderr io.Writer, prefix string) int {
	if err == nil {
		return -1
	}
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	_, _ = fmt.Fprintf(stderr, "%s: %v\n", prefix, err)
	return 2
}

// resolveMaxInputBytes returns the effective max-input-size in bytes.
// CLI flag overrides config; if neither is set, the default (2 MB) is used.
func resolveMaxInputBytes(cfg *config.Config, cliFlag string) (int64, error) {
	raw := cliFlag
	if raw == "" {
		raw = cfg.MaxInputSize
	}
	if raw == "" {
		return bytelimit.DefaultMaxInputBytes, nil
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

// sessionForCLI builds a pkg/mdsmith.Session over the host filesystem
// for the already-loaded config. cfg has been merged and had its CLI
// side effects applied (build-recipe injection, the include-extract
// projector) by loadConfig, so it is wrapped with mdsmith.ConfigCompiled
// and handed over as-is — NewSession must not re-merge it. The OS
// workspace is rooted at the project root so ReadFile and the engine's
// FS view agree on a workspace-relative uri.
//
// NewSession only fails when its ConfigSource fails to load; a compiled
// source cannot (ConfigCompiled.loadConfig returns the config verbatim
// with a nil error), so the error is always nil here. Rather than carry
// a dead, untestable error branch at every call site, the impossible
// error is dropped — matching the in-tree NewFileFromSource //nolint
// pattern. If a future compiled source becomes fallible, NewSession's
// signature forces this line to surface the error again.
func sessionForCLI(cfg *config.Config, cfgPath string) *mdsmith.Session {
	sess, _ := mdsmith.NewSession(mdsmith.SessionOptions{ //nolint:errcheck // ConfigCompiled never fails to load
		Workspace: mdsmith.OSWorkspace{Root: rootDirFromConfig(cfgPath)},
		Config:    mdsmith.ConfigCompiled(cfg, cfgPath),
	})
	return sess
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
	config.InjectBuildConfig(cfg, path)
	installIncludeExtractProjector(path)
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
