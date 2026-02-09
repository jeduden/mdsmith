package main

import (
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"sort"

	flag "github.com/spf13/pflag"

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
	_ "github.com/jeduden/tidymark/internal/rules/fencedcodelanguage"
	_ "github.com/jeduden/tidymark/internal/rules/fencedcodestyle"
	_ "github.com/jeduden/tidymark/internal/rules/firstlineheading"
	_ "github.com/jeduden/tidymark/internal/rules/generatedsection"
	_ "github.com/jeduden/tidymark/internal/rules/headingincrement"
	_ "github.com/jeduden/tidymark/internal/rules/headingstyle"
	_ "github.com/jeduden/tidymark/internal/rules/linelength"
	_ "github.com/jeduden/tidymark/internal/rules/listindent"
	_ "github.com/jeduden/tidymark/internal/rules/nobareurls"
	_ "github.com/jeduden/tidymark/internal/rules/noduplicateheadings"
	_ "github.com/jeduden/tidymark/internal/rules/noemphasisasheading"
	_ "github.com/jeduden/tidymark/internal/rules/nohardtabs"
	_ "github.com/jeduden/tidymark/internal/rules/nomultipleblanks"
	_ "github.com/jeduden/tidymark/internal/rules/notrailingpunctuation"
	_ "github.com/jeduden/tidymark/internal/rules/notrailingspaces"
	_ "github.com/jeduden/tidymark/internal/rules/singletrailingnewline"
)

func main() {
	os.Exit(run())
}

func run() int {
	var (
		showVersion bool
		configPath  string
		fix         bool
		format      string
		noColor     bool
		quiet       bool
	)

	flag.BoolVarP(&showVersion, "version", "v", false, "Print version and exit")
	flag.StringVarP(&configPath, "config", "c", "", "Override config file path")
	flag.BoolVar(&fix, "fix", false, "Auto-fix issues in place")
	flag.StringVarP(&format, "format", "f", "text", "Output format: text, json")
	flag.BoolVar(&noColor, "no-color", false, "Disable ANSI colors")
	flag.BoolVarP(&quiet, "quiet", "q", false, "Suppress non-error output")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: tidymark [flags] [files...]\n\n"+
			"Files can be paths, directories (walked recursively for *.md), or glob patterns.\n"+
			"With no arguments, reads from stdin if piped, otherwise exits 0.\n\n"+
			"Flags:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if showVersion {
		version := "(devel)"
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
			version = info.Main.Version
		}
		fmt.Printf("tidymark %s\n", version)
		return 0
	}

	args := flag.Args()

	// No args: check if stdin is a pipe.
	if len(args) == 0 {
		if !isStdinPipe() {
			return 0
		}
		return runStdin(fix, format, noColor, quiet, configPath)
	}

	// Resolve files from positional arguments.
	files, err := lint.ResolveFiles(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tidymark: %v\n", err)
		return 2
	}

	if len(files) == 0 {
		return 0
	}

	// Load configuration.
	cfg, err := loadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tidymark: %v\n", err)
		return 2
	}

	stripFM := frontMatterEnabled(cfg)

	if fix {
		fixer := &fixpkg.Fixer{
			Config:           cfg,
			Rules:            rule.All(),
			StripFrontMatter: stripFM,
		}

		fixResult := fixer.Fix(files)

		// Report errors.
		for _, e := range fixResult.Errors {
			fmt.Fprintf(os.Stderr, "tidymark: %v\n", e)
		}

		// Format remaining diagnostics.
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

	// Create runner with config and all registered rules.
	runner := &engine.Runner{
		Config:           cfg,
		Rules:            rule.All(),
		StripFrontMatter: stripFM,
	}

	result := runner.Run(files)

	// Report any runtime errors.
	for _, e := range result.Errors {
		fmt.Fprintf(os.Stderr, "tidymark: %v\n", e)
	}

	if len(result.Errors) > 0 && len(result.Diagnostics) == 0 {
		return 2
	}

	// Format and output diagnostics.
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

// runStdin reads from stdin, lints the content, and returns the appropriate
// exit code. It uses "<stdin>" as the file name in diagnostics.
func runStdin(fix bool, format string, noColor, quiet bool, configPath string) int {
	if fix {
		fmt.Fprintf(os.Stderr, "tidymark: cannot fix stdin in place\n")
		return 2
	}

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

	if frontMatterEnabled(cfg) {
		_, source = lint.StripFrontMatter(source)
	}

	f, err := lint.NewFile("<stdin>", source)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tidymark: parsing stdin: %v\n", err)
		return 2
	}

	effective := config.Effective(cfg, "<stdin>")

	var diags []lint.Diagnostic
	for _, rl := range rule.All() {
		rcfg, ok := effective[rl.Name()]
		if !ok || !rcfg.Enabled {
			continue
		}
		diags = append(diags, rl.Check(f)...)
	}

	sort.Slice(diags, func(i, j int) bool {
		di, dj := diags[i], diags[j]
		if di.Line != dj.Line {
			return di.Line < dj.Line
		}
		return di.Column < dj.Column
	})

	if !quiet && len(diags) > 0 {
		var formatter output.Formatter
		switch format {
		case "json":
			formatter = &output.JSONFormatter{}
		default:
			formatter = &output.TextFormatter{Color: !noColor}
		}

		if err := formatter.Format(os.Stderr, diags); err != nil {
			fmt.Fprintf(os.Stderr, "tidymark: error writing output: %v\n", err)
			return 2
		}
	}

	if len(diags) > 0 {
		return 1
	}

	return 0
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
