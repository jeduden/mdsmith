package mdsmith

import (
	"bytes"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path"
	"path/filepath"
	"sort"
	"sync"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/jeduden/mdsmith/internal/bytelimit"
	"github.com/jeduden/mdsmith/internal/lint"
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
	root     string
	mu       sync.RWMutex
	overlay  map[string][]byte
	diskOnce sync.Once
	diskFS   fs.FS
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
// otherwise it falls through to disk. The disk fall-through reads through
// the cached, RESOLVE_BENEATH-contained disk FS (ensureDiskFS) directly,
// not through FS(), so a symlink escaping Root is refused the same way
// FS() refuses it without paying FS()'s per-call overlay-map clone (see
// FS's doc comment) — a miss should stay O(1), not O(len(overlay)).
func (w *OverlayWorkspace) ReadFile(p string) ([]byte, error) {
	key := cleanKey(p)
	w.mu.RLock()
	data, ok := w.overlay[key]
	w.mu.RUnlock()
	if ok {
		return bytes.Clone(data), nil
	}
	if w.root == "" || filepath.IsAbs(p) {
		return os.ReadFile(p) //nolint:gosec // path is caller-controlled; this is the native disk seam
	}
	return fs.ReadFile(w.ensureDiskFS(), key)
}

// readFileLimited is ReadFile's bounded counterpart: an overlaid buffer
// over max is rejected with a "file too large" error (the buffer is
// already resident — Set copies open-editor bytes the LSP already
// holds — so this is a size-cap check, not a read reduction), and the
// disk fall-through is capped via bytelimit so an oversized on-disk file
// is never fully read into memory. frontMatterFor uses this so
// resolving a file's kind never pulls an arbitrarily large document
// fully resident just to read its leading front-matter block.
func (w *OverlayWorkspace) readFileLimited(p string, max int64) ([]byte, error) {
	key := cleanKey(p)
	w.mu.RLock()
	data, ok := w.overlay[key]
	w.mu.RUnlock()
	if ok {
		if max > 0 && max != math.MaxInt64 && int64(len(data)) > max {
			return nil, fmt.Errorf("file too large (%d bytes, max %d)", len(data), max)
		}
		return bytes.Clone(data), nil
	}
	if w.root == "" || filepath.IsAbs(p) {
		return bytelimit.ReadFileLimited(p, max)
	}
	return bytelimit.ReadFSFileLimited(w.ensureDiskFS(), key, max)
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
	disk := w.ensureDiskFS()
	w.mu.RLock()
	snap := make(map[string][]byte, len(w.overlay))
	for k, v := range w.overlay {
		snap[k] = bytes.Clone(v)
	}
	w.mu.RUnlock()
	return &overlayFS{disk: disk, overlay: snap}
}

// ensureDiskFS lazily builds the RESOLVE_BENEATH-contained disk view once
// and caches it for the OverlayWorkspace's lifetime. FS() wraps it in an
// overlay snapshot for shadowing; ReadFile's disk fall-through reads it
// directly so a cache miss does not pay FS()'s per-call overlay-map clone.
func (w *OverlayWorkspace) ensureDiskFS() fs.FS {
	w.diskOnce.Do(func() {
		root := w.root
		if root == "" {
			root = "."
		}
		w.diskFS = lint.OpenRootFS(root)
	})
	return w.diskFS
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
	return doublestar.Glob(o.disk, pattern, doublestar.WithFailOnIOErrors())
}
