//go:build !tinygo

package requiredstructure

import "os"

// sameFile wraps os.SameFile. On non-tinygo builds this is a direct call.
func sameFile(fi1, fi2 os.FileInfo) bool {
	return os.SameFile(fi1, fi2)
}
