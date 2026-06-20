//go:build tinygo

package mdsmith

import (
	"io/fs"
	"os"
)

// FS returns os.DirFS(Root) on tinygo/wasm builds. The wasm sandbox has no
// real filesystem symlinks, so RESOLVE_BENEATH containment via os.OpenRoot
// (unavailable in TinyGo) is unnecessary. OSWorkspace is not used in the
// wasm build (MemWorkspace drives it); this stub satisfies the interface.
func (w OSWorkspace) FS() fs.FS {
	root := w.Root
	if root == "" {
		root = "."
	}
	return os.DirFS(root)
}
