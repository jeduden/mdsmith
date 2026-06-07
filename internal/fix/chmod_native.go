//go:build !wasm

package fix

import "os"

// chmodFile applies mode to the file at path. It is its own function
// so the WASM build can stub it out: tinygo's wasm runtime omits
// os.Chmod, and a WASM host writes through an in-memory workspace
// rather than this on-disk atomic-write path anyway. See
// docs/background/concepts/engine-api.md.
func chmodFile(path string, mode os.FileMode) error {
	return os.Chmod(path, mode)
}
