package mdsmith

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

// TestMemWorkspaceFSConformance runs the standard library's fstest
// suite against a MemWorkspace's FS view. It exercises Open, Read (incl.
// EOF), ReadDir over nested directories, Stat, and the DirEntry /
// FileInfo accessors — the plumbing the engine relies on when a WASM
// host drives cross-file rules off an in-memory workspace.
func TestMemWorkspaceFSConformance(t *testing.T) {
	ws := NewMemWorkspace(map[string][]byte{
		"a.md":            []byte("alpha"),
		"docs/b.md":       []byte("bravo"),
		"docs/sub/c.md":   []byte("charlie"),
		"docs/sub/d.json": []byte("{}"),
	})
	if err := fstest.TestFS(ws.FS(),
		"a.md", "docs/b.md", "docs/sub/c.md", "docs/sub/d.json"); err != nil {
		t.Fatalf("fstest.TestFS: %v", err)
	}
}

// TestOSWorkspaceFS covers OSWorkspace.FS for both an explicit Root and
// the empty-Root default (".").
func TestOSWorkspaceFS(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("disk"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got, err := fs.ReadFile(OSWorkspace{Root: dir}.FS(), "a.md")
	if err != nil {
		t.Fatalf("fs.ReadFile via OSWorkspace.FS: %v", err)
	}
	if string(got) != "disk" {
		t.Fatalf("OSWorkspace.FS read = %q, want disk", got)
	}
	if (OSWorkspace{}).FS() == nil {
		t.Fatal("OSWorkspace{}.FS() (Root defaulting to \".\") returned nil")
	}
}

// TestWorkspaceGlobErrors covers the error return of both Glob
// implementations on a malformed doublestar pattern.
func TestWorkspaceGlobErrors(t *testing.T) {
	if _, err := NewMemWorkspace(map[string][]byte{"a.md": []byte("x")}).Glob("[bad"); err == nil {
		t.Fatal("MemWorkspace.Glob: expected error for malformed pattern")
	}
	if _, err := (OSWorkspace{}).Glob("[bad"); err == nil {
		t.Fatal("OSWorkspace.Glob: expected error for malformed pattern")
	}
}

// TestMemFSGlob covers memFS.Glob directly via the fs.GlobFS interface.
func TestMemFSGlob(t *testing.T) {
	ws := NewMemWorkspace(map[string][]byte{
		"a.md":      []byte("x"),
		"b.txt":     []byte("y"),
		"docs/c.md": []byte("z"),
	})
	g, ok := ws.FS().(fs.GlobFS)
	if !ok {
		t.Fatal("MemWorkspace.FS() does not implement fs.GlobFS")
	}
	matches, err := g.Glob("*.md")
	if err != nil {
		t.Fatalf("memFS.Glob: %v", err)
	}
	if len(matches) != 1 || matches[0] != "a.md" {
		t.Fatalf("memFS.Glob(*.md) = %v, want [a.md]", matches)
	}
}

// TestMemFSEdgeCases covers the error and accessor paths fstest does not
// reach: reading a directory as a byte stream, ReadDir on a file and a
// missing path, and FileInfo.Sys.
func TestMemFSEdgeCases(t *testing.T) {
	fsys := NewMemWorkspace(map[string][]byte{
		"a.md":      []byte("alpha"),
		"docs/b.md": []byte("bravo"),
	}).FS()

	// Reading a directory as a byte stream is an error.
	d, err := fsys.Open("docs")
	if err != nil {
		t.Fatalf("Open(docs): %v", err)
	}
	if _, err := d.Read(make([]byte, 4)); err == nil {
		t.Error("memDir.Read: expected an error reading a directory as a file")
	}
	_ = d.Close()

	// ReadDir on a file and on a missing path both error.
	if _, err := fs.ReadDir(fsys, "a.md"); err == nil {
		t.Error("ReadDir(file): expected an error")
	}
	if _, err := fs.ReadDir(fsys, "nope"); err == nil {
		t.Error("ReadDir(missing): expected an error")
	}

	// FileInfo.Sys is nil for in-memory files.
	info, err := fs.Stat(fsys, "a.md")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Sys() != nil {
		t.Errorf("Sys() = %v, want nil", info.Sys())
	}
}
