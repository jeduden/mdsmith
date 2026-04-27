package fix

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFix_ExplainAttachesProvenanceToRemainingDiagnostics covers the
// fixer's attachExplanations path: when Explain is true, each
// diagnostic remaining after the fix pass gets per-leaf source info.
func TestFix_ExplainAttachesProvenanceToRemainingDiagnostics(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "test.md")
	require.NoError(t, os.WriteFile(mdFile, []byte("# Hello\n"), 0o644))

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-nonfixable": {Enabled: true, Settings: map[string]any{"max": 80}},
		},
	}

	fixer := &Fixer{
		Config: cfg,
		Rules: []rule.Rule{
			&mockNonFixableRule{id: "MDS999", name: "mock-nonfixable"},
		},
		Explain: true,
	}
	result := fixer.Fix([]string{mdFile})
	require.Len(t, result.Diagnostics, 1)
	exp := result.Diagnostics[0].Explanation
	require.NotNil(t, exp, "explanation must be populated when Explain is true")
	assert.Equal(t, "mock-nonfixable", exp.Rule)

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

// TestFix_ExplainOmittedWhenFlagUnset ensures the fixer does not
// populate Diagnostic.Explanation when Explain is false.
func TestFix_ExplainOmittedWhenFlagUnset(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "test.md")
	require.NoError(t, os.WriteFile(mdFile, []byte("# Hello\n"), 0o644))

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-nonfixable": {Enabled: true},
		},
	}
	fixer := &Fixer{
		Config: cfg,
		Rules:  []rule.Rule{&mockNonFixableRule{id: "MDS999", name: "mock-nonfixable"}},
	}
	result := fixer.Fix([]string{mdFile})
	require.Len(t, result.Diagnostics, 1)
	assert.Nil(t, result.Diagnostics[0].Explanation,
		"Explanation must remain nil when Explain is false")
}
