//go:build tinygo

package schema

import "os"

// chmodFile is a no-op on tinygo/wasm builds. The wasm host has no
// POSIX mode bits; skipping the chmod degrades nothing the engine
// reads back.
var chmodFile = func(_ string, _ os.FileMode) error {
	return nil
}
