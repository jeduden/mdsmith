//go:build !tinygo

package githooks

import "os"

// sameFile wraps os.SameFile. This is a caller-level seam rather than
// delegating to oscompat.SameFile because the tinygo stub returns true
// (let the write proceed) rather than false — a deliberate TOCTOU
// trade-off specific to atomicWriteGitattributes.
func sameFile(fi1, fi2 os.FileInfo) bool {
	return os.SameFile(fi1, fi2)
}
