//go:build wasm

package query

import "errors"

// ErrUnsupported is returned by the WASM build's query stubs. The CUE
// engine that backs front-matter querying is build-tagged out of
// GOOS=js GOARCH=wasm builds to keep the artifact under its size
// budget (CUE pulls in ~95 packages). See
// docs/background/concepts/engine-api.md.
var ErrUnsupported = errors.New("front-matter query is not available in the WASM build")

// Matcher is the WASM-build placeholder for the CUE-backed matcher.
// It carries no state because Compile never succeeds under WASM.
type Matcher struct{}

// Compile reports that front-matter querying is unavailable in the
// WASM build. The catalog rule treats a query-compile error as an
// invalid `where:` expression and surfaces it as a diagnostic rather
// than crashing, so a `where:` clause under WASM degrades gracefully.
func Compile(_ string) (*Matcher, error) {
	return nil, ErrUnsupported
}

// Match always reports false: a WASM build never compiles a Matcher,
// so this exists only to satisfy the type's method set.
func (m *Matcher) Match(_ map[string]any) bool { return false }

// Match reports that front-matter querying is unavailable in the WASM
// build, mirroring the native convenience function's signature.
func Match(_ string, _ map[string]any) (bool, error) {
	return false, ErrUnsupported
}
