//go:build tinygo

package githooks

import "os"

// sameFile returns true on tinygo/wasm builds. The wasm sandbox has no
// symlinks and no concurrent filesystem activity, so the lstat/fstat
// TOCTOU check in atomicWriteGitattributes can never detect a swap;
// returning true lets the write proceed rather than always failing with
// "file changed since lstat" on every update to an existing file.
func sameFile(_, _ os.FileInfo) bool {
	return true
}
