//go:build !tinygo

package mdsmith

import (
	"io/fs"
	"os"

	"github.com/jeduden/mdsmith/internal/bytelimit"
	"github.com/jeduden/mdsmith/internal/lint"
)

// FS returns an fs.FS rooted at Root (or at "." when Root is empty) backed
// by os.OpenRoot so that symlinks inside the workspace cannot escape to paths
// outside Root (RESOLVE_BENEATH semantics). Relative within-workspace
// symlinks whose targets stay inside Root continue to work; absolute symlinks
// are blocked by os.OpenRoot unconditionally. When os.OpenRoot itself fails,
// every subsequent Open returns the construction error rather than silently
// falling back to an unconstrained fs.FS.
//
// The returned fs.FS holds an open os.Root directory handle for as long as
// it is reachable. Callers that fetch a fresh FS() per operation (rather
// than caching one for a longer-lived object, as OverlayWorkspace does) must
// not do so in a tight per-read loop — see readFileRooted, which ReadFile
// uses instead so a single read does not leak a handle.
func (w OSWorkspace) FS() fs.FS {
	root := w.Root
	if root == "" {
		root = "."
	}
	return lint.OpenRootFS(root)
}

// readFileRooted reads relPath from disk rooted at root through a
// short-lived os.Root handle that is closed before returning, so a caller
// making many ReadFile calls (e.g. resolving front matter file by file)
// does not accumulate open directory handles the way repeatedly calling
// FS() would (see FS's doc comment).
func readFileRooted(root, relPath string) ([]byte, error) {
	r, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer r.Close() //nolint:errcheck // best-effort close on read-only handle
	return fs.ReadFile(r.FS(), relPath)
}

// readFileRootedLimited mirrors readFileRooted but caps the read via
// bytelimit, so a file over max is rejected without being fully read
// into memory — see OSWorkspace.readFileLimited.
func readFileRootedLimited(root, relPath string, max int64) ([]byte, error) {
	r, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer r.Close() //nolint:errcheck // best-effort close on read-only handle
	return bytelimit.ReadFSFileLimited(r.FS(), relPath, max)
}
