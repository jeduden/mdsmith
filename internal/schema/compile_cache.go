package schema

import (
	"github.com/jeduden/mdsmith/cue/cuelite"
	"github.com/jeduden/mdsmith/internal/lint"
)

// CompiledCUE wraps a compiled cuelite.Value for the schema-side CUE
// constraint. A cuelite.Value is context-free at this façade boundary —
// it retains the source it was compiled from, so a cross-context Unify
// rebuilds it into the data's context rather than requiring callers to
// carry a *cue.Context. The wrapper is what RunCache.CompiledCUE stores,
// shared by the schema package's frontmatter validator and the
// requiredstructure rule's validateCUESchemaSyntax / validateFrontMatterCUE.
//
// Sharing a cached wrapper across goroutines is safe: the data value is
// always the Unify RECEIVER (dataVal.Unify(schemaVal)), so the per-file
// data context is the one rebuilt into, and the shared schema value is
// only read for its retained source — never mutated. See validate.go.
type CompiledCUE struct {
	Value cuelite.Value
}

// Err reports the compile error from the cached compile. The frontmatter
// validator inspects the schema-side error before going further;
// exposing it here keeps the cache hit cheap (no per-call Unify on a
// known-broken schema). A nil wrapper reports no error.
func (c *CompiledCUE) Err() error {
	if c == nil {
		return nil
	}
	return c.Value.Err()
}

// CachedCompile returns the cached compile of source through cache when
// non-nil, falling back to a fresh compile when the cache is missing.
// The returned wrapper carries a source-retaining cuelite.Value so a
// caller that unifies document front matter against it (as the DATA-side
// receiver) gets a rebuildable schema operand.
//
// MDS020's validate.go ValidateFrontmatterDiags and the requiredstructure
// rule's validateCUESchemaSyntax / validateFrontMatterCUE both call this
// helper so two host files sharing a schema CUE source compile it exactly
// once per Run.
func CachedCompile(cache *lint.RunCache, source string) *CompiledCUE {
	build := func() any {
		// cuelite.Compile returns a bottom Value on a compile error; Err()
		// (Validate) replays that error, so the failed compile is cached and
		// the broken-schema branch in validateFrontmatterDiags fires on a
		// cache hit without recompiling.
		val, _ := cuelite.Compile(source)
		return &CompiledCUE{Value: val}
	}
	if cache == nil {
		return build().(*CompiledCUE)
	}
	v := cache.CompiledCUE(source, build)
	return v.(*CompiledCUE)
}
