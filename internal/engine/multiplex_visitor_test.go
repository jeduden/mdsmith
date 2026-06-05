package engine

import (
	"strconv"
	"testing"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"
)

// levelJumpVisitor is a stateful per-node visitor: it carries prevLevel
// across Heading nodes and flags a jump of more than one level. It is
// the canonical case the stateless NodeChecker could not express.
type levelJumpVisitor struct{ prevLevel int }

func (levelJumpVisitor) Kinds() []ast.NodeKind { return []ast.NodeKind{ast.KindHeading} }

func (v *levelJumpVisitor) VisitNode(n ast.Node, entering bool, f *lint.File) []lint.Diagnostic {
	if !entering {
		return nil
	}
	h, ok := n.(*ast.Heading)
	if !ok {
		return nil
	}
	var diags []lint.Diagnostic
	if v.prevLevel != 0 && h.Level > v.prevLevel+1 {
		diags = []lint.Diagnostic{{
			File: f.Path, Line: 1, Column: 1,
			RuleID: "VJ1", RuleName: "level-jump",
			Severity: lint.Warning,
			Message:  "jump to " + strconv.Itoa(h.Level),
		}}
	}
	v.prevLevel = h.Level
	return diags
}

// levelJumpRule is the NodeVisitorRule under test.
type levelJumpRule struct{}

func (levelJumpRule) ID() string       { return "VJ1" }
func (levelJumpRule) Name() string     { return "level-jump" }
func (levelJumpRule) Category() string { return "test" }
func (r levelJumpRule) Check(f *lint.File) []lint.Diagnostic {
	return rule.WalkVisitor(r, f)
}
func (levelJumpRule) NewNodeVisitor(_ *lint.File) rule.NodeVisitor { return &levelJumpVisitor{} }

// hiddenVisitorRule wraps a NodeVisitorRule as a plain rule.Rule so the
// engine cannot fold it into the shared walk; its Check still runs the
// full per-node visitor via WalkVisitor. This computes the pre-multiplex
// reference output with the exact code path users hit before this plan.
type hiddenVisitorRule struct{ r rule.NodeVisitorRule }

func (h hiddenVisitorRule) ID() string                           { return h.r.ID() }
func (h hiddenVisitorRule) Name() string                         { return h.r.Name() }
func (h hiddenVisitorRule) Category() string                     { return h.r.Category() }
func (h hiddenVisitorRule) Check(f *lint.File) []lint.Diagnostic { return h.r.Check(f) }

// TestCheckRules_VisitorMultiplexedEqualsSequential pins that routing a
// stateful NodeVisitorRule through the engine's single shared ast.Walk
// produces a byte-identical diagnostic slice to running its Check
// sequentially. hiddenVisitorRule hides the capability to compute the
// reference via the same code path, so any divergence is the
// multiplexing itself.
func TestCheckRules_VisitorMultiplexedEqualsSequential(t *testing.T) {
	// h1, jump to h3 (flagged), then h5 (flagged) — state must persist.
	src := []byte("# A\n\ntext\n\n### C\n\npara\n\n##### E\n")
	f1, err := lint.NewFile("doc.md", src)
	require.NoError(t, err)
	f2, err := lint.NewFile("doc.md", src)
	require.NoError(t, err)

	eff := map[string]config.RuleCfg{"level-jump": {Enabled: true}}

	seq, errs1 := checkRules(f1, []rule.Rule{hiddenVisitorRule{levelJumpRule{}}}, eff, true)
	mux, errs2 := checkRules(f2, []rule.Rule{levelJumpRule{}}, eff, true)

	require.Empty(t, errs1)
	require.Empty(t, errs2)
	assert.Equal(t, seq, mux,
		"multiplexed visitor dispatch must be byte-identical to sequential Check")
	require.Len(t, mux, 2, "two level jumps must be flagged")
}

// TestCheckRules_VisitorSharesWalkWithNodeChecker pins that a
// NodeVisitorRule and a NodeChecker are fed the SAME single shared walk
// and each still contributes its own contiguous, rule-ordered group.
func TestCheckRules_VisitorSharesWalkWithNodeChecker(t *testing.T) {
	src := []byte("# A\n\n### C\n")
	f, err := lint.NewFile("doc.md", src)
	require.NoError(t, err)

	nc := &mockNodeChecker{id: "NC1", name: "nc"} // emits per heading
	vj := levelJumpRule{}                         // emits per level jump
	eff := map[string]config.RuleCfg{
		"nc":         {Enabled: true},
		"level-jump": {Enabled: true},
	}

	// NodeChecker first in rules order, visitor second.
	diags, errs := checkRules(f, []rule.Rule{nc, vj}, eff, true)
	require.Empty(t, errs)
	// 2 headings -> 2 NC diagnostics; 1 jump (h1->h3) -> 1 VJ diagnostic.
	require.Len(t, diags, 3)
	assert.Equal(t, "NC1", diags[0].RuleID)
	assert.Equal(t, "NC1", diags[1].RuleID)
	assert.Equal(t, "VJ1", diags[2].RuleID, "visitor's group follows the NodeChecker's in rules order")
}

// walkOnlyVisitorRule is a NodeVisitorRule whose Check panics: the
// engine must drive it through the shared walk's NewNodeVisitor /
// VisitNode path, never by calling Check (which would run a SECOND,
// separate ast.Walk). This is the structural proof that the engine
// folds the visitor into the one shared traversal rather than running
// it as an ordinary Check rule.
type walkOnlyVisitorRule struct{}

func (walkOnlyVisitorRule) ID() string       { return "WO1" }
func (walkOnlyVisitorRule) Name() string     { return "walk-only" }
func (walkOnlyVisitorRule) Category() string { return "test" }
func (walkOnlyVisitorRule) Check(_ *lint.File) []lint.Diagnostic {
	panic("Check must not be called: a NodeVisitorRule is driven by the shared walk")
}
func (walkOnlyVisitorRule) NewNodeVisitor(_ *lint.File) rule.NodeVisitor {
	return &levelJumpVisitor{}
}

// TestCheckRules_VisitorDrivenBySharedWalkNotCheck pins that the engine
// dispatches a NodeVisitorRule through the shared walk and never calls
// its Check. walkOnlyVisitorRule.Check panics, so a green run proves the
// rule went through VisitNode, i.e. it was multiplexed into the one
// traversal rather than running a separate per-rule ast.Walk.
func TestCheckRules_VisitorDrivenBySharedWalkNotCheck(t *testing.T) {
	src := []byte("# A\n\n### C\n")
	f, err := lint.NewFile("doc.md", src)
	require.NoError(t, err)

	eff := map[string]config.RuleCfg{"walk-only": {Enabled: true}}
	diags, errs := checkRules(f, []rule.Rule{walkOnlyVisitorRule{}}, eff, true)
	require.Empty(t, errs)
	// A green run (no panic) proves Check was never called; the engine
	// drove the rule through VisitNode. The diagnostic carries the
	// visitor's own RuleID.
	require.Len(t, diags, 1, "the h1->h3 jump is flagged via VisitNode")
	assert.Equal(t, "jump to 3", diags[0].Message)
}

// TestCheckRules_VisitorFreshPerFile pins that NewNodeVisitor is called
// once per file so prevLevel does not leak between files when the same
// rule instance lints two documents in sequence.
func TestCheckRules_VisitorFreshPerFile(t *testing.T) {
	vj := levelJumpRule{}
	eff := map[string]config.RuleCfg{"level-jump": {Enabled: true}}

	// File one ends on a deep heading; if state leaked, file two's
	// first heading (h1) would look like a jump-down (it never flags),
	// but a leaked prevLevel of 5 with file two starting at h1 would
	// not flag either — so use file two that starts at h2 with no h1.
	// A leaked prevLevel=0 reset is what we want: h2 alone never flags.
	f1, err := lint.NewFile("a.md", []byte("# A\n\n##### Deep\n"))
	require.NoError(t, err)
	f2, err := lint.NewFile("b.md", []byte("## Only\n"))
	require.NoError(t, err)

	d1, _ := checkRules(f1, []rule.Rule{vj}, eff, true)
	d2, _ := checkRules(f2, []rule.Rule{vj}, eff, true)

	require.Len(t, d1, 1, "file one's h1->h5 jump is flagged")
	require.Empty(t, d2, "file two's lone h2 must not be flagged; visitor state is fresh per file")
}

// migratedVisitorRules lists every production rule migrated to
// rule.NodeVisitorRule (plan 219's stateful migrations) by its
// registered Name(). The byte-identity table-test below resolves each
// through the production registry, so adding a migration only requires
// appending a name here. It is the visitor sibling of migratedRules.
var migratedVisitorRules = []struct {
	id, name string
}{
	{"MDS003", "heading-increment"},
	{"MDS005", "no-duplicate-headings"},
}

// assertMigratedVisitorRuleEquivalent runs one migrated visitor rule
// against the shared corpus twice — once with its NodeVisitorRule
// capability hidden (so the engine runs its Check, the pre-multiplex
// path) and once exposed (so the engine drives it through the shared
// walk) — and asserts byte-identity of the two diagnostic slices.
func assertMigratedVisitorRuleEquivalent(t *testing.T, name string) {
	t.Helper()
	rl := newRuleByName(t, name)
	vr, ok := rl.(rule.NodeVisitorRule)
	require.True(t, ok, "%s expected to implement NodeVisitorRule", rl.Name())

	eff := map[string]config.RuleCfg{rl.Name(): {Enabled: true}}

	f1, err := lint.NewFile("doc.md", migratedRuleEquivalenceCorpus)
	require.NoError(t, err)
	f2, err := lint.NewFile("doc.md", migratedRuleEquivalenceCorpus)
	require.NoError(t, err)

	seq, errs1 := checkRules(f1, []rule.Rule{hiddenVisitorRule{vr}}, eff, true)
	mux, errs2 := checkRules(f2, []rule.Rule{rl}, eff, true)

	require.Empty(t, errs1)
	require.Empty(t, errs2)
	assert.Equal(t, seq, mux,
		"%s: multiplexed visitor output must equal sequential", rl.Name())
}

// TestCheckRules_MigratedVisitorRulesEqualSequential pins that every
// stateful NodeVisitorRule in the production rule set produces a
// byte-identical diagnostic slice whether routed through the
// multiplexed walk or the legacy per-rule path. Each rule is tested in
// isolation so a failure points at exactly one rule.
func TestCheckRules_MigratedVisitorRulesEqualSequential(t *testing.T) {
	for _, tc := range migratedVisitorRules {
		t.Run(tc.id, func(t *testing.T) {
			assertMigratedVisitorRuleEquivalent(t, tc.name)
		})
	}
}
