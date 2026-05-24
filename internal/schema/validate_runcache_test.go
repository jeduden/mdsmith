package schema

import (
	"sync/atomic"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidateFrontmatterDiags_CompilesSchemaOncePerRunCache pins the
// production-hot site: N host files sharing one schema + one RunCache
// must compile the schema CUE expression exactly once. Without the
// RunCache wiring on validateFrontmatterDiags's CompileString site,
// validate.go would build a fresh cue.Context per host file — exactly
// the per-Check allocation plan 195 closes.
//
// The test asserts the validator populated the cache by checking that
// a follow-up CompiledCUE lookup with the schema's canonical CUE
// expression key hits the cache (its build closure never runs). The
// only way the slot is populated is if the validator itself routed
// through cache.CompiledCUE — a fresh-context fallback would leave
// the slot empty and the assertion would fail.
func TestValidateFrontmatterDiags_CompilesSchemaOncePerRunCache(t *testing.T) {
	cache := lint.NewRunCache()
	sch := &Schema{
		Source: "kind shared",
		Frontmatter: map[string]string{
			"id": `string`,
		},
	}

	// Three independent host files share the cache. The validator
	// runs frontmatter validation for each; the compile must not
	// re-run.
	for i := 0; i < 3; i++ {
		doc := newDocFile(t, "doc.md", "# T\n")
		doc.RunCache = cache
		diags := ValidateFrontmatterDiags(doc, sch,
			map[string]any{"id": "ok"}, makeDiagForTest)
		require.Empty(t, diags)
	}

	// A follow-up lookup with a unique build closure must hit the
	// cache (build never runs) — proving the validator populated the
	// slot. Without the RunCache wiring, ValidateFrontmatterDiags
	// would compile against a fresh cue.Context and leave the slot
	// empty.
	expr := sch.FrontmatterCUE()
	var rebuilds int32
	_ = cache.CompiledCUE(expr, func() any {
		atomic.AddInt32(&rebuilds, 1)
		return CachedCompile(nil, expr)
	})
	assert.Equal(t, int32(0), atomic.LoadInt32(&rebuilds),
		"after ValidateFrontmatterDiags the schema's CompiledCUE entry "+
			"must be populated; a follow-up lookup must hit the cache")
}

// TestValidateFrontmatterDiags_NilRunCacheStillValidates regresses the
// call sites that pass a *lint.File with no RunCache (struct-literal
// tests). The validator must still compile and emit diagnostics
// without crashing.
func TestValidateFrontmatterDiags_NilRunCacheStillValidates(t *testing.T) {
	sch := &Schema{
		Source: "kind t",
		Frontmatter: map[string]string{
			"id": `string`,
		},
	}
	doc := newDocFile(t, "doc.md", "# T\n")
	doc.RunCache = nil
	diags := ValidateFrontmatterDiags(doc, sch,
		map[string]any{"id": float64(1)}, makeDiagForTest)
	require.NotEmpty(t, diags,
		"a wrong-typed value must still produce a diagnostic without a RunCache")
}
