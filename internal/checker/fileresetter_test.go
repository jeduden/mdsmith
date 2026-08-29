package checker

import (
	"sync"
	"testing"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// htResetNodeRule is a kind-scoped NodeChecker that carries per-file
// state (a heading counter) and resets it in BeginFile, the shape MDS003
// and MDS005 have. It records how often BeginFile ran so the dispatch
// paths can be pinned.
type htResetNodeRule struct {
	htPlainRule
	beginCalls int
	headings   int
}

func (r *htResetNodeRule) Check(f *lint.File) []lint.Diagnostic { return rule.WalkNodes(r, f) }
func (r *htResetNodeRule) CheckNode(n ast.Node, entering bool, _ *lint.File) []lint.Diagnostic {
	if entering && n.Kind() == ast.KindHeading {
		r.headings++
	}
	return nil
}
func (r *htResetNodeRule) EnteringKinds() []ast.NodeKind { return []ast.NodeKind{ast.KindHeading} }
func (r *htResetNodeRule) BeginFile(_ *lint.File) {
	r.beginCalls++
	r.headings = 0
}

var (
	_ rule.KindScopedChecker = (*htResetNodeRule)(nil)
	_ rule.FileResetter      = (*htResetNodeRule)(nil)
)

// htResetBlockRule is a BlockChecker that also carries per-file state,
// the combination the block dispatch must reset. Before the block path
// honoured FileResetter, such a rule read the previous file's state.
type htResetBlockRule struct {
	htPlainRule
	beginCalls int
	spans      int
}

func (r *htResetBlockRule) Check(f *lint.File) []lint.Diagnostic { return rule.WalkBlocks(r, f) }
func (r *htResetBlockRule) CheckNode(_ ast.Node, _ bool, _ *lint.File) []lint.Diagnostic {
	return nil
}
func (r *htResetBlockRule) CheckBlock(_ lint.BlockSpan, _ *lint.File) []lint.Diagnostic {
	r.spans++
	return nil
}
func (r *htResetBlockRule) BlockKinds() []lint.BlockKind {
	return []lint.BlockKind{lint.BlockThematicBreak}
}
func (r *htResetBlockRule) BeginFile(_ *lint.File) {
	r.beginCalls++
	r.spans = 0
}

var (
	_ rule.BlockChecker = (*htResetBlockRule)(nil)
	_ rule.FileResetter = (*htResetBlockRule)(nil)
)

// TestPrepareNodeCheckers_ResetsFileState pins the BeginFile half of
// prepareNodeCheckers's single pass: every rule.FileResetter slot is reset
// exactly once per file, before any node is dispatched, and a slot that is
// not a FileResetter is simply skipped.
func TestPrepareNodeCheckers_ResetsFileState(t *testing.T) {
	f, err := lint.NewFile("t.md", []byte("# H\n"))
	require.NoError(t, err)

	resetter := &htResetNodeRule{htPlainRule: htPlainRule{id: "TST001"}}
	resetter.headings = 7 // stale value from a previous file
	plain := &htKindScopedRule{htPlainRule: htPlainRule{id: "TST002"}}
	slots := []*ruleSlot{{nc: resetter}, {nc: plain}}

	tbl := prepareNodeCheckers(f, slots)
	assert.Equal(t, 1, resetter.beginCalls, "BeginFile runs once per file")
	assert.Zero(t, resetter.headings, "BeginFile runs before any dispatch")
	require.Len(t, tbl.scoped, 2, "both rules are still indexed by kind")
	releaseKindTable(tbl)
}

// TestRunNodeCheckers_ResetsBetweenFiles pins that one rule instance
// reused across two files starts each file clean.
func TestRunNodeCheckers_ResetsBetweenFiles(t *testing.T) {
	r := &htResetNodeRule{htPlainRule: htPlainRule{id: "TST001"}}
	slots := []*ruleSlot{{nc: r}}

	first, err := lint.NewFile("a.md", []byte("# A\n\n## B\n"))
	require.NoError(t, err)
	runNodeCheckers(first, slots)
	require.Equal(t, 2, r.headings)

	second, err := lint.NewFile("b.md", []byte("# C\n"))
	require.NoError(t, err)
	runNodeCheckers(second, slots)
	assert.Equal(t, 1, r.headings, "the second file must not inherit the first file's count")
	assert.Equal(t, 2, r.beginCalls)
}

// TestRunBlockCheckers_ResetsFileState pins that the Layer 0 block
// dispatch honours FileResetter too. Without the reset, a rule that is
// both BlockChecker and FileResetter would carry the previous file's
// state into this one — and, for a map-valued field, write into a nil map
// and take down the process.
func TestRunBlockCheckers_ResetsFileState(t *testing.T) {
	r := &htResetBlockRule{htPlainRule: htPlainRule{id: "TST001"}}
	r.spans = 9 // stale value from a previous file
	slots := []*ruleSlot{{bc: r}}

	f := lint.NewFileLines("t.md", []byte("para\n\n---\n\nmore\n\n---\n"))
	runBlockCheckers(f, slots)

	assert.Equal(t, 1, r.beginCalls, "BeginFile runs once per file")
	assert.Equal(t, 2, r.spans, "the stale count must be cleared before the spans are dispatched")
}

// TestConfigureEnabledRules_IsolatesFileResetters pins that a
// rule.FileResetter is never handed out shared: two configured lists built
// from the same input slice must hold distinct instances, so concurrent
// callers cannot race on the rule's per-file fields. A stateless rule is
// still passed through untouched — the clone costs an allocation and only
// stateful rules need it.
func TestConfigureEnabledRules_IsolatesFileResetters(t *testing.T) {
	stateful := &htResetNodeRule{htPlainRule: htPlainRule{id: "TST001"}}
	stateless := &htNodeRule{htPlainRule: htPlainRule{id: "TST002"}}
	rules := []rule.Rule{stateful, stateless}
	eff := map[string]config.RuleCfg{
		"TST001": {Enabled: true},
		"TST002": {Enabled: true},
	}

	first, errs := ConfigureEnabledRules(rules, eff)
	require.Empty(t, errs)
	second, errs := ConfigureEnabledRules(rules, eff)
	require.Empty(t, errs)
	require.Len(t, first, 2)
	require.Len(t, second, 2)

	assert.NotSame(t, stateful, first[0], "a FileResetter must not be the shared input instance")
	assert.NotSame(t, first[0], second[0], "each configured list gets its own FileResetter")
	assert.Same(t, stateless, first[1], "a stateless rule needs no clone")
	assert.Same(t, stateless, second[1], "a stateless rule needs no clone")
}

// TestIsolateFileState pins the helper's three branches directly: an
// already-private clone passes through, a shared FileResetter is cloned,
// and a shared stateless rule is returned as is.
func TestIsolateFileState(t *testing.T) {
	t.Run("alreadyCloned", func(t *testing.T) {
		rl := &htResetNodeRule{htPlainRule: htPlainRule{id: "TST001"}}
		cr := &htResetNodeRule{htPlainRule: htPlainRule{id: "TST001"}}
		assert.Same(t, cr, isolateFileState(rl, cr))
	})
	t.Run("sharedFileResetterCloned", func(t *testing.T) {
		rl := &htResetNodeRule{htPlainRule: htPlainRule{id: "TST001"}}
		got := isolateFileState(rl, rl)
		assert.NotSame(t, rl, got)
		assert.Equal(t, "TST001", got.ID())
	})
	t.Run("sharedStatelessRulePassedThrough", func(t *testing.T) {
		rl := &htNodeRule{htPlainRule: htPlainRule{id: "TST002"}}
		assert.Same(t, rl, isolateFileState(rl, rl))
	})
}

// TestCheckRules_ConcurrentCallersDoNotShareFileState is the end-to-end
// guard for the isolation above: two goroutines running the same rule
// slice must not write through one instance's per-file fields. Under
// `go test -race` a regression here reports a write/write data race (and,
// for a map-valued field, can fatal outright with a concurrent map write).
func TestCheckRules_ConcurrentCallersDoNotShareFileState(t *testing.T) {
	rules := []rule.Rule{&htResetNodeRule{htPlainRule: htPlainRule{id: "TST001"}}}
	eff := map[string]config.RuleCfg{"TST001": {Enabled: true}}
	src := []byte("# A\n\n## B\n\n### C\n")

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 20 {
				f, err := lint.NewFile("t.md", src)
				if !assert.NoError(t, err) {
					return
				}
				_, errs := CheckRules(f, rules, eff)
				assert.Empty(t, errs)
			}
		}()
	}
	wg.Wait()
}
