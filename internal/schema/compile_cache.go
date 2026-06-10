package schema

import (
	"github.com/jeduden/mdsmith/cue/cuelite"
	"github.com/jeduden/mdsmith/internal/lint"
)

// CompiledCUE wraps a compiled cuelite.Value for the schema-side CUE
// constraint. Post-flip a cuelite.Value is an immutable, context-free value
// in the in-house engine: it owns no *cue.Context and is never rebuilt, so a
// single compiled schema validates many documents with no per-document
// recompile. The wrapper is what RunCache.CompiledCUE stores, shared by the
// schema package's frontmatter validator and the requiredstructure rule's
// validateCUESchemaSyntax / validateFrontMatterCUE.
//
// Sharing a cached wrapper across goroutines is safe because the Value is
// immutable: Unify reads both operands and returns a fresh Value, mutating
// neither, so operand order no longer matters and no synchronization is
// needed. See validate.go.
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
// The returned wrapper carries an immutable, context-free cuelite.Value, so a
// caller that unifies document front matter against it shares one compiled
// schema across files and goroutines with no recompile and no synchronization.
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
