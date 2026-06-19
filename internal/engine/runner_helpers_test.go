package engine

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
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
	seq := mk(1).runFiles(work, lint.NewRunCache()) // workers<=1 branch
	par := mk(4).runFiles(work, lint.NewRunCache()) // parallel + cloneRules branch
	require.Len(t, seq, len(work))
	require.Len(t, par, len(work))
	for i := range work {
		require.Len(t, seq[i].diags, 1)
		assert.Equal(t, seq[i].diags, par[i].diags, "outcome %d differs seq vs parallel", i)
		assert.Equal(t, work[i], seq[i].diags[0].File, "index must map to its own file")
	}
}

// newRunResolve builds the per-run resolver lintFile expects, mirroring
// what runFiles constructs, for tests that call lintFile directly.
func (r *Runner) newRunResolve() runResolve {
	return runResolve{
		mdRules:   markdownRulesFrom(r.Rules, r.ConfigPath),
		catLookup: ruleCategoryLookup(r.Rules),
		effCache:  &effectiveCache{},
		confCache: map[string]configuredRules{},
	}
}

func TestLintFile_ReadErrorReturnsErrs(t *testing.T) {
	r := &Runner{Config: &config.Config{}}
	out := r.lintFile(filepath.Join(t.TempDir(), "missing.md"), 1, lint.NewRunCache(), r.newRunResolve())
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
	out := r.lintFile(p, 1, lint.NewRunCache(), r.newRunResolve())
	require.Len(t, out.diags, 1)
	assert.Empty(t, out.errs)
	assert.Equal(t, p, out.diags[0].File)
}

func TestEffectiveCached_MemoizesBySignature(t *testing.T) {
	r := &Runner{
		Config: &config.Config{Rules: map[string]config.RuleCfg{"mock-rule": {Enabled: true}}},
		Rules:  []rule.Rule{&mockRule{id: "MDS999", name: "mock-rule"}},
	}
	rr := r.newRunResolve()
	// No kinds and no overrides: both files share one signature, so the
	// second lookup must return the map the first one cached, not a copy.
	a, _ := r.effectiveCached("a.md", nil, nil, rr)
	b, _ := r.effectiveCached("b.md", nil, nil, rr)
	require.NotNil(t, a)
	assert.Equal(t, reflect.ValueOf(a).Pointer(), reflect.ValueOf(b).Pointer(),
		"a matching signature must return the memoized map instance")
}

// TestSortDiagnostics_TieBreaksByRuleID pins that sortDiagnostics produces a
// deterministic order even when two diagnostics share File, Line, Column, and
// Message but come from different rules. Without a RuleID tiebreak, the order
// of such a pair depends on the caller's input order — which differs between
// the parse-skip block-walk path and the full-parse node-walk path — so the
// Layer-0 equivalence assertions (TestParityConvention_DiagnosticsMatchFullParse,
// the corpus gate) that compare full ordered slices could fail spuriously.
func TestSortDiagnostics_TieBreaksByRuleID(t *testing.T) {
	// Same File/Line/Column/Message, different RuleID, fed in reverse order.
	diags := []lint.Diagnostic{
		{File: "a.md", Line: 1, Column: 1, RuleID: "MDS099", Message: "same"},
		{File: "a.md", Line: 1, Column: 1, RuleID: "MDS001", Message: "same"},
	}
	sortDiagnostics(diags)
	assert.Equal(t, "MDS001", diags[0].RuleID,
		"diagnostics equal on File/Line/Column/Message must order by RuleID")
	assert.Equal(t, "MDS099", diags[1].RuleID)
}
