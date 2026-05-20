package engine

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
	vlog "github.com/jeduden/mdsmith/internal/log"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// configTargetMock implements rule.Rule and rule.ConfigTarget so the
// markdownRulesFrom filter branch can be exercised both ways.
type configTargetMock struct {
	id, name string
	isCfg    bool
}

func (r *configTargetMock) ID() string                           { return r.id }
func (r *configTargetMock) Name() string                         { return r.name }
func (r *configTargetMock) Category() string                     { return "test" }
func (r *configTargetMock) Check(_ *lint.File) []lint.Diagnostic { return nil }
func (r *configTargetMock) IsConfigFileRule() bool               { return r.isCfg }

func TestFilterIgnored(t *testing.T) {
	r := &Runner{Config: &config.Config{Ignore: []string{"vendor/**", "**/skip.md"}}}
	in := []string{"a.md", "vendor/lib.md", "b.md", "x/skip.md", "c.md"}
	assert.Equal(t, []string{"a.md", "b.md", "c.md"}, r.filterIgnored(in),
		"ignored paths dropped, input order preserved")
}

func TestFilterIgnored_NoPatternsKeepsAll(t *testing.T) {
	r := &Runner{Config: &config.Config{}}
	in := []string{"a.md", "b.md"}
	assert.Equal(t, in, r.filterIgnored(in))
}

func TestCloneRules_IndependentInstances(t *testing.T) {
	orig := []rule.Rule{
		&mockRule{id: "MDS1", name: "m1"},
		&silentRule{id: "MDS2", name: "s2"},
	}
	cl := cloneRules(orig)
	require.Len(t, cl, len(orig))
	for i := range orig {
		assert.NotSame(t, orig[i], cl[i], "clone %d must be a distinct pointer", i)
		assert.Equal(t, orig[i].ID(), cl[i].ID())
		assert.Equal(t, orig[i].Name(), cl[i].Name())
	}
}

func TestCloneRules_Empty(t *testing.T) {
	assert.Empty(t, cloneRules(nil))
}

func TestMarkdownRulesFrom_NoConfigPathReturnsAll(t *testing.T) {
	rules := []rule.Rule{
		&mockRule{id: "A", name: "a"},
		&configTargetMock{id: "C", name: "c", isCfg: true},
	}
	assert.Equal(t, rules, markdownRulesFrom(rules, ""),
		"with no config path every rule is kept")
}

func TestMarkdownRulesFrom_FiltersConfigFileRules(t *testing.T) {
	md := &mockRule{id: "A", name: "a"}
	cfgRule := &configTargetMock{id: "C", name: "c", isCfg: true}
	nonCfg := &configTargetMock{id: "D", name: "d", isCfg: false}
	got := markdownRulesFrom([]rule.Rule{md, cfgRule, nonCfg}, "/x/.mdsmith.yml")
	assert.Equal(t, []rule.Rule{md, nonCfg}, got,
		"only IsConfigFileRule()==true rules are filtered out")
}

func TestLogRulesTo_DisabledLoggerNoOutput(t *testing.T) {
	var buf bytes.Buffer
	logRulesTo(&vlog.Logger{Enabled: false, W: &buf},
		[]rule.Rule{&mockRule{id: "M1", name: "m1"}},
		map[string]config.RuleCfg{"m1": {Enabled: true}})
	assert.Empty(t, buf.String(), "disabled logger writes nothing")
}

func TestLogRulesTo_LogsOnlyEnabledRules(t *testing.T) {
	var buf bytes.Buffer
	rules := []rule.Rule{
		&mockRule{id: "M1", name: "m1"}, // enabled  -> logged
		&mockRule{id: "M2", name: "m2"}, // disabled -> skipped
		&mockRule{id: "M3", name: "m3"}, // absent   -> skipped
	}
	eff := map[string]config.RuleCfg{"m1": {Enabled: true}, "m2": {Enabled: false}}
	logRulesTo(&vlog.Logger{Enabled: true, W: &buf}, rules, eff)
	assert.Equal(t, "rule: M1 m1\n", buf.String())
}

func TestRunFiles_SequentialMatchesParallel(t *testing.T) {
	dir := t.TempDir()
	var work []string
	for i := 0; i < 6; i++ {
		p := filepath.Join(dir, fmt.Sprintf("f%d.md", i))
		require.NoError(t, os.WriteFile(p, []byte("# H\n"), 0o644))
		work = append(work, p)
	}
	cfg := &config.Config{Rules: map[string]config.RuleCfg{"mock-rule": {Enabled: true}}}
	mk := func(c int) *Runner {
		return &Runner{
			Config:      cfg,
			Rules:       []rule.Rule{&mockRule{id: "MDS999", name: "mock-rule"}},
			Concurrency: c,
		}
	}
	seq := mk(1).runFiles(work) // workers<=1 branch
	par := mk(4).runFiles(work) // parallel + cloneRules branch
	require.Len(t, seq, len(work))
	require.Len(t, par, len(work))
	for i := range work {
		require.Len(t, seq[i].diags, 1)
		assert.Equal(t, seq[i].diags, par[i].diags, "outcome %d differs seq vs parallel", i)
		assert.Equal(t, work[i], seq[i].diags[0].File, "index must map to its own file")
	}
}

func TestLintFile_ReadErrorReturnsErrs(t *testing.T) {
	r := &Runner{Config: &config.Config{}}
	out := r.lintFile(filepath.Join(t.TempDir(), "missing.md"), nil, 1)
	assert.Empty(t, out.diags)
	require.Len(t, out.errs, 1)
	assert.Contains(t, out.errs[0].Error(), "reading")
}

func TestLintFile_HappyReturnsDiags(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.md")
	require.NoError(t, os.WriteFile(p, []byte("# H\n"), 0o644))
	r := &Runner{
		Config: &config.Config{Rules: map[string]config.RuleCfg{"mock-rule": {Enabled: true}}},
		Rules:  []rule.Rule{&mockRule{id: "MDS999", name: "mock-rule"}},
	}
	out := r.lintFile(p, r.Rules, 1)
	require.Len(t, out.diags, 1)
	assert.Empty(t, out.errs)
	assert.Equal(t, p, out.diags[0].File)
}
