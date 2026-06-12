//go:build tinygo

package oscompat

import "os"

// SameFile always returns false on tinygo/wasm builds because os.SameFile
// is not implemented for the wasm target. A false return does not mean the
// files are distinct; callers that need a definitive same-file answer should
// also compare resolved absolute paths.
func SameFile(_, _ os.FileInfo) bool {
	return false
}
