//go:build wasm

package schema

import "github.com/jeduden/mdsmith/internal/lint"

// CompiledCUE is the WASM-build placeholder for the CUE-backed
// compiled schema. CUE schema validation is build-tagged out of
// GOOS=js GOARCH=wasm builds to keep the artifact under its size
// budget (CUE pulls in ~95 packages). See
// docs/background/concepts/engine-api.md.
type CompiledCUE struct{}

// Err always returns nil for the WASM stub: CachedCompile returns
// nil under WASM, so callers that do cachedCompiledCUEWith(...).Err()
// reach the nil-receiver branch and skip validation gracefully.
func (c *CompiledCUE) Err() error { return nil }

// CachedCompile returns nil in the WASM build. Callers that check
// Err() before proceeding will short-circuit via the nil-safe Err
// above; callers that use the Value/Ctx fields are in files already
// gated behind //go:build !wasm.
func CachedCompile(_ *lint.RunCache, _ string) *CompiledCUE { return nil }
