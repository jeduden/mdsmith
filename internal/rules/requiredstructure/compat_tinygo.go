//go:build tinygo

package requiredstructure

import "os"

// sameFile always returns false on tinygo/wasm builds because
// os.SameFile is not implemented for the wasm target. A false return
// does not mean the files are distinct; callers that need a definitive
// same-file answer should also compare resolved absolute paths.
func sameFile(_, _ os.FileInfo) bool {
	return false
}
