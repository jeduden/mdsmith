package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/jeduden/mdsmith/internal/archetype/gensection"
	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/explain"
	"github.com/jeduden/mdsmith/internal/lint"
	vlog "github.com/jeduden/mdsmith/internal/log"
	"github.com/jeduden/mdsmith/internal/rule"
)

// Runner drives the linting pipeline: for each file it reads the content,
// builds a File (parsing the AST once), determines the effective rule
// configuration, runs enabled rules, and collects diagnostics.
type Runner struct {
	Config           *config.Config
	Rules            []rule.Rule
	StripFrontMatter bool
	Logger           *vlog.Logger
	// RootDir is the project root directory (parent of .mdsmith.yml).
	// Used by rules that need to read files relative to the project root.
	RootDir string
	// MaxInputBytes is the maximum file size in bytes before a file is
	// skipped with an error. Zero or negative means unlimited.
	MaxInputBytes int64
	// Explain, when true, attaches per-leaf rule provenance to each
	// diagnostic so output formatters can render an explanation trailer.
	Explain bool
	// ConfigPath is the path to the loaded .mdsmith.yml. When set,
	// config-target rules (rule.ConfigTarget) are run once against a
	// synthetic lint.File for this path before per-file processing.
	ConfigPath string
	// gitignoreCache caches GitignoreMatchers by directory to avoid
	// re-walking the filesystem for each file.
	gitignoreCache map[string]*lint.GitignoreMatcher
}

// Result holds the output of a lint run.
type Result struct {
	// FilesChecked is the number of files processed (after ignore filtering).
	FilesChecked int
	Diagnostics  []lint.Diagnostic
	Errors       []error
}

// Run lints the files at the given paths and returns a Result containing
// all diagnostics (sorted by file, line, column) and any errors encountered.
func (r *Runner) Run(paths []string) *Result {
	res := &Result{}

	// Run config-target rules once against the config file before per-file
	// markdown processing. These rules (e.g. recipe-safety / MDS040) validate
	// the project config rather than individual Markdown files.
	r.runConfigTargetRules(res)

	for _, path := range paths {
		if config.IsIgnored(r.Config.Ignore, path) {
			continue
		}
		res.FilesChecked++

		r.log().Printf("file: %s", path)

		source, err := lint.ReadFileLimited(path, r.MaxInputBytes)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Errorf("reading %q: %w", path, err))
			continue
		}

		f, err := lint.NewFileFromSource(path, source, r.StripFrontMatter)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Errorf("parsing %q: %w", path, err))
			continue
		}
		f.MaxInputBytes = r.MaxInputBytes
		dir := filepath.Dir(path)
		f.FS = os.DirFS(dir)
		gitignoreDir := dir
		if r.RootDir != "" {
			f.SetRootDir(r.RootDir)
			gitignoreDir = r.RootDir
		}
		gd := gitignoreDir // capture for closure
		f.GitignoreFunc = func() *lint.GitignoreMatcher {
			return r.cachedGitignore(gd)
		}

		fmKinds, err := r.parseFrontMatterKinds(path, f.FrontMatter)
		if err != nil {
			res.Errors = append(res.Errors, err)
			continue
		}

		f.GeneratedRanges = gensection.FindAllGeneratedRanges(f)

		effective := r.effectiveWithCategories(path, fmKinds)

		r.logRules(effective)

		diags, errs := CheckRules(f, r.Rules, effective)
		if r.Explain {
			explain.Attach(diags, r.Config, path, fmKinds)
		}
		res.Diagnostics = append(res.Diagnostics, diags...)
		res.Errors = append(res.Errors, errs...)
	}

	sortDiagnostics(res.Diagnostics)
	return res
}

// RunSource lints in-memory source bytes (e.g. from stdin) and returns a
// Result. It creates a File via NewFileFromSource, determines the
// effective config, and uses CheckRules (which includes clone+settings
// logic and line-offset adjustment).
//
// The File's FS field is left nil because in-memory source has no
// meaningful filesystem context. Rules that access f.FS must handle nil
// (include short-circuits when FS is nil). RootFS is set when RootDir
// is configured for potential future use, but currently has no effect
// on stdin since the include rule requires FS to be non-nil.
func (r *Runner) RunSource(path string, source []byte) *Result {
	res := &Result{FilesChecked: 1}

	r.log().Printf("file: %s", path)

	f, err := lint.NewFileFromSource(path, source, r.StripFrontMatter)
	if err != nil {
		res.Errors = append(res.Errors, fmt.Errorf("parsing %q: %w", path, err))
		return res
	}
	f.MaxInputBytes = r.MaxInputBytes
	if r.RootDir != "" {
		f.SetRootDir(r.RootDir)
	}

	fmKinds, err := r.parseFrontMatterKinds(path, f.FrontMatter)
	if err != nil {
		res.Errors = append(res.Errors, err)
		return res
	}

	f.GeneratedRanges = gensection.FindAllGeneratedRanges(f)

	effective := r.effectiveWithCategories(path, fmKinds)

	r.logRules(effective)

	diags, errs := CheckRules(f, r.Rules, effective)
	if r.Explain {
		explain.Attach(diags, r.Config, path, fmKinds)
	}
	res.Diagnostics = append(res.Diagnostics, diags...)
	res.Errors = append(res.Errors, errs...)

	sortDiagnostics(res.Diagnostics)
	return res
}

// parseFrontMatterKinds parses and validates the kinds list from a file's
// front-matter block, returning a combined error on parse or validation failure.
func (r *Runner) parseFrontMatterKinds(path string, fm []byte) ([]string, error) {
	kinds, err := lint.ParseFrontMatterKinds(fm)
	if err != nil {
		return nil, fmt.Errorf("parsing front-matter kinds in %q: %w", path, err)
	}
	if err := config.ValidateFrontMatterKinds(r.Config, path, kinds); err != nil {
		return nil, err
	}
	return kinds, nil
}

// effectiveWithCategories computes the effective rule config for a file
// path, applying category-based enable/disable on top of per-rule settings.
func (r *Runner) effectiveWithCategories(path string, fmKinds []string) map[string]config.RuleCfg {
	effective, categories, explicit := config.EffectiveAll(r.Config, path, fmKinds)
	return config.ApplyCategories(effective, categories, ruleCategoryLookup(r.Rules), explicit)
}

// cachedGitignore returns a GitignoreMatcher for the given directory,
// creating and caching it on first use to avoid re-walking the filesystem.
// The cache key is normalized to an absolute path so equivalent paths
// (e.g. "sub" vs "./sub") share the same entry.
func (r *Runner) cachedGitignore(dir string) *lint.GitignoreMatcher {
	if r.gitignoreCache == nil {
		r.gitignoreCache = make(map[string]*lint.GitignoreMatcher)
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		absDir = filepath.Clean(dir)
	}
	if m, ok := r.gitignoreCache[absDir]; ok {
		return m
	}
	m := lint.NewGitignoreMatcher(absDir)
	r.gitignoreCache[absDir] = m
	return m
}

// log returns the runner's logger. If no logger is set, it returns a
// disabled logger so callers don't need nil checks.
func (r *Runner) log() *vlog.Logger {
	if r.Logger != nil {
		return r.Logger
	}
	return &vlog.Logger{}
}

// logRules logs each enabled rule in the effective config.
func (r *Runner) logRules(effective map[string]config.RuleCfg) {
	l := r.log()
	if !l.Enabled {
		return
	}
	for _, rl := range r.Rules {
		cfg, ok := effective[rl.Name()]
		if !ok || !cfg.Enabled {
			continue
		}
		l.Printf("rule: %s %s", rl.ID(), rl.Name())
	}
}

// ruleCategoryLookup returns a function that maps a rule name to its category.
func ruleCategoryLookup(rules []rule.Rule) func(string) string {
	m := make(map[string]string, len(rules))
	for _, rl := range rules {
		m[rl.Name()] = rl.Category()
	}
	return func(name string) string {
		return m[name]
	}
}

// runConfigTargetRules runs rules that implement rule.ConfigTarget once
// against a synthetic lint.File for the config file. These rules validate
// the project config rather than individual Markdown files. They are skipped
// in the normal per-file loop because their Check method returns nil for
// non-config file paths.
func (r *Runner) runConfigTargetRules(res *Result) {
	if r.ConfigPath == "" {
		return
	}
	effective := r.effectiveWithCategories(r.ConfigPath, nil)
	f, err := lint.NewFile(r.ConfigPath, []byte{})
	if err != nil {
		res.Errors = append(res.Errors, fmt.Errorf("creating config lint.File: %w", err))
		return
	}
	for _, rl := range r.Rules {
		if _, ok := rl.(rule.ConfigTarget); !ok {
			continue
		}
		cfg, ok := effective[rl.Name()]
		if !ok || !cfg.Enabled {
			continue
		}
		configured, err := ConfigureRule(rl, cfg)
		if err != nil {
			res.Errors = append(res.Errors, err)
			continue
		}
		diags := configured.Check(f)
		res.Diagnostics = append(res.Diagnostics, diags...)
	}
}

// sortDiagnostics sorts diagnostics by file, line, column, then message.
// sort.SliceStable preserves the pre-sorted order of same-position diagnostics
// from rules that already emit in deterministic order (e.g. MDS040).
func sortDiagnostics(diags []lint.Diagnostic) {
	sort.SliceStable(diags, func(i, j int) bool {
		di, dj := diags[i], diags[j]
		if di.File != dj.File {
			return di.File < dj.File
		}
		if di.Line != dj.Line {
			return di.Line < dj.Line
		}
		if di.Column != dj.Column {
			return di.Column < dj.Column
		}
		return di.Message < dj.Message
	})
}
