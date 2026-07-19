package fix

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/foreignregion"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/maxfilelength"
	"github.com/jeduden/mdsmith/internal/rules/notrailingspaces"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const apmStart = "<!-- apm:start -->"
const apmEnd = "<!-- apm:end -->"

func apmConfig(rules map[string]config.RuleCfg) *config.Config {
	return &config.Config{
		Rules: rules,
		ForeignRegions: []config.ForeignRegion{
			{Start: apmStart, End: apmEnd},
		},
	}
}

// TestFix_ForeignRegionBytesUnchanged proves the fix pipeline leaves the
// bytes between a declared marker pair untouched — including the
// otherwise-fixable trailing spaces inside — while still trimming the
// trailing spaces outside the region.
func TestFix_ForeignRegionBytesUnchanged(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")
	src := "# Title\n\noutside line   \n\n" +
		apmStart + "\ninside line   \n" + apmEnd + "\n"
	require.NoError(t, os.WriteFile(path, []byte(src), 0o644))

	fixer := &Fixer{
		Config: apmConfig(map[string]config.RuleCfg{
			"no-trailing-spaces": {Enabled: true},
		}),
		Rules: []rule.Rule{&notrailingspaces.Rule{}},
	}

	result := fixer.Fix([]string{path})
	require.Empty(t, result.Errors, "unexpected errors: %v", result.Errors)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	want := "# Title\n\noutside line\n\n" +
		apmStart + "\ninside line   \n" + apmEnd + "\n"
	assert.Equal(t, want, string(got),
		"trailing spaces outside the region must be trimmed; inside must be preserved verbatim")
}

// TestFix_ForeignRegionSuppressesDiagnostics proves a style-rule
// violation inside the region emits no diagnostic while the same
// violation outside it still does.
func TestFix_ForeignRegionSuppressesDiagnostics(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")
	// Only an inside violation: after fixing the outside (none here),
	// the post-fix check must report nothing from inside the region.
	src := "# Title\n\n" + apmStart + "\ninside line   \n" + apmEnd + "\n"
	require.NoError(t, os.WriteFile(path, []byte(src), 0o644))

	fixer := &Fixer{
		Config: apmConfig(map[string]config.RuleCfg{
			"no-trailing-spaces": {Enabled: true},
		}),
		Rules: []rule.Rule{&notrailingspaces.Rule{}},
	}
	result := fixer.Fix([]string{path})
	require.Empty(t, result.Errors)
	for _, d := range result.Diagnostics {
		if d.RuleName == "no-trailing-spaces" {
			t.Errorf("trailing-spaces diagnostic surfaced from inside foreign region: line %d", d.Line)
		}
	}
	assert.Zero(t, result.Failures,
		"pre-fix Failures must exclude the in-region trailing-space violation")
}

// TestFix_ForeignRegionWholeFileRulesStillCount proves MDS022 counts the
// bytes inside the region toward file length.
func TestFix_ForeignRegionWholeFileRulesStillCount(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")
	// 3 lines outside + a 4-line region = 7 content lines. Set max to 5
	// so the file is over budget only because the region's lines count.
	src := "# Title\n\nbody\n" + apmStart + "\na\nb\n" + apmEnd + "\n"
	require.NoError(t, os.WriteFile(path, []byte(src), 0o644))

	maxRule := &maxfilelength.Rule{Max: 5}
	fixer := &Fixer{
		Config: apmConfig(map[string]config.RuleCfg{
			"max-file-length": {Enabled: true},
		}),
		Rules: []rule.Rule{maxRule},
	}
	result := fixer.Fix([]string{path})
	require.Empty(t, result.Errors)

	found := false
	for _, d := range result.Diagnostics {
		if d.RuleID == "MDS022" {
			found = true
		}
	}
	assert.True(t, found,
		"MDS022 must still fire — the region's lines count toward file length")
}

// TestFix_ForeignRegionMalformedDiagnostic proves a start marker with no
// matching end produces an MDS073 diagnostic.
func TestFix_ForeignRegionMalformedDiagnostic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")
	src := "# Title\n\n" + apmStart + "\ndangling body\n"
	require.NoError(t, os.WriteFile(path, []byte(src), 0o644))

	fixer := &Fixer{
		Config: apmConfig(map[string]config.RuleCfg{
			"no-trailing-spaces": {Enabled: true},
		}),
		Rules: []rule.Rule{&notrailingspaces.Rule{}},
	}
	result := fixer.Fix([]string{path})
	require.Empty(t, result.Errors)

	found := false
	for _, d := range result.Diagnostics {
		if d.RuleID == foreignregion.RuleID {
			found = true
			assert.Contains(t, d.Message, "no matching end")
			assert.Equal(t, 3, d.Line)
		}
	}
	assert.True(t, found, "expected a malformed foreign-region diagnostic (MDS073)")
}

var _ = lint.File{}
