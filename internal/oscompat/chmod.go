//go:build !tinygo

package oscompat

import "os"

// Chmod sets the permission bits of the named file.
// It wraps os.Chmod on standard builds.
func Chmod(path string, mode os.FileMode) error {
	return os.Chmod(path, mode)
}
