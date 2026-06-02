package main

import (
	"fmt"
	"os"
	"path/filepath"

	flag "github.com/spf13/pflag"

	"github.com/jeduden/mdsmith/internal/archetype/gensection"
	"github.com/jeduden/mdsmith/internal/bytelimit"
	"github.com/jeduden/mdsmith/internal/checker"
	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/export"
	"github.com/jeduden/mdsmith/internal/gitignore"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
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
	switch len(posArgs) {
	case 0:
		fmt.Fprintf(os.Stderr, "mdsmith: export requires a file argument\n")
		return 2
	case 1:
		return doExport(posArgs[0], flags)
	default:
		fmt.Fprintf(os.Stderr,
			"mdsmith: export takes a single file argument (got %d)\n", len(posArgs))
		return 2
	}
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
		return flags, nil, reportFlagParseErr(err, os.Stderr, "mdsmith: export")
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

	data, err := bytelimit.ReadFileLimited(path, maxBytes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}

	f, rules, err := prepareExportFile(path, data, cfg, cfgPath, maxBytes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}

	out, diags := export.Export(f, exportMode(flags), rules)
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

// prepareExportFile mirrors fix.Fixer.prepareFile + fixableRules: it
// parses the file, wires FS / RootDir / MaxInputBytes / GitignoreFunc,
// resolves the effective rule config from .mdsmith.yml kinds and
// overrides (and any front-matter `kinds:`), then returns the set of
// directive rules to consult for staleness/regeneration. Each
// returned rule is a clone with its per-file settings applied via
// engine.ConfigureRule, and disabled rules are excluded — matching
// `mdsmith check`/`fix` so a directive turned off in `.mdsmith.yml`
// neither flags a stale body nor gets regenerated on `--fix`.
//
// lint.NewFileFromSource never errors with the current goldmark
// configuration (same invariant fix.Fixer.buildPostFixFile relies
// on), so the parse is unchecked.
func prepareExportFile(
	path string, source []byte,
	cfg *config.Config, cfgPath string, maxBytes int64,
) (*lint.File, []rule.Rule, error) {
	f, _ := lint.NewFileFromSource(path, source, frontMatterEnabled(cfg)) // never errors today
	f.MaxInputBytes = maxBytes
	dir := filepath.Dir(path)
	f.FS = os.DirFS(dir)
	gitignoreDir := dir
	root := rootDirFromConfig(cfgPath)
	if root != "" {
		f.SetRootDir(root)
		gitignoreDir = root
	}
	f.GitignoreFunc = func() *gitignore.Matcher {
		return gitignore.NewMatcher(gitignoreDir)
	}
	// Match engine.Runner.processFile so staleness diagnostics inside
	// an outer include/catalog body are suppressed: the host file is
	// not responsible for those bytes.
	f.GeneratedRanges = gensection.FindAllGeneratedRanges(f)

	all := rule.All()
	effective, err := effectiveExportConfig(cfg, path, f.FrontMatter, all)
	if err != nil {
		return nil, nil, err
	}
	rules, err := configuredEnabledRules(all, effective)
	if err != nil {
		return nil, nil, err
	}
	return f, rules, nil
}

// effectiveExportConfig parses front-matter kinds/fields (with the
// same leniency `fix.Fixer` uses) and resolves the file's effective
// rule config including category overrides.
func effectiveExportConfig(
	cfg *config.Config, path string, fm []byte, all []rule.Rule,
) (map[string]config.RuleCfg, error) {
	fmKinds, err := lint.ParseFrontMatterKinds(fm)
	if err != nil {
		return nil, fmt.Errorf("parsing front-matter kinds in %s: %w", path, err)
	}
	if err := config.ValidateFrontMatterKinds(cfg, path, fmKinds); err != nil {
		return nil, err
	}
	fmFields, err := exportFrontMatterFields(cfg, path, fm)
	if err != nil {
		return nil, err
	}
	effective, categories, explicit := config.EffectiveAll(cfg, path, fmKinds, fmFields)
	categoryFor := make(map[string]string, len(all))
	for _, rl := range all {
		categoryFor[rl.Name()] = rl.Category()
	}
	return config.ApplyCategories(
		effective, categories,
		func(name string) string { return categoryFor[name] },
		explicit,
	), nil
}

// exportFrontMatterFields decodes the full front-matter mapping only
// when a kind-assignment entry's `fields-present:` selector could
// match this file path, mirroring fix.parseFieldsForSelector.
func exportFrontMatterFields(
	cfg *config.Config, path string, fm []byte,
) (map[string]any, error) {
	if !config.NeedsFieldsForFile(cfg, path) {
		return nil, nil
	}
	fields, err := lint.ParseFrontMatterFields(fm)
	if err != nil {
		return nil, fmt.Errorf("parsing front matter in %s: %w", path, err)
	}
	return fields, nil
}

// configuredEnabledRules clones+configures every enabled rule via
// checker.ConfigureRule, the same path fix.Fixer.fixableRules uses.
// A settings-apply error short-circuits the export with a clear
// message rather than silently dropping the rule.
func configuredEnabledRules(
	all []rule.Rule, effective map[string]config.RuleCfg,
) ([]rule.Rule, error) {
	var out []rule.Rule
	for _, rl := range all {
		cfg, ok := effective[rl.Name()]
		if !ok {
			continue
		}
		if !cfg.Enabled {
			continue
		}
		configured, err := checker.ConfigureRule(rl, cfg)
		if err != nil {
			return nil, err
		}
		out = append(out, configured)
	}
	return out, nil
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
// path is empty. A stdout write failure is treated as fatal because
// the caller has no other channel to surface it; an os.Stdout.Write
// failure is theoretical and not exercised by tests.
func writeExportOutput(path string, data []byte) error {
	if path == "" {
		_, err := os.Stdout.Write(data)
		return err
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}
