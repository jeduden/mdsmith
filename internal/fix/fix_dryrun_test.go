package fix

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/rule"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDryRun_LeavesFileUnchanged verifies that a dry-run does not write
// any bytes to disk regardless of what fixes would apply.
func TestDryRun_LeavesFileUnchanged(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "test.md")
	original := []byte("# Hello  \nworld  \n")
	require.NoError(t, os.WriteFile(mdFile, original, 0o644))

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-trailing": {Enabled: true},
		},
	}
	fixer := &Fixer{
		Config: cfg,
		Rules:  []rule.Rule{&mockFixableRule{id: "MDS100", name: "mock-trailing"}},
		DryRun: true,
	}

	result := fixer.Fix([]string{mdFile})
	require.Empty(t, result.Errors)

	got, err := os.ReadFile(mdFile)
	require.NoError(t, err)
	assert.True(t, bytes.Equal(original, got), "dry-run must not modify the file")
}

// TestDryRun_WouldFixCountMatchesRealRun checks that WouldFix equals the
// number of pre-fix diagnostics a real run sees, and that Modified is empty.
func TestDryRun_WouldFixCountMatchesRealRun(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "test.md")
	content := []byte("# Hello  \nworld  \n")
	require.NoError(t, os.WriteFile(mdFile, content, 0o644))

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-trailing": {Enabled: true},
		},
	}

	// Real run to get baseline fix count.
	realFile := filepath.Join(dir, "real.md")
	require.NoError(t, os.WriteFile(realFile, content, 0o644))
	realFixer := &Fixer{
		Config: cfg,
		Rules:  []rule.Rule{&mockFixableRule{id: "MDS100", name: "mock-trailing"}},
	}
	realResult := realFixer.Fix([]string{realFile})
	require.Empty(t, realResult.Errors)

	// Dry-run on same content.
	dryFixer := &Fixer{
		Config: cfg,
		Rules:  []rule.Rule{&mockFixableRule{id: "MDS100", name: "mock-trailing"}},
		DryRun: true,
	}
	dryResult := dryFixer.Fix([]string{mdFile})
	require.Empty(t, dryResult.Errors)

	assert.Equal(t, realResult.Failures, dryResult.WouldFix,
		"WouldFix should equal pre-fix failure count from real run")
	assert.Empty(t, dryResult.Modified, "Modified must be empty on dry-run")
}

// TestDryRun_WouldFixRulesListed checks that WouldFixRules is populated with
// the rule IDs that would have applied fixes.
func TestDryRun_WouldFixRulesListed(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "test.md")
	require.NoError(t, os.WriteFile(mdFile, []byte("# Hello  \nhello\t \n"), 0o644))

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-trailing": {Enabled: true},
			"mock-tabs":     {Enabled: true},
		},
	}
	fixer := &Fixer{
		Config: cfg,
		Rules: []rule.Rule{
			&mockFixableRule{id: "MDS100", name: "mock-trailing"},
			&mockFixableRuleB{id: "MDS200", name: "mock-tabs"},
		},
		DryRun: true,
	}
	result := fixer.Fix([]string{mdFile})
	require.Empty(t, result.Errors)

	assert.Contains(t, result.WouldFixRules, "MDS100")
}

// TestDryRun_ExitCodeMatchesRealRun verifies that the exit code logic on the
// Result is identical between a dry-run and a real run on the same input.
// Specifically: if a non-fixable rule fires, both runs have Diagnostics.
func TestDryRun_ExitCodeMatchesRealRun(t *testing.T) {
	dir := t.TempDir()
	content := []byte("# Hello  \n")

	// File for real run.
	realFile := filepath.Join(dir, "real.md")
	require.NoError(t, os.WriteFile(realFile, content, 0o644))

	// File for dry-run (same content).
	dryFile := filepath.Join(dir, "dry.md")
	require.NoError(t, os.WriteFile(dryFile, content, 0o644))

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-trailing":   {Enabled: true},
			"mock-nonfixable": {Enabled: true},
		},
	}
	rules := []rule.Rule{
		&mockFixableRule{id: "MDS100", name: "mock-trailing"},
		&mockNonFixableRule{id: "MDS999", name: "mock-nonfixable"},
	}

	realFixer := &Fixer{Config: cfg, Rules: rules}
	realResult := realFixer.Fix([]string{realFile})

	dryFixer := &Fixer{Config: cfg, Rules: rules, DryRun: true}
	dryResult := dryFixer.Fix([]string{dryFile})

	// Both should have the same number of remaining (non-fixable) diagnostics.
	assert.Equal(t, len(realResult.Diagnostics), len(dryResult.Diagnostics),
		"remaining diagnostic count must match between real and dry-run")
}

// TestDryRun_NoFixesNoOutput verifies that a file with no fixable violations
// produces WouldFix=0 and no WouldFixRules entries for that file.
func TestDryRun_NoFixesNoOutput(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "clean.md")
	require.NoError(t, os.WriteFile(mdFile, []byte("# Clean\n"), 0o644))

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-trailing": {Enabled: true},
		},
	}
	fixer := &Fixer{
		Config: cfg,
		Rules:  []rule.Rule{&mockFixableRule{id: "MDS100", name: "mock-trailing"}},
		DryRun: true,
	}
	result := fixer.Fix([]string{mdFile})
	require.Empty(t, result.Errors)

	assert.Equal(t, 0, result.WouldFix)
	assert.Empty(t, result.WouldFixRules)
	assert.Empty(t, result.Modified)
}
