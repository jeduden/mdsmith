package fix

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/jeduden/tidymark/internal/config"
	"github.com/jeduden/tidymark/internal/engine"
	"github.com/jeduden/tidymark/internal/lint"
	"github.com/jeduden/tidymark/internal/rule"
)

// Fixer applies auto-fixes for fixable rules and reports remaining diagnostics.
type Fixer struct {
	Config           *config.Config
	Rules            []rule.Rule
	StripFrontMatter bool
}

// Result holds the outcome of a fix run.
type Result struct {
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

		source, err := os.ReadFile(path)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Errorf("reading %q: %w", path, err))
			continue
		}

		info, err := os.Stat(path)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Errorf("stat %q: %w", path, err))
			continue
		}

		// Parse the file, stripping front matter if configured.
		lf, err := lint.NewFileFromSource(path, source, f.StripFrontMatter)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Errorf("parsing %q: %w", path, err))
			continue
		}

		dirFS := os.DirFS(filepath.Dir(path))

		effective := config.Effective(f.Config, path)

		// Collect enabled fixable rules, sorted by ID.
		fixable, settingsErrs := f.fixableRules(effective)
		res.Errors = append(res.Errors, settingsErrs...)

		// Apply fixable rules in repeated passes until stable.
		// A later rule's fix may introduce violations caught by an
		// earlier rule, so we loop until no pass produces changes.
		const maxPasses = 10
		current := lf.Source
		for pass := 0; pass < maxPasses; pass++ {
			before := current
			for _, fr := range fixable {
				parsedFile, err := lint.NewFile(path, current)
				if err != nil {
					res.Errors = append(res.Errors, fmt.Errorf("parsing %q: %w", path, err))
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
				break
			}
		}

		// Write back only if content changed.
		if !bytes.Equal(lf.Source, current) {
			out := lf.FullSource(current)
			if err := os.WriteFile(path, out, info.Mode()); err != nil {
				res.Errors = append(res.Errors, fmt.Errorf("writing %q: %w", path, err))
				continue
			}
			res.Modified = append(res.Modified, path)
		}

		// Final lint pass with ALL enabled rules to collect remaining diagnostics.
		finalFile, err := lint.NewFile(path, current)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Errorf("parsing %q after fix: %w", path, err))
			continue
		}
		finalFile.FS = dirFS
		finalFile.FrontMatter = lf.FrontMatter
		finalFile.LineOffset = lf.LineOffset

		diags, errs := engine.CheckRules(finalFile, f.Rules, effective)
		res.Diagnostics = append(res.Diagnostics, diags...)
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
