package engine

import (
	"fmt"
	"strings"
	"testing"

	"github.com/jeduden/tidymark/internal/config"
	"github.com/jeduden/tidymark/internal/lint"
	"github.com/jeduden/tidymark/internal/rule"
)

func TestCheckRules_BasicDiagnostics(t *testing.T) {
	f, err := lint.NewFile("test.md", []byte("# Hello\n"))
	if err != nil {
		t.Fatal(err)
	}

	effective := map[string]config.RuleCfg{
		"mock-rule": {Enabled: true},
	}
	rules := []rule.Rule{&mockRule{id: "TM999", name: "mock-rule"}}

	diags, errs := CheckRules(f, rules, effective)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	if diags[0].RuleID != "TM999" {
		t.Errorf("expected RuleID TM999, got %s", diags[0].RuleID)
	}
}

func TestCheckRules_DisabledRuleSkipped(t *testing.T) {
	f, err := lint.NewFile("test.md", []byte("# Hello\n"))
	if err != nil {
		t.Fatal(err)
	}

	effective := map[string]config.RuleCfg{
		"mock-rule": {Enabled: false},
	}
	rules := []rule.Rule{&mockRule{id: "TM999", name: "mock-rule"}}

	diags, errs := CheckRules(f, rules, effective)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d", len(diags))
	}
}

func TestCheckRules_UnconfiguredRuleSkipped(t *testing.T) {
	f, err := lint.NewFile("test.md", []byte("# Hello\n"))
	if err != nil {
		t.Fatal(err)
	}

	effective := map[string]config.RuleCfg{}
	rules := []rule.Rule{&mockRule{id: "TM999", name: "mock-rule"}}

	diags, errs := CheckRules(f, rules, effective)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d", len(diags))
	}
}

func TestCheckRules_AppliesSettings(t *testing.T) {
	// 100-char line with max=120 should not trigger.
	line := strings.Repeat("a", 100) + "\n"
	f, err := lint.NewFile("test.md", []byte(line))
	if err != nil {
		t.Fatal(err)
	}

	effective := map[string]config.RuleCfg{
		"line-length": {
			Enabled:  true,
			Settings: map[string]any{"max": 120},
		},
	}
	rules := []rule.Rule{&configurableLengthRule{Max: 80}}

	diags, errs := CheckRules(f, rules, effective)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics with max=120, got %d", len(diags))
	}
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
	if err != nil {
		t.Fatal(err)
	}

	effective := map[string]config.RuleCfg{
		"bad-rule": {
			Enabled:  true,
			Settings: map[string]any{"key": "val"},
		},
	}
	rules := []rule.Rule{&mockConfigurableErrorRule{id: "TM900", name: "bad-rule"}}

	diags, errs := CheckRules(f, rules, effective)

	// The rule should be skipped (no diagnostics from it).
	if len(diags) != 0 {
		t.Errorf("expected 0 diagnostics, got %d: %v", len(diags), diags)
	}

	// The error should be returned in the errors slice.
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
	if !strings.Contains(errs[0].Error(), "bad settings") {
		t.Errorf("expected error to contain 'bad settings', got: %v", errs[0])
	}
}

func TestCheckRules_AdjustsLineOffset(t *testing.T) {
	f, err := lint.NewFileFromSource("test.md", []byte("---\ntitle: x\n---\n# Heading\n"), true)
	if err != nil {
		t.Fatal(err)
	}

	effective := map[string]config.RuleCfg{
		"mock-rule": {Enabled: true},
	}
	rules := []rule.Rule{&mockRule{id: "TM999", name: "mock-rule"}}

	diags, errs := CheckRules(f, rules, effective)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	// mockRule reports line 1; front matter has 3 lines, so adjusted = 4.
	if diags[0].Line != 4 {
		t.Errorf("expected adjusted line 4, got %d", diags[0].Line)
	}
}

// --- ConfigureRule tests ---

func TestConfigureRule_NoSettings(t *testing.T) {
	rl := &mockRule{id: "TM999", name: "mock-rule"}
	cfg := config.RuleCfg{Enabled: true, Settings: nil}

	got, err := ConfigureRule(rl, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != rl {
		t.Error("expected same rule instance when no settings")
	}
}

func TestConfigureRule_NonConfigurable(t *testing.T) {
	rl := &mockRule{id: "TM999", name: "mock-rule"}
	cfg := config.RuleCfg{Enabled: true, Settings: map[string]any{"key": "val"}}

	got, err := ConfigureRule(rl, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// mockRule does not implement Configurable, so the same instance is returned.
	if got != rl {
		t.Error("expected same rule instance for non-configurable rule")
	}
}

func TestConfigureRule_AppliesSettings(t *testing.T) {
	rl := &configurableLengthRule{Max: 80}
	cfg := config.RuleCfg{
		Enabled:  true,
		Settings: map[string]any{"max": 120},
	}

	got, err := ConfigureRule(rl, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should be a different instance (cloned).
	if got == rl {
		t.Error("expected a cloned rule, got same instance")
	}

	// The cloned rule should have max=120 applied.
	cloned, ok := got.(*configurableLengthRule)
	if !ok {
		t.Fatalf("expected *configurableLengthRule, got %T", got)
	}
	if cloned.Max != 120 {
		t.Errorf("expected Max=120, got %d", cloned.Max)
	}

	// Original should be unchanged.
	if rl.Max != 80 {
		t.Errorf("original Max changed to %d, want 80", rl.Max)
	}
}

func TestConfigureRule_ApplySettingsError(t *testing.T) {
	rl := &mockConfigurableErrorRule{id: "TM900", name: "bad-rule"}
	cfg := config.RuleCfg{
		Enabled:  true,
		Settings: map[string]any{"key": "val"},
	}

	got, err := ConfigureRule(rl, cfg)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got != nil {
		t.Errorf("expected nil rule on error, got %v", got)
	}
	if !strings.Contains(err.Error(), "bad settings") {
		t.Errorf("expected error to contain 'bad settings', got: %v", err)
	}
}
