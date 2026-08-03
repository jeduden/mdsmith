package engine

import (
	"sync"

	"github.com/jeduden/mdsmith/internal/checker"
	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/rule"
)

// SourceConfigCache memoizes checker.ConfigureEnabledRules' result for
// RunSource / RunSourceWithVersion, keyed by config.EffectiveSignature.
//
// runFiles already avoids repeat rule configuration: its per-worker
// runResolve.confCache spares a corpus-wide `mdsmith check` from
// re-cloning a Configurable rule (rule.CloneRule — a reflect.New plus
// ApplySettings) once per file, because one worker walks many files
// sequentially under runFiles' loop. RunSource has no such loop — the
// LSP calls it once per debounced edit — so nothing amortizes the
// reflect cost across calls; every keystroke's re-lint paid it fresh
// for every Configurable rule with settings.
//
// A long-lived caller installs one SourceConfigCache and reuses it
// across calls (the Session wires it the same way it wires RunCache
// and ParseCache: a persistent field a fresh per-call Runner points
// at). nil means "no cache" — RunSource behaves exactly as before.
//
// Safe for concurrent use.
type SourceConfigCache struct {
	m sync.Map // string (EffectiveSignature key) -> configuredRules
}

// NewSourceConfigCache returns an empty cache ready for use.
func NewSourceConfigCache() *SourceConfigCache {
	return &SourceConfigCache{}
}

// configured returns the configured enabled rule list for the effective
// config identified by key, configuring and memoizing it on first use.
// Concurrent misses on the same key converge on one shared clone via
// LoadOrStore, mirroring runResolve.configured's per-worker cache.
func (c *SourceConfigCache) configured(
	key string, rules []rule.Rule, effective map[string]config.RuleCfg,
) ([]rule.Rule, []error) {
	if v, ok := c.m.Load(key); ok {
		cr := v.(configuredRules)
		return cr.rules, cr.errs
	}
	configuredList, errs := checker.ConfigureEnabledRules(rules, effective)
	actual, _ := c.m.LoadOrStore(key, configuredRules{rules: configuredList, errs: errs})
	cr := actual.(configuredRules)
	return cr.rules, cr.errs
}
