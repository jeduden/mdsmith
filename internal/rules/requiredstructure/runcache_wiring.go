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
		sch, _, err := parseSchemaWithCache(data, schemaPath, fileMaxBytes(f), nil)
		return sch, err
	}
	absPath := absSchemaCacheKey(f, schemaPath)
	return cachedParseSchemaWith(cache, absPath, func() (*parsedSchema, []string, error) {
		return parseSchemaWithCache(data, schemaPath, fileMaxBytes(f), cache)
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
// Include paths returned by the build are normalised to absolute
// filesystem paths anchored at absPath's directory before they go on
// the schemaParseResult. The producer (parseSchemaWithCache /
// extractSchemaHeadings) constructs include paths relative to
// whatever schemaPath the caller passed (typically the project-
// relative path the rule loads schemas with), so without
// normalisation the cache key in RunCache's reverse-include index
// would mismatch the absolute path the LSP passes to Invalidate.
func cachedParseSchemaWith(
	cache *lint.RunCache, absPath string,
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
			includes:   absoluteIncludes(absPath, includes),
			cueSources: schemaCUESources(sch),
		}
	})
	r := v.(schemaParseResult)
	return r.schema, r.err
}

// absoluteIncludes resolves each entry in includes to an absolute
// filesystem path. Absolute entries are passed through (after
// Clean); relative entries are joined against the directory of
// parentAbsPath (the schema's absolute cache key). The result
// matches the convention the LSP uses when calling Invalidate.
//
// parentAbsPath is expected to be absolute (it is the
// absSchemaCacheKey output for the parent schema). When it is not
// absolute we still Clean each entry but leave its relative shape
// alone — the cache machinery treats that as a no-op for invalidate
// purposes, which is the safe default.
func absoluteIncludes(parentAbsPath string, includes []string) []string {
	if len(includes) == 0 {
		return nil
	}
	out := make([]string, 0, len(includes))
	parentDir := filepath.Dir(parentAbsPath)
	for _, inc := range includes {
		if inc == "" {
			continue
		}
		if filepath.IsAbs(inc) {
			out = append(out, filepath.Clean(inc))
			continue
		}
		if filepath.IsAbs(parentDir) {
			out = append(out, filepath.Clean(filepath.Join(parentDir, inc)))
			continue
		}
		// parentAbsPath is not absolute (struct-literal test
		// path). Leave the entry as-is — the reverse-include
		// index just keys whatever string comes in, and the LSP
		// only invalidates absolute paths in production.
		out = append(out, filepath.Clean(inc))
	}
	return out
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
// returned wrapper carries the *cue.Context the value was compiled
// against so callers that need to Unify additional values use the same
// context (cue.Value cannot cross contexts).
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
