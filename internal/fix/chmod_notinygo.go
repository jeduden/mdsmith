//go:build !tinygo

package fix

import "os"

// chmodFile sets the permission bits of the named file.
// Exposed as a variable so tests can inject failures without OS tricks.
var chmodFile = func(name string, mode os.FileMode) error {
	return os.Chmod(name, mode)
}
