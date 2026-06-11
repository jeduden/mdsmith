package integration

// kindscope_test.go gates the kind-scoped NodeChecker dispatch
// (rule.KindScopedChecker). The engine's shared walk calls a
// kind-scoped rule's CheckNode only for the node kinds it declares,
// entering visits only — so a declaration that under-reports the kinds
// CheckNode reacts to would silently drop diagnostics. Two gates:
//
//  1. Every registered NodeChecker must declare its kinds. A new
//     NodeChecker rule that skips the declaration falls back to the
//     call-for-every-node path and quietly re-grows the dispatch cost
//     this optimization removed; the gate makes that a conscious step.
//  2. The declaration must be sound: CheckNode must return nil for
//     every (kind, direction) outside its declared (kinds, entering)
//     set, probed over a document that exercises every block and
//     inline construct the parser produces.

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	gmast "github.com/jeduden/mdsmith/pkg/goldmark/ast"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// kindProbeDoc exercises every construct the production rules declare
// interest in: ATX/setext headings, paragraphs, ordered/unordered/
// nested lists, links, images, autolinks, emphasis, code spans,
// fenced and indented code, HTML blocks, inline HTML, thematic
// breaks, blockquotes, and tables.
const kindProbeDoc = "# Heading one. #\n" +
	"\n" +
	"A paragraph with [link](https://example.com), ![img](pic.png) and\n" +
	"*emphasis*, **strong**, `code span`, <span>inline html</span>, and\n" +
	"a bare https://example.com/url plus <https://example.com/auto>.\n" +
	"\n" +
	"Setext\n" +
	"------\n" +
	"\n" +
	"- item one\n" +
	"- item two\n" +
	"  1. nested ordered\n" +
	"  3. misnumbered\n" +
	"\n" +
	"1. ordered\n" +
	"1. ordered two\n" +
	"\n" +
	"> blockquote line\n" +
	"\n" +
	"```\n" +
	"fenced, no language\n" +
	"\n" +
	"<div>html block</div>\n" +
	"\n" +
	"---\n" +
	"\n" +
	"| a | b |\n" +
	"| - | - |\n" +
	"| 1 | 2 |\n" +
	"\n" +
	"    indented code\n" +
	"\n" +
	"[undefined ref][nope] and (reversed)[link] shapes.\n"

func registeredNodeCheckers(t *testing.T) []rule.NodeChecker {
	t.Helper()
	var ncs []rule.NodeChecker
	for _, r := range rule.All() {
		if nc, ok := r.(rule.NodeChecker); ok {
			ncs = append(ncs, nc)
		}
	}
	require.NotEmpty(t, ncs, "registry must contain NodeChecker rules")
	return ncs
}

// TestNodeCheckersDeclareEnteringKinds pins that every registered
// NodeChecker opts into kind-scoped dispatch with a non-empty, stable,
// allocation-free declaration.
func TestNodeCheckersDeclareEnteringKinds(t *testing.T) {
	for _, nc := range registeredNodeCheckers(t) {
		ks, ok := nc.(rule.KindScopedChecker)
		if !assert.True(t, ok,
			"%s implements rule.NodeChecker but not rule.KindScopedChecker; "+
				"declare EnteringKinds() so the shared walk can skip it for "+
				"unrelated nodes (see internal/rule/walk.go)", nc.ID()) {
			continue
		}
		kinds := ks.EnteringKinds()
		assert.NotEmpty(t, kinds, "%s: EnteringKinds must not be empty", nc.ID())
		// The dispatch table reads the declaration twice per file; a
		// fresh slice per call would be two allocations per rule per
		// linted file.
		assert.Zero(t, testing.AllocsPerRun(10, func() { _ = ks.EnteringKinds() }),
			"%s: EnteringKinds must return a package-level slice, not allocate", nc.ID())
	}
}

// TestEnteringKindsAreSound probes each kind-scoped rule's CheckNode
// over every node of a construct-rich document: any diagnostic
// produced for a kind outside EnteringKinds, or on a leaving visit,
// means the declaration under-reports what CheckNode reacts to and
// kind-scoped dispatch would drop diagnostics.
func TestEnteringKindsAreSound(t *testing.T) {
	f, err := lint.NewFile("probe.md", []byte(kindProbeDoc))
	require.NoError(t, err)

	for _, nc := range registeredNodeCheckers(t) {
		if _, ok := nc.(rule.KindScopedChecker); !ok {
			continue
		}
		probe, ok := rule.CloneInstance(nc).(rule.KindScopedChecker)
		require.True(t, ok, "%s: clone must keep the KindScopedChecker capability", nc.ID())
		declared := make(map[gmast.NodeKind]bool, len(probe.EnteringKinds()))
		for _, k := range probe.EnteringKinds() {
			declared[k] = true
		}
		_ = gmast.Walk(f.AST, func(n gmast.Node, entering bool) (gmast.WalkStatus, error) {
			diags := probe.CheckNode(n, entering, f)
			if len(diags) == 0 {
				return gmast.WalkContinue, nil
			}
			assert.True(t, entering,
				"%s: CheckNode emitted on a leaving visit of %s; "+
					"kind-scoped dispatch only delivers entering visits",
				nc.ID(), n.Kind())
			assert.True(t, declared[n.Kind()],
				"%s: CheckNode emitted for undeclared kind %s; add it to EnteringKinds",
				nc.ID(), n.Kind())
			return gmast.WalkContinue, nil
		})
	}
}
