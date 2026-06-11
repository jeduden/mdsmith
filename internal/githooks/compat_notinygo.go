//go:build !tinygo

package githooks

import "os"

// chmodFn is exposed as a variable so tests can inject failures into
// atomicWriteGitattributes without OS tricks.
var chmodFn = os.Chmod

// sameFile wraps os.SameFile.
func sameFile(fi1, fi2 os.FileInfo) bool {
	return os.SameFile(fi1, fi2)
}
