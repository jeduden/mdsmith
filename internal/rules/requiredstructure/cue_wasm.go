//go:build wasm

package requiredstructure

import "github.com/jeduden/mdsmith/internal/lint"

// validateCUESchemaSyntaxWith is a no-op under WASM: CUE schema
// validation is build-tagged out of GOOS=js GOARCH=wasm builds to
// keep the artifact under its size budget. Front-matter CUE validation
// degrades gracefully rather than crashing.
// See docs/background/concepts/engine-api.md.
func validateCUESchemaSyntaxWith(_ *lint.RunCache, _ string) error { return nil }

// validateFrontMatterCUE is a no-op under WASM for the same reason.
func validateFrontMatterCUE(_ string, _ map[string]any) error { return nil }
