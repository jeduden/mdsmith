//go:build !wasm

package schema

import "cuelang.org/go/cue/cuecontext"

// checkUnifiable reports whether a CUE expression can be reduced
// without contradiction. It compiles the expression in a fresh CUE
// context; the compiled value's Err() is non-nil whenever the
// expression reduces to bottom (CUE's "no value satisfies"
// outcome), so a simple Err()-check covers every conflict shape
// the plan cares about — `int & string`, conflicting bounds,
// closed-struct violations, unresolved references.
//
// This CUE-backed implementation is built only on native
// (//go:build !wasm). The WASM build replaces it with a no-op stub
// (extend_wasm.go) so CUE stays out of the artifact; kind `extends`
// merging still runs there, but a contradictory unified constraint
// is not detected at merge time. See
// docs/background/concepts/engine-api.md.
func checkUnifiable(expr string) error {
	v := cuecontext.New().CompileString(expr)
	return v.Err()
}
