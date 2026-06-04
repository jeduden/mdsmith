package mdsmith

import (
	"bytes"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
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

// ReadFile reads path from the host filesystem. When Root is set and
// path is workspace-relative, it is resolved against Root so ReadFile
// and FS (which is rooted at Root) name the same file for one uri — see
// the Root field doc. An absolute path is read unchanged.
func (w OSWorkspace) ReadFile(p string) ([]byte, error) {
	return os.ReadFile(w.resolve(p)) //nolint:gosec // path is caller-controlled; OSWorkspace is the native disk seam
}

// resolve maps a workspace-relative path to an absolute one rooted at
// Root, mirroring how FS (os.DirFS(Root)) interprets the same path.
// An absolute path, or any path when Root is empty, is returned
// unchanged so the zero-value workspace and absolute-path callers keep
// reading paths exactly as passed.
func (w OSWorkspace) resolve(p string) string {
	if w.Root == "" || filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(w.Root, filepath.FromSlash(p))
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

// FS returns an os.DirFS rooted at Root, or rooted at "." when Root is
// empty.
func (w OSWorkspace) FS() fs.FS {
	root := w.Root
	if root == "" {
		root = "."
	}
	return os.DirFS(root)
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
	snap := make(memFS, len(w.files))
	for k, v := range w.files {
		snap[k] = bytes.Clone(v)
	}
	return snap
}

// memFS is a minimal read-only fs.FS over a map of slash-paths to
// bytes. It implements fs.ReadFileFS and fs.GlobFS so doublestar.Glob
// and bytelimit.ReadFSFileLimited operate without per-file Open overhead,
// and fs.ReadDirFS so directory walks (e.g. doublestar's recursive
// descent) resolve.
type memFS map[string][]byte

func (m memFS) Open(name string) (fs.File, error) {
	if name == "." {
		return &memDir{name: ".", entries: m.dirEntries(".")}, nil
	}
	if data, ok := m[name]; ok {
		return &memFile{name: name, data: data}, nil
	}
	// A name with descendants is a directory.
	if ents := m.dirEntries(name); len(ents) > 0 {
		return &memDir{name: name, entries: ents}, nil
	}
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}

func (m memFS) ReadFile(name string) ([]byte, error) {
	data, ok := m[name]
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return bytes.Clone(data), nil
}

func (m memFS) Glob(pattern string) ([]string, error) {
	var out []string
	for key := range m {
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
		if _, isFile := m[name]; isFile {
			return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrInvalid}
		}
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
	}
	return ents, nil
}

// dirEntries returns the immediate children of dir (files and
// subdirectories), deduplicated and sorted by name.
func (m memFS) dirEntries(dir string) []fs.DirEntry {
	prefix := ""
	if dir != "." {
		prefix = dir + "/"
	}
	seen := make(map[string]bool)
	var ents []fs.DirEntry
	for key := range m {
		if prefix != "" {
			if len(key) <= len(prefix) || key[:len(prefix)] != prefix {
				continue
			}
		}
		rest := key[len(prefix):]
		if i := indexSlash(rest); i >= 0 {
			name := rest[:i]
			if name != "" && !seen[name] {
				seen[name] = true
				ents = append(ents, memDirEntry{name: name, dir: true})
			}
			continue
		}
		if rest != "" && !seen[rest] {
			seen[rest] = true
			ents = append(ents, memDirEntry{name: rest, size: int64(len(m[key])), dir: false})
		}
	}
	sort.Slice(ents, func(i, j int) bool { return ents[i].Name() < ents[j].Name() })
	return ents
}

func indexSlash(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == '/' {
			return i
		}
	}
	return -1
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

func (f *memFile) Close() error { return nil }

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
	return 0, &fs.PathError{Op: "read", Path: d.name, Err: fs.ErrInvalid}
}

func (d *memDir) Close() error { return nil }

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

func (e memDirEntry) Name() string { return e.name }
func (e memDirEntry) IsDir() bool  { return e.dir }
func (e memDirEntry) Type() fs.FileMode {
	if e.dir {
		return fs.ModeDir
	}
	return 0
}
func (e memDirEntry) Info() (fs.FileInfo, error) {
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

func (i memFileInfo) Name() string { return i.name }
func (i memFileInfo) Size() int64  { return i.size }
func (i memFileInfo) Mode() fs.FileMode {
	if i.dir {
		return fs.ModeDir | 0o555
	}
	return 0o444
}
func (i memFileInfo) ModTime() time.Time { return time.Time{} }
func (i memFileInfo) IsDir() bool        { return i.dir }
func (i memFileInfo) Sys() any           { return nil }
