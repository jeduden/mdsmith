//go:build !tinygo

package oscompat

import "os"

// SameFile wraps os.SameFile. It reports whether fi1 and fi2 describe the
// same file.
func SameFile(fi1, fi2 os.FileInfo) bool {
	return os.SameFile(fi1, fi2)
}
