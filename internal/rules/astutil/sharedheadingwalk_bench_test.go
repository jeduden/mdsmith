// Package astutil_test benchmarks cross-rule use of astutil's shared,
// memoized AST collectors. It lives in the external test package
// because it exercises concrete rule packages (headingincrement,
// noduplicateheadings) rather than astutil internals.
package astutil_test

import (
	"fmt"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
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

// TestHeadingRulesTogether_ShareMemoizedHeadingNodes pins the actual
// mechanism the fix relies on with a deterministic result rather than
// a timing/allocation proxy (which ordinary noise, or an unrelated
// per-file memo like LineOfOffset's newline index, could easily
// confound): both heading-increment (MDS003) and no-duplicate-headings
// (MDS005) must read headings through astutil.CollectHeadingNodes's
// File-level memo, not their own private ast.Walk.
//
// f.MemoFile's contract is first-build-wins (internal/lint/file.go):
// once a key is populated, later calls with a different builder still
// return the original cached value. So pre-seeding the
// "astutil.headingNodes" key with an empty slice before either rule
// runs means a rule that goes through the shared cache sees zero
// headings and emits nothing, while a rule that still does its own
// ast.Walk would see the real (diagnostic-triggering) headings
// regardless of the pre-seed.
func TestHeadingRulesTogether_ShareMemoizedHeadingNodes(t *testing.T) {
	// A level-3 first heading trips MDS003; the repeated heading text
	// trips MDS005 — both would report a diagnostic on this file if
	// they read the real AST.
	src := []byte("### First heading\n\n### First heading\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	_ = f.MemoFile("astutil.headingNodes", func(*lint.File) any {
		return []*ast.Heading(nil)
	})

	hi := &headingincrement.Rule{}
	nd := &noduplicateheadings.Rule{}
	assert.Empty(t, hi.Check(f),
		"heading-increment must read headings via the pre-seeded astutil memo, not re-walk f.AST directly")
	assert.Empty(t, nd.Check(f),
		"no-duplicate-headings must read headings via the pre-seeded astutil memo, not re-walk f.AST directly")
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
