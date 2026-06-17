package checker

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	_ "github.com/jeduden/mdsmith/internal/rules/all"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCheckConfiguredRules_MatchesCheckRules pins the cached configured
// path byte-identical to the per-file CheckRulesWithIntraFile path.
func TestCheckConfiguredRules_MatchesCheckRules(t *testing.T) {
	src := []byte("# Title\n\nSome  text with trailing spaces   \n\n## Next\nNo blank before.\n")
	f, err := lint.NewFile("doc.md", src)
	require.NoError(t, err)
	f.RunCache = lint.NewRunCache()
	f2, err := lint.NewFile("doc.md", src)
	require.NoError(t, err)
	f2.RunCache = lint.NewRunCache()

	rules := rule.All()
	eff := map[string]config.RuleCfg{}
	for _, r := range rules {
		eff[r.Name()] = config.RuleCfg{Enabled: true}
	}

	want, wantErrs := CheckRulesWithIntraFile(f, rules, eff, true, 1)

	configured, cErrs := ConfigureEnabledRules(rules, eff)
	got := CheckConfiguredRules(f2, configured, true, 1)

	assert.Equal(t, len(wantErrs), len(cErrs), "config error count matches")
	require.Equal(t, len(want), len(got), "diagnostic count matches")
	for i := range want {
		assert.Equal(t, want[i].RuleID, got[i].RuleID)
		assert.Equal(t, want[i].Line, got[i].Line)
		assert.Equal(t, want[i].Column, got[i].Column)
		assert.Equal(t, want[i].Message, got[i].Message)
	}
}

// TestConfigureEnabledRules_ConfiguresOnce verifies the configured slice
// can be reused: configuring once yields instances reusable across files.
func TestConfigureEnabledRules_ConfiguresOnce(t *testing.T) {
	rules := rule.All()
	eff := map[string]config.RuleCfg{}
	for _, r := range rules {
		eff[r.Name()] = config.RuleCfg{Enabled: true}
	}
	a, _ := ConfigureEnabledRules(rules, eff)
	b, _ := ConfigureEnabledRules(rules, eff)
	require.Equal(t, len(a), len(b))
	assert.NotEmpty(t, a)
}
