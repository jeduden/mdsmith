package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunner_ExplainAttachesProvenanceToDiagnostics covers the runner's
// attachExplanations path: when Explain is true, each diagnostic emitted
// for a file gets per-leaf source info from the merged config.
func TestRunner_ExplainAttachesProvenanceToDiagnostics(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	require.NoError(t, os.WriteFile(path, []byte("# Hello\n"), 0o644))

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-rule": {Enabled: true, Settings: map[string]any{"max": 80}},
		},
	}

	runner := &Runner{
		Config:  cfg,
		Rules:   []rule.Rule{&mockRule{id: "MDS999", name: "mock-rule"}},
		Explain: true,
	}
	result := runner.Run([]string{path})
	require.Len(t, result.Diagnostics, 1)

	exp := result.Diagnostics[0].Explanation
	require.NotNil(t, exp, "explanation must be populated when Explain is true")
	assert.Equal(t, "mock-rule", exp.Rule)

	var sawMax bool
	for _, l := range exp.Leaves {
		if l.Path == "settings.max" {
			sawMax = true
			assert.Equal(t, "default", l.Source)
			assert.Equal(t, 80, l.Value)
		}
	}
	assert.True(t, sawMax, "settings.max leaf must appear in the explanation")
}

// TestRunner_ExplainSkipsDiagnosticsForUnknownRule covers the branch in
// attachExplanations that skips rules absent from the resolved config.
func TestRunner_ExplainSkipsDiagnosticsForUnknownRule(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	require.NoError(t, os.WriteFile(path, []byte("# Hello\n"), 0o644))

	// Rule is enabled (so it runs) but we attach explanations using a config
	// where the rule name is missing from cfg.Rules — simulating a rule that
	// emits a diagnostic with a RuleName not tracked by config.
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-rule": {Enabled: true},
		},
	}

	runner := &Runner{
		Config:  cfg,
		Rules:   []rule.Rule{&mockRule{id: "MDS999", name: "mock-rule"}},
		Explain: true,
	}
	result := runner.Run([]string{path})
	require.Len(t, result.Diagnostics, 1)
	// Manually overwrite the diagnostic's RuleName to one missing from cfg.Rules
	// and re-run attachExplanations to exercise the skip branch.
	result.Diagnostics[0].RuleName = "absent-rule"
	result.Diagnostics[0].Explanation = nil
	runner.attachExplanations(result.Diagnostics, path, nil)
	assert.Nil(t, result.Diagnostics[0].Explanation,
		"unknown rule should leave Explanation nil")
}

// TestRunner_RunSourceExplainAttachesProvenance covers the RunSource
// branch when Explain is set.
func TestRunner_RunSourceExplainAttachesProvenance(t *testing.T) {
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-rule": {Enabled: true},
		},
	}
	runner := &Runner{
		Config:  cfg,
		Rules:   []rule.Rule{&mockRule{id: "MDS999", name: "mock-rule"}},
		Explain: true,
	}
	result := runner.RunSource("<stdin>", []byte("# Hi\n"))
	require.Len(t, result.Diagnostics, 1)
	require.NotNil(t, result.Diagnostics[0].Explanation)
	assert.Equal(t, "mock-rule", result.Diagnostics[0].Explanation.Rule)
}
