//go:build !wasm

package githooks

import "os"

// chmodFn is a variable so tests can inject failures into
// atomicWriteGitattributes without OS tricks. It is built only on
// native: tinygo's wasm runtime omits os.Chmod, and a WASM host never
// installs git hooks. See docs/background/concepts/engine-api.md.
var chmodFn = os.Chmod

// sameFile reports whether two FileInfos describe the same file. It
// wraps os.SameFile so the WASM build (whose runtime omits it) can
// stub it out.
func sameFile(a, b os.FileInfo) bool {
	return os.SameFile(a, b)
}
