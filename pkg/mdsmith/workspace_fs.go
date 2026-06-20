//go:build !tinygo

package mdsmith

import (
	"io/fs"

	"github.com/jeduden/mdsmith/internal/lint"
)

// FS returns an fs.FS rooted at Root (or at "." when Root is empty) backed
// by os.OpenRoot so that symlinks inside the workspace cannot escape to paths
// outside Root (RESOLVE_BENEATH semantics). Relative within-workspace
// symlinks whose targets stay inside Root continue to work; absolute symlinks
// are blocked by os.OpenRoot unconditionally. When os.OpenRoot itself fails,
// every subsequent Open returns the construction error rather than silently
// falling back to an unconstrained fs.FS.
func (w OSWorkspace) FS() fs.FS {
	root := w.Root
	if root == "" {
		root = "."
	}
	return lint.OpenRootFS(root)
}
