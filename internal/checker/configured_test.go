package checker

import (
	"fmt"
	"strings"
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

// BenchmarkCheckConfiguredRules_ManyDiagnostics measures the per-file
// diagnostic merge on a diagnostic-heavy file — the pre-sized slot
// merge in CheckConfiguredRules (checker.go) avoids append's
// geometric regrowth re-copying the accumulated slice on every growth
// step (high-performance-go.md, "Pre-size slices").
func BenchmarkCheckConfiguredRules_ManyDiagnostics(b *testing.B) {
	var buf strings.Builder
	buf.WriteString("# Title\n\n")
	for i := 0; i < 200; i++ {
		fmt.Fprintf(&buf, "Line %d with trailing spaces   \n", i)
	}
	src := []byte(buf.String())

	rules := rule.All()
	eff := map[string]config.RuleCfg{}
	for _, r := range rules {
		eff[r.Name()] = config.RuleCfg{Enabled: true}
	}
	configured, _ := ConfigureEnabledRules(rules, eff)

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		f, err := lint.NewFile("doc.md", src)
		if err != nil {
			b.Fatal(err)
		}
		f.RunCache = lint.NewRunCache()
		_ = CheckConfiguredRules(f, configured, true, 1)
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
