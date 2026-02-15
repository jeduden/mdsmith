package fix

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/engine"
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
	Modified []string
	// Errors contains any errors encountered during the fix process.
	Errors []error
}

// Fix applies auto-fixes to the files at the given paths and returns a Result
// containing remaining diagnostics, modified file paths, and any errors.
func (f *Fixer) Fix(paths []string) *Result {
	res := &Result{}

	for _, path := range paths {
		if config.IsIgnored(f.Config.Ignore, path) {
			continue
		}
		res.FilesChecked++
		f.log().Printf("file: %s", path)
		beforeDiags, remainingDiags, modified, errs := f.fixFile(path)
		res.Failures += len(beforeDiags)
		res.Diagnostics = append(res.Diagnostics, remainingDiags...)
		if modified != "" {
			res.Modified = append(res.Modified, modified)
		}
		res.Errors = append(res.Errors, errs...)
	}

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

// fixFile applies auto-fixes to a single file and returns diagnostics before
// fixing, remaining diagnostics after fixing, the path if modified, and any
// errors encountered.
func (f *Fixer) fixFile(path string) ([]lint.Diagnostic, []lint.Diagnostic, string, []error) {
	var errs []error

	source, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, "", []error{fmt.Errorf("reading %q: %w", path, err)}
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, nil, "", []error{fmt.Errorf("stat %q: %w", path, err)}
	}

	lf, err := lint.NewFileFromSource(path, source, f.StripFrontMatter)
	if err != nil {
		return nil, nil, "", []error{fmt.Errorf("parsing %q: %w", path, err)}
	}

	dirFS := os.DirFS(filepath.Dir(path))
	lf.FS = dirFS
	effective := f.effectiveWithCategories(path)

	f.logRules(effective)

	fixable, settingsErrs := f.fixableRules(effective)
	errs = append(errs, settingsErrs...)
	beforeDiags, _ := engine.CheckRules(lf, f.Rules, effective)

	current := f.applyFixPasses(path, lf.Source, fixable, dirFS, &errs)

	var modified string
	if !bytes.Equal(lf.Source, current) {
		out := lf.FullSource(current)
		if err := os.WriteFile(path, out, info.Mode()); err != nil {
			errs = append(errs, fmt.Errorf("writing %q: %w", path, err))
			return beforeDiags, beforeDiags, "", errs
		}
		modified = path
	}

	finalFile, err := lint.NewFile(path, current)
	if err != nil {
		errs = append(errs, fmt.Errorf("parsing %q after fix: %w", path, err))
		return beforeDiags, beforeDiags, modified, errs
	}
	finalFile.FS = dirFS
	finalFile.FrontMatter = lf.FrontMatter
	finalFile.LineOffset = lf.LineOffset

	diags, checkErrs := engine.CheckRules(finalFile, f.Rules, effective)
	errs = append(errs, checkErrs...)
	return beforeDiags, diags, modified, errs
}

// applyFixPasses repeatedly applies fixable rules until the content stabilizes.
func (f *Fixer) applyFixPasses(
	path string, source []byte, fixable []rule.FixableRule, dirFS fs.FS, errs *[]error,
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
			parsedFile.FS = dirFS

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

// effectiveWithCategories computes the effective rule config for a file
// path, applying category-based enable/disable on top of per-rule settings.
func (f *Fixer) effectiveWithCategories(path string) map[string]config.RuleCfg {
	effective := config.Effective(f.Config, path)
	categories := config.EffectiveCategories(f.Config, path)
	explicit := config.EffectiveExplicitRules(f.Config, path)

	// Build rule-name-to-category lookup from the fixer's rule list.
	m := make(map[string]string, len(f.Rules))
	for _, rl := range f.Rules {
		m[rl.Name()] = rl.Category()
	}
	catLookup := func(name string) string { return m[name] }

	return config.ApplyCategories(effective, categories, catLookup, explicit)
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
