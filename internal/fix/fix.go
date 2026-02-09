package fix

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/gobwas/glob"
	"github.com/jeduden/tidymark/internal/config"
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
		if f.isIgnored(path) {
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

		// Strip front matter before fixing; re-prepend when writing.
		var fmPrefix []byte
		content := source
		if f.StripFrontMatter {
			fmPrefix, content = lint.StripFrontMatter(source)
		}

		dirFS := os.DirFS(filepath.Dir(path))

		effective := config.Effective(f.Config, path)

		// Collect enabled fixable rules, sorted by ID.
		fixable := f.fixableRules(effective)

		// Apply fixable rules in repeated passes until stable.
		// A later rule's fix may introduce violations caught by an
		// earlier rule, so we loop until no pass produces changes.
		const maxPasses = 10
		current := content
		for pass := 0; pass < maxPasses; pass++ {
			before := current
			for _, fr := range fixable {
				lf, err := lint.NewFile(path, current)
				if err != nil {
					res.Errors = append(res.Errors, fmt.Errorf("parsing %q: %w", path, err))
					break
				}
				lf.FS = dirFS

				diags := fr.Check(lf)
				if len(diags) == 0 {
					continue
				}

				current = fr.Fix(lf)
			}
			if bytes.Equal(before, current) {
				break
			}
		}

		// Write back only if content changed.
		if !bytes.Equal(content, current) {
			out := append(fmPrefix, current...)
			if err := os.WriteFile(path, out, info.Mode()); err != nil {
				res.Errors = append(res.Errors, fmt.Errorf("writing %q: %w", path, err))
				continue
			}
			res.Modified = append(res.Modified, path)
		}

		// Final lint pass with ALL enabled rules to collect remaining diagnostics.
		lf, err := lint.NewFile(path, current)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Errorf("parsing %q after fix: %w", path, err))
			continue
		}
		lf.FS = dirFS

		for _, rl := range f.Rules {
			cfg, ok := effective[rl.Name()]
			if !ok || !cfg.Enabled {
				continue
			}
			diags := rl.Check(lf)
			res.Diagnostics = append(res.Diagnostics, diags...)
		}
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
func (f *Fixer) fixableRules(effective map[string]config.RuleCfg) []rule.FixableRule {
	var fixable []rule.FixableRule
	for _, rl := range f.Rules {
		cfg, ok := effective[rl.Name()]
		if !ok || !cfg.Enabled {
			continue
		}
		if fr, ok := rl.(rule.FixableRule); ok {
			fixable = append(fixable, fr)
		}
	}
	sort.Slice(fixable, func(i, j int) bool {
		return fixable[i].ID() < fixable[j].ID()
	})
	return fixable
}

// isIgnored returns true if the file path matches any of the configured
// ignore patterns.
func (f *Fixer) isIgnored(path string) bool {
	cleanPath := filepath.Clean(path)

	for _, pattern := range f.Config.Ignore {
		g, err := glob.Compile(pattern)
		if err != nil {
			continue
		}
		if g.Match(path) || g.Match(cleanPath) || g.Match(filepath.Base(path)) {
			return true
		}
	}
	return false
}
