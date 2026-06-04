package mdsmith

import (
	"bytes"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"sync"

	"github.com/bmatcuk/doublestar/v4"
)

// OverlayWorkspace reads from the host filesystem rooted at Root, but
// lets in-memory buffers shadow the on-disk content of specific paths.
// It is the LSP server's workspace: an editor's unsaved-buffer bytes,
// pushed in through [Session.Invalidate] (which calls Set), shadow the
// file on disk so cross-file rules — catalog, include, links — read the
// live buffer rather than the last saved version.
//
// Only file content is overlaid. Open buffers still exist on disk, so
// directory listing and globbing defer to disk; the overlay supplies the
// shadowed bytes when a path is read. That keeps the fs.FS view cheap to
// build per lint pass — it clones only the small open-buffer map, never
// the whole corpus — so a per-keystroke Check pays no O(corpus) snapshot
// cost.
//
// OverlayWorkspace is safe for concurrent use. Reads take a read lock;
// Set and Delete take a write lock.
type OverlayWorkspace struct {
	root    string
	mu      sync.RWMutex
	overlay map[string][]byte
}

// NewOverlayWorkspace returns an OverlayWorkspace rooted at root with no
// buffers overlaid. Mutate the overlay through Set and Delete.
func NewOverlayWorkspace(root string) *OverlayWorkspace {
	return &OverlayWorkspace{root: root, overlay: map[string][]byte{}}
}

// cleanKey normalises p to the slash-separated, cleaned form the overlay
// map is keyed by, matching how the engine's FS view names paths.
func cleanKey(p string) string {
	return path.Clean(filepath.ToSlash(p))
}

// ReadFile returns the overlaid bytes for p when a buffer shadows it,
// otherwise it reads from disk resolved against Root (an absolute path
// is read unchanged), mirroring OSWorkspace so the on-disk fall-through
// and the FS view name the same file for one uri.
func (w *OverlayWorkspace) ReadFile(p string) ([]byte, error) {
	key := cleanKey(p)
	w.mu.RLock()
	data, ok := w.overlay[key]
	w.mu.RUnlock()
	if ok {
		return bytes.Clone(data), nil
	}
	return os.ReadFile(w.diskPath(p)) //nolint:gosec // path is caller-controlled; this is the native disk seam
}

// diskPath maps a workspace-relative path to an absolute on-disk path
// rooted at Root, matching OSWorkspace.resolve. Absolute paths and an
// empty Root pass through unchanged.
func (w *OverlayWorkspace) diskPath(p string) string {
	if w.root == "" || filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(w.root, filepath.FromSlash(p))
}

// Glob expands a doublestar pattern against the on-disk tree rooted at
// Root. Open buffers exist on disk, so they are already discovered here;
// the overlay only shadows content on read.
func (w *OverlayWorkspace) Glob(pattern string) ([]string, error) {
	matches, err := doublestar.FilepathGlob(filepath.Join(w.root, filepath.FromSlash(pattern)))
	if err != nil {
		return nil, err
	}
	sort.Strings(matches)
	return matches, nil
}

// FS returns an fs.FS that shadows disk with the current overlay
// contents. The overlay map is snapshotted (cloned) so a later Set or
// Delete does not affect an already-returned FS; the engine fetches a
// fresh FS per lint pass, so an overlay edit applied through Invalidate
// lands on the next Check. The snapshot copies only the open-buffer map,
// not the corpus.
func (w *OverlayWorkspace) FS() fs.FS {
	w.mu.RLock()
	snap := make(map[string][]byte, len(w.overlay))
	for k, v := range w.overlay {
		snap[k] = bytes.Clone(v)
	}
	w.mu.RUnlock()
	root := w.root
	if root == "" {
		root = "."
	}
	return &overlayFS{disk: os.DirFS(root), overlay: snap}
}

// Set stores data (cloned) as the overlay for p, shadowing disk on the
// next read.
func (w *OverlayWorkspace) Set(p string, data []byte) {
	key := cleanKey(p)
	w.mu.Lock()
	w.overlay[key] = bytes.Clone(data)
	w.mu.Unlock()
}

// Delete drops the overlay for p so the next read falls through to disk.
func (w *OverlayWorkspace) Delete(p string) {
	key := cleanKey(p)
	w.mu.Lock()
	delete(w.overlay, key)
	w.mu.Unlock()
}

// overlayFS is an fs.FS that serves overlaid bytes for shadowed paths
// and defers everything else (directory walks, globs, unshadowed reads)
// to disk. It implements fs.ReadFileFS so bytelimit.ReadFSFileLimited
// and the catalog/include rules read a shadowed path's buffer bytes
// without opening the on-disk file.
type overlayFS struct {
	disk    fs.FS
	overlay map[string][]byte
}

func (o *overlayFS) Open(name string) (fs.File, error) {
	if data, ok := o.overlay[name]; ok {
		return &memFile{name: name, data: data}, nil
	}
	return o.disk.Open(name)
}

func (o *overlayFS) ReadFile(name string) ([]byte, error) {
	if data, ok := o.overlay[name]; ok {
		return bytes.Clone(data), nil
	}
	return fs.ReadFile(o.disk, name)
}

func (o *overlayFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return fs.ReadDir(o.disk, name)
}

func (o *overlayFS) Glob(pattern string) ([]string, error) {
	return doublestar.Glob(o.disk, pattern)
}
