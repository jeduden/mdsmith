//go:build tinygo

package mdsmith

import (
	"io/fs"
	"os"

	"github.com/jeduden/mdsmith/internal/bytelimit"
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

// readFileRooted reads relPath from disk rooted at root. os.DirFS opens and
// closes its own file handle per Open call, so there is no per-call handle
// to accumulate here the way the !tinygo os.OpenRoot-backed variant must
// guard against.
func readFileRooted(root, relPath string) ([]byte, error) {
	return fs.ReadFile(os.DirFS(root), relPath)
}

// readFileRootedLimited mirrors readFileRooted but caps the read via
// bytelimit — see OSWorkspace.readFileLimited.
func readFileRootedLimited(root, relPath string, max int64) ([]byte, error) {
	return bytelimit.ReadFSFileLimited(os.DirFS(root), relPath, max)
}
