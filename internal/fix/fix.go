package fix

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/jeduden/mdsmith/internal/archetype/gensection"
	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/engine"
	"github.com/jeduden/mdsmith/internal/explain"
	"github.com/jeduden/mdsmith/internal/lint"
	vlog "github.com/jeduden/mdsmith/internal/log"
	"github.com/jeduden/mdsmith/internal/rule"
)

// Fixer applies auto-fixes for fixable rules and reports remaining diagnostics.
type Fixer struct {
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
	// remaining diagnostic so output formatters can render an
	// explanation trailer.
	Explain bool
	// SourceFS, when non-nil, overrides the per-file dirFS that
	// prepareFile would otherwise derive from filepath.Dir(path).
	// Used by Source / SourceWithRules so callers can pass a
	// workspace-relative path for config matching while still giving
	// include/catalog/cross-file rules a real filesystem rooted at
	// the document's actual directory. Disk-based Fix() (path-based)
	// leaves this nil and continues to derive dirFS from each file's
	// absolute path.
	SourceFS fs.FS

	// DryRun, when true, skips the write step. The Result will have
	// WouldFix and WouldFixRules populated; Modified will be empty.
	DryRun bool

	// gitignoreCache caches GitignoreMatchers by directory so the
	// matcher tree is walked once per directory across a fix run,
	// matching the engine.Runner cache contract that catalog and
	// other gitignore-aware rules expect.
	gitignoreCache map[string]*lint.GitignoreMatcher
}

// cachedGitignore returns a GitignoreMatcher for the given directory,
// creating and caching it on first use so the matcher tree is walked
// once per (Fixer, dir). Mirrors engine.Runner so the fix path's
// lint.File values give catalog (and any other rule that calls
// f.GetGitignore()) the same matcher the check path would.
//
// The cache key is filepath.Clean(dir). Clean is total (no error
// path) and idempotent, and it collapses equivalent forms like
// "./sub" and "sub" / "sub/" so callers passing the same logical
// directory in slightly different syntactic forms share one cache
// entry. lint.NewGitignoreMatcher canonicalizes its argument
// internally (filepath.Abs) before walking, so the matcher itself is
// correctly rooted even when the cleaned key is still relative.
func (f *Fixer) cachedGitignore(dir string) *lint.GitignoreMatcher {
	if f.gitignoreCache == nil {
		f.gitignoreCache = make(map[string]*lint.GitignoreMatcher)
	}
	key := filepath.Clean(dir)
	if m, ok := f.gitignoreCache[key]; ok {
		return m
	}
	m := lint.NewGitignoreMatcher(key)
	f.gitignoreCache[key] = m
	return m
}

// DryRunFileResult holds per-file dry-run information.
type DryRunFileResult struct {
	// Path is the file path.
	Path string
	// WouldFix is the total count of violations that would be fixed.
	WouldFix int
	// RuleCounts maps rule ID to the number of violations that would be fixed.
	RuleCounts map[string]int
	// Diagnostics contains remaining (non-fixable) diagnostics for this file.
	Diagnostics []lint.Diagnostic
}

// Result holds the outcome of a fix run.
type Result struct {
	// FilesChecked is the number of files processed (after ignore filtering).
	FilesChecked int
	// Failures is the number of diagnostics found before attempting fixes.
	Failures int
	// Diagnostics contains remaining diagnostics after fixing (from non-fixable
	// rules and any violations that could not be auto-fixed).
	Diagnostics []lint.Diagnostic
	// Modified lists file paths that were written back to disk.
	// Always empty on a dry-run.
	Modified []string
	// Errors contains any errors encountered during the fix process.
	Errors []error
	// WouldFix is the count of pre-fix violations that would have been
	// auto-fixed. Populated only on dry-run; zero on a real run.
	WouldFix int
	// WouldFixRules is the deduplicated list of rule IDs that would have
	// applied at least one fix. Populated only on dry-run; nil on a real run.
	WouldFixRules []string
	// DryRunFiles holds per-file dry-run results ordered by path.
	// Populated only on dry-run.
	DryRunFiles []DryRunFileResult
}

// Fix applies auto-fixes to the files at the given paths and returns a Result
// containing remaining diagnostics, modified file paths, and any errors.
func (f *Fixer) Fix(paths []string) *Result {
	res := &Result{}

	// Aggregate `before` diagnostics across files so the Failures
	// count can be deduped after the loop. Repo-level rules
	// (notably MDS048 git-hook-sync) anchor a single warning to a
	// repository artifact for every linted file in the repo, so
	// raw len(beforeDiags) summed per file would inflate Failures
	// to N when only one underlying issue exists.
	var allBefore []lint.Diagnostic
	wouldFixRuleSet := map[string]struct{}{}
	for _, path := range paths {
		if config.IsIgnored(f.Config.Ignore, path) {
			continue
		}
		res.FilesChecked++
		f.log().Printf("file: %s", path)
		beforeDiags, remainingDiags, modified, ruleCounts, errs := f.fixFile(path)
		allBefore = append(allBefore, beforeDiags...)
		res.Diagnostics = append(res.Diagnostics, remainingDiags...)
		if modified != "" {
			res.Modified = append(res.Modified, modified)
		}
		if f.DryRun {
			f.accumulateDryRun(res, path, remainingDiags, ruleCounts, wouldFixRuleSet)
		}
		res.Errors = append(res.Errors, errs...)
	}
	res.Failures = len(engine.DedupeDiagnostics(allBefore))

	if f.DryRun {
		res.WouldFixRules = sortedKeys(wouldFixRuleSet)
	}

	res.Diagnostics = engine.DedupeDiagnostics(res.Diagnostics)
	sort.Slice(res.Diagnostics, func(i, j int) bool {
		di, dj := res.Diagnostics[i], res.Diagnostics[j]
		if di.File != dj.File {
			return di.File < dj.File
		}
		if di.Line != dj.Line {
			return di.Line < dj.Line
		}
		return di.Column < dj.Column
	})

	return res
}

// accumulateDryRun updates res and wouldFixRuleSet with per-file dry-run data.
func (f *Fixer) accumulateDryRun(
	res *Result,
	path string,
	remainingDiags []lint.Diagnostic,
	ruleCounts map[string]int,
	wouldFixRuleSet map[string]struct{},
) {
	var fileWouldFix int
	for id, n := range ruleCounts {
		fileWouldFix += n
		wouldFixRuleSet[id] = struct{}{}
	}
	res.WouldFix += fileWouldFix
	if fileWouldFix > 0 || len(remainingDiags) > 0 {
		var fileDiags []lint.Diagnostic
		for _, d := range remainingDiags {
			if d.File == path {
				fileDiags = append(fileDiags, d)
			}
		}
		res.DryRunFiles = append(res.DryRunFiles, DryRunFileResult{
			Path:        path,
			WouldFix:    fileWouldFix,
			RuleCounts:  ruleCounts,
			Diagnostics: fileDiags,
		})
	}
}

// sortedKeys returns a sorted slice of keys from a string set (map[string]struct{}).
func sortedKeys(m map[string]struct{}) []string {
	if len(m) == 0 {
		return nil
	}
	ids := make([]string, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// fixFile applies auto-fixes to a single file and returns:
//   - beforeDiags: diagnostics before any fixing
//   - remainingDiags: diagnostics remaining after fixing
//   - modified: the path if the file was written to disk (empty on dry-run)
//   - ruleCounts: per-rule violation counts that would be fixed (dry-run only)
//   - errs: any errors encountered
func (f *Fixer) fixFile(path string) ([]lint.Diagnostic, []lint.Diagnostic, string, map[string]int, []error) {
	var errs []error

	source, err := lint.ReadFileLimited(path, f.MaxInputBytes)
	if err != nil {
		return nil, nil, "", nil, []error{fmt.Errorf("reading %q: %w", path, err)}
	}

	var info os.FileInfo
	if !f.DryRun {
		info, err = os.Stat(path)
		if err != nil {
			return nil, nil, "", nil, []error{fmt.Errorf("stat %q: %w", path, err)}
		}
	}

	lf, dirFS, fmKinds, fmFields, prepErr := f.prepareFile(path, source)
	if prepErr != nil {
		return nil, nil, "", nil, []error{prepErr}
	}

	effective := f.effectiveWithCategories(path, fmKinds, fmFields)

	f.logRules(effective)

	fixable, settingsErrs := f.fixableRules(effective)
	lf.GeneratedRanges = gensection.FindAllGeneratedRanges(lf)
	beforeDiags, checkErrs := engine.CheckRules(lf, f.Rules, effective)
	errs = append(errs, append(settingsErrs, checkErrs...)...)

	current := f.applyFixPasses(path, lf.Source, fixable, lf, dirFS, &errs)

	changed := !bytes.Equal(lf.Source, current)

	var (
		modified   string
		ruleCounts map[string]int
	)

	if changed {
		if f.DryRun {
			ruleCounts = countWouldFix(path, fixable, lf, dirFS)
		} else {
			if writeErr := f.writeFixed(path, lf.FullSource(current), info.Mode(), &errs); writeErr {
				return beforeDiags, beforeDiags, "", nil, errs
			}
			modified = path
		}
	}

	finalFile := buildPostFixFile(path, current, lf, dirFS)

	diags, checkErrs := engine.CheckRules(finalFile, f.Rules, effective)
	errs = append(errs, checkErrs...)
	if f.Explain {
		explain.Attach(diags, f.Config, path, fmKinds, fmFields)
	}
	return beforeDiags, diags, modified, ruleCounts, errs
}

// countWouldFix checks each fixable rule against the original source and
// returns a map of rule ID to violation count.
func countWouldFix(path string, fixable []rule.FixableRule, lf *lint.File, dirFS fs.FS) map[string]int {
	counts := make(map[string]int)
	for _, fr := range fixable {
		origFile, parseErr := lint.NewFile(path, lf.Source)
		if parseErr != nil {
			continue
		}
		hydrateLintFile(origFile, lf, dirFS)
		if diags := fr.Check(origFile); len(diags) > 0 {
			counts[fr.ID()] += len(diags)
		}
	}
	return counts
}

// writeFixed atomically writes data to path. Returns true if an error was appended.
func (f *Fixer) writeFixed(path string, data []byte, mode os.FileMode, errs *[]error) bool {
	if err := atomicWriteFile(path, data, mode); err != nil {
		*errs = append(*errs, fmt.Errorf("writing %q: %w", path, err))
		return true
	}
	return false
}

// hydrateLintFile copies onto a freshly-parsed *lint.File the parse-
// time and resolution context that the engine.Runner sets per-file
// (see runner.go ~line 90-108): FS, RootFS/RootDir, FrontMatter,
// LineOffset, StripFrontMatter, MaxInputBytes, GitignoreFunc, and
// GeneratedRanges (recomputed for the parsed bytes). Used by both
// the post-fix CheckRules call and the parsedFile inside each
// applyFixPasses iteration so rules see the same File regardless of
// which Fixer phase invokes them. Without this, fixable rules like
// catalog (consults GetGitignore for glob filtering) and include
// (consults MaxInputBytes for secondary reads) silently produce
// different post-fix bytes than `mdsmith check` would have validated.
func hydrateLintFile(parsed *lint.File, lf *lint.File, dirFS fs.FS) {
	parsed.FS = dirFS
	parsed.RootFS = lf.RootFS
	parsed.RootDir = lf.RootDir
	parsed.FrontMatter = lf.FrontMatter
	parsed.LineOffset = lf.LineOffset
	parsed.StripFrontMatter = lf.StripFrontMatter
	parsed.MaxInputBytes = lf.MaxInputBytes
	parsed.GitignoreFunc = lf.GitignoreFunc
	parsed.GeneratedRanges = gensection.FindAllGeneratedRanges(parsed)
}

// buildPostFixFile parses post-fix bytes and hydrates them with the
// per-file context from lf so the post-fix CheckRules call sees the
// same lint.File the runner would.
func buildPostFixFile(path string, source []byte, lf *lint.File, dirFS fs.FS) *lint.File {
	finalFile, _ := lint.NewFile(path, source) // NewFile never errors with current implementation
	hydrateLintFile(finalFile, lf, dirFS)
	return finalFile
}

// applyFixPasses repeatedly applies fixable rules until the content stabilizes.
func (f *Fixer) applyFixPasses(
	path string, source []byte, fixable []rule.FixableRule, lf *lint.File, dirFS fs.FS, errs *[]error,
) []byte {
	const maxPasses = 10
	current := source
	for pass := 0; pass < maxPasses; pass++ {
		f.log().Printf("fix: pass %d on %s", pass+1, path)
		before := current
		for _, fr := range fixable {
			parsedFile, err := lint.NewFile(path, current)
			if err != nil {
				*errs = append(*errs, fmt.Errorf("parsing %q: %w", path, err))
				break
			}
			hydrateLintFile(parsedFile, lf, dirFS)

			diags := fr.Check(parsedFile)
			if len(diags) == 0 {
				continue
			}

			current = fr.Fix(parsedFile)
		}
		if bytes.Equal(before, current) {
			f.log().Printf("fix: %s stable after %d passes", path, pass+1)
			break
		}
	}
	return current
}

// log returns the fixer's logger. If no logger is set, it returns a
// disabled logger so callers don't need nil checks.
func (f *Fixer) log() *vlog.Logger {
	if f.Logger != nil {
		return f.Logger
	}
	return &vlog.Logger{}
}

// logRules logs each enabled fixable rule in the effective config.
func (f *Fixer) logRules(effective map[string]config.RuleCfg) {
	l := f.log()
	if !l.Enabled {
		return
	}
	for _, rl := range f.Rules {
		cfg, ok := effective[rl.Name()]
		if !ok || !cfg.Enabled {
			continue
		}
		l.Printf("rule: %s %s", rl.ID(), rl.Name())
	}
}

// prepareFile parses a lint.File from source, configures its FS/RootDir,
// and resolves the file's front-matter kinds and full FM mapping. Returns
// the file, its dirFS, the validated kind list, the FM mapping (for the
// kind-assignment `fields-present:` selector), and any error.
func (f *Fixer) prepareFile(path string, source []byte) (*lint.File, fs.FS, []string, map[string]any, error) {
	lf, err := lint.NewFileFromSource(path, source, f.StripFrontMatter)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("parsing %q: %w", path, err)
	}
	lf.MaxInputBytes = f.MaxInputBytes
	dir := filepath.Dir(path)
	var dirFS fs.FS
	if f.SourceFS != nil {
		// In-memory callers (LSP) supply an explicit FS rooted at the
		// document's real on-disk directory; the path itself can be
		// workspace-relative for config glob matching.
		dirFS = f.SourceFS
	} else {
		dirFS = os.DirFS(dir)
	}
	lf.FS = dirFS
	gitignoreDir := dir
	if f.RootDir != "" {
		lf.SetRootDir(f.RootDir)
		gitignoreDir = f.RootDir
	}
	gd := gitignoreDir // capture for closure
	lf.GitignoreFunc = func() *lint.GitignoreMatcher {
		return f.cachedGitignore(gd)
	}
	kinds, err := lint.ParseFrontMatterKinds(lf.FrontMatter)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("parsing front-matter kinds in %q: %w", path, err)
	}
	if err := config.ValidateFrontMatterKinds(f.Config, path, kinds); err != nil {
		return nil, nil, nil, nil, err
	}
	fields, err := parseFieldsForSelector(f.Config, path, lf.FrontMatter)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return lf, dirFS, kinds, fields, nil
}

// parseFieldsForSelector decodes the full front-matter mapping only when
// a kind-assignment entry with `fields-present:` could match this file
// path. Skipping the parse for files outside every fields-present glob
// preserves the kinds-only parse path's leniency toward FM YAML errors
// that ParseFrontMatterKinds' fast path ignores.
func parseFieldsForSelector(cfg *config.Config, path string, fm []byte) (map[string]any, error) {
	if !config.NeedsFieldsForFile(cfg, path) {
		return nil, nil
	}
	fields, err := lint.ParseFrontMatterFields(fm)
	if err != nil {
		return nil, fmt.Errorf("parsing front matter in %q: %w", path, err)
	}
	return fields, nil
}

// effectiveWithCategories computes the effective rule config for a file
// path, applying category-based enable/disable on top of per-rule settings.
func (f *Fixer) effectiveWithCategories(
	path string, fmKinds []string, fmFields map[string]any,
) map[string]config.RuleCfg {
	effective, categories, explicit := config.EffectiveAll(f.Config, path, fmKinds, fmFields)
	m := make(map[string]string, len(f.Rules))
	for _, rl := range f.Rules {
		m[rl.Name()] = rl.Category()
	}
	return config.ApplyCategories(effective, categories, func(name string) string { return m[name] }, explicit)
}

// atomicWriteFile writes data to path using a temp-file-then-rename strategy
// to reduce the risk of partial writes on crash. Directory fsync is omitted
// for simplicity; full power-loss durability is not guaranteed.
func atomicWriteFile(path string, data []byte, mode os.FileMode) error {
	// Verify an existing target is writable before creating a temp file.
	// os.Rename can replace read-only targets (it only needs
	// directory write permission), so check explicitly.
	if _, err := os.Stat(path); err == nil {
		f, err := os.OpenFile(path, os.O_WRONLY, 0)
		if err != nil {
			return err
		}
		_ = f.Close()
	} else if !os.IsNotExist(err) {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), ".mdsmith-fix-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		if tmpPath != "" {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, mode); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	tmpPath = ""
	return nil
}

// fixableRules returns enabled rules that implement FixableRule, sorted by ID.
// If a rule implements Configurable and has settings, it is cloned and
// configured before being returned.
func (f *Fixer) fixableRules(effective map[string]config.RuleCfg) ([]rule.FixableRule, []error) {
	var fixable []rule.FixableRule
	var errs []error
	for _, rl := range f.Rules {
		cfg, ok := effective[rl.Name()]
		if !ok || !cfg.Enabled {
			continue
		}

		configured, err := engine.ConfigureRule(rl, cfg)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		if fr, ok := configured.(rule.FixableRule); ok {
			fixable = append(fixable, fr)
		}
	}
	sort.Slice(fixable, func(i, j int) bool {
		return fixable[i].ID() < fixable[j].ID()
	})
	return fixable, errs
}
