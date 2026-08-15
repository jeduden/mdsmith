package engine

import (
	"sync"
	"testing"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sourceCacheMockRule is a Configurable rule with settings, the shape
// checker.ConfigureEnabledRules clones via rule.CloneRule (a reflect.New
// + ApplySettings pair) whenever it configures the rule. applyCalls is a
// package-level counter (not a per-instance field) so it survives
// rule.CloneRule's zero-value reflect.New, letting the test observe how
// many times ApplySettings actually ran across two configured() calls.
type sourceCacheMockRule struct {
	name     string
	appliedX int
}

var sourceCacheMockApplyCalls int

func (r *sourceCacheMockRule) ID() string       { return r.name }
func (r *sourceCacheMockRule) Name() string     { return r.name }
func (r *sourceCacheMockRule) Category() string { return "test" }
func (r *sourceCacheMockRule) Check(f *lint.File) []lint.Diagnostic {
	return nil
}
func (r *sourceCacheMockRule) ApplySettings(s map[string]any) error {
	sourceCacheMockApplyCalls++
	if v, ok := s["x"].(int); ok {
		r.appliedX = v
	}
	return nil
}
func (r *sourceCacheMockRule) DefaultSettings() map[string]any { return map[string]any{} }

// TestSourceConfigCache_ReusesConfiguredRules is the red/green test for
// the RunSource reflect-clone cache: the CLI's per-worker confCache
// (runResolve.configured) already spares a corpus-wide lint pass from
// re-cloning a Configurable rule per file; RunSource / RunSourceWithVersion
// (the LSP's per-keystroke entry point) had no equivalent, so every call
// paid checker.ConfigureEnabledRules' rule.CloneRule (reflect.New + two
// ApplySettings calls) again. SourceConfigCache closes that gap by
// memoizing the settings-application work per signature — but every
// call, hit or miss, still gets its own cloneRules copy (never the
// cached template's own pointer), since RunSourceWithVersion can run
// concurrently for different documents that resolve to the same
// effective config, and a rule with per-Check mutable state (e.g.
// internal/rules/include's visited/chain) must never see two Check
// calls racing on the same instance. See
// docs/development/high-performance-go.md "Skip work you don't need"
// and "reflect in hot paths".
func TestSourceConfigCache_ReusesConfiguredRules(t *testing.T) {
	sourceCacheMockApplyCalls = 0
	rules := []rule.Rule{&sourceCacheMockRule{name: "mock-rule"}}
	effective := map[string]config.RuleCfg{
		"mock-rule": {Enabled: true, Settings: map[string]any{"x": 1}},
	}

	cache := NewSourceConfigCache()
	got1, errs1 := cache.configured("sig-a", rules, effective)
	got2, errs2 := cache.configured("sig-a", rules, effective)

	require.Empty(t, errs1)
	require.Empty(t, errs2)
	require.Len(t, got1, 1)
	require.Len(t, got2, 1)

	// ApplySettings runs twice total (CloneRule's DefaultSettings pass
	// plus ConfigureRule's cfg.Settings pass) from the single
	// checker.ConfigureEnabledRules call the cache miss triggers — the
	// second configured() call must not trigger it again.
	assert.Equal(t, 2, sourceCacheMockApplyCalls,
		"second call under the same signature must reuse the cached template's settings work")

	got1Mock := got1[0].(*sourceCacheMockRule)
	got2Mock := got2[0].(*sourceCacheMockRule)
	assert.Equal(t, 1, got1Mock.appliedX)
	assert.Equal(t, 1, got2Mock.appliedX)
	assert.NotSame(t, got1[0], got2[0],
		"each call must get its own rule instance so concurrent callers never share Check-time mutable state")
}

// TestSourceConfigCache_DistinctSignaturesConfigureIndependently guards
// against a key collision silently serving one config's rule set under a
// different effective config.
func TestSourceConfigCache_DistinctSignaturesConfigureIndependently(t *testing.T) {
	rules := []rule.Rule{&sourceCacheMockRule{name: "mock-rule"}}
	enabled := map[string]config.RuleCfg{"mock-rule": {Enabled: true}}
	disabled := map[string]config.RuleCfg{"mock-rule": {Enabled: false}}

	cache := NewSourceConfigCache()
	gotEnabled, _ := cache.configured("sig-enabled", rules, enabled)
	gotDisabled, _ := cache.configured("sig-disabled", rules, disabled)

	assert.Len(t, gotEnabled, 1)
	assert.Empty(t, gotDisabled)
}

// TestSourceConfigCache_NilRunnerFieldPreservesOldBehavior verifies a
// Runner with no SourceConfigCache installed (every non-LSP caller today)
// configures fresh on each call, matching pre-change behaviour exactly.
func TestSourceConfigCache_NilRunnerFieldPreservesOldBehavior(t *testing.T) {
	r := &Runner{
		Config: &config.Config{},
		Rules:  []rule.Rule{&sourceCacheMockRule{name: "mock-rule"}},
	}
	assert.Nil(t, r.SourceConfigCache)
}

// TestSourceConfigCache_ConcurrentCallsGetDistinctInstances is a -race
// regression test for the concurrency-safety contract itself: many
// goroutines sharing one cache and one signature must never observe
// (or be able to corrupt) a shared *Rule pointer.
func TestSourceConfigCache_ConcurrentCallsGetDistinctInstances(t *testing.T) {
	rules := []rule.Rule{&sourceCacheMockRule{name: "mock-rule"}}
	effective := map[string]config.RuleCfg{
		"mock-rule": {Enabled: true, Settings: map[string]any{"x": 7}},
	}
	cache := NewSourceConfigCache()

	const n = 50
	seen := make([]rule.Rule, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			got, _ := cache.configured("sig-concurrent", rules, effective)
			require.Len(t, got, 1)
			mock := got[0].(*sourceCacheMockRule)
			mock.appliedX = i // would race under -race if instances were shared
			seen[i] = got[0]
		}(i)
	}
	wg.Wait()

	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			assert.NotSame(t, seen[i], seen[j], "every concurrent caller must get its own instance")
		}
	}
}
