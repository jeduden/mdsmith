package requiredstructure

import (
	"path/filepath"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/jeduden/mdsmith/internal/lint"
)

// compiledCUE pairs a cue.Value with the *cue.Context that produced
// it. A cue.Value is tied to its context and must not cross contexts,
// so a cached value is only meaningful alongside its context — callers
// that need to compile additional data (e.g. the document front
// matter) must reuse the wrapper's Context so the resulting Unify
// stays on one context.
type compiledCUE struct {
	Ctx   *cue.Context
	Value cue.Value
}

// Err reports the CompileString error from the cached compile.
// validateCUESchemaSyntax / validateFrontMatterCUE both inspect the
// schema-side error before going further; exposing it here keeps the
// cache hit cheap (no per-call Unify on a known-broken schema).
func (c *compiledCUE) Err() error {
	if c == nil {
		return nil
	}
	return c.Value.Err()
}

// schemaParseResult is the cached payload for ParsedSchema: a
// parsedSchema pointer plus the parse error. Caching the error
// alongside the value lets a broken schema short-circuit on the
// second host file's lookup instead of re-running parseSchema.
type schemaParseResult struct {
	schema *parsedSchema
	err    error
}

// cachedParseSchema reads + parses the schema for f, going through
// f.RunCache when available so two host files referencing the same
// schema parse it exactly once. absPath is computed from f.RootDir +
// schemaPath; an empty absPath bypasses the cache (the struct-literal
// unit-test path).
//
// The build closure routes through parseSchemaWithCache so the inner
// CUE compile is also cached: schemas with the same CUE source share
// the compiled value via RunCache.CompiledCUE even when their parsed
// schemas differ.
//
// This is the thin wiring layer over the cachedParseSchemaWith
// primitive — the latter is package-private only because the tests
// exercise it without f.
func cachedParseSchema(
	f *lint.File, data []byte, schemaPath string,
) (*parsedSchema, error) {
	var cache *lint.RunCache
	if f != nil {
		cache = f.RunCache
	}
	if cache == nil {
		return parseSchemaWithCache(data, schemaPath, fileMaxBytes(f), nil)
	}
	absPath := absSchemaCacheKey(f, schemaPath)
	return cachedParseSchemaWith(cache, absPath, func() (*parsedSchema, error) {
		return parseSchemaWithCache(data, schemaPath, fileMaxBytes(f), cache)
	})
}

// cachedParseSchemaWith is the primitive cached-parse helper. Tests
// drive it directly with a synthetic build to assert single-build
// semantics without setting up a *lint.File.
//
// When cache is nil the build is invoked directly. An empty absPath
// also bypasses the cache (a parsed schema with no filesystem
// identity must not be shared between unrelated callers).
func cachedParseSchemaWith(
	cache *lint.RunCache, absPath string,
	build func() (*parsedSchema, error),
) (*parsedSchema, error) {
	if cache == nil || absPath == "" {
		return build()
	}
	v := cache.ParsedSchema(absPath, func() any {
		sch, err := build()
		return schemaParseResult{schema: sch, err: err}
	})
	r := v.(schemaParseResult)
	return r.schema, r.err
}

// cachedCompiledCUEWith returns the cached compile of source through
// cache when non-nil, falling back to a fresh compile when the cache
// is missing. The returned wrapper carries the *cue.Context the value
// was compiled against so callers that need to Unify additional
// values use the same context (cue.Value cannot cross contexts).
//
// Tests drive this directly to assert single-build semantics; the
// rule path reaches it through validateCUESchemaSyntaxWith /
// validateFrontMatterCUE, both of which sit inside the already-cached
// parseSchema closure. The CompiledCUE slot adds a second-tier win:
// two distinct schema files producing identical CUE source share one
// compile.
func cachedCompiledCUEWith(cache *lint.RunCache, source string) *compiledCUE {
	build := func() any {
		ctx := cuecontext.New()
		val := ctx.CompileString(source)
		return &compiledCUE{Ctx: ctx, Value: val}
	}
	if cache == nil {
		return build().(*compiledCUE)
	}
	v := cache.CompiledCUE(source, build)
	return v.(*compiledCUE)
}

// absSchemaCacheKey resolves a schema path to an absolute filesystem
// key for ParsedSchema. Returns "" when no stable identity can be
// derived (no RootDir and a non-absolute schemaPath); the caller then
// bypasses the cache and parses inline.
func absSchemaCacheKey(f *lint.File, schemaPath string) string {
	if schemaPath == "" {
		return ""
	}
	if filepath.IsAbs(schemaPath) {
		return filepath.Clean(schemaPath)
	}
	if f.RootDir == "" {
		return ""
	}
	absRoot, err := filepath.Abs(f.RootDir)
	if err != nil {
		return ""
	}
	return filepath.Clean(filepath.Join(absRoot, schemaPath))
}

// fileMaxBytes returns f.MaxInputBytes when f is non-nil, falling back
// to 0 (unbounded — matching parseSchema's existing zero-as-unbounded
// convention) when the caller has no file context.
func fileMaxBytes(f *lint.File) int64 {
	if f == nil {
		return 0
	}
	return f.MaxInputBytes
}
