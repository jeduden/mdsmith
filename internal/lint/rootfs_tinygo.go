//go:build tinygo

package lint

import (
	"io/fs"
	"os"
)

// OpenRootFS returns os.DirFS(dir) on tinygo/wasm builds. The wasm sandbox
// has no real filesystem symlinks, so RESOLVE_BENEATH containment via
// os.OpenRoot (unavailable in TinyGo) is unnecessary.
func OpenRootFS(dir string) fs.FS {
	return os.DirFS(dir)
}
