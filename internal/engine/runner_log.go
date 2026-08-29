package engine

import (
	"cmp"
	"slices"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
	vlog "github.com/jeduden/mdsmith/internal/log"
	"github.com/jeduden/mdsmith/internal/rule"
)

// disabledLogger is the shared zero-value logger (*Runner).log
// returns when no Logger is configured — the common case, since -v is
// off by default. Every caller only reads Enabled or calls Printf,
// which no-ops before touching the logger's mutex when Enabled is
// false (internal/log.Logger.Printf), so nothing ever mutates this
// instance; sharing it instead of allocating a fresh &vlog.Logger{}
// on every call avoids one allocation per file in the workspace walk
// (log runs once per file via Runner.checkFile).
var disabledLogger = &vlog.Logger{}

// log returns the runner's logger. If no logger is set, it returns a
// disabled logger so callers don't need nil checks.
func (r *Runner) log() *vlog.Logger {
	if r.Logger != nil {
		return r.Logger
	}
	return disabledLogger
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
// assertions compare full ordered slices. slices.SortStableFunc then
// preserves input order only for diagnostics equal on every compared field.
//
// slices.SortStableFunc sorts the concrete lint.Diagnostic values directly,
// unlike sort.SliceStable, which drives reflect.Swapper under the hood —
// see docs/development/high-performance-go.md's "reflect in hot paths"
// anti-pattern. This runs once per Runner.Run/RunSource call over the full
// result set, so it's the one place in the codebase this fix — already
// applied to the equivalent per-rule sortDiagnostics helpers — was missing.
func sortDiagnostics(diags []lint.Diagnostic) {
	slices.SortStableFunc(diags, func(a, b lint.Diagnostic) int {
		if a.File != b.File {
			return cmp.Compare(a.File, b.File)
		}
		if a.Line != b.Line {
			return cmp.Compare(a.Line, b.Line)
		}
		if a.Column != b.Column {
			return cmp.Compare(a.Column, b.Column)
		}
		if a.Message != b.Message {
			return cmp.Compare(a.Message, b.Message)
		}
		return cmp.Compare(a.RuleID, b.RuleID)
	})
}
