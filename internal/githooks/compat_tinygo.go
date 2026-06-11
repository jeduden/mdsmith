//go:build tinygo

package githooks

import "os"

// chmodFn is a no-op on tinygo/wasm builds. The wasm host has no POSIX mode
// bits; skipping the chmod degrades nothing the engine reads back.
var chmodFn = func(_ string, _ os.FileMode) error { return nil }

// sameFile returns true on tinygo/wasm builds. The wasm sandbox has no
// symlinks and no concurrent filesystem activity, so the lstat/fstat
// TOCTOU check in atomicWriteGitattributes can never detect a swap;
// returning true lets the write proceed rather than always failing with
// "file changed since lstat" on every update to an existing file.
func sameFile(_, _ os.FileInfo) bool {
	return true
}
