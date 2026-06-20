//go:build !tinygo

package lint

import (
	"io/fs"
	"os"
)

// OpenRootFS returns an fs.FS rooted at dir that enforces RESOLVE_BENEATH
// containment via os.OpenRoot: any Open that resolves through a symlink to
// a path outside dir is denied with an error. This prevents within-workspace
// symlinks from escaping the project root during include and catalog
// generation.
//
// If os.OpenRoot itself fails (e.g. dir does not exist), the error from
// every subsequent Open call propagates to the caller.
//
// Within-workspace symlinks whose targets are also inside dir continue to
// work: os.OpenRoot follows symlinks that stay inside the root.
func OpenRootFS(dir string) fs.FS {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return &openRootErrFS{err: err}
	}
	return root.FS()
}

// openRootErrFS is an fs.FS that always returns the stored error on Open.
// Used when os.OpenRoot fails (e.g. directory does not exist) so callers
// that hold an fs.FS see a consistent Open-time error rather than a panic.
type openRootErrFS struct {
	err error
}

func (e *openRootErrFS) Open(name string) (fs.File, error) {
	return nil, &fs.PathError{Op: "open", Path: name, Err: e.err}
}
