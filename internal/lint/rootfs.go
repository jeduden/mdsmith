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
// every subsequent Open call propagates to the caller rather than silently
// falling back to an unconstrained fs.FS.
//
// Relative within-workspace symlinks whose targets resolve inside dir
// continue to work. Absolute symlinks are blocked unconditionally by
// os.OpenRoot (RESOLVE_BENEATH semantics), regardless of whether their
// target is inside or outside the root.
//
// The returned RootFS holds the os.Root directory handle open until its
// Close; a caller that walks the tree once closes it when done.
func OpenRootFS(dir string) RootFS {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return &openRootErrFS{err: err}
	}
	return &closingFS{fsys: root.FS(), close: root.Close}
}

// openRootErrFS is an fs.FS that always returns the stored error on Open.
// Used when os.OpenRoot fails (e.g. directory does not exist) so callers
// that hold an fs.FS see a consistent Open-time error rather than a panic.
type openRootErrFS struct {
	err error
}

func (e *openRootErrFS) Open(name string) (fs.File, error) {
	return nil, e.err
}

// Close is a no-op: no directory handle was opened.
func (e *openRootErrFS) Close() error { return nil }
