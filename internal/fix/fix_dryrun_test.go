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

func TestFix_DryRun_WritesNothing(t *testing.T) {
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
		Config:  cfg,
		Rules:   []rule.Rule{&mockFixableRule{id: "MDS100", name: "mock-trailing"}},
		DryRun:  true,
	}

	result := fixer.Fix([]string{mdFile})
	require.Empty(t, result.Errors, "unexpected errors: %v", result.Errors)

	// File must be byte-identical after a dry run.
	got, err := os.ReadFile(mdFile)
	require.NoError(t, err)
	assert.Equal(t, original, got, "dry run must not modify the file")

	// Modified list must be empty.
	assert.Empty(t, result.Modified, "dry run must not report modified files")
}

func TestFix_DryRun_ReportsWouldFix(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "test.md")
	require.NoError(t, os.WriteFile(mdFile, []byte("# Hello  \nworld  \n"), 0o644))

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-trailing": {Enabled: true},
		},
	}
	fixer := &Fixer{
		Config:  cfg,
		Rules:   []rule.Rule{&mockFixableRule{id: "MDS100", name: "mock-trailing"}},
		DryRun:  true,
	}

	result := fixer.Fix([]string{mdFile})
	require.Empty(t, result.Errors)

	// WouldFix must record the total violations the real run would fix.
	require.Len(t, result.DryRunEntries, 1, "expected 1 dry-run entry")
	entry := result.DryRunEntries[0]
	assert.Equal(t, mdFile, entry.Path)
	assert.Greater(t, entry.WouldFix, 0, "expected WouldFix > 0")
	assert.Contains(t, entry.Rules, "MDS100", "expected MDS100 in rule list")
}

func TestFix_DryRun_NoChanges_NoEntry(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "clean.md")
	require.NoError(t, os.WriteFile(mdFile, []byte("# Hello\nworld\n"), 0o644))

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-trailing": {Enabled: true},
		},
	}
	fixer := &Fixer{
		Config:  cfg,
		Rules:   []rule.Rule{&mockFixableRule{id: "MDS100", name: "mock-trailing"}},
		DryRun:  true,
	}

	result := fixer.Fix([]string{mdFile})
	require.Empty(t, result.Errors)
	assert.Empty(t, result.DryRunEntries, "no dry-run entries for clean file")
}

func TestFix_DryRun_MultipleFiles(t *testing.T) {
	dir := t.TempDir()
	file1 := filepath.Join(dir, "a.md")
	file2 := filepath.Join(dir, "b.md")
	file3 := filepath.Join(dir, "c.md")
	require.NoError(t, os.WriteFile(file1, []byte("# A  \n"), 0o644))
	require.NoError(t, os.WriteFile(file2, []byte("# B\n"), 0o644))
	require.NoError(t, os.WriteFile(file3, []byte("# C  \n"), 0o644))

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-trailing": {Enabled: true},
		},
	}
	fixer := &Fixer{
		Config:  cfg,
		Rules:   []rule.Rule{&mockFixableRule{id: "MDS100", name: "mock-trailing"}},
		DryRun:  true,
	}

	result := fixer.Fix([]string{file1, file2, file3})
	require.Empty(t, result.Errors)

	// Only files with changes appear in DryRunEntries.
	assert.Len(t, result.DryRunEntries, 2, "expected entries for file1 and file3 only")

	// No files modified on disk.
	for _, f := range []string{file1, file2, file3} {
		content, err := os.ReadFile(f)
		require.NoError(t, err)
		assert.NotEmpty(t, content)
	}
	content1, _ := os.ReadFile(file1)
	assert.Equal(t, "# A  \n", string(content1), "file1 must be unmodified")
	content3, _ := os.ReadFile(file3)
	assert.Equal(t, "# C  \n", string(content3), "file3 must be unmodified")
}

func TestFix_DryRun_WouldFixCountMatchesRealRun(t *testing.T) {
	// Dry-run fix count must equal the pre-fix failure count from a real run.
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "test.md")
	require.NoError(t, os.WriteFile(mdFile, []byte("# Hello  \nworld  \n"), 0o644))

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-trailing": {Enabled: true},
		},
	}

	// Real run.
	realFixer := &Fixer{
		Config: cfg,
		Rules:  []rule.Rule{&mockFixableRule{id: "MDS100", name: "mock-trailing"}},
	}
	realResult := realFixer.Fix([]string{mdFile})
	require.Empty(t, realResult.Errors)

	// Restore file for dry run.
	require.NoError(t, os.WriteFile(mdFile, []byte("# Hello  \nworld  \n"), 0o644))

	dryFixer := &Fixer{
		Config:  cfg,
		Rules:   []rule.Rule{&mockFixableRule{id: "MDS100", name: "mock-trailing"}},
		DryRun:  true,
	}
	dryResult := dryFixer.Fix([]string{mdFile})
	require.Empty(t, dryResult.Errors)
	require.Len(t, dryResult.DryRunEntries, 1)

	// The WouldFix count should match what the real run fixed.
	// (real run had Failures=2, fixed both).
	assert.Equal(t, realResult.Failures, dryResult.DryRunEntries[0].WouldFix,
		"dry-run WouldFix must match real-run pre-fix failure count")
}

func TestFix_DryRun_ExitCodeMatchesRealRun_AllFixable(t *testing.T) {
	// When all issues are fixable, dry-run exit code = 0 (same as real run).
	// We test this by checking the Result is consistent with exit 0:
	// no remaining diagnostics.
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "test.md")
	require.NoError(t, os.WriteFile(mdFile, []byte("# Hello  \n"), 0o644))

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-trailing": {Enabled: true},
		},
	}
	fixer := &Fixer{
		Config:  cfg,
		Rules:   []rule.Rule{&mockFixableRule{id: "MDS100", name: "mock-trailing"}},
		DryRun:  true,
	}

	result := fixer.Fix([]string{mdFile})
	require.Empty(t, result.Errors)
	// Dry run computes diagnostics the same way the real run would.
	// All issues are fixable so no remaining diagnostics (exit 0).
	assert.Empty(t, result.Diagnostics, "all issues fixable: no remaining diagnostics expected")
}

func TestFix_DryRun_ExitCodeMatchesRealRun_WithUnfixable(t *testing.T) {
	// When some issues are not fixable, dry-run reports the same remaining
	// diagnostics as a real run (exit 1).
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "test.md")
	require.NoError(t, os.WriteFile(mdFile, []byte("# Hello  \n"), 0o644))

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"mock-trailing":   {Enabled: true},
			"mock-nonfixable": {Enabled: true},
		},
	}
	fixer := &Fixer{
		Config: cfg,
		Rules: []rule.Rule{
			&mockFixableRule{id: "MDS100", name: "mock-trailing"},
			&mockNonFixableRule{id: "MDS999", name: "mock-nonfixable"},
		},
		DryRun: true,
	}

	result := fixer.Fix([]string{mdFile})
	require.Empty(t, result.Errors)
	// Non-fixable issues remain, so exit code should be 1 (len(Diagnostics) > 0).
	assert.NotEmpty(t, result.Diagnostics, "non-fixable issue must remain in dry-run")
	assert.Equal(t, "MDS999", result.Diagnostics[0].RuleID)
}

func TestFix_DryRun_MultipleRulesTracked(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "test.md")
	// Has trailing spaces (MDS100) and tabs (MDS200).
	require.NoError(t, os.WriteFile(mdFile, []byte("# He\tllo  \n"), 0o644))

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
	require.Len(t, result.DryRunEntries, 1)
	entry := result.DryRunEntries[0]
	assert.Contains(t, entry.Rules, "MDS100")
	assert.Contains(t, entry.Rules, "MDS200")
}
