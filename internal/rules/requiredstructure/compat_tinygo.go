//go:build tinygo

package requiredstructure

import "os"

// sameFile returns false on tinygo/wasm builds. os.SameFile is not
// implemented on tinygo's wasm target; returning false lets isSchemaFile
// fall through to its path-equality comparison, which is an accurate
// substitute in the wasm sandbox (no hard links or symlinks).
func sameFile(_, _ os.FileInfo) bool {
	return false
}
