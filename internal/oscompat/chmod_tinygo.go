//go:build tinygo

package oscompat

import "os"

// Chmod is a no-op on tinygo/wasm builds. The wasm host has no POSIX mode
// bits; skipping the chmod degrades nothing the engine reads back.
func Chmod(_ string, _ os.FileMode) error {
	return nil
}
