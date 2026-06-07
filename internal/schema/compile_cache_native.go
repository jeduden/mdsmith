//go:build !wasm

package schema

import (
	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/jeduden/mdsmith/internal/lint"
)

// CompiledCUE pairs a cue.Value with the *cue.Context that produced
// it. A cue.Value is tied to its context and must not cross contexts,
// so a cached value is only meaningful alongside its context — callers
// that need to compile additional data (e.g. the document front
// matter) must reuse the wrapper's Context so the resulting Unify
// stays on one context.
//
// Exported so callers across packages (the schema package's
// frontmatter validator and the requiredstructure rule's
// validateCUESchemaSyntax / validateFrontMatterCUE) share one shape
// for the cached entry stored in lint.RunCache.CompiledCUE.
type CompiledCUE struct {
	Ctx   *cue.Context
	Value cue.Value
}

// Err reports the CompileString error from the cached compile. The
// frontmatter validator inspects the schema-side error before going
// further; exposing it here keeps the cache hit cheap (no per-call
// Unify on a known-broken schema).
func (c *CompiledCUE) Err() error {
	if c == nil {
		return nil
	}
	return c.Value.Err()
}

// CachedCompile returns the cached compile of source through cache
// when non-nil, falling back to a fresh compile when the cache is
// missing. The returned wrapper carries the *cue.Context the value
// was compiled against so callers that need to Unify additional
// values use the same context (cue.Value cannot cross contexts).
//
// MDS020's validate.go ValidateFrontmatterDiags and the
// requiredstructure rule's validateCUESchemaSyntax /
// validateFrontMatterCUE both call this helper so two host files
// sharing a schema CUE source compile it exactly once per Run.
func CachedCompile(cache *lint.RunCache, source string) *CompiledCUE {
	build := func() any {
		ctx := cuecontext.New()
		val := ctx.CompileString(source)
		return &CompiledCUE{Ctx: ctx, Value: val}
	}
	if cache == nil {
		return build().(*CompiledCUE)
	}
	v := cache.CompiledCUE(source, build)
	return v.(*CompiledCUE)
}
