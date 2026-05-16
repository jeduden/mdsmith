package main

import (
	"fmt"
	"os"
	"path/filepath"

	flag "github.com/spf13/pflag"

	"github.com/jeduden/mdsmith/internal/archetype/gensection"
	"github.com/jeduden/mdsmith/internal/export"
	"github.com/jeduden/mdsmith/internal/lint"
)

type exportFlags struct {
	configPath, output, maxInputSize string
	fixStale, noCheck                bool
}

// runExport implements the "export" subcommand: write a portable,
// directive-free copy of a Markdown file. Markers are stripped,
// generated bodies are kept, and `<?include?>` content is inlined.
//
// The source file is never modified. By default, a stale directive
// body refuses the export with an error. `--fix` regenerates stale
// bodies in memory before stripping; `--no-check` skips the check
// and emits on-disk bytes verbatim; passing both is a usage error.
func runExport(args []string) int {
	flags, posArgs, code := parseExportFlags(args)
	if code >= 0 {
		return code
	}
	if flags.fixStale && flags.noCheck {
		fmt.Fprintf(os.Stderr,
			"mdsmith: export: --fix and --no-check are mutually exclusive\n")
		return 2
	}
	if len(posArgs) == 0 {
		fmt.Fprintf(os.Stderr, "mdsmith: export requires a file argument\n")
		return 2
	}
	if len(posArgs) > 1 {
		fmt.Fprintf(os.Stderr,
			"mdsmith: export takes a single file argument (got %d)\n", len(posArgs))
		return 2
	}
	return doExport(posArgs[0], flags)
}

// parseExportFlags binds the flagset and parses args. Returns
// (flags, positional, code) — when code is non-negative the caller
// should return it directly.
func parseExportFlags(args []string) (exportFlags, []string, int) {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	var flags exportFlags
	fs.StringVarP(&flags.configPath, "config", "c", "", "Override config file path")
	fs.StringVarP(&flags.output, "output", "o", "",
		"Write output to <path> instead of stdout")
	fs.StringVar(&flags.maxInputSize, "max-input-size", "",
		"Maximum file size to process (e.g. 2MB, 500KB, 0=unlimited)")
	fs.BoolVar(&flags.fixStale, "fix", false,
		"Regenerate stale directive bodies in memory before stripping")
	fs.BoolVar(&flags.noCheck, "no-check", false,
		"Skip the staleness check; export on-disk bytes as-is")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr,
			"Usage: mdsmith export [flags] <file>\n\n"+
				"Write a portable, directive-free copy of a Markdown file.\n"+
				"Generated section markers are removed; bodies are kept "+
				"and `<?include?>` content is inlined.\n\n"+
				"The source file is never modified.\n\nFlags:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if code := reportFlagParseErr(err, os.Stderr, "mdsmith: export"); code >= 0 {
			return flags, nil, code
		}
	}
	return flags, fs.Args(), -1
}

// doExport loads the file, hydrates a *lint.File, runs export.Export,
// and writes the result.
func doExport(path string, flags exportFlags) int {
	cfg, cfgPath, err := loadConfig(flags.configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}
	maxBytes, err := resolveMaxInputBytes(cfg, flags.maxInputSize)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}

	data, err := lint.ReadFileLimited(path, maxBytes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}
	f, err := lint.NewFileFromSource(path, data, frontMatterEnabled(cfg))
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: parsing %s: %v\n", path, err)
		return 2
	}
	f.MaxInputBytes = maxBytes
	f.FS = os.DirFS(filepath.Dir(path))
	if root := rootDirFromConfig(cfgPath); root != "" {
		f.SetRootDir(root)
	}
	// Match the engine.Runner setup so staleness diagnostics
	// inside an outer include/catalog body are suppressed: the
	// host file is not responsible for those bytes.
	f.GeneratedRanges = gensection.FindAllGeneratedRanges(f)

	out, diags := export.Export(f, exportMode(flags))
	if len(diags) > 0 {
		if code := formatDiagnostics(diags, "text", false); code != 0 {
			return code
		}
		return 1
	}
	if err := writeExportOutput(flags.output, out); err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}
	return 0
}

// exportMode maps the parsed CLI flags onto the staleness mode the
// export package understands.
func exportMode(flags exportFlags) export.Mode {
	switch {
	case flags.fixStale:
		return export.Fix
	case flags.noCheck:
		return export.NoCheck
	default:
		return export.Check
	}
}

// writeExportOutput writes data to a file at path, or to stdout when
// path is empty.
func writeExportOutput(path string, data []byte) error {
	if path == "" {
		if _, err := os.Stdout.Write(data); err != nil {
			return fmt.Errorf("writing output: %w", err)
		}
		return nil
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}
