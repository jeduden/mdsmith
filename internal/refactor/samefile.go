//go:build !(tinygo && js)

package refactor

import (
	"io/fs"
	"os"
)

// sameDiskFile reports whether fi1 and fi2 describe one file on disk
// (os.SameFile). TinyGo's js port has no os.SameFile; see
// samefile_tinygo.go.
func sameDiskFile(fi1, fi2 fs.FileInfo) bool {
	return os.SameFile(fi1, fi2)
}
