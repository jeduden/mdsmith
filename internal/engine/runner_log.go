package engine

import (
	"sort"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
	vlog "github.com/jeduden/mdsmith/internal/log"
	"github.com/jeduden/mdsmith/internal/rule"
)

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

// sortDiagnostics sorts diagnostics by file, line, column, message, then
// rule id. The RuleID tiebreak makes the order independent of the walk path
// that produced the diagnostics: the parse-skip block-walk and the full-parse
// node-walk can emit two same-position, same-message diagnostics from
// different rules in different input orders, and the Layer-0 equivalence
// assertions compare full ordered slices. sort.SliceStable then preserves
// input order only for diagnostics equal on every compared field.
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
		if di.Message != dj.Message {
			return di.Message < dj.Message
		}
		return di.RuleID < dj.RuleID
	})
}
