package uniquefrontmatter

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCheck_ConfiguredSteadyStateAllocs pins the configured-path
// allocation budget the repo-wide gate cannot see: the gate's
// fixture configures no scopes, so only the inert path runs there.
// With the index warm (one build per run via the RunCache), Check
// must stay within the ≤ 10 allocs/op rule budget for clean and
// flagged in-scope files alike.
func TestCheck_ConfiguredSteadyStateAllocs(t *testing.T) {
	fsys := planFS()
	r := planRule()
	rc := lint.NewRunCache()

	clean := file(t, "plan/c.md", fsys)
	clean.RunCache = rc
	require.Nil(t, r.Check(clean), "warm the index")

	allocs := testing.AllocsPerRun(100, func() { _ = r.Check(clean) })
	assert.LessOrEqual(t, allocs, 10.0,
		"clean in-scope file, warm index: %v allocs", allocs)

	flagged := file(t, "plan/b.md", fsys)
	flagged.RunCache = rc
	require.Len(t, r.Check(flagged), 1)

	allocs = testing.AllocsPerRun(100, func() { _ = r.Check(flagged) })
	assert.LessOrEqual(t, allocs, 10.0,
		"flagged in-scope file, warm index: %v allocs", allocs)
}

// TestCheck_InertAllocatesNothing pins the unconfigured fast path
// the repo-wide budget gate exercises.
func TestCheck_InertAllocatesNothing(t *testing.T) {
	f := file(t, "plan/b.md", planFS())
	inert := &Rule{}

	allocs := testing.AllocsPerRun(100, func() { _ = inert.Check(f) })
	assert.Equal(t, 0.0, allocs)
}

// TestCheck_OutOfScopeConfiguredStaysCheap pins the cost every
// non-matching file pays when the rule is configured: an interned
// scope key plus a warm cache hit, with no per-Check key build.
func TestCheck_OutOfScopeConfiguredStaysCheap(t *testing.T) {
	fsys := planFS()
	r := planRule()
	rc := lint.NewRunCache()

	f := file(t, "other/d.md", fsys)
	f.RunCache = rc
	require.Nil(t, r.Check(f), "warm the index")

	allocs := testing.AllocsPerRun(100, func() { _ = r.Check(f) })
	assert.LessOrEqual(t, allocs, 3.0,
		"configured out-of-scope file, warm index: %v allocs", allocs)
}
