//go:build unix || windows || plan9

package main

// These tests capture stderr through an os.Pipe, and all but the
// usage-error one run the real go toolchain; a js/wasm test binary can
// do neither.

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

// captureStderr runs fn with os.Stderr redirected to a pipe and returns
// everything written to it, including by child processes that inherit
// os.Stderr.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	return captureFile(t, &os.Stderr, fn)
}

// TestRunTestJSWasmRequireJSOnlyNeedsAll rejects --require-js-only
// without --all as a usage error instead of silently ignoring it.
func TestRunTestJSWasmRequireJSOnlyNeedsAll(t *testing.T) {
	var code int
	stderr := captureStderr(t, func() {
		code = run([]string{"test-js-wasm", "--require-js-only", "."})
	})
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "mdsmith-release: test-js-wasm: --require-js-only needs --all")
}

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
