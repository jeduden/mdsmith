//go:build wasm

package schema

import "github.com/jeduden/mdsmith/internal/lint"

// This file holds the WASM-build stubs for the CUE-backed
// front-matter validators. The CUE engine that powers MDS020's
// front-matter constraint checking is build-tagged out of
// GOOS=js GOARCH=wasm builds to keep the artifact under its size
// budget (CUE pulls in ~95 packages). MDS020's heading-structure,
// filename, and content checks — none of which need CUE — still run
// under WASM; only the front-matter CUE-constraint validation is
// dropped. See docs/background/concepts/engine-api.md.

// validateFrontmatterDiags is the WASM no-op stub for the CUE-backed
// front-matter validator. It returns no diagnostics so Validate's
// heading-structure path runs unchanged while the CUE constraint
// check is skipped.
func validateFrontmatterDiags(
	_ *lint.File, _ *Schema, _ map[string]any, _ MakeDiag,
) []lint.Diagnostic {
	return nil
}

// ValidateFrontmatter reports no error in the WASM build: there is no
// CUE engine to validate against, so the constraint check is a no-op.
func ValidateFrontmatter(_ *Schema, _ map[string]any) error { return nil }

// ValidateFrontmatterSyntax reports no error in the WASM build: the
// CUE compiler that checks schema syntax is build-tagged out.
func ValidateFrontmatterSyntax(_ *Schema) error { return nil }
