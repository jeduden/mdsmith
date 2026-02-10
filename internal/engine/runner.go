package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

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
		if config.IsIgnored(r.Config.Ignore, path) {
			continue
		}

		source, err := os.ReadFile(path)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Errorf("reading %q: %w", path, err))
			continue
		}

		f, err := lint.NewFileFromSource(path, source, r.StripFrontMatter)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Errorf("parsing %q: %w", path, err))
			continue
		}
		f.FS = os.DirFS(filepath.Dir(path))

		effective := config.Effective(r.Config, path)

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
// meaningful filesystem context. Rules that access f.FS must handle nil.
func (r *Runner) RunSource(path string, source []byte) *Result {
	res := &Result{}

	f, err := lint.NewFileFromSource(path, source, r.StripFrontMatter)
	if err != nil {
		res.Errors = append(res.Errors, fmt.Errorf("parsing %q: %w", path, err))
		return res
	}

	effective := config.Effective(r.Config, path)

	diags, errs := CheckRules(f, r.Rules, effective)
	res.Diagnostics = append(res.Diagnostics, diags...)
	res.Errors = append(res.Errors, errs...)

	sortDiagnostics(res.Diagnostics)
	return res
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
