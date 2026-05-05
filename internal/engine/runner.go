package engine

import (
	"fmt"
	"io/fs"
	"math"
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
	// SourceFS, when non-nil, is set as lint.File.FS for RunSource so
	// in-memory linting (e.g. an LSP buffer) sees the same filesystem
	// view processFile constructs for on-disk runs. Rules like
	// include/catalog short-circuit on a nil FS; without this hook
	// LSP diagnostics drift from the CLI's. Run() (path-based)
	// ignores this field and continues to derive FS from filepath.Dir
	// per file.
	SourceFS fs.FS
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
// all diagnostics (sorted by file, line, column, message) and any errors encountered.
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
		r.processFile(path, res)
	}

	res.Diagnostics = DedupeDiagnostics(res.Diagnostics)
	sortDiagnostics(res.Diagnostics)
	return res
}

// processFile reads, parses, and checks a single file, appending results to res.
func (r *Runner) processFile(path string, res *Result) {
	r.log().Printf("file: %s", path)

	source, err := lint.ReadFileLimited(path, r.MaxInputBytes)
	if err != nil {
		res.Errors = append(res.Errors, fmt.Errorf("reading %q: %w", path, err))
		return
	}

	f, err := lint.NewFileFromSource(path, source, r.StripFrontMatter)
	if err != nil {
		res.Errors = append(res.Errors, fmt.Errorf("parsing %q: %w", path, err))
		return
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
		return
	}

	f.GeneratedRanges = gensection.FindAllGeneratedRanges(f)

	effective := r.effectiveWithCategories(path, fmKinds)
	mdRules := r.markdownRules()
	r.logRules(mdRules, effective)

	diags, errs := CheckRules(f, mdRules, effective)
	if r.Explain {
		explain.Attach(diags, r.Config, path, fmKinds)
	}
	res.Diagnostics = append(res.Diagnostics, diags...)
	res.Errors = append(res.Errors, errs...)
}

// DedupeDiagnostics returns a new slice with duplicate (file, line,
// column, rule, message) tuples collapsed to a single entry. Repo-
// level rules (notably MDS048 git-hook-sync) emit a diagnostic
// anchored to the repository artifact for every linted file in the
// repo, so a fresh `mdsmith check` over a large tree would otherwise
// print the same warning N times and duplicate that entry in the
// returned diagnostics. Earlier-encountered duplicates win so the
// diagnostic order from the first hit is preserved. The input slice
// is never modified; nil input returns nil and a non-nil input
// always produces a freshly-allocated slice so callers can keep the
// original around without worrying about aliasing.
func DedupeDiagnostics(diags []lint.Diagnostic) []lint.Diagnostic {
	if len(diags) == 0 {
		return nil
	}
	if len(diags) == 1 {
		return append([]lint.Diagnostic(nil), diags[0])
	}
	type key struct {
		File    string
		Line    int
		Column  int
		RuleID  string
		Message string
	}
	seen := make(map[key]struct{}, len(diags))
	out := make([]lint.Diagnostic, 0, len(diags))
	for _, d := range diags {
		k := key{
			File:    d.File,
			Line:    d.Line,
			Column:  d.Column,
			RuleID:  d.RuleID,
			Message: d.Message,
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, d)
	}
	return out
}

// RunSource lints in-memory source bytes (e.g. from stdin or an LSP
// buffer) and returns a Result. It creates a File via
// NewFileFromSource, determines the effective config, and uses
// CheckRules (which includes clone+settings logic and line-offset
// adjustment).
//
// When Runner.SourceFS is non-nil, RunSource wires it onto the File
// as f.FS so include/catalog/cross-file rules see the same
// filesystem view processFile sets up for on-disk runs.
//
// f.GitignoreFunc is wired against a directory chosen in this order:
//
//  1. Runner.RootDir, when set (matches processFile's anchoring).
//  2. filepath.Dir(path), when path is absolute and RootDir is empty.
//
// With a relative path and no RootDir (the bare `<stdin>` case),
// GitignoreFunc stays nil — the matcher would have no meaningful
// root to walk anyway. FS-aware rules still see SourceFS regardless.
//
// When SourceFS is nil (the stdin case), FS stays nil and rules that
// require it short-circuit just as they did before.
func (r *Runner) RunSource(path string, source []byte) *Result {
	res := &Result{FilesChecked: 1}

	// Run config-target rules once before processing the in-memory source,
	// matching the behavior of Run() so config diagnostics surface via stdin.
	// This must happen before the size guard so an oversized buffer cannot
	// hide config-level errors that Run() would have surfaced regardless of
	// any individual file's size.
	r.runConfigTargetRules(res)

	// Mirror the on-disk size cap that lint.ReadFileLimited /
	// readStdinLimited apply to file and stdin reads. Without this
	// guard, in-memory callers (LSP, other integrations) would parse
	// arbitrarily large buffers and diverge from `mdsmith check`'s
	// "file too large" failure mode.
	if r.MaxInputBytes > 0 && r.MaxInputBytes != math.MaxInt64 &&
		int64(len(source)) > r.MaxInputBytes {
		// Match the on-disk error shape — processFile wraps
		// lint.ReadFileLimited's "file too large" via
		// `reading %q: %w`, so editor / log output stays
		// uniform whether the source came from stdin, an LSP
		// buffer, or a real file on disk.
		res.Errors = append(res.Errors,
			fmt.Errorf("reading %q: file too large (%d bytes, max %d)",
				path, len(source), r.MaxInputBytes))
		return res
	}

	r.log().Printf("file: %s", path)

	f, err := lint.NewFileFromSource(path, source, r.StripFrontMatter)
	if err != nil {
		res.Errors = append(res.Errors, fmt.Errorf("parsing %q: %w", path, err))
		return res
	}
	f.MaxInputBytes = r.MaxInputBytes
	if r.SourceFS != nil {
		f.FS = r.SourceFS
	}
	// Mirror processFile's gitignore wiring so on-disk Run() and
	// in-memory RunSource() agree on whether a path is ignored.
	// Anchor at RootDir when set, otherwise fall back to the
	// document directory (filepath.Dir(path)) when path is absolute.
	// In-memory callers with neither RootDir nor an absolute path
	// (the bare `<stdin>` case) leave GitignoreFunc nil — the
	// matcher would have no meaningful root to walk anyway.
	gitignoreDir := ""
	switch {
	case r.RootDir != "":
		f.SetRootDir(r.RootDir)
		gitignoreDir = r.RootDir
	case filepath.IsAbs(path):
		gitignoreDir = filepath.Dir(path)
	}
	if gitignoreDir != "" {
		gd := gitignoreDir
		f.GitignoreFunc = func() *lint.GitignoreMatcher {
			return r.cachedGitignore(gd)
		}
	}

	fmKinds, err := r.parseFrontMatterKinds(path, f.FrontMatter)
	if err != nil {
		res.Errors = append(res.Errors, err)
		return res
	}

	f.GeneratedRanges = gensection.FindAllGeneratedRanges(f)

	effective := r.effectiveWithCategories(path, fmKinds)

	mdRules := r.markdownRules()
	r.logRules(mdRules, effective)

	diags, errs := CheckRules(f, mdRules, effective)
	if r.Explain {
		explain.Attach(diags, r.Config, path, fmKinds)
	}
	res.Diagnostics = append(res.Diagnostics, diags...)
	res.Errors = append(res.Errors, errs...)

	sortDiagnostics(res.Diagnostics)
	return res
}

// markdownRules returns the subset of rules to run against individual Markdown
// files. When ConfigPath is set, config-target rules are excluded because they
// have already run once (via runConfigTargetRules) and their Check method
// returns nil for any non-config path anyway.
func (r *Runner) markdownRules() []rule.Rule {
	if r.ConfigPath == "" {
		return r.Rules
	}
	filtered := make([]rule.Rule, 0, len(r.Rules))
	for _, rl := range r.Rules {
		if ct, ok := rl.(rule.ConfigTarget); ok && ct.IsConfigFileRule() {
			continue
		}
		filtered = append(filtered, rl)
	}
	return filtered
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

// logRules logs each enabled rule in the effective config from the provided slice.
func (r *Runner) logRules(rules []rule.Rule, effective map[string]config.RuleCfg) {
	l := r.log()
	if !l.Enabled {
		return
	}
	for _, rl := range rules {
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
		configTarget, ok := rl.(rule.ConfigTarget)
		if !ok || !configTarget.IsConfigFileRule() {
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
// sort.SliceStable preserves the input order only for diagnostics that are
// equal on all compared fields, including Message.
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
