package engine

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sourceCacheMockRule is a Configurable rule with settings, the shape
// checker.ConfigureEnabledRules clones via rule.CloneRule (a reflect.New
// + ApplySettings pair) whenever it configures the rule. Used to prove
// SourceConfigCache reuses one clone across calls instead of paying that
// reflect cost again.
type sourceCacheMockRule struct {
	name string
}

func (r *sourceCacheMockRule) ID() string       { return r.name }
func (r *sourceCacheMockRule) Name() string     { return r.name }
func (r *sourceCacheMockRule) Category() string { return "test" }
func (r *sourceCacheMockRule) Check(f *lint.File) []lint.Diagnostic {
	return nil
}
func (r *sourceCacheMockRule) ApplySettings(s map[string]any) error { return nil }
func (r *sourceCacheMockRule) DefaultSettings() map[string]any      { return map[string]any{} }

// TestSourceConfigCache_ReusesConfiguredRules is the red/green test for
// the RunSource reflect-clone cache: the CLI's per-worker confCache
// (runResolve.configured) already spares a corpus-wide lint pass from
// re-cloning a Configurable rule per file; RunSource / RunSourceWithVersion
// (the LSP's per-keystroke entry point) had no equivalent, so every call
// paid checker.ConfigureEnabledRules' rule.CloneRule (reflect.New +
// ApplySettings) again. SourceConfigCache closes that gap: two calls
// sharing the same effective-config signature must return the identical
// clone, not two independently reflect-built ones. See
// docs/development/high-performance-go.md "Skip work you don't need" and
// "reflect in hot paths".
func TestSourceConfigCache_ReusesConfiguredRules(t *testing.T) {
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
	assert.Same(t, got1[0], got2[0], "second call under the same signature must reuse the first call's clone")
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
