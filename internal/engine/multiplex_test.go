package engine

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"
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
