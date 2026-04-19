package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/jeduden/mdsmith/internal/config"
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

		effective := r.effectiveWithCategories(path)

		r.logRules(effective)

		diags, errs := CheckRules(f, r.Rules, effective)
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

	effective := r.effectiveWithCategories(path)

	r.logRules(effective)

	diags, errs := CheckRules(f, r.Rules, effective)
	res.Diagnostics = append(res.Diagnostics, diags...)
	res.Errors = append(res.Errors, errs...)

	sortDiagnostics(res.Diagnostics)
	return res
}

// effectiveWithCategories computes the effective rule config for a file
// path, applying category-based enable/disable on top of per-rule settings.
func (r *Runner) effectiveWithCategories(path string) map[string]config.RuleCfg {
	effective := config.Effective(r.Config, path)
	categories := config.EffectiveCategories(r.Config, path)
	explicit := config.EffectiveExplicitRules(r.Config, path)

	// Build rule-name-to-category lookup from the runner's rule list.
	catLookup := ruleCategoryLookup(r.Rules)

	return config.ApplyCategories(effective, categories, catLookup, explicit)
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

// sortDiagnostics sorts diagnostics by file, line, column.
func sortDiagnostics(diags []lint.Diagnostic) {
	sort.Slice(diags, func(i, j int) bool {
		di, dj := diags[i], diags[j]
		if di.File != dj.File {
			return di.File < dj.File
		}
		if di.Line != dj.Line {
			return di.Line < dj.Line
		}
		return di.Column < dj.Column
	})
}
