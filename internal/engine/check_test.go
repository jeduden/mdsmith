package engine

import (
	"fmt"
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckRules_BasicDiagnostics(t *testing.T) {
	f, err := lint.NewFile("test.md", []byte("# Hello\n"))
	require.NoError(t, err)

	effective := map[string]config.RuleCfg{
		"mock-rule": {Enabled: true},
	}
	rules := []rule.Rule{&mockRule{id: "MDS999", name: "mock-rule"}}

	diags, errs := CheckRules(f, rules, effective)
	require.Len(t, errs, 0, "unexpected errors: %v", errs)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d", len(diags))
	if diags[0].RuleID != "MDS999" {
		t.Errorf("expected RuleID MDS999, got %s", diags[0].RuleID)
	}
}

func TestCheckRules_DisabledRuleSkipped(t *testing.T) {
	f, err := lint.NewFile("test.md", []byte("# Hello\n"))
	require.NoError(t, err)

	effective := map[string]config.RuleCfg{
		"mock-rule": {Enabled: false},
	}
	rules := []rule.Rule{&mockRule{id: "MDS999", name: "mock-rule"}}

	diags, errs := CheckRules(f, rules, effective)
	require.Len(t, errs, 0, "unexpected errors: %v", errs)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d", len(diags))
}

func TestCheckRules_UnconfiguredRuleSkipped(t *testing.T) {
	f, err := lint.NewFile("test.md", []byte("# Hello\n"))
	require.NoError(t, err)

	effective := map[string]config.RuleCfg{}
	rules := []rule.Rule{&mockRule{id: "MDS999", name: "mock-rule"}}

	diags, errs := CheckRules(f, rules, effective)
	require.Len(t, errs, 0, "unexpected errors: %v", errs)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d", len(diags))
}

func TestCheckRules_AppliesSettings(t *testing.T) {
	// 100-char line with max=120 should not trigger.
	line := strings.Repeat("a", 100) + "\n"
	f, err := lint.NewFile("test.md", []byte(line))
	require.NoError(t, err)

	effective := map[string]config.RuleCfg{
		"line-length": {
			Enabled:  true,
			Settings: map[string]any{"max": 120},
		},
	}
	rules := []rule.Rule{&configurableLengthRule{Max: 80}}

	diags, errs := CheckRules(f, rules, effective)
	require.Len(t, errs, 0, "unexpected errors: %v", errs)
	require.Len(t, diags, 0, "expected 0 diagnostics with max=120, got %d", len(diags))
}

// mockConfigurableErrorRule implements both Rule and Configurable.
// Its ApplySettings always returns an error.
type mockConfigurableErrorRule struct {
	id   string
	name string
}

func (r *mockConfigurableErrorRule) ID() string       { return r.id }
func (r *mockConfigurableErrorRule) Name() string     { return r.name }
func (r *mockConfigurableErrorRule) Category() string { return "test" }
func (r *mockConfigurableErrorRule) Check(_ *lint.File) []lint.Diagnostic {
	return []lint.Diagnostic{
		{
			Line:     1,
			Column:   1,
			RuleID:   r.id,
			RuleName: r.name,
			Severity: lint.Warning,
			Message:  "should not appear",
		},
	}
}
func (r *mockConfigurableErrorRule) ApplySettings(_ map[string]any) error {
	return fmt.Errorf("bad settings")
}
func (r *mockConfigurableErrorRule) DefaultSettings() map[string]any {
	return map[string]any{}
}

var _ rule.Configurable = (*mockConfigurableErrorRule)(nil)

func TestCheckRules_ApplySettingsError(t *testing.T) {
	f, err := lint.NewFile("test.md", []byte("# Hello\n"))
	require.NoError(t, err)

	effective := map[string]config.RuleCfg{
		"bad-rule": {
			Enabled:  true,
			Settings: map[string]any{"key": "val"},
		},
	}
	rules := []rule.Rule{&mockConfigurableErrorRule{id: "MDS900", name: "bad-rule"}}

	diags, errs := CheckRules(f, rules, effective)

	// The rule should be skipped (no diagnostics from it).
	assert.Len(t, diags, 0, "expected 0 diagnostics, got %d: %v", len(diags), diags)

	// The error should be returned in the errors slice.
	require.Len(t, errs, 1, "expected 1 error, got %d", len(errs))
	assert.Contains(t, errs[0].Error(), "bad settings", "expected error to contain 'bad settings', got: %v", errs[0])
}

func TestCheckRules_AdjustsLineOffset(t *testing.T) {
	f, err := lint.NewFileFromSource("test.md", []byte("---\ntitle: x\n---\n# Heading\n"), true)
	require.NoError(t, err)

	effective := map[string]config.RuleCfg{
		"mock-rule": {Enabled: true},
	}
	rules := []rule.Rule{&mockRule{id: "MDS999", name: "mock-rule"}}

	diags, errs := CheckRules(f, rules, effective)
	require.Len(t, errs, 0, "unexpected errors: %v", errs)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d", len(diags))
	// mockRule reports line 1; front matter has 3 lines, so adjusted = 4.
	if diags[0].Line != 4 {
		t.Errorf("expected adjusted line 4, got %d", diags[0].Line)
	}
}

// --- ConfigureRule tests ---

func TestConfigureRule_NoSettings(t *testing.T) {
	rl := &mockRule{id: "MDS999", name: "mock-rule"}
	cfg := config.RuleCfg{Enabled: true, Settings: nil}

	got, err := ConfigureRule(rl, cfg)
	require.NoError(t, err, "unexpected error: %v", err)
	assert.Same(t, rl, got, "expected same rule instance when no settings")
}

func TestConfigureRule_NonConfigurable(t *testing.T) {
	rl := &mockRule{id: "MDS999", name: "mock-rule"}
	cfg := config.RuleCfg{Enabled: true, Settings: map[string]any{"key": "val"}}

	got, err := ConfigureRule(rl, cfg)
	require.NoError(t, err, "unexpected error: %v", err)
	// mockRule does not implement Configurable, so the same instance is returned.
	assert.Same(t, rl, got, "expected same rule instance for non-configurable rule")
}

func TestConfigureRule_AppliesSettings(t *testing.T) {
	rl := &configurableLengthRule{Max: 80}
	cfg := config.RuleCfg{
		Enabled:  true,
		Settings: map[string]any{"max": 120},
	}

	got, err := ConfigureRule(rl, cfg)
	require.NoError(t, err, "unexpected error: %v", err)
	// Should be a different instance (cloned).
	assert.NotSame(t, rl, got, "expected a cloned rule, got same instance")

	// The cloned rule should have max=120 applied.
	cloned, ok := got.(*configurableLengthRule)
	require.True(t, ok, "expected *configurableLengthRule, got %T", got)
	assert.Equal(t, 120, cloned.Max, "expected Max=120, got %d", cloned.Max)

	// Original should be unchanged.
	assert.Equal(t, 80, rl.Max, "original Max changed to %d, want 80", rl.Max)
}

func TestConfigureRule_ApplySettingsError(t *testing.T) {
	rl := &mockConfigurableErrorRule{id: "MDS900", name: "bad-rule"}
	cfg := config.RuleCfg{
		Enabled:  true,
		Settings: map[string]any{"key": "val"},
	}

	got, err := ConfigureRule(rl, cfg)
	require.Error(t, err, "expected error, got nil")
	assert.Nil(t, got, "expected nil rule on error, got %v", got)
	assert.Contains(t, err.Error(), "bad settings", "expected error to contain 'bad settings', got: %v", err)
}
