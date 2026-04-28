package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// configTargetRule is a test rule that implements rule.ConfigTarget and always
// reports one diagnostic, regardless of the file path.
type configTargetRule struct {
	id   string
	name string
}

func (r *configTargetRule) ID() string             { return r.id }
func (r *configTargetRule) Name() string           { return r.name }
func (r *configTargetRule) Category() string       { return "meta" }
func (r *configTargetRule) IsConfigFileRule() bool { return true }
func (r *configTargetRule) Check(f *lint.File) []lint.Diagnostic {
	return []lint.Diagnostic{{
		File:     f.Path,
		Line:     1,
		Column:   1,
		RuleID:   r.id,
		RuleName: r.name,
		Severity: lint.Error,
		Message:  "config target violation",
	}}
}

// errorConfigTargetRule is a configTargetRule that also implements
// rule.Configurable so ConfigureRule can be forced to error via bad settings.
type errorConfigTargetRule struct {
	configTargetRule
}

func (r *errorConfigTargetRule) DefaultSettings() map[string]any { return nil }
func (r *errorConfigTargetRule) ApplySettings(_ map[string]any) error {
	return &applySettingsError{msg: "forced error"}
}

type applySettingsError struct{ msg string }

func (e *applySettingsError) Error() string { return e.msg }

// configTargetRuleClone satisfies rule.CloneRule requirements — it needs
// to implement the Cloneable interface. The engine uses rule.CloneRule which
// requires *Rule to implement encoding.BinaryMarshaler/Unmarshaler or just
// copies fields via the clone package. To keep tests simple, we use a
// configurable rule from the existing fakes.

// --- runConfigTargetRules tests ---

func TestRunConfigTargetRules_NoConfigPath_Skipped(t *testing.T) {
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"ct-rule": {Enabled: true},
		},
	}
	runner := &Runner{
		Config: cfg,
		Rules:  []rule.Rule{&configTargetRule{id: "MDS999", name: "ct-rule"}},
		// ConfigPath deliberately empty
	}
	result := runner.Run([]string{})
	assert.Empty(t, result.Diagnostics, "no config-target diagnostics when ConfigPath is empty")
	assert.Empty(t, result.Errors)
}

func TestRunConfigTargetRules_WithConfigPath_EmitsConfigDiagnostic(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(""), 0o644))

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"ct-rule": {Enabled: true},
		},
	}
	runner := &Runner{
		Config:     cfg,
		Rules:      []rule.Rule{&configTargetRule{id: "MDS999", name: "ct-rule"}},
		ConfigPath: cfgPath,
	}
	result := runner.Run([]string{})
	require.Len(t, result.Diagnostics, 1)
	assert.Equal(t, cfgPath, result.Diagnostics[0].File)
	assert.Equal(t, "MDS999", result.Diagnostics[0].RuleID)
}

func TestRunConfigTargetRules_DisabledRule_NodiagnosticEmitted(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(""), 0o644))

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"ct-rule": {Enabled: false},
		},
	}
	runner := &Runner{
		Config:     cfg,
		Rules:      []rule.Rule{&configTargetRule{id: "MDS999", name: "ct-rule"}},
		ConfigPath: cfgPath,
	}
	result := runner.Run([]string{})
	assert.Empty(t, result.Diagnostics)
}

func TestRunConfigTargetRules_RuleNotInEffective_Skipped(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(""), 0o644))

	// Rules map is empty — ct-rule has no entry, so it's not enabled.
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{},
	}
	runner := &Runner{
		Config:     cfg,
		Rules:      []rule.Rule{&configTargetRule{id: "MDS999", name: "ct-rule"}},
		ConfigPath: cfgPath,
	}
	result := runner.Run([]string{})
	assert.Empty(t, result.Diagnostics)
}

func TestRunConfigTargetRules_NonConfigTargetRule_NotRunAsConfigTarget(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(""), 0o644))

	// mockRule does not implement ConfigTarget — it must not be run as one.
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-rule": {Enabled: true},
		},
	}
	runner := &Runner{
		Config:     cfg,
		Rules:      []rule.Rule{&mockRule{id: "MDS999", name: "mock-rule"}},
		ConfigPath: cfgPath,
	}
	// Run with no markdown files so only config-target path executes.
	result := runner.Run([]string{})
	assert.Empty(t, result.Diagnostics, "non-ConfigTarget rule must not run in config-target pass")
}

func TestRunConfigTargetRules_ConfigureRuleError_AddsError(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(""), 0o644))

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"ct-rule": {
				Enabled:  true,
				Settings: map[string]any{"anything": "value"},
			},
		},
	}
	runner := &Runner{
		Config:     cfg,
		Rules:      []rule.Rule{&errorConfigTargetRule{configTargetRule{id: "MDS999", name: "ct-rule"}}},
		ConfigPath: cfgPath,
	}
	result := runner.Run([]string{})
	require.Len(t, result.Errors, 1)
	assert.Contains(t, result.Errors[0].Error(), "forced error")
}
