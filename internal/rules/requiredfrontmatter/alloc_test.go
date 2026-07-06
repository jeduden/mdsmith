package requiredfrontmatter

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCheck_BudgetPaths pins the allocation behaviour the repo-wide
// gate measures. The gate (internal/integration/alloc_budget_test.go)
// runs every rule with its DEFAULT settings on a front-matter-free
// fixture, so for this opt-in rule it only ever exercises the inert
// fast path. Both that path and the out-of-scope skip must allocate
// nothing. The configured in-scope path parses the file's YAML front
// matter once per call (~tens of allocs), which is parse-bound work
// the rule only pays when a project enables it; it is not on the gate's
// default-settings path and is intentionally not pinned here.
func TestCheck_BudgetPaths(t *testing.T) {
	src := []byte("---\ntype: BigQuery Table\n---\n# Schema\n")
	f, err := lint.NewFileFromSource("doc.md", src, true)
	require.NoError(t, err)

	inert := &Rule{}
	require.Nil(t, inert.Check(f))
	allocs := testing.AllocsPerRun(200, func() { _ = inert.Check(f) })
	assert.Equal(t, 0.0, allocs, "inert (no fields) path must not allocate")

	oos := &Rule{Fields: []string{"type"}, Include: []string{"other/**"}}
	allocs = testing.AllocsPerRun(200, func() { _ = oos.Check(f) })
	assert.LessOrEqual(t, allocs, 1.0, "out-of-scope skip must stay cheap")
}
