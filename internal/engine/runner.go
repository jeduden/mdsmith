package engine

import (
	"bytes"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"

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
	// SkipSourceContext, when true, suppresses per-diagnostic
	// SourceLines population. Set it for callers that never render the
	// source window (the check benchmark/gate, machine output that
	// omits it) to avoid its large per-diagnostic allocation. Default
	// false preserves the CLI text formatter's context display.
	SkipSourceContext bool
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
	// Concurrency controls how many files Run lints in parallel.
	// Zero or negative means "use runtime.GOMAXPROCS"; 1 forces the
	// sequential path; n>1 uses n workers. The worker count is
	// clamped to the file count. Output is merged in input order and
	// then sorted, so the value never changes observable results —
	// it only trades CPU for wall time. RunSource (single in-memory
	// file) ignores this field.
	Concurrency int
	// gitignoreCache caches GitignoreMatchers by directory to avoid
	// re-walking the filesystem for each file. gitignoreMu guards it
	// because Run lints files on multiple goroutines and the
	// GitignoreFunc closure each file carries reaches back into the
	// shared cache lazily during rule execution.
	gitignoreCache map[string]*lint.GitignoreMatcher
	gitignoreMu    sync.Mutex
}

// fileOutcome is one file's contribution to a run. Workers fill a
// pre-sized slice of these by index, so the merge is order-stable and
// needs no lock on the shared Result. log holds the file's verbose
// lines (empty unless the logger is enabled); Run flushes them in
// input order so -v output is deterministic regardless of scheduling.
type fileOutcome struct {
	diags []lint.Diagnostic
	errs  []error
	log   []byte
}

// Result holds the output of a lint run.
type Result struct {
	// FilesChecked is the number of files processed (after ignore filtering).
	FilesChecked int
	Diagnostics  []lint.Diagnostic
	Errors       []error
}

// Run lints the files at the given paths and returns a Result containing
// all diagnostics (sorted by file, line, column, message) and any errors
// encountered. Files are linted concurrently (see Runner.Concurrency);
// per-file results are merged in input order before dedupe and sort, so
// the output is identical to a sequential run regardless of scheduling.
func (r *Runner) Run(paths []string) *Result {
	res := &Result{}

	// Run config-target rules once against the config file before per-file
	// markdown processing. These rules (e.g. recipe-safety / MDS040) validate
	// the project config rather than individual Markdown files.
	r.runConfigTargetRules(res)

	work := r.filterIgnored(paths)
	res.FilesChecked = len(work)

	sink := r.log()
	for _, o := range r.runFiles(work) {
		if len(o.log) > 0 && sink.W != nil {
			_, _ = sink.W.Write(o.log)
		}
		res.Diagnostics = append(res.Diagnostics, o.diags...)
		res.Errors = append(res.Errors, o.errs...)
	}

	// DedupeDiagnostics is only needed when a repo-scoped rule is
	// enabled. Repo-scoped rules (e.g. git-hook-sync / MDS048) anchor
	// their diagnostic to a repository artifact rather than the linted
	// file, so the same tuple can recur across files. When no such rule
	// is enabled, every diagnostic tuple is anchored to its linted file
	// and cannot collide, so the map+slice allocation is pure waste.
	// RunSource (single in-memory file) is exempt: duplicates cannot
	// arise from a single-file lint.
	if r.anyRepoScopedEnabled() {
		res.Diagnostics = DedupeDiagnostics(res.Diagnostics)
	}
	sortDiagnostics(res.Diagnostics)
	return res
}

// filterIgnored drops paths matched by the config ignore list, keeping
// input order.
func (r *Runner) filterIgnored(paths []string) []string {
	work := make([]string, 0, len(paths))
	for _, path := range paths {
		if config.IsIgnored(r.Config.Ignore, path) {
			continue
		}
		work = append(work, path)
	}
	return work
}

// ResolveWorkers maps the Runner.Concurrency knob to an actual worker
// count for a run over n files. concurrency <= 0 means "use
// runtime.GOMAXPROCS"; a positive value is taken literally. The result
// is clamped to n (never more workers than files) and is 0 when there
// is nothing to do.
func ResolveWorkers(concurrency, n int) int {
	if n <= 0 {
		return 0
	}
	w := concurrency
	if w <= 0 {
		w = runtime.GOMAXPROCS(0)
	}
	if w > n {
		w = n
	}
	return w
}

// runFiles lints work into a pre-sized, index-addressed slice. With one
// worker it stays on the calling goroutine. With more, each worker
// clones the rule set once (so rules carrying per-Check state — include's
// visited/chain, the directive engines — never touch another
// goroutine's instance) and pulls file indices off an atomic counter
// for even load balancing. No worker writes a slot another reads, so
// the result needs no lock.
func (r *Runner) runFiles(work []string) []fileOutcome {
	outcomes := make([]fileOutcome, len(work))
	workers := ResolveWorkers(r.Concurrency, len(work))
	if workers <= 1 {
		for i, path := range work {
			outcomes[i] = r.lintFile(path, r.Rules)
		}
		return outcomes
	}
	var next atomic.Int64
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rules := cloneRules(r.Rules)
			for {
				i := int(next.Add(1)) - 1
				if i >= len(work) {
					return
				}
				outcomes[i] = r.lintFile(work[i], rules)
			}
		}()
	}
	wg.Wait()
	return outcomes
}

// cloneRules returns an independent copy of rules for one worker so
// concurrent Check calls never share a rule instance's mutable state.
func cloneRules(rules []rule.Rule) []rule.Rule {
	out := make([]rule.Rule, len(rules))
	for i, rl := range rules {
		out[i] = rule.CloneInstance(rl)
	}
	return out
}

// lintFile reads, parses, and checks a single file with the given rule
// set (the worker's private clones) and returns its diagnostics and
// errors. It touches no shared Runner state except the mutex-guarded
// gitignore cache.
func (r *Runner) lintFile(path string, rules []rule.Rule) (out fileOutcome) {
	// When verbose, log into a per-file buffer instead of the shared
	// logger; Run flushes these in input order so concurrent workers
	// don't interleave -v output. The named return + defer attaches
	// the buffer no matter which early return fires.
	flog := r.log()
	if flog.Enabled {
		var buf bytes.Buffer
		flog = &vlog.Logger{Enabled: true, W: &buf}
		defer func() { out.log = bytes.Clone(buf.Bytes()) }()
	}
	flog.Printf("file: %s", path)

	source, err := lint.ReadFileLimited(path, r.MaxInputBytes)
	if err != nil {
		return fileOutcome{errs: []error{fmt.Errorf("reading %q: %w", path, err)}}
	}

	f, err := lint.NewFileFromSource(path, source, r.StripFrontMatter)
	if err != nil {
		return fileOutcome{errs: []error{fmt.Errorf("parsing %q: %w", path, err)}}
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

	fmKinds, fmFields, err := r.parseFrontMatter(path, f.FrontMatter)
	if err != nil {
		return fileOutcome{errs: []error{err}}
	}

	f.GeneratedRanges = gensection.FindAllGeneratedRanges(f)

	effective := r.effectiveWithCategories(path, fmKinds, fmFields)
	mdRules := markdownRulesFrom(rules, r.ConfigPath)
	logRulesTo(flog, mdRules, effective)

	diags, errs := checkRules(f, mdRules, effective, r.SkipSourceContext)
	if r.Explain {
		explain.Attach(diags, r.Config, path, fmKinds, fmFields)
	}
	return fileOutcome{diags: diags, errs: errs}
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

	fmKinds, fmFields, err := r.parseFrontMatter(path, f.FrontMatter)
	if err != nil {
		res.Errors = append(res.Errors, err)
		return res
	}

	f.GeneratedRanges = gensection.FindAllGeneratedRanges(f)

	effective := r.effectiveWithCategories(path, fmKinds, fmFields)

	mdRules := r.markdownRules()
	r.logRules(mdRules, effective)

	diags, errs := checkRules(f, mdRules, effective, r.SkipSourceContext)
	if r.Explain {
		explain.Attach(diags, r.Config, path, fmKinds, fmFields)
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
	return markdownRulesFrom(r.Rules, r.ConfigPath)
}

// markdownRulesFrom filters config-target rules out of rules when a
// config path is set (they ran once via runConfigTargetRules and their
// Check returns nil for non-config paths anyway). It operates on the
// passed slice so each worker filters its own clones.
func markdownRulesFrom(rules []rule.Rule, configPath string) []rule.Rule {
	if configPath == "" {
		return rules
	}
	filtered := make([]rule.Rule, 0, len(rules))
	for _, rl := range rules {
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

// parseFrontMatterFields parses a file's front-matter block into a
// top-level map. It feeds the kind-assignment `fields-present:` selector
// and returns (nil, nil) when no entry could match this file — skipping
// the full YAML decode for files no fields-present entry would ever
// claim. Files outside every fields-present glob keep the kinds-only
// parse path (and its narrower error surface).
func (r *Runner) parseFrontMatterFields(path string, fm []byte) (map[string]any, error) {
	if !config.NeedsFieldsForFile(r.Config, path) {
		return nil, nil
	}
	fields, err := lint.ParseFrontMatterFields(fm)
	if err != nil {
		return nil, fmt.Errorf("parsing front matter in %q: %w", path, err)
	}
	return fields, nil
}

// parseFrontMatter is the shared kinds+fields parse used by both Run and
// RunSource; pulling it out keeps each entry point under the funlen cap.
func (r *Runner) parseFrontMatter(path string, fm []byte) ([]string, map[string]any, error) {
	kinds, err := r.parseFrontMatterKinds(path, fm)
	if err != nil {
		return nil, nil, err
	}
	fields, err := r.parseFrontMatterFields(path, fm)
	if err != nil {
		return nil, nil, err
	}
	return kinds, fields, nil
}

// effectiveWithCategories computes the effective rule config for a file
// path, applying category-based enable/disable on top of per-rule settings.
func (r *Runner) effectiveWithCategories(
	path string, fmKinds []string, fmFields map[string]any,
) map[string]config.RuleCfg {
	effective, categories, explicit := config.EffectiveAll(r.Config, path, fmKinds, fmFields)
	return config.ApplyCategories(effective, categories, ruleCategoryLookup(r.Rules), explicit)
}

// cachedGitignore returns a GitignoreMatcher for the given directory,
// creating and caching it on first use to avoid re-walking the filesystem.
// The cache key is normalized to an absolute path so equivalent paths
// (e.g. "sub" vs "./sub") share the same entry.
func (r *Runner) cachedGitignore(dir string) *lint.GitignoreMatcher {
	r.gitignoreMu.Lock()
	defer r.gitignoreMu.Unlock()
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
	logRulesTo(r.log(), rules, effective)
}

// logRulesTo logs each enabled rule to l. Split from logRules so the
// per-file buffered logger in lintFile can reuse the same formatting
// without going through the shared Runner logger.
func logRulesTo(l *vlog.Logger, rules []rule.Rule, effective map[string]config.RuleCfg) {
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
	effective := r.effectiveWithCategories(r.ConfigPath, nil, nil)
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

// anyRepoScopedEnabled reports whether any markdown rule (excluding
// ConfigTarget rules) implements rule.RepoScoped and is enabled in the
// global effective configuration. Run uses this once to decide whether
// DedupeDiagnostics is needed: when no enabled rule is repo-scoped,
// every diagnostic tuple is anchored to its linted file and cross-file
// duplicates cannot occur, so the map+slice allocation is skipped.
//
// The effective config is queried with an empty path and nil front-matter
// so kind-specific overrides do not influence the result. A repo-scoped
// rule enabled only for a specific kind is conservatively treated as
// potentially enabled (it is still surfaced by its global config entry).
//
// RunSource is a single in-memory file and is exempt from this check:
// a single-file lint cannot produce cross-file duplicates.
func (r *Runner) anyRepoScopedEnabled() bool {
	mdRules := markdownRulesFrom(r.Rules, r.ConfigPath)
	effective := r.effectiveWithCategories("", nil, nil)
	for _, rl := range mdRules {
		rs, ok := rl.(rule.RepoScoped)
		if !ok || !rs.RepoScopedDiagnostics() {
			continue
		}
		cfg, ok := effective[rl.Name()]
		if ok && cfg.Enabled {
			return true
		}
	}
	return false
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
