//go:build wasm

package schema

// checkUnifiable is the WASM no-op stub for the CUE conflict check.
// The CUE compiler that detects a contradictory unified frontmatter
// constraint is build-tagged out of GOOS=js GOARCH=wasm builds to
// keep the artifact under its size budget. Kind `extends` merging
// still runs under WASM; only the merge-time contradiction check is
// dropped. See docs/background/concepts/engine-api.md.
func checkUnifiable(_ string) error { return nil }
