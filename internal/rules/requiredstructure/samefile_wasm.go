//go:build wasm

package requiredstructure

// sameStatFile always reports ok=false under WASM: there is no OS disk
// to stat, and tinygo's wasm runtime omits os.SameFile. The caller
// then falls back to a cleaned-absolute-path comparison, which is the
// only meaningful test for a MemWorkspace document. See
// docs/background/concepts/engine-api.md.
func sameStatFile(_, _ string) (same, ok bool) { return false, false }
