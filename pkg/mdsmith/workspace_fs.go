//go:build !tinygo

package mdsmith

import (
	"io/fs"
	"os"
)

// FS returns an fs.FS rooted at Root (or at "." when Root is empty) backed
// by os.OpenRoot so that symlinks inside the workspace cannot escape to paths
// outside Root (RESOLVE_BENEATH semantics). Within-workspace symlinks to
// files inside Root continue to work.
func (w OSWorkspace) FS() fs.FS {
	root := w.Root
	if root == "" {
		root = "."
	}
	r, err := os.OpenRoot(root)
	if err != nil {
		// Fall back to DirFS on open failure (e.g. root does not exist)
		// so callers that check err on Open rather than at construction
		// still see a consistent read error.
		return os.DirFS(root)
	}
	return r.FS()
}
