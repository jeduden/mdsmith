package main

import (
	"fmt"
	"io"
	"os"
	"runtime/debug"

	flag "github.com/spf13/pflag"
	"gopkg.in/yaml.v3"

	tidymark "github.com/jeduden/tidymark"
	"github.com/jeduden/tidymark/internal/config"
	"github.com/jeduden/tidymark/internal/engine"
	fixpkg "github.com/jeduden/tidymark/internal/fix"
	"github.com/jeduden/tidymark/internal/lint"
	"github.com/jeduden/tidymark/internal/output"
	"github.com/jeduden/tidymark/internal/rule"

	// Import all rule packages so their init() functions register rules.
	_ "github.com/jeduden/tidymark/internal/rules/blanklinearoundfencedcode"
	_ "github.com/jeduden/tidymark/internal/rules/blanklinearoundheadings"
	_ "github.com/jeduden/tidymark/internal/rules/blanklinearoundlists"
	_ "github.com/jeduden/tidymark/internal/rules/catalog"
	_ "github.com/jeduden/tidymark/internal/rules/fencedcodelanguage"
	_ "github.com/jeduden/tidymark/internal/rules/fencedcodestyle"
	_ "github.com/jeduden/tidymark/internal/rules/firstlineheading"
	_ "github.com/jeduden/tidymark/internal/rules/headingincrement"
	_ "github.com/jeduden/tidymark/internal/rules/headingstyle"
	_ "github.com/jeduden/tidymark/internal/rules/include"
	_ "github.com/jeduden/tidymark/internal/rules/linelength"
	_ "github.com/jeduden/tidymark/internal/rules/listindent"
	_ "github.com/jeduden/tidymark/internal/rules/maxfilelength"
	_ "github.com/jeduden/tidymark/internal/rules/nobareurls"
	_ "github.com/jeduden/tidymark/internal/rules/noduplicateheadings"
	_ "github.com/jeduden/tidymark/internal/rules/noemphasisasheading"
	_ "github.com/jeduden/tidymark/internal/rules/nohardtabs"
	_ "github.com/jeduden/tidymark/internal/rules/nomultipleblanks"
	_ "github.com/jeduden/tidymark/internal/rules/notrailingpunctuation"
	_ "github.com/jeduden/tidymark/internal/rules/notrailingspaces"
	_ "github.com/jeduden/tidymark/internal/rules/paragraphreadability"
	_ "github.com/jeduden/tidymark/internal/rules/paragraphstructure"
	_ "github.com/jeduden/tidymark/internal/rules/requiredstructure"
	_ "github.com/jeduden/tidymark/internal/rules/singletrailingnewline"
	_ "github.com/jeduden/tidymark/internal/rules/tableformat"
)

func main() {
	os.Exit(run())
}

const usageText = `Usage: tidymark <command> [flags] [files...]

Commands:
  check     Lint Markdown files (default when given file arguments)
  fix       Auto-fix lint issues in place
  help      Show help for rules and topics
  init      Generate a default .tidymark.yml config file
  version   Print version and exit

Global flags:
  -h, --help      Show this help

Run 'tidymark <command> --help' for more information on a command.
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
		fmt.Fprintf(os.Stderr, "tidymark: unknown command %q\n\n%s", first, usageText)
		return 2
	}
}

func printVersion() {
	version := "(devel)"
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		version = info.Main.Version
	}
	fmt.Printf("tidymark %s\n", version)
}

// runCheck implements the "check" subcommand: lint files.
func runCheck(args []string) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	var (
		configPath  string
		format      string
		noColor     bool
		quiet       bool
		noGitignore bool
	)

	fs.StringVarP(&configPath, "config", "c", "", "Override config file path")
	fs.StringVarP(&format, "format", "f", "text", "Output format: text, json")
	fs.BoolVar(&noColor, "no-color", false, "Disable ANSI colors")
	fs.BoolVarP(&quiet, "quiet", "q", false, "Suppress non-error output")
	fs.BoolVar(&noGitignore, "no-gitignore", false, "Disable .gitignore filtering when walking directories")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: tidymark check [flags] [files...]\n\n"+
			"Lint Markdown files for style issues.\n\n"+
			"Files can be paths, directories (walked recursively for *.md), or glob patterns.\n"+
			"With no file arguments, reads from stdin if piped.\n\n"+
			"Flags:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return 2
	}

	files := fs.Args()

	// No file args: check if stdin is a pipe.
	if len(files) == 0 {
		if !isStdinPipe() {
			return 0
		}
		return checkStdin(format, noColor, quiet, configPath)
	}

	return checkFiles(files, configPath, format, noColor, quiet, noGitignore)
}

// runFix implements the "fix" subcommand: auto-fix lint issues in place.
func runFix(args []string) int {
	fs := flag.NewFlagSet("fix", flag.ContinueOnError)
	var (
		configPath  string
		format      string
		noColor     bool
		quiet       bool
		noGitignore bool
	)

	fs.StringVarP(&configPath, "config", "c", "", "Override config file path")
	fs.StringVarP(&format, "format", "f", "text", "Output format: text, json")
	fs.BoolVar(&noColor, "no-color", false, "Disable ANSI colors")
	fs.BoolVarP(&quiet, "quiet", "q", false, "Suppress non-error output")
	fs.BoolVar(&noGitignore, "no-gitignore", false, "Disable .gitignore filtering when walking directories")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: tidymark fix [flags] [files...]\n\n"+
			"Auto-fix lint issues in Markdown files.\n\n"+
			"Files can be paths, directories (walked recursively for *.md), or glob patterns.\n"+
			"Stdin is not supported (files must be writable).\n\n"+
			"Flags:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return 2
	}

	files := fs.Args()

	// Fix rejects stdin.
	if len(files) == 0 {
		if isStdinPipe() {
			fmt.Fprintf(os.Stderr, "tidymark: cannot fix stdin in place\n")
			return 2
		}
		return 0
	}

	return fixFiles(files, configPath, format, noColor, quiet, noGitignore)
}

// runInit implements the "init" subcommand: generate .tidymark.yml.
func runInit(args []string) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: tidymark init\n\n"+
			"Generate a default .tidymark.yml config file in the current directory.\n")
	}

	if err := fs.Parse(args); err != nil {
		return 2
	}

	if fs.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "tidymark: init takes no arguments\n")
		return 2
	}

	const configFile = ".tidymark.yml"

	// Check if config file already exists.
	if _, err := os.Stat(configFile); err == nil {
		fmt.Fprintf(os.Stderr, "tidymark: %s already exists\n", configFile)
		return 2
	}

	cfg := config.DumpDefaults()

	// Set front-matter: true as default.
	fm := true
	cfg.FrontMatter = &fm

	data, err := yaml.Marshal(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tidymark: marshalling config: %v\n", err)
		return 2
	}

	if err := os.WriteFile(configFile, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "tidymark: writing %s: %v\n", configFile, err)
		return 2
	}

	fmt.Fprintf(os.Stderr, "tidymark: created %s\n", configFile)
	return 0
}

// checkFiles lints the given file paths and returns the appropriate exit code.
func checkFiles(fileArgs []string, configPath, format string, noColor, quiet, noGitignore bool) int {
	useGitignore := !noGitignore
	opts := lint.ResolveOpts{UseGitignore: &useGitignore}
	files, err := lint.ResolveFilesWithOpts(fileArgs, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tidymark: %v\n", err)
		return 2
	}

	if len(files) == 0 {
		return 0
	}

	cfg, err := loadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tidymark: %v\n", err)
		return 2
	}

	stripFM := frontMatterEnabled(cfg)

	runner := &engine.Runner{
		Config:           cfg,
		Rules:            rule.All(),
		StripFrontMatter: stripFM,
	}

	result := runner.Run(files)

	for _, e := range result.Errors {
		fmt.Fprintf(os.Stderr, "tidymark: %v\n", e)
	}

	if len(result.Errors) > 0 && len(result.Diagnostics) == 0 {
		return 2
	}

	if !quiet && len(result.Diagnostics) > 0 {
		var formatter output.Formatter
		switch format {
		case "json":
			formatter = &output.JSONFormatter{}
		default:
			formatter = &output.TextFormatter{Color: !noColor}
		}

		if err := formatter.Format(os.Stderr, result.Diagnostics); err != nil {
			fmt.Fprintf(os.Stderr, "tidymark: error writing output: %v\n", err)
			return 2
		}
	}

	if len(result.Diagnostics) > 0 {
		return 1
	}

	return 0
}

// fixFiles fixes lint issues in the given file paths.
func fixFiles(fileArgs []string, configPath, format string, noColor, quiet, noGitignore bool) int {
	useGitignore := !noGitignore
	opts := lint.ResolveOpts{UseGitignore: &useGitignore}
	files, err := lint.ResolveFilesWithOpts(fileArgs, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tidymark: %v\n", err)
		return 2
	}

	if len(files) == 0 {
		return 0
	}

	cfg, err := loadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tidymark: %v\n", err)
		return 2
	}

	stripFM := frontMatterEnabled(cfg)

	fixer := &fixpkg.Fixer{
		Config:           cfg,
		Rules:            rule.All(),
		StripFrontMatter: stripFM,
	}

	fixResult := fixer.Fix(files)

	for _, e := range fixResult.Errors {
		fmt.Fprintf(os.Stderr, "tidymark: %v\n", e)
	}

	if !quiet && len(fixResult.Diagnostics) > 0 {
		var formatter output.Formatter
		switch format {
		case "json":
			formatter = &output.JSONFormatter{}
		default:
			formatter = &output.TextFormatter{Color: !noColor}
		}

		if err := formatter.Format(os.Stderr, fixResult.Diagnostics); err != nil {
			fmt.Fprintf(os.Stderr, "tidymark: error writing output: %v\n", err)
			return 2
		}
	}

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
func checkStdin(format string, noColor, quiet bool, configPath string) int {
	source, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tidymark: reading stdin: %v\n", err)
		return 2
	}

	cfg, err := loadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tidymark: %v\n", err)
		return 2
	}

	stripFM := frontMatterEnabled(cfg)

	runner := &engine.Runner{
		Config:           cfg,
		Rules:            rule.All(),
		StripFrontMatter: stripFM,
	}

	result := runner.RunSource("<stdin>", source)

	for _, e := range result.Errors {
		fmt.Fprintf(os.Stderr, "tidymark: %v\n", e)
	}

	if len(result.Errors) > 0 && len(result.Diagnostics) == 0 {
		return 2
	}

	if !quiet && len(result.Diagnostics) > 0 {
		var formatter output.Formatter
		switch format {
		case "json":
			formatter = &output.JSONFormatter{}
		default:
			formatter = &output.TextFormatter{Color: !noColor}
		}

		if err := formatter.Format(os.Stderr, result.Diagnostics); err != nil {
			fmt.Fprintf(os.Stderr, "tidymark: error writing output: %v\n", err)
			return 2
		}
	}

	if len(result.Diagnostics) > 0 {
		return 1
	}

	return 0
}

// isStdinPipe returns true if stdin is a pipe (not a terminal).
func isStdinPipe() bool {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) == 0
}

// frontMatterEnabled returns whether front matter stripping is enabled.
// Defaults to true if not set in config.
func frontMatterEnabled(cfg *config.Config) bool {
	if cfg.FrontMatter != nil {
		return *cfg.FrontMatter
	}
	return true
}

// loadConfig loads configuration by either using the specified path or
// discovering a config file from the current directory.
func loadConfig(configPath string) (*config.Config, error) {
	defaults := config.Defaults()

	if configPath != "" {
		loaded, err := config.Load(configPath)
		if err != nil {
			return nil, err
		}
		return config.Merge(defaults, loaded), nil
	}

	// Try to discover a config file.
	cwd, err := os.Getwd()
	if err != nil {
		return config.Merge(defaults, nil), nil
	}

	discovered, err := config.Discover(cwd)
	if err != nil {
		return config.Merge(defaults, nil), nil
	}

	if discovered == "" {
		return config.Merge(defaults, nil), nil
	}

	loaded, err := config.Load(discovered)
	if err != nil {
		return nil, err
	}

	return config.Merge(defaults, loaded), nil
}

const helpUsageText = `Usage: tidymark help <topic>

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
		fmt.Fprintf(os.Stderr, "tidymark: help: unknown topic %q\n", args[0])
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
	rules, err := tidymark.ListRules()
	if err != nil {
		fmt.Fprintf(os.Stderr, "tidymark: %v\n", err)
		return 2
	}

	for _, r := range rules {
		fmt.Printf("%-6s %-40s %s\n", r.ID, r.Name, r.Description)
	}
	return 0
}

func showRule(query string) int {
	content, err := tidymark.LookupRule(query)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tidymark: %v\n", err)
		return 2
	}
	fmt.Print(content)
	return 0
}
