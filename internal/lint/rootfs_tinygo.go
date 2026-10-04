//go:build tinygo

package lint

import "os"

// OpenRootFS returns os.DirFS(dir) on tinygo/wasm builds. The wasm sandbox
// has no real filesystem symlinks, so RESOLVE_BENEATH containment via
// os.OpenRoot (unavailable in TinyGo) is unnecessary. os.DirFS holds no
// handle, so Close is a no-op.
func OpenRootFS(dir string) RootFS {
	return &closingFS{fsys: os.DirFS(dir), close: func() error { return nil }}
}
