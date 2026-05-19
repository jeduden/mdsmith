package engine

import (
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"

	// Force-load every production rule registration so the
	// migrated-equivalence test can look rules up by name. The
	// engine package itself never imports rules — only tests need
	// this — so the blank import lives here.
	_ "github.com/jeduden/mdsmith/internal/rules/all"
)

// mockNodeChecker implements rule.NodeChecker, emitting one
// diagnostic per Heading on entering — a pure per-node rule.
type mockNodeChecker struct {
	id, name string
}

func (m *mockNodeChecker) ID() string       { return m.id }
func (m *mockNodeChecker) Name() string     { return m.name }
func (m *mockNodeChecker) Category() string { return "test" }
func (m *mockNodeChecker) Check(f *lint.File) []lint.Diagnostic {
	return rule.WalkNodes(m, f)
}
func (m *mockNodeChecker) CheckNode(n ast.Node, entering bool, f *lint.File) []lint.Diagnostic {
	if !entering || n.Kind() != ast.KindHeading {
		return nil
	}
	return []lint.Diagnostic{{
		File: f.Path, Line: 1, Column: 1,
		RuleID: m.id, RuleName: m.name,
		Severity: lint.Warning, Message: "heading seen",
	}}
}

// plainView wraps a NodeChecker but exposes ONLY the Rule interface,
// so checkRules cannot detect the NodeChecker capability and falls
// back to calling Check sequentially — i.e. the pre-multiplex path.
type plainView struct{ nc *mockNodeChecker }

func (p plainView) ID() string                           { return p.nc.id }
func (p plainView) Name() string                         { return p.nc.name }
func (p plainView) Category() string                     { return "test" }
func (p plainView) Check(f *lint.File) []lint.Diagnostic { return p.nc.Check(f) }

// TestCheckRules_MultiplexedEqualsSequential pins that routing a
// NodeChecker through the engine's single shared ast.Walk produces a
// byte-identical diagnostic slice — same content AND order — to
// running every rule's Check sequentially. plainView hides the
// NodeChecker capability to compute the pre-multiplex reference with
// the exact same code path, so any divergence is the multiplexing
// itself.
func TestCheckRules_MultiplexedEqualsSequential(t *testing.T) {
	src := []byte("# A\n\npara one\n\n## B\n\npara two\n\n### C\n")
	f1, err := lint.NewFile("doc.md", src)
	require.NoError(t, err)
	f2, err := lint.NewFile("doc.md", src)
	require.NoError(t, err)

	nc := &mockNodeChecker{id: "MDSX02", name: "mux-stub"}
	eff := map[string]config.RuleCfg{
		"mock-a":   {Enabled: true},
		"mux-stub": {Enabled: true},
		"mock-b":   {Enabled: true},
	}

	// Sequential reference: NodeChecker hidden behind plainView.
	seq, errs1 := checkRules(f1, []rule.Rule{
		&mockRule{id: "MDA", name: "mock-a"},
		plainView{nc},
		&mockRule{id: "MDB", name: "mock-b"},
	}, eff, true)

	// Multiplexed: real NodeChecker, driven by the shared walk.
	mux, errs2 := checkRules(f2, []rule.Rule{
		&mockRule{id: "MDA", name: "mock-a"},
		nc,
		&mockRule{id: "MDB", name: "mock-b"},
	}, eff, true)

	require.Empty(t, errs1)
	require.Empty(t, errs2)
	assert.Equal(t, seq, mux,
		"multiplexed dispatch must be byte-identical to sequential Check")

	// The NodeChecker's diagnostics appear exactly once (3 headings),
	// and grouped between the two mock rules' single diagnostics.
	require.Len(t, mux, 5)
	assert.Equal(t, "MDA", mux[0].RuleID)
	assert.Equal(t, "MDSX02", mux[1].RuleID)
	assert.Equal(t, "MDSX02", mux[2].RuleID)
	assert.Equal(t, "MDSX02", mux[3].RuleID)
	assert.Equal(t, "MDB", mux[4].RuleID)
}

// TestCheckRules_MultipleNodeCheckersShareOneWalk pins that several
// NodeCheckers are all fed the same single walk and each still
// contributes its own contiguous, correctly ordered group.
func TestCheckRules_MultipleNodeCheckersShareOneWalk(t *testing.T) {
	f, err := lint.NewFile("doc.md", []byte("# H1\n\ntext\n\n## H2\n"))
	require.NoError(t, err)

	a := &mockNodeChecker{id: "AAA", name: "nc-a"}
	b := &mockNodeChecker{id: "BBB", name: "nc-b"}
	eff := map[string]config.RuleCfg{
		"nc-a": {Enabled: true},
		"nc-b": {Enabled: true},
	}

	diags, errs := checkRules(f, []rule.Rule{a, b}, eff, true)
	require.Empty(t, errs)
	require.Len(t, diags, 4, "2 headings x 2 rules")
	// nc-a's group first (rules order), then nc-b's group.
	assert.Equal(t, "AAA", diags[0].RuleID)
	assert.Equal(t, "AAA", diags[1].RuleID)
	assert.Equal(t, "BBB", diags[2].RuleID)
	assert.Equal(t, "BBB", diags[3].RuleID)
}

// hiddenNodeChecker wraps a real NodeChecker as a plain rule.Rule so
// the engine cannot fold it into the shared walk; its Check method
// still runs the rule's full per-node logic via rule.WalkNodes. Used
// to compute the pre-multiplex reference output for every migrated
// rule with the exact code path users would hit pre-plan-189.
type hiddenNodeChecker struct {
	nc rule.NodeChecker
}

func (h hiddenNodeChecker) ID() string                           { return h.nc.ID() }
func (h hiddenNodeChecker) Name() string                         { return h.nc.Name() }
func (h hiddenNodeChecker) Category() string                     { return h.nc.Category() }
func (h hiddenNodeChecker) Check(f *lint.File) []lint.Diagnostic { return h.nc.Check(f) }

// TestCheckRules_MigratedRulesEqualSequential pins that every
// migrated NodeChecker in the production rule set produces a
// byte-identical diagnostic slice whether routed through the
// multiplexed walk or the legacy per-rule path. Each rule is tested
// in isolation so a failure points at exactly one rule.
func TestCheckRules_MigratedRulesEqualSequential(t *testing.T) {
	src := []byte(strings.Join([]string{
		"# A heading",
		"",
		"## A sub-heading",
		"",
		"Some paragraph with a link to <https://example.com> and **bold**.",
		"A bare URL: https://bare.example.com appears here.",
		"",
		"- item 1",
		"- item 2",
		"",
		"1. one",
		"2. two",
		"",
		"```",
		"unlanguaged code",
		"```",
		"",
		"```go",
		"func main() {}",
		"```",
		"",
		"---",
		"",
		"![](image.png)",
		"![alt](img.png)",
		"",
		"[click here](https://x.example.com)",
		"",
		"<div>html</div>",
		"",
		"A code span with `  spaces  ` inside.",
		"",
		"A heading with trailing period.",
		"",
		"#### A jumped heading",
		"",
		"[TOC]",
		"",
	}, "\n"))

	cases := []struct {
		name  string
		rules []rule.Rule
	}{
		{"MDS002", []rule.Rule{newRuleByName(t, "heading-style")}},
		{"MDS010", []rule.Rule{newRuleByName(t, "fenced-code-style")}},
		{"MDS011", []rule.Rule{newRuleByName(t, "fenced-code-language")}},
		{"MDS012", []rule.Rule{newRuleByName(t, "no-bare-urls")}},
		{"MDS013", []rule.Rule{newRuleByName(t, "blank-line-around-headings")}},
		{"MDS014", []rule.Rule{newRuleByName(t, "blank-line-around-lists")}},
		{"MDS015", []rule.Rule{newRuleByName(t, "blank-line-around-fenced-code")}},
		{"MDS016", []rule.Rule{newRuleByName(t, "list-indent")}},
		{"MDS017", []rule.Rule{newRuleByName(t, "no-trailing-punctuation-in-heading")}},
		{"MDS018", []rule.Rule{newRuleByName(t, "no-emphasis-as-heading")}},
		{"MDS031", []rule.Rule{newRuleByName(t, "unclosed-code-block")}},
		{"MDS032", []rule.Rule{newRuleByName(t, "no-empty-alt-text")}},
		{"MDS035", []rule.Rule{newRuleByName(t, "toc-directive")}},
		{"MDS041", []rule.Rule{newRuleByName(t, "no-inline-html")}},
		{"MDS042", []rule.Rule{newRuleByName(t, "emphasis-style")}},
		{"MDS044", []rule.Rule{newRuleByName(t, "horizontal-rule-style")}},
		{"MDS045", []rule.Rule{newRuleByName(t, "list-marker-style")}},
		{"MDS046", []rule.Rule{newRuleByName(t, "ordered-list-numbering")}},
		{"MDS049", []rule.Rule{newRuleByName(t, "no-space-in-link-text")}},
		{"MDS052", []rule.Rule{newRuleByName(t, "no-space-in-code-spans")}},
		{"MDS055", []rule.Rule{newRuleByName(t, "forbidden-paragraph-starts")}},
		{"MDS056", []rule.Rule{newRuleByName(t, "forbidden-text")}},
		{"MDS061", []rule.Rule{newRuleByName(t, "list-marker-space")}},
		{"MDS063", []rule.Rule{newRuleByName(t, "descriptive-link-text")}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rl := tc.rules[0]
			nc, ok := rl.(rule.NodeChecker)
			require.True(t, ok, "%s expected to implement NodeChecker", rl.Name())

			eff := map[string]config.RuleCfg{
				rl.Name(): {Enabled: true},
			}

			f1, err := lint.NewFile("doc.md", src)
			require.NoError(t, err)
			f2, err := lint.NewFile("doc.md", src)
			require.NoError(t, err)

			seq, errs1 := checkRules(f1, []rule.Rule{hiddenNodeChecker{nc}}, eff, true)
			mux, errs2 := checkRules(f2, []rule.Rule{rl}, eff, true)

			require.Empty(t, errs1)
			require.Empty(t, errs2)
			assert.Equal(t, seq, mux,
				"%s: multiplexed output must equal sequential", rl.Name())
		})
	}
}

// newRuleByName fetches a registered rule by its Name(). Each
// production registration creates a stateless instance suitable for
// tests; the cloning the engine does is per-Check, so there is no
// risk of contaminating other tests.
func newRuleByName(t *testing.T, name string) rule.Rule {
	t.Helper()
	for _, r := range rule.All() {
		if r.Name() == name {
			return rule.CloneInstance(r)
		}
	}
	t.Fatalf("rule %q not registered", name)
	return nil
}
