package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/gobwas/glob"
	"github.com/jeduden/tidymark/internal/config"
	"github.com/jeduden/tidymark/internal/lint"
	"github.com/jeduden/tidymark/internal/rule"
)

// Runner drives the linting pipeline: for each file it reads the content,
// builds a File (parsing the AST once), determines the effective rule
// configuration, runs enabled rules, and collects diagnostics.
type Runner struct {
	Config           *config.Config
	Rules            []rule.Rule
	StripFrontMatter bool
}

// Result holds the output of a lint run.
type Result struct {
	Diagnostics []lint.Diagnostic
	Errors      []error
}

// Run lints the files at the given paths and returns a Result containing
// all diagnostics (sorted by file, line, column) and any errors encountered.
func (r *Runner) Run(paths []string) *Result {
	res := &Result{}

	for _, path := range paths {
		if r.isIgnored(path) {
			continue
		}

		source, err := os.ReadFile(path)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Errorf("reading %q: %w", path, err))
			continue
		}

		if r.StripFrontMatter {
			_, source = lint.StripFrontMatter(source)
		}

		f, err := lint.NewFile(path, source)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Errorf("parsing %q: %w", path, err))
			continue
		}
		f.FS = os.DirFS(filepath.Dir(path))

		effective := config.Effective(r.Config, path)

		for _, rl := range r.Rules {
			cfg, ok := effective[rl.Name()]
			if !ok || !cfg.Enabled {
				continue
			}

			diags := rl.Check(f)
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

// isIgnored returns true if the file path matches any of the configured
// ignore patterns.
func (r *Runner) isIgnored(path string) bool {
	cleanPath := filepath.Clean(path)

	for _, pattern := range r.Config.Ignore {
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
