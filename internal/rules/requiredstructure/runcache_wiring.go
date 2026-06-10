package requiredstructure

import (
	"path/filepath"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/schema"
)

// schemaParseResult is the cached payload for ParsedSchema: a
// parsedSchema pointer plus the parse error. Caching the error
// alongside the value lets a broken schema short-circuit on the
// second host file's lookup instead of re-running parseSchema.
//
// includes and cueSources carry the parsed schema's dependency
// footprint so RunCache.Invalidate can evict downstream cache
// entries when a fragment or schema source the parse depended on
// changes (the LSP edit-then-invalidate loop). Both are slices
// rather than scalars so a future schema with multiple frontmatter
// CUE expressions can populate them without a re-shape; today
// includes is the resolved <?include?> chain and cueSources
// is exactly one entry (the schema's frontmatter CUE).
type schemaParseResult struct {
	schema     *parsedSchema
	err        error
	includes   []string
	cueSources []string
}

// SchemaIncludes implements lint.ParsedSchemaMetadata so
// RunCache.Invalidate can read the include chain without a
// dependency on this package's private types. Returns nil for a
// schema that reached no <?include?> directives.
func (r schemaParseResult) SchemaIncludes() []string {
	return r.includes
}

// SchemaCUESources implements lint.ParsedSchemaMetadata. Returns
// every distinct CUE source string the schema's frontmatter
// produced; nil when the schema declared no frontmatter
// constraints.
func (r schemaParseResult) SchemaCUESources() []string {
	return r.cueSources
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
// schemas differ. The closure also surfaces the schema's include
// set and frontmatter CUE source so cachedParseSchemaWith can record
// the dependency footprint on the cache for Invalidate to walk.
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
		sch, _, err := parseSchemaWithRootFS(data, schemaPath, fileMaxBytes(f), nil, schemaRootFS(f))
		return sch, err
	}
	absPath := absSchemaCacheKey(f, schemaPath)
	absRoot := absRootDir(f)
	return cachedParseSchemaWith(cache, absPath, absRoot, func() (*parsedSchema, []string, error) {
		return parseSchemaWithRootFS(data, schemaPath, fileMaxBytes(f), cache, schemaRootFS(f))
	})
}

// cachedParseSchemaWith is the primitive cached-parse helper. Tests
// drive it directly with a synthetic build to assert single-build
// semantics without setting up a *lint.File.
//
// When cache is nil the build is invoked directly. An empty absPath
// also bypasses the cache (a parsed schema with no filesystem
// identity must not be shared between unrelated callers). The build
// returns its include set; cachedParseSchemaWith records it (plus
// the schema's frontmatter CUE source, read from the returned
// *parsedSchema) on the schemaParseResult so RunCache's
// reverse-include index and per-schema compiledCUE-eviction work
// end-to-end.
//
// Include paths returned by the build (via resolveSchemaIncludePath)
// are joined onto the schema-file's directory; in mdsmith's normal
// flow that directory is itself relative to the workspace root, so
// the resulting include strings end up workspace-root-relative.
// absRoot is the workspace root's absolute filesystem path; the
// helper joins each non-absolute include onto absRoot (NOT onto the
// schema's own dir, which would double-prefix paths for any schema
// living in a subdirectory) so the keys match the LSP's
// Invalidate(absPath) calls byte-for-byte. An empty absRoot leaves
// entries Clean'd-but-relative (the struct-literal test path).
func cachedParseSchemaWith(
	cache *lint.RunCache, absPath, absRoot string,
	build func() (*parsedSchema, []string, error),
) (*parsedSchema, error) {
	if cache == nil || absPath == "" {
		sch, _, err := build()
		return sch, err
	}
	v := cache.ParsedSchema(absPath, func() any {
		sch, includes, err := build()
		return schemaParseResult{
			schema:     sch,
			err:        err,
			includes:   absoluteIncludes(absRoot, includes),
			cueSources: schemaCUESources(sch),
		}
	})
	r := v.(schemaParseResult)
	return r.schema, r.err
}

// absoluteIncludes resolves each entry in includes to an absolute
// filesystem path anchored at absRoot. Absolute entries are passed
// through (after Clean); relative entries are joined against
// absRoot (NOT the schema's own dir — resolveSchemaIncludePath
// already prefixes each include with the schema's dir, so joining
// onto that dir a second time would double-prefix the path and
// break the reverse-include index for any schema in a subdirectory).
// The result matches the convention the LSP uses when calling
// Invalidate.
//
// absRoot is expected to be the workspace root's absolute path. An
// empty absRoot (struct-literal test path with no File context)
// leaves each entry Clean'd-but-relative — the cache machinery
// treats that as a deterministic key, even if it does not match the
// LSP's invalidation keys.
func absoluteIncludes(absRoot string, includes []string) []string {
	if len(includes) == 0 {
		return nil
	}
	out := make([]string, 0, len(includes))
	for _, inc := range includes {
		if inc == "" {
			continue
		}
		if filepath.IsAbs(inc) {
			out = append(out, filepath.Clean(inc))
			continue
		}
		if absRoot != "" {
			out = append(out, filepath.Clean(filepath.Join(absRoot, inc)))
			continue
		}
		out = append(out, filepath.Clean(inc))
	}
	return out
}

// absRootDir returns f.RootDir resolved to an absolute path. Returns
// "" when f is nil or f.RootDir is empty (the struct-literal test
// path); a Getwd failure inside filepath.Abs is treated the same
// way by ignoring the error — the wiring chain pairs the empty
// result with absoluteIncludes's no-absRoot branch.
func absRootDir(f *lint.File) string {
	if f == nil || f.RootDir == "" {
		return ""
	}
	abs, _ := filepath.Abs(f.RootDir)
	return abs
}

// schemaCUESources returns the distinct CUE source strings the
// parsed schema's frontmatter produced. Today every schema has at
// most one CUE source (cfg.FrontMatterCUE). The slice shape exists
// so a future schema that derives multiple CUE sources (e.g. one
// per declared field) can extend the producer without re-shaping
// the cached payload. Returns nil when the schema is nil or has no
// frontmatter CUE — the cache wrapper treats both as "no
// CompiledCUE entries to invalidate".
func schemaCUESources(sch *parsedSchema) []string {
	if sch == nil {
		return nil
	}
	expr := sch.Config.FrontMatterCUE
	if expr == "" {
		return nil
	}
	return []string{expr}
}

// cachedCompiledCUEWith returns the cached compile of source. It is a
// thin forward to schema.CachedCompile so the rule package keeps the
// historical name at its call sites (validateCUESchemaSyntaxWith /
// validateFrontMatterCUE) and the wrapper type lives in one place. The
// returned wrapper carries an immutable, context-free cuelite.Value, so a
// caller that needs to Unify additional values does so without carrying or
// matching a *cue.Context — the in-house engine has none.
//
// Tests drive this directly to assert single-build semantics; the
// rule path reaches it through validateCUESchemaSyntaxWith /
// validateFrontMatterCUE, both of which sit inside the already-cached
// parseSchema closure. The CompiledCUE slot adds a second-tier win:
// two distinct schema files producing identical CUE source share one
// compile.
func cachedCompiledCUEWith(cache *lint.RunCache, source string) *schema.CompiledCUE {
	return schema.CachedCompile(cache, source)
}

// absSchemaCacheKey resolves a schema path to an absolute filesystem
// key for ParsedSchema. Returns "" when no stable identity can be
// derived (no RootDir / Abs failure / non-absolute schemaPath); the
// caller then bypasses the cache and parses inline. Going through
// absRootDir consolidates the no-RootDir and Abs-failure branches
// onto one absRoot == "" check, so the cache contract stays
// "key is absolute or empty" — never a relative path that could
// collide across workspaces.
func absSchemaCacheKey(f *lint.File, schemaPath string) string {
	if schemaPath == "" {
		return ""
	}
	if filepath.IsAbs(schemaPath) {
		return filepath.Clean(schemaPath)
	}
	absRoot := absRootDir(f)
	if absRoot == "" {
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
