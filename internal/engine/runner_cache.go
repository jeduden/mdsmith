package engine

import (
	"path/filepath"
	"sync"

	"github.com/jeduden/mdsmith/internal/checker"
	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/gitignore"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
)

// effectiveCache memoizes effective rule configs by config signature for
// the span of one runFiles call. File workers read and write it
// concurrently, so it wraps sync.Map. The stored maps are read-only
// downstream — classifyRules clones a rule's settings before applying
// them — so sharing one map across files is safe.
type effectiveCache struct{ m sync.Map }

func (c *effectiveCache) get(key string) (map[string]config.RuleCfg, bool) {
	v, ok := c.m.Load(key)
	if !ok {
		return nil, false
	}
	return v.(map[string]config.RuleCfg), true
}

// loadOrStore returns the cached map for key, storing and returning v
// when none exists yet. Concurrent misses on the same key thus converge
// on one shared instance instead of each keeping its own equal copy.
func (c *effectiveCache) loadOrStore(key string, v map[string]config.RuleCfg) map[string]config.RuleCfg {
	actual, _ := c.m.LoadOrStore(key, v)
	return actual.(map[string]config.RuleCfg)
}

// runResolve bundles the resolution state lintFile needs, across two
// lifetimes: mdRules is per-worker (each worker filters its own rule
// clones), while catLookup and effCache are per-run — built once and
// shared across all workers (effCache is concurrency-safe). Bundling
// keeps lintFile's signature small and hoists work the per-file loop
// used to repeat.
type runResolve struct {
	mdRules   []rule.Rule
	catLookup func(string) string
	effCache  *effectiveCache
	// confCache memoizes the configured (cloned + settings-applied) enabled
	// rule list per effective-config signature. It is private to one worker,
	// so it needs no lock: each worker builds its own runResolve and walks
	// its files sequentially. A corpus that shares one config configures its
	// rules once here instead of once per file — the clone + ApplySettings
	// work the allocation profile showed as per-file overhead. Reusing a
	// configured rule across files is safe for the same reason the worker
	// reuses its mdRules clones across files: a rule's Check holds no state
	// between calls (see checker.ConfigureEnabledRules).
	confCache map[string]configuredRules
}

// configuredRules is one cache entry: the enabled, configured rule list
// for a config signature plus the settings-application errors that
// configuring it produced (surfaced once, not re-derived per file).
type configuredRules struct {
	rules []rule.Rule
	errs  []error
}

// configured returns the configured enabled rule list for the effective
// config identified by key, building and memoizing it on first use. The
// map is written through the by-value runResolve copy, which is safe
// because the map header aliases the one instance the worker created.
func (rr runResolve) configured(
	key string, effective map[string]config.RuleCfg,
) ([]rule.Rule, []error) {
	if c, ok := rr.confCache[key]; ok {
		return c.rules, c.errs
	}
	rules, errs := checker.ConfigureEnabledRules(rr.mdRules, effective)
	rr.confCache[key] = configuredRules{rules: rules, errs: errs}
	return rules, errs
}

// effectiveCached is the hot-path config resolver: it memoizes the
// effective rule config on rr.effCache keyed by the file's config
// signature and reuses the prebuilt rr.catLookup, so a corpus that
// shares one config resolves it once instead of per file. It returns the
// signature key alongside the config so the caller can reuse it to key the
// per-worker configured-rule cache without recomputing it. Cold callers
// (RunSource, config-target, defaults) keep using effectiveWithCategories.
func (r *Runner) effectiveCached(
	path string, fmKinds []string, fmFields map[string]any, rr runResolve,
) (map[string]config.RuleCfg, string) {
	key, kinds := config.EffectiveSignature(r.Config, path, fmKinds, fmFields)
	if v, ok := rr.effCache.get(key); ok {
		return v, key
	}
	effective, categories, explicit := config.EffectiveAllForKinds(r.Config, path, kinds)
	res := config.ApplyCategories(effective, categories, rr.catLookup, explicit)
	return rr.effCache.loadOrStore(key, res), key
}

// runCacheForCall returns the run-scoped read cache to thread through
// the next Run / RunSource pass. A caller-supplied r.RunCache (the
// LSP installs one on Server and invalidates entries on document
// events) is reused as-is. When nil, a fresh cache is built for this
// call only and never stored on r — a Runner re-used for a second
// Run starts clean and cannot serve stale reads from the previous
// corpus.
func (r *Runner) runCacheForCall() *lint.RunCache {
	if r.RunCache != nil {
		return r.RunCache
	}
	return lint.NewRunCache()
}

// cachedGitignore returns a *gitignore.Matcher for the given directory,
// creating and caching it on first use to avoid re-walking the filesystem.
// The cache key is normalized to an absolute path so equivalent paths
// (e.g. "sub" vs "./sub") share the same entry.
func (r *Runner) cachedGitignore(dir string) *gitignore.Matcher {
	r.gitignoreMu.Lock()
	defer r.gitignoreMu.Unlock()
	if r.gitignoreCache == nil {
		r.gitignoreCache = make(map[string]*gitignore.Matcher)
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		absDir = filepath.Clean(dir)
	}
	if m, ok := r.gitignoreCache[absDir]; ok {
		return m
	}
	m := gitignore.NewMatcher(absDir)
	r.gitignoreCache[absDir] = m
	return m
}
