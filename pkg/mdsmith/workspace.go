package mdsmith

import (
	"bytes"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bmatcuk/doublestar/v4"
)

// Workspace is the filesystem seam the engine reads through. The same
// engine code runs against a real disk ([OSWorkspace]) and against an
// in-memory map ([MemWorkspace]), so a WebAssembly host with no
// filesystem can drive the linter by supplying its files as bytes.
//
// Paths use forward slashes and are interpreted relative to the
// workspace root (an [OSWorkspace] root, or the keys of a
// [MemWorkspace]). Glob patterns use doublestar syntax (`**`,
// brace alternatives), matching the syntax the rest of mdsmith's
// config and directives accept.
type Workspace interface {
	// ReadFile returns the bytes of the file at path, or an error
	// wrapping fs.ErrNotExist when it does not exist.
	ReadFile(path string) ([]byte, error)
	// Glob returns the paths matching the doublestar pattern, sorted.
	Glob(pattern string) ([]string, error)
	// FS returns an fs.FS view of the workspace. The engine wires this
	// onto each lint.File so cross-file rules (catalog, include) read
	// through the same backing store ReadFile uses.
	FS() fs.FS
}

// mutableWorkspace is the optional overlay interface a [Workspace] can
// implement to accept buffer edits through [Session.Invalidate]. The
// session calls Set to shadow a path with open-document bytes and Delete
// to drop a path. A workspace that does not implement it (e.g. a bare
// OSWorkspace) re-reads disk instead. MemWorkspace and the LSP's overlay
// workspace satisfy it, so the session reaches both without a concrete
// type assertion — see [Session.Invalidate].
type mutableWorkspace interface {
	Set(path string, data []byte)
	Delete(path string)
}

// OSWorkspace reads from the host filesystem. It is the native
// implementation used by the CLI and the LSP server. The zero value is
// usable and reads paths exactly as passed (absolute or relative to the
// process working directory).
type OSWorkspace struct {
	// Root, when non-empty, is the directory both ReadFile and FS are
	// anchored at. A workspace-relative path (e.g. "docs/a.md") resolves
	// against Root in ReadFile, so ReadFile and FS agree on the file a
	// given uri names; an absolute path is read as-is. Glob still expands
	// the pattern exactly as passed. The CLI sets Root to the project
	// root so catalog/include and frontMatterFor resolve the same
	// workspace-relative target. With an empty Root, paths are read
	// exactly as passed (the zero-value behaviour).
	Root string
}

// ReadFile reads path from the host filesystem. When Root is set and path
// is workspace-relative, it is read through readFileRooted so a symlink
// escaping Root is refused the same way FS() refuses it — see the Root
// field doc — without leaking an open directory handle per call the way
// repeatedly fetching FS() would (see FS's doc comment). This containment
// is RESOLVE_BENEATH via os.OpenRoot on non-tinygo builds; readFileRooted's
// tinygo variant uses os.DirFS with no containment, matching FS()'s own
// per-build-tag split (OSWorkspace is not used in wasm builds — see
// workspace_fs_tinygo.go). An absolute path, or any path when Root is
// empty, is read unchanged with no workspace boundary to enforce.
func (w OSWorkspace) ReadFile(p string) ([]byte, error) {
	if w.Root == "" || filepath.IsAbs(p) {
		return os.ReadFile(p) //nolint:gosec // path is caller-controlled; OSWorkspace is the native disk seam
	}
	return readFileRooted(w.Root, path.Clean(filepath.ToSlash(p)))
}

// Glob expands a doublestar pattern against the host filesystem.
func (w OSWorkspace) Glob(pattern string) ([]string, error) {
	matches, err := doublestar.FilepathGlob(pattern)
	if err != nil {
		return nil, err
	}
	sort.Strings(matches)
	return matches, nil
}

// MemWorkspace is an in-memory Workspace backed by a map from
// slash-separated path to file bytes. It drives WebAssembly (where
// there is no disk) and native tests. Construct it with
// [NewMemWorkspace]; mutate it through [MemWorkspace.Set] and
// [MemWorkspace.Delete] (the engine session does this via Invalidate).
//
// MemWorkspace is safe for concurrent reads; Set and Delete take a
// write lock. Glob is a linear scan of the key set, so the lint hot
// loop must not call it per file.
type MemWorkspace struct {
	mu        sync.RWMutex
	files     map[string][]byte
	globCalls atomic.Int64
}

// NewMemWorkspace returns a MemWorkspace seeded with files. The input
// map is copied (keys cleaned to slash form, values cloned), so later
// mutations of the argument do not leak into the workspace. A nil map
// yields an empty workspace.
func NewMemWorkspace(files map[string][]byte) *MemWorkspace {
	w := &MemWorkspace{files: make(map[string][]byte, len(files))}
	for k, v := range files {
		w.files[path.Clean(filepath.ToSlash(k))] = bytes.Clone(v)
	}
	return w
}

// ReadFile returns a copy of the bytes stored for p, or an error
// wrapping fs.ErrNotExist when p is absent.
func (w *MemWorkspace) ReadFile(p string) ([]byte, error) {
	key := path.Clean(filepath.ToSlash(p))
	w.mu.RLock()
	data, ok := w.files[key]
	w.mu.RUnlock()
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: p, Err: fs.ErrNotExist}
	}
	return bytes.Clone(data), nil
}

// Glob returns the keys matching the doublestar pattern, sorted. It is
// a linear scan over every stored path, so it must not be called per
// file in the lint hot loop — the engine globs through the FS view
// instead. GlobCalls exposes the call count for the benchmark that
// guards this.
func (w *MemWorkspace) Glob(pattern string) ([]string, error) {
	w.globCalls.Add(1)
	pat := path.Clean(filepath.ToSlash(pattern))
	w.mu.RLock()
	defer w.mu.RUnlock()
	var out []string
	for key := range w.files {
		ok, err := doublestar.Match(pat, key)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, key)
		}
	}
	sort.Strings(out)
	return out, nil
}

// GlobCalls returns how many times Glob has been called on this
// workspace. It is a benchmark/test seam used to assert the lint hot
// loop never calls the linear Glob per file.
func (w *MemWorkspace) GlobCalls() int64 {
	return w.globCalls.Load()
}

// Set stores data (cloned) at p, overwriting any existing entry.
func (w *MemWorkspace) Set(p string, data []byte) {
	key := path.Clean(filepath.ToSlash(p))
	w.mu.Lock()
	w.files[key] = bytes.Clone(data)
	w.mu.Unlock()
}

// Delete removes the entry for p. It is a no-op when p is absent.
func (w *MemWorkspace) Delete(p string) {
	key := path.Clean(filepath.ToSlash(p))
	w.mu.Lock()
	delete(w.files, key)
	w.mu.Unlock()
}

// FS returns an fs.FS view of the in-memory files. The view is a
// snapshot of the current contents; later Set/Delete calls do not
// affect an already-returned FS. The engine fetches a fresh FS per
// lint pass, so edits applied through the session's Invalidate seam
// are picked up on the next Check.
func (w *MemWorkspace) FS() fs.FS {
	w.mu.RLock()
	defer w.mu.RUnlock()
	snap := make(map[string][]byte, len(w.files))
	for k, v := range w.files {
		snap[k] = bytes.Clone(v)
	}
	return newMemFS(snap)
}

// memFS is a minimal read-only fs.FS over a map of slash-paths to
// bytes. It implements fs.ReadFileFS and fs.GlobFS so doublestar.Glob
// and bytelimit.ReadFSFileLimited operate without per-file Open overhead,
// and fs.ReadDirFS so directory walks (e.g. doublestar's recursive
// descent) resolve.
//
// dirs indexes every directory's immediate children, built once in
// newMemFS rather than rescanned by dirEntries on every ReadDir call.
// A snapshot is immutable for its lifetime (see FS), so the index
// never goes stale: fs.WalkDir calls ReadDir once per directory
// visited, and rescanning every file on every one of those calls
// turned an O(files) index build into O(files x directories)
// (docs/development/high-performance-go.md "Memoize per-input
// computations").
type memFS struct {
	files map[string][]byte
	dirs  map[string][]fs.DirEntry
}

// newMemFS wraps files and builds its directory index once.
func newMemFS(files map[string][]byte) memFS {
	return memFS{files: files, dirs: buildDirIndex(files)}
}

func (m memFS) Open(name string) (fs.File, error) {
	if name == "." {
		return &memDir{name: ".", entries: m.dirEntries(".")}, nil
	}
	if data, ok := m.files[name]; ok {
		return &memFile{name: name, data: data}, nil
	}
	// A name with descendants is a directory.
	if ents := m.dirEntries(name); len(ents) > 0 {
		return &memDir{name: name, entries: ents}, nil
	}
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}

func (m memFS) ReadFile(name string) ([]byte, error) {
	data, ok := m.files[name]
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return bytes.Clone(data), nil
}

func (m memFS) Glob(pattern string) ([]string, error) {
	var out []string
	for key := range m.files {
		// doublestar.Match (not stdlib path.Match) so the fs.GlobFS view
		// honours `**` and brace alternatives, matching MemWorkspace.Glob
		// and every other glob surface in mdsmith. stdlib path.Match does
		// not cross `/` on `*`/`**`, so a `docs/**/x.md` pattern would
		// silently miss nested files.
		ok, err := doublestar.Match(pattern, key)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, key)
		}
	}
	sort.Strings(out)
	return out, nil
}

func (m memFS) ReadDir(name string) ([]fs.DirEntry, error) {
	ents := m.dirEntries(name)
	if name != "." && len(ents) == 0 {
		if _, isFile := m.files[name]; isFile {
			return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrInvalid}
		}
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
	}
	return ents, nil
}

// dirEntries returns the immediate children of dir (files and
// subdirectories), sorted by name; a lookup into the index newMemFS
// already built.
func (m memFS) dirEntries(dir string) []fs.DirEntry {
	return m.dirs[dir]
}

// buildDirIndex walks every file path once and records each path
// segment as a child of its parent directory (root is "."),
// deduplicating repeated ancestors and sorting each directory's
// entries by name.
//
// A path segment that is empty (a "//" in the raw key, which
// NewMemWorkspace's path.Clean normally prevents) stops indexing that
// key from that point on, without registering an empty-named entry —
// the original per-call dirEntries never surfaced such a key past the
// malformed segment either, since fs.WalkDir cannot descend into a
// directory whose parent never listed it as a child.
func buildDirIndex(files map[string][]byte) map[string][]fs.DirEntry {
	idx := make(map[string][]fs.DirEntry)
	seen := make(map[string]struct{}, len(files))
	for key, data := range files {
		dir := "."
		start := 0
		for {
			rest := key[start:]
			i := indexSlash(rest)
			if i < 0 {
				addDirEntry(idx, seen, dir, rest, false, int64(len(data)))
				break
			}
			name := rest[:i]
			if name == "" {
				break
			}
			addDirEntry(idx, seen, dir, name, true, 0)
			if dir == "." {
				dir = name
			} else {
				dir = dir + "/" + name
			}
			start += i + 1
		}
	}
	for dir, ents := range idx {
		sort.Slice(ents, func(i, j int) bool { return ents[i].Name() < ents[j].Name() })
		idx[dir] = ents
	}
	return idx
}

// addDirEntry appends a name/dir entry once per (dir, name) pair,
// deduplicating the many files that share a common ancestor directory.
func addDirEntry(idx map[string][]fs.DirEntry, seen map[string]struct{}, dir, name string, isDir bool, size int64) {
	if name == "" {
		return
	}
	key := dir + "\x00" + name
	if _, ok := seen[key]; ok {
		return
	}
	seen[key] = struct{}{}
	idx[dir] = append(idx[dir], memDirEntry{name: name, size: size, dir: isDir})
}

func indexSlash(s string) int {
	return strings.IndexByte(s, '/')
}

// memFile is an fs.File over a byte slice.
type memFile struct {
	name string
	data []byte
	off  int
}

func (f *memFile) Stat() (fs.FileInfo, error) {
	return memFileInfo{name: path.Base(f.name), size: int64(len(f.data))}, nil
}

func (f *memFile) Read(p []byte) (int, error) {
	if f.off >= len(f.data) {
		return 0, io.EOF
	}
	n := copy(p, f.data[f.off:])
	f.off += n
	return n, nil
}

func (f *memFile) Close() error {
	// no test by design — trivial accessor
	return nil
}

// memDir is an fs.ReadDirFile for in-memory directories.
type memDir struct {
	name    string
	entries []fs.DirEntry
	off     int
}

func (d *memDir) Stat() (fs.FileInfo, error) {
	return memFileInfo{name: path.Base(d.name), dir: true}, nil
}

func (d *memDir) Read([]byte) (int, error) {
	// no test by design — trivial accessor
	return 0, &fs.PathError{Op: "read", Path: d.name, Err: fs.ErrInvalid}
}

func (d *memDir) Close() error {
	// no test by design — trivial accessor
	return nil
}

func (d *memDir) ReadDir(n int) ([]fs.DirEntry, error) {
	if n <= 0 {
		ents := d.entries[d.off:]
		d.off = len(d.entries)
		return ents, nil
	}
	if d.off >= len(d.entries) {
		return nil, io.EOF
	}
	end := d.off + n
	if end > len(d.entries) {
		end = len(d.entries)
	}
	ents := d.entries[d.off:end]
	d.off = end
	return ents, nil
}

// memDirEntry is an fs.DirEntry for an in-memory file or directory.
type memDirEntry struct {
	name string
	size int64
	dir  bool
}

func (e memDirEntry) Name() string {
	// no test by design — trivial accessor
	return e.name
}
func (e memDirEntry) IsDir() bool {
	// no test by design — trivial accessor
	return e.dir
}
func (e memDirEntry) Type() fs.FileMode {
	if e.dir {
		return fs.ModeDir
	}
	return 0
}
func (e memDirEntry) Info() (fs.FileInfo, error) {
	// no test by design — trivial accessor
	// memDirEntry and memFileInfo share an identical field layout, so
	// the conversion copies name/size/dir across one-for-one.
	return memFileInfo(e), nil
}

// memFileInfo is an fs.FileInfo for an in-memory file or directory.
type memFileInfo struct {
	name string
	size int64
	dir  bool
}

func (i memFileInfo) Name() string {
	// no test by design — trivial accessor
	return i.name
}
func (i memFileInfo) Size() int64 {
	// no test by design — trivial accessor
	return i.size
}
func (i memFileInfo) Mode() fs.FileMode {
	if i.dir {
		return fs.ModeDir | 0o555
	}
	return 0o444
}
func (i memFileInfo) ModTime() time.Time {
	// no test by design — trivial accessor
	return time.Time{}
}
func (i memFileInfo) IsDir() bool {
	// no test by design — trivial accessor
	return i.dir
}
func (i memFileInfo) Sys() any {
	// no test by design — trivial accessor
	return nil
}
