//go:build tinygo && js

package refactor

import "io/fs"

// sameDiskFile reports false: TinyGo's js port has no os.SameFile, and
// a WebAssembly workspace keeps its files in memory, with no on-disk
// info for two paths to share.
func sameDiskFile(fs.FileInfo, fs.FileInfo) bool {
	return false
}
