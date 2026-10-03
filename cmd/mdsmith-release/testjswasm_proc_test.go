//go:build unix || windows

package main

// These tests run the real go toolchain, which a js/wasm test binary
// cannot spawn.

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestRunTestJSWasm dispatches through `run test-js-wasm` on this
// package, which has no js/wasm-only test files. The runner must fail
// after go list and before go test (no Node needed): exit 1 with that
// specific error, not some earlier failure such as a broken go list.
func TestRunTestJSWasm(t *testing.T) {
	var code int
	stderr := captureStderr(t, func() { code = run([]string{"test-js-wasm", "."}) })
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "mdsmith-release: no js/wasm-only test files in .")
}

// TestRunTestJSWasmAll dispatches `test-js-wasm --all` at a package
// that does not exist: the js/wasm go list fails, before go test or
// Node runs, and names the load error.
func TestRunTestJSWasmAll(t *testing.T) {
	var code int
	stderr := captureStderr(t, func() { code = run([]string{"test-js-wasm", "--all", "./internal/does-not-exist"}) })
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "mdsmith-release: go list (js/wasm) ./internal/does-not-exist")
}

// TestRunTestJSWasmAllRequireJSOnly dispatches `test-js-wasm --all
// --require-js-only` at this package, which has no js/wasm-only test
// files: it fails after go list and before go test, so no Node.
func TestRunTestJSWasmAllRequireJSOnly(t *testing.T) {
	var code int
	stderr := captureStderr(t, func() {
		code = run([]string{"test-js-wasm", "--all", "--require-js-only", "."})
	})
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "mdsmith-release: no js/wasm-only Test functions in .")
}
