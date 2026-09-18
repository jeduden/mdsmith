// Package astutil_test holds the cross-rule tests and benchmarks for the
// two heading rules that read their heading text and lines through
// astutil (headingincrement/MDS003 and noduplicateheadings/MDS005). It
// lives in the external test package because it exercises those concrete
// rule packages rather than astutil internals — importing them from an
// internal (non-_test-suffixed) test file would be an import cycle, since
// those packages import astutil's production code.
package astutil_test

import (
	"fmt"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rules/headingincrement"
	"github.com/jeduden/mdsmith/internal/rules/noduplicateheadings"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// manyHeadingsSource builds a document with one H1 followed by n
// unique level-2 headings, so both MDS003 (heading-increment) and
// MDS005 (no-duplicate-headings) see n+1 headings to scan per Check.
func manyHeadingsSource(n int) []byte {
	src := []byte("# Title\n\n")
	for i := 0; i < n; i++ {
		src = append(src, []byte(fmt.Sprintf("## Section %d\n\nSome body text.\n\n", i))...)
	}
	return src
}

// TestHeadingRulesTogether_CorrectDiagnostics pins that MDS003 and MDS005
// each still emit their own diagnostic when both run over one File. The
// two rules are rule.KindScopedChecker implementors driven by the engine's
// single shared AST walk, so a regression in the shared dispatch (a rule
// dropped from the kind table, or one rule's per-file state bleeding into
// the other's) would show up here as a missing or duplicated diagnostic.
func TestHeadingRulesTogether_CorrectDiagnostics(t *testing.T) {
	// A level-3 first heading trips MDS003; the repeated heading text
	// trips MDS005 — both diagnostics must appear.
	src := []byte("### First heading\n\n### First heading\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	hi := &headingincrement.Rule{}
	nd := &noduplicateheadings.Rule{}
	hiDiags := hi.Check(f)
	ndDiags := nd.Check(f)
	require.Len(t, hiDiags, 1, "heading-increment must flag level-3 first heading")
	assert.Equal(t, "MDS003", hiDiags[0].RuleID)
	require.Len(t, ndDiags, 1, "no-duplicate-headings must flag the repeated heading text")
	assert.Equal(t, "MDS005", ndDiags[0].RuleID)

	// Order must not matter: running MDS005 first on a second File gives
	// the identical pair of verdicts, so neither rule depends on the other
	// having populated a shared per-File cache.
	f2, err := lint.NewFile("test2.md", src)
	require.NoError(t, err)
	nd2 := &noduplicateheadings.Rule{}
	hi2 := &headingincrement.Rule{}
	assert.Len(t, nd2.Check(f2), 1, "no-duplicate-headings must not depend on run order")
	assert.Len(t, hi2.Check(f2), 1, "heading-increment must not depend on run order")
}

// BenchmarkHeadingRulesTogether exercises MDS003 and MDS005 back to
// back against fresh Files, mirroring how internal/checker runs every
// enabled rule against one File per workspace file. benchstat-friendly
// (no assertion): it is the guard for the standalone Check path, where
// each rule pays its own ast.Walk. rule.WalkNodes applies the same
// EnteringKinds scoping the engine's dispatch applies, so the walk's
// per-node cost is a kind comparison rather than an interface call into
// a rule that immediately returns nil. Compare with benchstat before and
// after any change to WalkNodes or to either rule's CheckNode.
func BenchmarkHeadingRulesTogether(b *testing.B) {
	hi := &headingincrement.Rule{}
	nd := &noduplicateheadings.Rule{}
	src := manyHeadingsSource(50)
	warm, err := lint.NewFile("warm.md", src)
	require.NoError(b, err)
	_ = hi.Check(warm)
	_ = nd.Check(warm)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f, err := lint.NewFile("bench.md", src)
		require.NoError(b, err)
		_ = hi.Check(f)
		_ = nd.Check(f)
	}
}
