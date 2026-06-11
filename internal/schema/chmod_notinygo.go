//go:build !tinygo

package schema

import "os"

// chmodFile sets the permission bits of the named file.
// On non-tinygo builds this is a direct call to os.Chmod.
func chmodFile(name string, mode os.FileMode) error {
	return os.Chmod(name, mode)
}
