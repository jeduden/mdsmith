// Package astutil_test benchmarks cross-rule use of astutil's shared,
// memoized AST collectors. It lives in the external test package
// because it exercises concrete rule packages (headingincrement,
// noduplicateheadings) rather than astutil internals — importing
// them from an internal (non-_test-suffixed) test file would be an
// import cycle, since those packages import astutil's production code.
package astutil_test

import (
	"fmt"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rules/astutil"
	"github.com/jeduden/mdsmith/internal/rules/headingincrement"
	"github.com/jeduden/mdsmith/internal/rules/noduplicateheadings"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
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

// TestHeadingRulesTogether_CorrectDiagnostics verifies that MDS003
// (heading-increment) and MDS005 (no-duplicate-headings) each emit the
// expected diagnostic on a shared File. Both rules are now
// rule.KindScopedChecker implementors that receive heading nodes from
// the engine's single shared AST walk rather than through
// astutil.CollectHeadingNodes's File-level memo. The previous
// memo-seeding mechanism is no longer the relevant shared-walk
// optimisation: the KindScopedChecker dispatch already achieves ONE
// walk for all heading rules without per-rule full-tree traversals.
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

	// Verify that astutil.HeadingNodesMemoKey pre-seeding does not suppress
	// either rule's diagnostics — both now walk the AST directly via
	// CheckNode rather than through CollectHeadingNodes.
	f2, err := lint.NewFile("test2.md", src)
	require.NoError(t, err)
	_ = f2.MemoFile(astutil.HeadingNodesMemoKey, func(*lint.File) any {
		return []*ast.Heading(nil) // empty — would suppress memo-reading rules
	})
	hi2 := &headingincrement.Rule{}
	nd2 := &noduplicateheadings.Rule{}
	assert.Len(t, hi2.Check(f2), 1,
		"heading-increment must not be suppressed by HeadingNodesMemoKey pre-seed: it now uses CheckNode directly")
	assert.Len(t, nd2.Check(f2), 1,
		"no-duplicate-headings must not be suppressed by HeadingNodesMemoKey pre-seed: it now uses CheckNode directly")
}

// BenchmarkHeadingRulesTogether exercises MDS003 and MDS005 back to
// back against fresh Files, mirroring how internal/checker runs every
// enabled rule against one File per workspace file. benchstat-friendly
// (no assertion): a prior manual benchstat comparison (10 runs each)
// showed the shared walk cuts combined wall-clock by ~7% (mean 101.9us
// -> 94.5us on a 201-heading document) even though allocs/op rises
// slightly (145 -> 157, from MemoFile's map-and-interface-boxing
// overhead) — fewer full tree walks wins on net despite the small
// extra bookkeeping cost.
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
