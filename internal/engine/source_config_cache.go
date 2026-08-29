package engine

import (
	"sync"

	"github.com/jeduden/mdsmith/internal/checker"
	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/rule"
)

// SourceConfigCache memoizes the expensive half of
// checker.ConfigureEnabledRules — the reflect.New plus two
// ApplySettings passes rule.CloneRule spends per Configurable rule
// with settings — for RunSource / RunSourceWithVersion, keyed by
// config.EffectiveSignature.
//
// runFiles already avoids repeat rule configuration: its per-worker
// runResolve.confCache spares a corpus-wide `mdsmith check` from
// re-cloning a Configurable rule once per file, because one worker
// walks many files sequentially under runFiles' loop. RunSource has no
// such loop — the LSP calls it once per debounced edit — so nothing
// amortized the reflect cost across calls; every keystroke's re-lint
// paid it fresh for every Configurable rule with settings.
//
// A cached entry is a template, never handed out directly: configured
// clones it per call via cloneRules (rule.CloneInstance — a plain
// reflect.New plus a shallow field copy, no ApplySettings) before
// returning, so no two callers ever share a *Rule pointer. That
// matches the isolation runFiles' own cloneRules already gives every
// CLI worker, and is why this is safe even though RunSourceWithVersion
// can be called concurrently for different documents that resolve to
// the same effective config (the LSP session doc explicitly promises
// concurrent-safe calls) — a rule with mutable state, whether per-Check
// (internal/rules/include's visited/chain) or per-file (a
// rule.FileResetter such as MDS003's prevLevel or MDS005's seen map),
// never observes two Check calls racing on the same instance. Only the
// settings-application work is shared; the instances handed to Check are
// not. The cached template is never walked, so its FileResetter fields
// stay zero and each cloneRules copy starts from a clean slate.
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

// configured returns a private, per-call clone of the configured
// enabled rule list for the effective config identified by key,
// configuring and memoizing the template on first use. Concurrent
// misses on the same key converge on one shared template via
// LoadOrStore, mirroring runResolve.configured's per-worker cache;
// every caller — cache hit or miss — still gets its own cloneRules
// copy of that template, never the template itself.
//
// rules and effective are consulted only on a cache miss — the first
// caller under a given key determines the template every later caller
// with that key reuses, unconditionally. That is safe for the same
// reason runResolve.configured's per-worker cache is: a caller with a
// stable rule set (one Session's Runner.Rules, fixed for the Session's
// lifetime — see Runner.SourceConfigCache) plus
// config.EffectiveSignature's own guarantee — equal signatures imply
// equal effective content — means any caller sharing key would have
// built an equivalent template anyway. Do not share one
// SourceConfigCache across Runners with different rule sets.
func (c *SourceConfigCache) configured(
	key string, rules []rule.Rule, effective map[string]config.RuleCfg,
) ([]rule.Rule, []error) {
	v, ok := c.m.Load(key)
	if !ok {
		configuredList, errs := checker.ConfigureEnabledRules(rules, effective)
		v, _ = c.m.LoadOrStore(key, configuredRules{rules: configuredList, errs: errs})
	}
	cr := v.(configuredRules)
	return cloneRules(cr.rules), cr.errs
}
