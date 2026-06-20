package checker

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Package-internal stubs for the unexported-helper tests below.
// The external checker_test.go (package checker_test) defines its own stubs
// for the exported API surface.

type htPlainRule struct{ id string }

func (r *htPlainRule) ID() string                           { return r.id }
func (r *htPlainRule) Name() string                         { return r.id }
func (r *htPlainRule) Category() string                     { return "test" }
func (r *htPlainRule) Check(_ *lint.File) []lint.Diagnostic { return nil }

type htDiagRule struct {
	htPlainRule
	diag lint.Diagnostic
}

func (r *htDiagRule) Check(_ *lint.File) []lint.Diagnostic { return []lint.Diagnostic{r.diag} }

type htNodeRule struct {
	htPlainRule
	msg string
}

func (r *htNodeRule) Check(_ *lint.File) []lint.Diagnostic { return nil }
func (r *htNodeRule) CheckNode(n ast.Node, entering bool, _ *lint.File) []lint.Diagnostic {
	if entering && n.Kind() == ast.KindDocument {
		return []lint.Diagnostic{{RuleID: r.id, Message: r.msg}}
	}
	return nil
}

var _ rule.NodeChecker = (*htNodeRule)(nil)

type htKindScopedRule struct{ htPlainRule }

func (r *htKindScopedRule) Check(_ *lint.File) []lint.Diagnostic { return nil }
func (r *htKindScopedRule) CheckNode(n ast.Node, entering bool, _ *lint.File) []lint.Diagnostic {
	if entering && n.Kind() == ast.KindHeading {
		return []lint.Diagnostic{{RuleID: r.id, Message: "heading"}}
	}
	return nil
}
func (r *htKindScopedRule) EnteringKinds() []ast.NodeKind { return []ast.NodeKind{ast.KindHeading} }

var _ rule.KindScopedChecker = (*htKindScopedRule)(nil)

type htPanicRule struct {
	htPlainRule
	msg string
}

func (r *htPanicRule) Check(_ *lint.File) []lint.Diagnostic { panic(r.msg) }

type htBlockRule struct{ htPlainRule }

func (r *htBlockRule) Check(_ *lint.File) []lint.Diagnostic                         { return nil }
func (r *htBlockRule) CheckNode(_ ast.Node, _ bool, _ *lint.File) []lint.Diagnostic { return nil }
func (r *htBlockRule) CheckBlock(span lint.BlockSpan, _ *lint.File) []lint.Diagnostic {
	return []lint.Diagnostic{{RuleID: r.id, Line: span.Start, Message: "block"}}
}
func (r *htBlockRule) BlockKinds() []lint.BlockKind { return []lint.BlockKind{lint.BlockThematicBreak} }

var _ rule.BlockChecker = (*htBlockRule)(nil)

type htInlineRule struct{ htPlainRule }

func (r *htInlineRule) Check(_ *lint.File) []lint.Diagnostic                         { return nil }
func (r *htInlineRule) CheckNode(_ ast.Node, _ bool, _ *lint.File) []lint.Diagnostic { return nil }
func (r *htInlineRule) InlineCapable() bool                                          { return true }

var _ rule.InlineChecker = (*htInlineRule)(nil)

// TestClassifyRules pins that classifyRules partitions enabled rules
// into the correct slots and dispatch sets.
func TestClassifyRules(t *testing.T) {
	t.Run("disabledRuleSkipped", func(t *testing.T) {
		f, err := lint.NewFile("t.md", []byte("# H\n"))
		require.NoError(t, err)
		r := &htPlainRule{id: "TST001"}
		eff := map[string]config.RuleCfg{"TST001": {Enabled: false}}
		slots, nc, bc, errs := classifyRules(f, []rule.Rule{r}, eff)
		assert.Empty(t, errs)
		assert.Empty(t, slots)
		assert.Nil(t, nc)
		assert.Nil(t, bc)
	})
	t.Run("plainRuleGetsCheckSlot", func(t *testing.T) {
		f, err := lint.NewFile("t.md", []byte("# H\n"))
		require.NoError(t, err)
		r := &htPlainRule{id: "TST001"}
		eff := map[string]config.RuleCfg{"TST001": {Enabled: true}}
		slots, nc, bc, errs := classifyRules(f, []rule.Rule{r}, eff)
		assert.Empty(t, errs)
		require.Len(t, slots, 1)
		assert.NotNil(t, slots[0].check)
		assert.Nil(t, slots[0].nc)
		assert.Nil(t, nc)
		assert.Nil(t, bc)
	})
	t.Run("nodeCheckerOnASTFile", func(t *testing.T) {
		f, err := lint.NewFile("t.md", []byte("# H\n"))
		require.NoError(t, err)
		r := &htNodeRule{htPlainRule: htPlainRule{id: "TST001"}}
		eff := map[string]config.RuleCfg{"TST001": {Enabled: true}}
		slots, nc, bc, errs := classifyRules(f, []rule.Rule{r}, eff)
		assert.Empty(t, errs)
		require.Len(t, slots, 1)
		assert.NotNil(t, slots[0].nc)
		require.Len(t, nc, 1)
		assert.Nil(t, bc)
	})
	t.Run("blockCheckerOnNilASTFile", func(t *testing.T) {
		f := lint.NewFileLines("t.md", []byte("para\n\n---\n"))
		r := &htBlockRule{htPlainRule: htPlainRule{id: "TST001"}}
		eff := map[string]config.RuleCfg{"TST001": {Enabled: true}}
		slots, nc, bc, errs := classifyRules(f, []rule.Rule{r}, eff)
		assert.Empty(t, errs)
		require.Len(t, slots, 1)
		assert.NotNil(t, slots[0].bc)
		assert.Nil(t, nc)
		require.Len(t, bc, 1)
	})
}

// TestClassifySlot pins each routing branch: plain rule, NodeChecker with
// an AST, BlockChecker on a nil-AST file, bare NodeChecker on nil-AST
// (dropped), and InlineChecker on nil-AST (routed to Check).
func TestClassifySlot(t *testing.T) {
	t.Run("nonNodeChecker", func(t *testing.T) {
		r := &htPlainRule{id: "TST001"}
		s := classifySlot(r, false)
		assert.NotNil(t, s.check)
		assert.Nil(t, s.nc)
		assert.Nil(t, s.bc)
	})
	t.Run("nodeCheckerWithAST", func(t *testing.T) {
		r := &htNodeRule{htPlainRule: htPlainRule{id: "TST001"}}
		s := classifySlot(r, false)
		assert.NotNil(t, s.nc)
		assert.Nil(t, s.check)
		assert.Nil(t, s.bc)
	})
	t.Run("blockCheckerAstNil", func(t *testing.T) {
		r := &htBlockRule{htPlainRule: htPlainRule{id: "TST001"}}
		s := classifySlot(r, true)
		assert.NotNil(t, s.bc)
		assert.Nil(t, s.nc)
		assert.Nil(t, s.check)
	})
	t.Run("nodeCheckerNotBlockAstNil", func(t *testing.T) {
		r := &htNodeRule{htPlainRule: htPlainRule{id: "TST001"}}
		s := classifySlot(r, true)
		assert.Nil(t, s.nc)
		assert.Nil(t, s.bc)
		assert.Nil(t, s.check)
	})
	t.Run("inlineCheckerAstNil", func(t *testing.T) {
		r := &htInlineRule{htPlainRule: htPlainRule{id: "TST001"}}
		s := classifySlot(r, true)
		assert.NotNil(t, s.check)
		assert.Nil(t, s.nc)
		assert.Nil(t, s.bc)
	})
}

// TestBuildKindTable pins the CSR construction: generic rules go to the
// generic list, kind-scoped rules are indexed by kind.
func TestBuildKindTable(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		tbl := buildKindTable(nil)
		assert.Empty(t, tbl.generic)
		assert.Empty(t, tbl.scoped)
		releaseKindTable(tbl)
	})
	t.Run("genericNodeRule", func(t *testing.T) {
		r := &htNodeRule{htPlainRule: htPlainRule{id: "TST001"}}
		s := &ruleSlot{nc: r}
		tbl := buildKindTable([]*ruleSlot{s})
		assert.Len(t, tbl.generic, 1)
		assert.Empty(t, tbl.scoped)
		releaseKindTable(tbl)
	})
	t.Run("kindScopedRule", func(t *testing.T) {
		r := &htKindScopedRule{htPlainRule: htPlainRule{id: "TST001"}}
		s := &ruleSlot{nc: r}
		tbl := buildKindTable([]*ruleSlot{s})
		assert.Empty(t, tbl.generic)
		require.Len(t, tbl.scoped, 1)
		k := ast.KindHeading
		assert.Equal(t, s, tbl.scoped[tbl.offsets[k]:tbl.offsets[k+1]][0])
		releaseKindTable(tbl)
	})
}

// TestBlockCheckerReactsTo pins the linear scan over BlockKinds: present
// returns true, absent returns false.
func TestBlockCheckerReactsTo(t *testing.T) {
	r := &htBlockRule{htPlainRule: htPlainRule{id: "TST001"}}
	t.Run("kindPresent", func(t *testing.T) {
		assert.True(t, blockCheckerReactsTo(r, lint.BlockThematicBreak))
	})
	t.Run("kindAbsent", func(t *testing.T) {
		assert.False(t, blockCheckerReactsTo(r, lint.BlockParagraph))
	})
}

// TestRunNonNodeCheckers pins that the serial path fills diag buckets for
// check-bearing slots and the concurrent path produces the same result.
func TestRunNonNodeCheckers(t *testing.T) {
	f := &lint.File{Path: "t.md"}
	diag := lint.Diagnostic{Line: 1, RuleID: "TST001", Message: "hit"}

	t.Run("serialFillsDiags", func(t *testing.T) {
		r := &htDiagRule{htPlainRule: htPlainRule{id: "TST001"}, diag: diag}
		slots := []ruleSlot{{check: r}}
		runNonNodeCheckers(f, slots, 1)
		require.Len(t, slots[0].diags, 1)
		assert.Equal(t, "hit", slots[0].diags[0].Message)
	})
	t.Run("serialSkipsNilCheckSlots", func(t *testing.T) {
		r := &htNodeRule{htPlainRule: htPlainRule{id: "TST001"}}
		slots := []ruleSlot{{nc: r}}
		runNonNodeCheckers(f, slots, 1)
		assert.Empty(t, slots[0].diags)
	})
	t.Run("concurrentFillsDiags", func(t *testing.T) {
		r := &htDiagRule{htPlainRule: htPlainRule{id: "TST001"}, diag: diag}
		slots := []ruleSlot{{check: r}}
		runNonNodeCheckers(f, slots, 4)
		require.Len(t, slots[0].diags, 1)
		assert.Equal(t, "hit", slots[0].diags[0].Message)
	})
	t.Run("concurrentPanicIsRecovered", func(t *testing.T) {
		r := &htPanicRule{htPlainRule: htPlainRule{id: "TST999"}, msg: "test panic"}
		slots := []ruleSlot{{check: r}}
		runNonNodeCheckers(f, slots, 4)
		require.Len(t, slots[0].diags, 1)
		assert.Equal(t, "internal-panic", slots[0].diags[0].RuleID)
		assert.Equal(t, lint.Error, slots[0].diags[0].Severity)
		assert.Contains(t, slots[0].diags[0].Message, "test panic")
		assert.Contains(t, slots[0].diags[0].Message, "goroutine")
	})
}

// TestRunNodeCheckers pins that the shared AST walk dispatches to each
// NodeChecker slot and appends diagnostics to it.
func TestRunNodeCheckers(t *testing.T) {
	f, err := lint.NewFile("t.md", []byte("# Hello\n"))
	require.NoError(t, err)
	r := &htNodeRule{htPlainRule: htPlainRule{id: "TST001"}, msg: "document"}
	slot := &ruleSlot{nc: r}
	runNodeCheckers(f, []*ruleSlot{slot})
	assert.NotEmpty(t, slot.diags)
	assert.Equal(t, "document", slot.diags[0].Message)
}

// TestRunBlockCheckers pins that the block-span dispatch fires CheckBlock
// for matching spans and collects diagnostics in the slot.
func TestRunBlockCheckers(t *testing.T) {
	f := lint.NewFileLines("t.md", []byte("para\n\n---\n\nmore\n"))
	r := &htBlockRule{htPlainRule: htPlainRule{id: "TST001"}}
	slot := &ruleSlot{bc: r}
	runBlockCheckers(f, []*ruleSlot{slot})
	require.Len(t, slot.diags, 1)
	assert.Equal(t, 3, slot.diags[0].Line)
}
