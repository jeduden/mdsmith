//go:build !wasm

package schema

import (
	"encoding/json"
	"fmt"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/errors"
	"github.com/jeduden/mdsmith/internal/lint"
)

// validateFrontmatterDiags compiles the schema's CUE expression,
// unifies it with the document front matter, and emits one
// diagnostic per resulting CUE error (rather than collapsing all
// of them into a single line). Each diagnostic is a
// SchemaDiagnostic rendered through Format(): the field that
// failed, the value the user wrote, the constraint they
// violated, and — when applicable — a hint.
//
// A schema-side compile or marshal failure surfaces as a single
// fallback diagnostic at line 1 so users still see a signal
// without needing to chase the underlying CUE error. The
// formerly-flat "front matter does not satisfy schema CUE
// constraints" message is intentionally retired; see plan 147.
//
// This is the CUE-backed implementation, built only on native
// (//go:build !wasm). The WASM build replaces it with a no-op stub
// (validate_wasm.go) so CUE stays out of the artifact; MDS020's
// heading-structure checks still run there, only the front-matter
// CUE-constraint validation is dropped. See
// docs/background/concepts/engine-api.md.
func validateFrontmatterDiags(
	f *lint.File, sch *Schema, docFM map[string]any, mkDiag MakeDiag,
) []lint.Diagnostic {
	expr := sch.FrontmatterCUE()
	if strings.TrimSpace(expr) == "" {
		return nil
	}
	anchor := nonBodyDiagLine(f)
	// Route the schema-side CompileString through RunCache.CompiledCUE
	// when one is in scope so N host files sharing a schema compile its
	// CUE source exactly once per Run. The wrapper carries the
	// *cue.Context the value was compiled against; the document
	// front-matter CompileBytes below must reuse that context because
	// cue.Value cannot cross contexts.
	var cache *lint.RunCache
	if f != nil {
		cache = f.RunCache
	}
	compiled := CachedCompile(cache, expr)
	schemaVal := compiled.Value
	ctx := compiled.Ctx
	if err := compiled.Err(); err != nil {
		return []lint.Diagnostic{
			compileFailureDiag(sch, "schema", "valid schema CUE", err).
				Emit(mkDiag, f.Path, anchor)}
	}
	if docFM == nil {
		docFM = map[string]any{}
	}
	data, err := json.Marshal(docFM)
	if err != nil {
		return []lint.Diagnostic{
			compileFailureDiag(sch, "front matter", "JSON-marshalable front matter", err).
				Emit(mkDiag, f.Path, anchor)}
	}
	// CompileBytes parses the JSON the marshal above produced, which is
	// always valid CUE, so it cannot error here. A bottom value would
	// still surface through the Unify + Validate path below, so there is
	// no separate (untestable) compile-failure branch for it.
	dataVal := ctx.CompileBytes(data)
	merged := schemaVal.Unify(dataVal)
	verr := merged.Validate(cue.Concrete(true))
	if verr == nil {
		// Skip the docFrontmatterKeyLines YAML re-parse when there
		// is nothing for the deprecation walker to do — the common
		// success path then pays no per-file overhead.
		if len(sch.FrontmatterMeta) == 0 || len(docFM) == 0 {
			return nil
		}
		return validateDeprecatedFieldsWithLines(
			f, sch, docFM, docFrontmatterKeyLines(f), mkDiag)
	}
	// errors.Errors returns a non-empty list for any non-nil CUE
	// validation error, so there is no separate (untestable) "valid CUE"
	// fallback; the per-error diagnostics below cover every reachable
	// validation failure.
	keyLines := docFrontmatterKeyLines(f)
	out := dedupedCUEErrorDiags(f, sch, docFM, errors.Errors(verr), keyLines, mkDiag)
	return append(out, validateDeprecatedFieldsWithLines(f, sch, docFM, keyLines, mkDiag)...)
}

// dedupedCUEErrorDiags maps each unique CUE error to a
// SchemaDiagnostic. A struct dedup key avoids accidental collisions
// when one of the components (notably the raw-CUE-expression
// Expected fallback and a placeholder-bearing Field) legitimately
// contains the same delimiter a flat string key would have used.
func dedupedCUEErrorDiags(
	f *lint.File, sch *Schema, docFM map[string]any,
	cueErrs []errors.Error, keyLines map[string]int, mkDiag MakeDiag,
) []lint.Diagnostic {
	type dedupKey struct{ field, actual, expected string }
	seen := make(map[dedupKey]bool, len(cueErrs))
	out := make([]lint.Diagnostic, 0, len(cueErrs))
	for _, ce := range cueErrs {
		d := schemaDiagFromCUEError(sch, docFM, ce)
		key := dedupKey{field: d.Field, actual: d.Actual, expected: d.Expected}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, d.Emit(mkDiag, f.Path, fmDiagLine(f, ce.Path(), keyLines)))
	}
	return out
}

// schemaDiagFromCUEError converts one CUE-error leaf into a
// SchemaDiagnostic. The CUE error's Path() names the offending
// field; we look up the raw constraint expression on the schema
// to render an "expected" string in user vocabulary, and pull
// the actual value out of docFM so the message shows exactly
// what the user wrote.
//
// Precision note: lookupConstraint and schemaRef both resolve
// against path[0] (the top-level frontmatter key the schema's
// Frontmatter map indexes by). For nested CUE errors — e.g.
// `meta.owner` against a schema whose `meta:` value is itself
// a struct constraint — Field still shows the full dotted
// path so the reader can locate the failing leaf, but Expected
// renders the parent constraint (the top-level CUE expression)
// rather than the leaf. Today every shipped schema in
// mdsmith uses single-segment frontmatter constraints, so
// this asymmetry is hypothetical; option (a) from the PR #284
// review (CUE LookupPath on ce.Path() for leaf-level
// resolution) is the natural follow-up if nested frontmatter
// constraints land later.
func schemaDiagFromCUEError(
	sch *Schema, docFM map[string]any, ce errors.Error,
) SchemaDiagnostic {
	path := ce.Path()
	field := "front matter"
	if len(path) > 0 {
		field = strings.Join(path, ".")
	}
	d := SchemaDiagnostic{
		Field:     field,
		SchemaRef: schemaRef(sch, schemaKeyForPath(sch, path)),
	}
	actualVal, hasActual := lookupFM(docFM, path)
	if hasActual {
		d.Actual = formatActual(actualVal)
	}
	if expr := lookupConstraint(sch, path); expr != "" {
		d.Expected = RenderExpected(expr)
		if hasActual {
			d.Hint = RenderHint(expr, actualVal)
		} else {
			// A required field that the document omitted. Show
			// the same <missing> sentinel structure diagnostics
			// use so every diagnostic answers the same three
			// questions: which field, what value, what's
			// expected.
			d.Actual = "<missing>"
		}
	} else {
		// Extra field: close() rejected a key that is not in the
		// schema's frontmatter map. There is no per-field
		// constraint to render, and the schema source already
		// names the declared set; the diagnostic body says so
		// explicitly so the reader can compare against the
		// schema file. <extra field> is the actual-slot sentinel
		// for the (rare) case where the key has no value to show
		// (e.g. an empty mapping entry).
		if !hasActual {
			d.Actual = "<extra field>"
		}
		d.Expected = "not declared in schema"
	}
	return d
}

// ValidateFrontmatter compiles sch.Frontmatter into a CUE schema and
// unifies it with fm (the document's parsed front matter).
func ValidateFrontmatter(sch *Schema, fm map[string]any) error {
	expr := sch.FrontmatterCUE()
	if strings.TrimSpace(expr) == "" {
		return nil
	}
	ctx := cuecontext.New()
	schemaVal := ctx.CompileString(expr)
	if err := schemaVal.Err(); err != nil {
		return fmt.Errorf("invalid CUE schema: %w", err)
	}
	if fm == nil {
		fm = map[string]any{}
	}
	data, err := json.Marshal(fm)
	if err != nil {
		return fmt.Errorf("serialize front matter: %w", err)
	}
	dataVal := ctx.CompileBytes(data)
	merged := schemaVal.Unify(dataVal)
	if err := merged.Validate(cue.Concrete(true)); err != nil {
		return err
	}
	return nil
}

// ValidateFrontmatterSyntax checks that the schema's frontmatter
// constraints compile as CUE. Returns nil if there are no
// constraints.
func ValidateFrontmatterSyntax(sch *Schema) error {
	expr := sch.FrontmatterCUE()
	if strings.TrimSpace(expr) == "" {
		return nil
	}
	ctx := cuecontext.New()
	v := ctx.CompileString(expr)
	if err := v.Err(); err != nil {
		return fmt.Errorf("invalid schema frontmatter CUE: %w", err)
	}
	return nil
}
