package engine

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// slowRule sleeps inside Check so the test can observe concurrency.
// It also records the maximum observed concurrency via an atomic
// counter, so the "honors cap" test can verify the cap was not
// exceeded. emitAtCol differentiates diagnostics from sibling
// instances so order assertions can match a specific instance.
type slowRule struct {
	id, name  string
	sleep     time.Duration
	current   *atomic.Int32
	peak      *atomic.Int32
	emitAtCol int
}

func (s *slowRule) ID() string       { return s.id }
func (s *slowRule) Name() string     { return s.name }
func (s *slowRule) Category() string { return "test" }
func (s *slowRule) Check(f *lint.File) []lint.Diagnostic {
	if s.current != nil {
		c := s.current.Add(1)
		for {
			peak := s.peak.Load()
			if c <= peak {
				break
			}
			if s.peak.CompareAndSwap(peak, c) {
				break
			}
		}
		defer s.current.Add(-1)
	}
	if s.sleep > 0 {
		time.Sleep(s.sleep)
	}
	col := s.emitAtCol
	if col == 0 {
		col = 1
	}
	return []lint.Diagnostic{{
		File:     f.Path,
		Line:     1,
		Column:   col,
		RuleID:   s.id,
		RuleName: s.name,
		Severity: lint.Warning,
		Message:  s.name + " says hello",
	}}
}

// TestCheckRules_ParallelEqualsSequential pins that running the
// non-NodeChecker slots concurrently produces a byte-identical
// diagnostic slice to the serial path.
func TestCheckRules_ParallelEqualsSequential(t *testing.T) {
	src := []byte("# Hello\n\nA paragraph.\n")
	f1, err := lint.NewFile("doc.md", src)
	require.NoError(t, err)
	f2, err := lint.NewFile("doc.md", src)
	require.NoError(t, err)

	rules := []rule.Rule{
		&mockRule{id: "MDA", name: "rule-a"},
		&mockRule{id: "MDB", name: "rule-b"},
		&mockRule{id: "MDC", name: "rule-c"},
	}
	eff := map[string]config.RuleCfg{
		"rule-a": {Enabled: true},
		"rule-b": {Enabled: true},
		"rule-c": {Enabled: true},
	}

	seq, errs1 := checkRulesWithIntraFile(f1, rules, eff, true, 1)
	par, errs2 := checkRulesWithIntraFile(f2, rules, eff, true, 4)

	require.Empty(t, errs1)
	require.Empty(t, errs2)
	assert.Equal(t, seq, par,
		"parallel intra-file dispatch must be byte-identical to sequential")
}

// TestCheckRules_ParallelRespectsRulesOrder pins that diagnostic
// groups emerge in rules order even though the goroutines that fill
// them complete in arbitrary order. The slow first rule would emit
// last under any goroutine-finish-order scheme, so passing this test
// requires the engine to write to a per-slot index and concatenate
// at the end.
func TestCheckRules_ParallelRespectsRulesOrder(t *testing.T) {
	src := []byte("# Hello\n")
	f, err := lint.NewFile("doc.md", src)
	require.NoError(t, err)

	slow := &slowRule{id: "MDS", name: "slow", sleep: 30 * time.Millisecond}
	fast := &mockRule{id: "MDF", name: "fast"}
	rules := []rule.Rule{slow, fast}
	eff := map[string]config.RuleCfg{
		"slow": {Enabled: true},
		"fast": {Enabled: true},
	}

	diags, errs := checkRulesWithIntraFile(f, rules, eff, true, 4)
	require.Empty(t, errs)
	require.Len(t, diags, 2)
	assert.Equal(t, "MDS", diags[0].RuleID, "slow rule's group should come first (rules order)")
	assert.Equal(t, "MDF", diags[1].RuleID, "fast rule's group should come second")
}

// TestCheckRules_ParallelHonorsCap pins that no more than `cap`
// rule.Check calls run concurrently at any moment. Each slow rule
// increments a shared counter on entry and decrements on exit; a
// shared peak watermark records the maximum it ever held.
func TestCheckRules_ParallelHonorsCap(t *testing.T) {
	const limit = 2
	src := []byte("# Hello\n")
	f, err := lint.NewFile("doc.md", src)
	require.NoError(t, err)

	var current, peak atomic.Int32
	rules := []rule.Rule{
		&slowRule{id: "S1", name: "s1", sleep: 30 * time.Millisecond, current: &current, peak: &peak, emitAtCol: 1},
		&slowRule{id: "S2", name: "s2", sleep: 30 * time.Millisecond, current: &current, peak: &peak, emitAtCol: 2},
		&slowRule{id: "S3", name: "s3", sleep: 30 * time.Millisecond, current: &current, peak: &peak, emitAtCol: 3},
		&slowRule{id: "S4", name: "s4", sleep: 30 * time.Millisecond, current: &current, peak: &peak, emitAtCol: 4},
		&slowRule{id: "S5", name: "s5", sleep: 30 * time.Millisecond, current: &current, peak: &peak, emitAtCol: 5},
	}
	eff := map[string]config.RuleCfg{
		"s1": {Enabled: true},
		"s2": {Enabled: true},
		"s3": {Enabled: true},
		"s4": {Enabled: true},
		"s5": {Enabled: true},
	}

	diags, errs := checkRulesWithIntraFile(f, rules, eff, true, limit)
	require.Empty(t, errs)
	require.Len(t, diags, 5)
	assert.LessOrEqual(t, int(peak.Load()), limit, "peak concurrency must not exceed cap=%d", limit)
}

// TestCheckRules_ParallelCapOne forces serial dispatch even when the
// caller passes cap=1, matching the documented "1 = disabled" knob.
func TestCheckRules_ParallelCapOne(t *testing.T) {
	src := []byte("# Hello\n")
	f, err := lint.NewFile("doc.md", src)
	require.NoError(t, err)

	var current, peak atomic.Int32
	rules := []rule.Rule{
		&slowRule{id: "S1", name: "s1", sleep: 5 * time.Millisecond, current: &current, peak: &peak},
		&slowRule{id: "S2", name: "s2", sleep: 5 * time.Millisecond, current: &current, peak: &peak},
		&slowRule{id: "S3", name: "s3", sleep: 5 * time.Millisecond, current: &current, peak: &peak},
	}
	eff := map[string]config.RuleCfg{
		"s1": {Enabled: true},
		"s2": {Enabled: true},
		"s3": {Enabled: true},
	}

	diags, errs := checkRulesWithIntraFile(f, rules, eff, true, 1)
	require.Empty(t, errs)
	require.Len(t, diags, 3)
	assert.Equal(t, int32(1), peak.Load(), "cap=1 must hold concurrency to 1")
}

// TestResolveIntraFileWorkers covers the auto-cap formula and the
// explicit-knob overrides.
func TestResolveIntraFileWorkers(t *testing.T) {
	cases := []struct {
		name      string
		setting   int
		gomaxproc int
		fileWk    int
		want      int
	}{
		{"explicit-1-is-off", 1, 16, 4, 1},
		{"explicit-n-passthrough", 3, 16, 4, 3},
		{"auto-with-headroom", 0, 16, 4, 4}, // 16/4=4
		{"auto-no-headroom", 0, 16, 16, 1},  // 16/16=1
		{"auto-tight", 0, 8, 3, 2},          // 8/3=2 (integer div)
		{"auto-single-worker", 0, 8, 1, 8},  // 8/1=8
		{"auto-zero-workers", 0, 8, 0, 8},   // guard against div0
		{"auto-negative-workers", 0, 8, -1, 8},
		{"auto-negative-gomaxproc", 0, 0, 4, 1}, // floor at 1
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveIntraFileWorkersFor(tc.setting, tc.gomaxproc, tc.fileWk)
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestRunner_IntraFileConcurrencyByteIdentical pins that diagnostic
// output through Runner.Run is identical regardless of the
// IntraFileConcurrency knob value, over a multi-file corpus that
// exercises the production rule set. All three runs share one temp
// dir so the absolute paths in d.File match byte-for-byte.
func TestRunner_IntraFileConcurrencyByteIdentical(t *testing.T) {
	src := strings.Join([]string{
		"# Title",
		"",
		"## Section",
		"",
		"Paragraph one with a link <https://example.com> here.",
		"",
		"- item 1",
		"- item 2",
		"",
		"```",
		"unlanguaged",
		"```",
		"",
	}, "\n")

	files := map[string]string{
		"a.md": src,
		"b.md": src + "\n## Extra\n",
		"c.md": src + "\n\n![](x.png)\n",
	}
	cfg := config.Defaults()
	dir := t.TempDir()
	paths := make([]string, 0, len(files))
	for name, body := range files {
		p := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
		paths = append(paths, p)
	}
	sort.Strings(paths)

	run := func(intraCap int) []lint.Diagnostic {
		runner := &Runner{
			Config:               cfg,
			Rules:                rule.All(),
			StripFrontMatter:     true,
			RootDir:              dir,
			SkipSourceContext:    true,
			IntraFileConcurrency: intraCap,
		}
		return runner.Run(paths).Diagnostics
	}

	lint1 := run(0)
	lint2 := run(1)
	lint3 := run(4)

	assert.Equal(t, lint1, lint2, "auto vs cap=1 must produce identical diagnostics")
	assert.Equal(t, lint1, lint3, "auto vs cap=4 must produce identical diagnostics")
}
