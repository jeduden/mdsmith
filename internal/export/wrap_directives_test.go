package export_test

import (
	"github.com/jeduden/mdsmith/internal/archetype/gensection"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
)

// directiveWrappers couples a real directive rule with call counters
// so tests can assert that Export gates Fix on Check results without
// reaching for goroutine-shared state at the package level.
type directiveWrappers struct {
	rules     []rule.Rule
	wrappers  map[string]*countingDirective
	byOrigPtr map[rule.Rule]*countingDirective
}

func (w *directiveWrappers) fixCalls(name string) int {
	c, ok := w.wrappers[name]
	if !ok {
		return -1
	}
	return c.fixCount
}

// wrapDirectives replaces every directive rule in src with a
// countingDirective that delegates to the original implementation
// while recording Check/Fix invocations.
func wrapDirectives(src []rule.Rule) *directiveWrappers {
	w := &directiveWrappers{
		wrappers:  map[string]*countingDirective{},
		byOrigPtr: map[rule.Rule]*countingDirective{},
	}
	for _, r := range src {
		fr, fok := r.(rule.FixableRule)
		d, dok := r.(gensection.Directive)
		if !fok || !dok {
			w.rules = append(w.rules, r)
			continue
		}
		cd := &countingDirective{wrapped: r, fix: fr, dir: d}
		w.wrappers[d.Name()] = cd
		w.byOrigPtr[r] = cd
		w.rules = append(w.rules, cd)
	}
	return w
}

// countingDirective forwards every Rule/FixableRule/Directive method
// to the wrapped implementation while incrementing call counters.
//
// The CLI/Export contract treats a wrapper that satisfies both
// rule.FixableRule and gensection.Directive identically to the rule
// it wraps, so we can swap one in without registering it.
type countingDirective struct {
	wrapped    rule.Rule
	fix        rule.FixableRule
	dir        gensection.Directive
	checkCount int
	fixCount   int
	// injectWarning, when true, appends a Warning-severity diagnostic
	// to whatever the wrapped Check returns. Lets tests assert that
	// Export's Check mode only refuses on Error severity.
	injectWarning bool
}

func (c *countingDirective) ID() string       { return c.wrapped.ID() }
func (c *countingDirective) Name() string     { return c.wrapped.Name() }
func (c *countingDirective) Category() string { return c.wrapped.Category() }

func (c *countingDirective) Check(f *lint.File) []lint.Diagnostic {
	c.checkCount++
	d := c.wrapped.Check(f)
	if c.injectWarning {
		d = append(d, lint.Diagnostic{
			File:     f.Path,
			Line:     1,
			Column:   1,
			Severity: lint.Warning,
			RuleID:   c.dir.RuleID(),
			RuleName: c.dir.RuleName(),
			Message:  "synthetic warning from countingDirective",
		})
	}
	return d
}

func (c *countingDirective) Fix(f *lint.File) []byte {
	c.fixCount++
	return c.fix.Fix(f)
}

// gensection.Directive shims.

func (c *countingDirective) RuleID() string   { return c.dir.RuleID() }
func (c *countingDirective) RuleName() string { return c.dir.RuleName() }

func (c *countingDirective) Validate(
	filePath string, line int, params map[string]string,
	columns map[string]gensection.ColumnConfig,
) []lint.Diagnostic {
	return c.dir.Validate(filePath, line, params, columns)
}

func (c *countingDirective) Generate(
	f *lint.File, filePath string, line int,
	params map[string]string, columns map[string]gensection.ColumnConfig,
) (string, []lint.Diagnostic) {
	return c.dir.Generate(f, filePath, line, params, columns)
}
