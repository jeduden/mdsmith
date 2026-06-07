//go:build wasm

package cuetemplate

import "errors"

// ErrUnsupported is returned by the WASM build's cuetemplate stubs.
// The CUE engine that evaluates row-expression templates is
// build-tagged out of GOOS=js GOARCH=wasm builds to keep the artifact
// under its size budget (CUE pulls in ~95 packages). See
// docs/background/concepts/engine-api.md.
var ErrUnsupported = errors.New("cue templates are not available in the WASM build")

// Template is the WASM-build placeholder for the CUE-backed template.
// It carries no state because Compile never succeeds under WASM.
type Template struct{}

// Compile reports that CUE templates are unavailable in the WASM
// build. The catalog rule treats a compile error as an invalid
// `row:` expression and surfaces it as a diagnostic, so a CUE
// `row:` expression under WASM degrades gracefully rather than
// crashing. Plain `{field}` row templates do not reach this path —
// they are handled by the CUE-free internal/fieldinterp.
func Compile(_ string) (*Template, error) {
	return nil, ErrUnsupported
}

// Render reports that CUE templates are unavailable in the WASM
// build. It exists only to satisfy the type's method set; Compile
// never returns a non-nil Template under WASM, so this is never
// reached on a real render path.
func (t *Template) Render(_ map[string]any) (string, error) {
	return "", ErrUnsupported
}
