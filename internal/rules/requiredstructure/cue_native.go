//go:build !wasm

package requiredstructure

import (
	"encoding/json"
	"fmt"
	"strings"

	"cuelang.org/go/cue"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/schema"
)

// This file holds MDS020's CUE-backed front-matter validation. It is
// built only on native (//go:build !wasm). The WASM build replaces
// these helpers with no-op stubs (cue_wasm.go) so CUE's ~95 packages
// stay out of the artifact; MDS020's heading-structure, filename, and
// content checks still run under WASM, but the front-matter
// CUE-constraint validation is dropped. See
// docs/background/concepts/engine-api.md.

// validateCUESchemaSyntax checks that schema compiles as CUE.
//
// The free function form (no RunCache) is the canonical entry point
// kept for tests. The cache-bound variant validateCUESchemaSyntaxWith
// is reached from parseSchemaFrontMatter when a cache is in scope,
// so two schemas with identical CUE source share one compile per
// Run.
func validateCUESchemaSyntax(schema string) error {
	return validateCUESchemaSyntaxWith(nil, schema)
}

// validateCUESchemaSyntaxWith is validateCUESchemaSyntax with the
// CompileString site routed through cache when non-nil. A nil cache
// compiles a fresh value (the test-direct path).
func validateCUESchemaSyntaxWith(cache *lint.RunCache, schema string) error {
	if strings.TrimSpace(schema) == "" {
		return nil
	}
	if err := cachedCompiledCUEWith(cache, schema).Err(); err != nil {
		return fmt.Errorf("invalid schema frontmatter CUE: %w", err)
	}
	return nil
}

func validateFrontMatterCUE(schemaSrc string, fm map[string]any) error {
	if strings.TrimSpace(schemaSrc) == "" {
		return nil
	}

	compiled := cachedCompiledCUEWith(nil, schemaSrc)
	if err := compiled.Err(); err != nil {
		return fmt.Errorf("invalid CUE schema: %w", err)
	}

	if fm == nil {
		fm = map[string]any{}
	}

	data, err := json.Marshal(fm)
	if err != nil {
		return fmt.Errorf("serialize front matter: %w", err)
	}

	// The data value must come from the same cue.Context as the
	// schema value — cue values cannot cross contexts. The cached
	// wrapper exposes its Context for exactly this case.
	// CompileBytes of json.Marshal output is always valid CUE, so
	// any error on dataVal would also surface through merged.Validate
	// below; the previous explicit check carried no testable path.
	dataVal := compiled.Ctx.CompileBytes(data)

	merged := compiled.Value.Unify(dataVal)
	if err := merged.Validate(cue.Concrete(true)); err != nil {
		return err
	}

	return nil
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
