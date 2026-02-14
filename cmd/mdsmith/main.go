package main

import (
	"fmt"
	"io"
	"os"
	"runtime/debug"

	flag "github.com/spf13/pflag"
	"gopkg.in/yaml.v3"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/discovery"
	"github.com/jeduden/mdsmith/internal/engine"
	fixpkg "github.com/jeduden/mdsmith/internal/fix"
	"github.com/jeduden/mdsmith/internal/lint"
	vlog "github.com/jeduden/mdsmith/internal/log"
	"github.com/jeduden/mdsmith/internal/output"
	"github.com/jeduden/mdsmith/internal/rule"
	ruledocs "github.com/jeduden/mdsmith/internal/rules"

	// Import all rule packages so their init() functions register rules.
	_ "github.com/jeduden/mdsmith/internal/rules/blanklinearoundfencedcode"
	_ "github.com/jeduden/mdsmith/internal/rules/blanklinearoundheadings"
	_ "github.com/jeduden/mdsmith/internal/rules/blanklinearoundlists"
	_ "github.com/jeduden/mdsmith/internal/rules/catalog"
	_ "github.com/jeduden/mdsmith/internal/rules/crossfilereferenceintegrity"
	_ "github.com/jeduden/mdsmith/internal/rules/fencedcodelanguage"
	_ "github.com/jeduden/mdsmith/internal/rules/fencedcodestyle"
	_ "github.com/jeduden/mdsmith/internal/rules/firstlineheading"
	_ "github.com/jeduden/mdsmith/internal/rules/headingincrement"
	_ "github.com/jeduden/mdsmith/internal/rules/headingstyle"
	_ "github.com/jeduden/mdsmith/internal/rules/include"
	_ "github.com/jeduden/mdsmith/internal/rules/linelength"
	_ "github.com/jeduden/mdsmith/internal/rules/listindent"
	_ "github.com/jeduden/mdsmith/internal/rules/maxfilelength"
	_ "github.com/jeduden/mdsmith/internal/rules/nobareurls"
	_ "github.com/jeduden/mdsmith/internal/rules/noduplicateheadings"
	_ "github.com/jeduden/mdsmith/internal/rules/noemphasisasheading"
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
)

func main() {
	os.Exit(run())
}

const usageText = `Usage: mdsmith <command> [flags] [files...]

Commands:
  check     Lint Markdown files (default when given file arguments)
  fix       Auto-fix lint issues in place
  help      Show help for rules and topics
  init      Generate a default .mdsmith.yml config file
  version   Print version and exit

Global flags:
  -h, --help      Show this help

Run 'mdsmith <command> --help' for more information on a command.
`

func run() int {
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
	case "help":
		return runHelp(os.Args[2:])
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

func printVersion() {
	version := "(devel)"
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		version = info.Main.Version
	}
	fmt.Printf("mdsmith %s\n", version)
}

// runCheck implements the "check" subcommand: lint files.
func runCheck(args []string) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	var (
		configPath       string
		format           string
		noColor          bool
		quiet            bool
		verbose          bool
		noGitignore      bool
		noFollowSymlinks bool
	)

	fs.StringVarP(&configPath, "config", "c", "", "Override config file path")
	fs.StringVarP(&format, "format", "f", "text", "Output format: text, json")
	fs.BoolVar(&noColor, "no-color", false, "Disable ANSI colors")
	fs.BoolVarP(&quiet, "quiet", "q", false, "Suppress non-error output")
	fs.BoolVarP(&verbose, "verbose", "v", false, "Show config, files, and rules on stderr")
	fs.BoolVar(&noGitignore, "no-gitignore", false, "Disable .gitignore filtering when walking directories")
	fs.BoolVar(&noFollowSymlinks, "no-follow-symlinks", false, "Skip symbolic links when walking directories")

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

	allArgs := fs.Args()

	// Check for explicit stdin argument "-".
	hasStdin, fileArgs := splitStdinArg(allArgs)

	if hasStdin {
		return checkStdin(format, noColor, quiet, verbose, configPath)
	}

	if len(fileArgs) > 0 {
		return checkFiles(fileArgs, configPath, format, noColor, quiet, verbose, noGitignore, noFollowSymlinks)
	}

	// No file args and no stdin: discover files from config.
	return checkDiscovered(configPath, format, noColor, quiet, verbose, noGitignore, noFollowSymlinks)
}

// runFix implements the "fix" subcommand: auto-fix lint issues in place.
func runFix(args []string) int {
	fs := flag.NewFlagSet("fix", flag.ContinueOnError)
	var (
		configPath       string
		format           string
		noColor          bool
		quiet            bool
		verbose          bool
		noGitignore      bool
		noFollowSymlinks bool
	)

	fs.StringVarP(&configPath, "config", "c", "", "Override config file path")
	fs.StringVarP(&format, "format", "f", "text", "Output format: text, json")
	fs.BoolVar(&noColor, "no-color", false, "Disable ANSI colors")
	fs.BoolVarP(&quiet, "quiet", "q", false, "Suppress non-error output")
	fs.BoolVarP(&verbose, "verbose", "v", false, "Show config, files, and rules on stderr")
	fs.BoolVar(&noGitignore, "no-gitignore", false, "Disable .gitignore filtering when walking directories")
	fs.BoolVar(&noFollowSymlinks, "no-follow-symlinks", false, "Skip symbolic links when walking directories")

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

	allArgs := fs.Args()

	// Check for explicit stdin argument "-".
	hasStdin, fileArgs := splitStdinArg(allArgs)

	if hasStdin {
		fmt.Fprintf(os.Stderr, "mdsmith: cannot fix stdin in place\n")
		return 2
	}

	if len(fileArgs) > 0 {
		return fixFiles(fileArgs, configPath, format, noColor, quiet, verbose, noGitignore, noFollowSymlinks)
	}

	// No file args: discover files from config.
	return fixDiscovered(configPath, format, noColor, quiet, verbose, noGitignore, noFollowSymlinks)
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

// formatDiagnostics writes diagnostics to stderr using the specified format.
// Returns a non-zero exit code on write error, or 0 on success.
func formatDiagnostics(diags []lint.Diagnostic, format string, noColor bool) int {
	var formatter output.Formatter
	switch format {
	case "json":
		formatter = &output.JSONFormatter{}
	default:
		formatter = &output.TextFormatter{Color: !noColor}
	}
	if err := formatter.Format(os.Stderr, diags); err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: error writing output: %v\n", err)
		return 2
	}
	return 0
}

// printErrors writes runtime errors to stderr.
func printErrors(errs []error) {
	for _, e := range errs {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", e)
	}
}

// checkFiles lints the given file paths and returns the appropriate exit code.
func checkFiles(
	fileArgs []string, configPath, format string,
	noColor, quiet, verbose, noGitignore, noFollowSymlinks bool,
) int {
	logger := &vlog.Logger{Enabled: verbose, W: os.Stderr}

	cfg, cfgPath, err := loadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}
	if cfgPath != "" {
		logger.Printf("config: %s", cfgPath)
	}

	opts := resolveOpts(cfg, noGitignore, noFollowSymlinks)
	files, err := lint.ResolveFilesWithOpts(fileArgs, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}
	if len(files) == 0 {
		return 0
	}

	runner := &engine.Runner{
		Config:           cfg,
		Rules:            rule.All(),
		StripFrontMatter: frontMatterEnabled(cfg),
		Logger:           logger,
	}
	result := runner.Run(files)
	printErrors(result.Errors)

	if len(result.Errors) > 0 && len(result.Diagnostics) == 0 {
		return 2
	}
	if !quiet && len(result.Diagnostics) > 0 {
		if code := formatDiagnostics(result.Diagnostics, format, noColor); code != 0 {
			return code
		}
	}
	logger.Printf("checked %d files, %d issues found", len(files), len(result.Diagnostics))

	if len(result.Diagnostics) > 0 {
		return 1
	}
	return 0
}

// fixFiles fixes lint issues in the given file paths.
func fixFiles(
	fileArgs []string, configPath, format string,
	noColor, quiet, verbose, noGitignore, noFollowSymlinks bool,
) int {
	logger := &vlog.Logger{Enabled: verbose, W: os.Stderr}

	cfg, cfgPath, err := loadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}
	if cfgPath != "" {
		logger.Printf("config: %s", cfgPath)
	}

	opts := resolveOpts(cfg, noGitignore, noFollowSymlinks)
	files, err := lint.ResolveFilesWithOpts(fileArgs, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}
	if len(files) == 0 {
		return 0
	}

	fixer := &fixpkg.Fixer{
		Config:           cfg,
		Rules:            rule.All(),
		StripFrontMatter: frontMatterEnabled(cfg),
		Logger:           logger,
	}
	fixResult := fixer.Fix(files)
	printErrors(fixResult.Errors)

	if !quiet && len(fixResult.Diagnostics) > 0 {
		if code := formatDiagnostics(fixResult.Diagnostics, format, noColor); code != 0 {
			return code
		}
	}
	logger.Printf("checked %d files, %d issues found", len(files), len(fixResult.Diagnostics))

	if len(fixResult.Errors) > 0 && len(fixResult.Diagnostics) == 0 {
		return 2
	}
	if len(fixResult.Diagnostics) > 0 {
		return 1
	}
	return 0
}

// checkStdin reads from stdin, lints the content, and returns the appropriate
// exit code. Uses runner.RunSource to ensure Configurable settings are applied.
func checkStdin(format string, noColor, quiet, verbose bool, configPath string) int {
	logger := &vlog.Logger{Enabled: verbose, W: os.Stderr}

	source, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: reading stdin: %v\n", err)
		return 2
	}

	cfg, cfgPath, err := loadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}
	if cfgPath != "" {
		logger.Printf("config: %s", cfgPath)
	}

	runner := &engine.Runner{
		Config:           cfg,
		Rules:            rule.All(),
		StripFrontMatter: frontMatterEnabled(cfg),
		Logger:           logger,
	}
	result := runner.RunSource("<stdin>", source)
	printErrors(result.Errors)

	if len(result.Errors) > 0 && len(result.Diagnostics) == 0 {
		return 2
	}
	if !quiet && len(result.Diagnostics) > 0 {
		if code := formatDiagnostics(result.Diagnostics, format, noColor); code != 0 {
			return code
		}
	}
	logger.Printf("checked 1 files, %d issues found", len(result.Diagnostics))

	if len(result.Diagnostics) > 0 {
		return 1
	}
	return 0
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

// checkDiscovered loads config, discovers files from config patterns,
// and lints them. Returns the appropriate exit code.
func checkDiscovered(
	configPath, format string,
	noColor, quiet, verbose, noGitignore, noFollowSymlinks bool,
) int {
	logger := &vlog.Logger{Enabled: verbose, W: os.Stderr}

	cfg, cfgPath, err := loadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}
	if cfgPath != "" {
		logger.Printf("config: %s", cfgPath)
	}

	// Discover files using config patterns.
	if len(cfg.Files) == 0 {
		return 0
	}

	files, err := discovery.Discover(discovery.Options{
		Patterns:         cfg.Files,
		UseGitignore:     !noGitignore,
		NoFollowSymlinks: resolveOpts(cfg, noGitignore, noFollowSymlinks).NoFollowSymlinks,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: discovering files: %v\n", err)
		return 2
	}
	if len(files) == 0 {
		return 0
	}

	runner := &engine.Runner{
		Config:           cfg,
		Rules:            rule.All(),
		StripFrontMatter: frontMatterEnabled(cfg),
		Logger:           logger,
	}
	result := runner.Run(files)
	printErrors(result.Errors)

	if len(result.Errors) > 0 && len(result.Diagnostics) == 0 {
		return 2
	}
	if !quiet && len(result.Diagnostics) > 0 {
		if code := formatDiagnostics(result.Diagnostics, format, noColor); code != 0 {
			return code
		}
	}
	logger.Printf("checked %d files, %d issues found", len(files), len(result.Diagnostics))

	if len(result.Diagnostics) > 0 {
		return 1
	}
	return 0
}

// fixDiscovered loads config, discovers files from config patterns,
// and fixes them. Returns the appropriate exit code.
func fixDiscovered(
	configPath, format string,
	noColor, quiet, verbose, noGitignore, noFollowSymlinks bool,
) int {
	logger := &vlog.Logger{Enabled: verbose, W: os.Stderr}

	cfg, cfgPath, err := loadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}
	if cfgPath != "" {
		logger.Printf("config: %s", cfgPath)
	}

	// Discover files using config patterns.
	if len(cfg.Files) == 0 {
		return 0
	}

	files, err := discovery.Discover(discovery.Options{
		Patterns:         cfg.Files,
		UseGitignore:     !noGitignore,
		NoFollowSymlinks: resolveOpts(cfg, noGitignore, noFollowSymlinks).NoFollowSymlinks,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: discovering files: %v\n", err)
		return 2
	}
	if len(files) == 0 {
		return 0
	}

	fixer := &fixpkg.Fixer{
		Config:           cfg,
		Rules:            rule.All(),
		StripFrontMatter: frontMatterEnabled(cfg),
		Logger:           logger,
	}
	fixResult := fixer.Fix(files)
	printErrors(fixResult.Errors)

	if !quiet && len(fixResult.Diagnostics) > 0 {
		if code := formatDiagnostics(fixResult.Diagnostics, format, noColor); code != 0 {
			return code
		}
	}
	logger.Printf("checked %d files, %d issues found", len(files), len(fixResult.Diagnostics))

	if len(fixResult.Errors) > 0 && len(fixResult.Diagnostics) == 0 {
		return 2
	}
	if len(fixResult.Diagnostics) > 0 {
		return 1
	}
	return 0
}

// frontMatterEnabled returns whether front matter stripping is enabled.
// Defaults to true if not set in config.
// resolveOpts builds ResolveOpts from config and CLI flags.
// CLI flags override config: --no-gitignore disables gitignore filtering,
// --no-follow-symlinks skips all symlinks (adds "**" to the pattern list).
func resolveOpts(cfg *config.Config, noGitignore, noFollowSymlinks bool) lint.ResolveOpts {
	useGitignore := !noGitignore
	opts := lint.ResolveOpts{
		UseGitignore:     &useGitignore,
		NoFollowSymlinks: cfg.NoFollowSymlinks,
	}
	if noFollowSymlinks {
		opts.NoFollowSymlinks = []string{"**"}
	}
	return opts
}

func frontMatterEnabled(cfg *config.Config) bool {
	if cfg.FrontMatter != nil {
		return *cfg.FrontMatter
	}
	return true
}

// loadConfig loads configuration by either using the specified path or
// discovering a config file from the current directory. It returns the
// merged config, the path that was loaded (empty if defaults only), and
// any error.
func loadConfig(configPath string) (*config.Config, string, error) {
	defaults := config.Defaults()

	if configPath != "" {
		loaded, err := config.Load(configPath)
		if err != nil {
			return nil, "", err
		}
		return config.Merge(defaults, loaded), configPath, nil
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

	return config.Merge(defaults, loaded), discovered, nil
}

const helpUsageText = `Usage: mdsmith help <topic>

Topics:
  rule [id|name]   Show rule documentation
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
	default:
		fmt.Fprintf(os.Stderr, "mdsmith: help: unknown topic %q\n", args[0])
		return 2
	}
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
		fmt.Printf("%-6s %-40s %s\n", r.ID, r.Name, r.Description)
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
