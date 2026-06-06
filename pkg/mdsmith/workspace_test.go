package mdsmith

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/bmatcuk/doublestar/v4"
)

func TestMemWorkspaceReadFile(t *testing.T) {
	ws := NewMemWorkspace(map[string][]byte{
		"docs/a.md": []byte("alpha"),
	})

	got, err := ws.ReadFile("docs/a.md")
	if err != nil {
		t.Fatalf("ReadFile: unexpected error: %v", err)
	}
	if string(got) != "alpha" {
		t.Fatalf("ReadFile: got %q, want %q", got, "alpha")
	}
}

func TestMemWorkspaceReadFileMissing(t *testing.T) {
	ws := NewMemWorkspace(nil)

	_, err := ws.ReadFile("nope.md")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("ReadFile missing: got err %v, want fs.ErrNotExist", err)
	}
}

func TestMemWorkspaceReadFileReturnsCopy(t *testing.T) {
	ws := NewMemWorkspace(map[string][]byte{"a.md": []byte("xy")})

	got, err := ws.ReadFile("a.md")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	got[0] = 'Z'

	again, err := ws.ReadFile("a.md")
	if err != nil {
		t.Fatalf("ReadFile second: %v", err)
	}
	if string(again) != "xy" {
		t.Fatalf("ReadFile mutated backing store: got %q, want %q", again, "xy")
	}
}

func TestMemWorkspaceGlob(t *testing.T) {
	ws := NewMemWorkspace(map[string][]byte{
		"docs/a.md":       []byte("a"),
		"docs/b.md":       []byte("b"),
		"docs/sub/c.md":   []byte("c"),
		"plan/1_x.md":     []byte("x"),
		"docs/notes.txt":  []byte("t"),
		"docs/sub/d.json": []byte("{}"),
	})

	got, err := ws.Glob("docs/*.md")
	if err != nil {
		t.Fatalf("Glob: %v", err)
	}
	sort.Strings(got)
	want := []string{"docs/a.md", "docs/b.md"}
	if len(got) != len(want) {
		t.Fatalf("Glob docs/*.md = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Glob docs/*.md = %v, want %v", got, want)
		}
	}
}

func TestMemWorkspaceSetAndDelete(t *testing.T) {
	ws := NewMemWorkspace(map[string][]byte{"a.md": []byte("old")})

	ws.Set("a.md", []byte("new"))
	got, err := ws.ReadFile("a.md")
	if err != nil {
		t.Fatalf("ReadFile after Set: %v", err)
	}
	if string(got) != "new" {
		t.Fatalf("Set: got %q, want %q", got, "new")
	}

	ws.Delete("a.md")
	if _, err := ws.ReadFile("a.md"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("ReadFile after Delete: got err %v, want fs.ErrNotExist", err)
	}
}

func TestOSWorkspaceReadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.md")
	if err := os.WriteFile(path, []byte("disk"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ws := OSWorkspace{}
	got, err := ws.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "disk" {
		t.Fatalf("ReadFile: got %q, want %q", got, "disk")
	}
}

// TestOSWorkspaceReadFileAndFSAgreeOnURI is the plan-219 footgun-1
// acceptance test. With a non-empty Root, a workspace-relative uri must
// resolve to the same on-disk file through ReadFile and through FS —
// the two read paths the session uses (frontMatterFor reads via
// ReadFile; the engine reads cross-file content via FS). Reading one
// uri both ways must yield one file's bytes, not two different files.
func TestOSWorkspaceReadFileAndFSAgreeOnURI(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	const uri = "docs/a.md"
	if err := os.WriteFile(filepath.Join(root, uri), []byte("rooted"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ws := OSWorkspace{Root: root}

	viaReadFile, err := ws.ReadFile(uri)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", uri, err)
	}
	viaFS, err := fs.ReadFile(ws.FS(), uri)
	if err != nil {
		t.Fatalf("fs.ReadFile(FS, %q): %v", uri, err)
	}
	if string(viaReadFile) != "rooted" || string(viaFS) != "rooted" {
		t.Fatalf("ReadFile=%q FS=%q, both should read the single rooted file %q",
			viaReadFile, viaFS, "rooted")
	}
}

// TestOSWorkspaceReadFileAbsoluteIgnoresRoot verifies that an absolute
// path passed to ReadFile is read as-is even when Root is set — the
// rooting only applies to workspace-relative paths, so callers that
// already hold an absolute path keep working.
func TestOSWorkspaceReadFileAbsoluteIgnoresRoot(t *testing.T) {
	root := t.TempDir()
	other := t.TempDir()
	abs := filepath.Join(other, "x.md")
	if err := os.WriteFile(abs, []byte("absolute"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	ws := OSWorkspace{Root: root}
	got, err := ws.ReadFile(abs)
	if err != nil {
		t.Fatalf("ReadFile(abs): %v", err)
	}
	if string(got) != "absolute" {
		t.Fatalf("ReadFile(abs) = %q, want %q", got, "absolute")
	}
}

func TestOSWorkspaceGlob(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.md", "b.md", "c.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatalf("WriteFile %s: %v", name, err)
		}
	}

	ws := OSWorkspace{}
	got, err := ws.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		t.Fatalf("Glob: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("Glob *.md returned %d entries, want 2: %v", len(got), got)
	}
}

// TestMemWorkspaceFSResolvesDoublestar verifies the fs.FS view a
// MemWorkspace exposes works with doublestar.Glob and fs.ReadFile —
// the exact operations the catalog and include rules perform through
// lint.File.FS, so the engine can drive cross-file rules under WASM
// off an in-memory workspace.
func TestMemWorkspaceFSResolvesDoublestar(t *testing.T) {
	ws := NewMemWorkspace(map[string][]byte{
		"docs/a.md":     []byte("a"),
		"docs/sub/c.md": []byte("c"),
		"docs/n.txt":    []byte("t"),
		"plan/1.md":     []byte("p"),
	})
	fsys := ws.FS()

	got, err := doublestar.Glob(fsys, "docs/**/*.md")
	if err != nil {
		t.Fatalf("doublestar.Glob: %v", err)
	}
	sort.Strings(got)
	want := []string{"docs/a.md", "docs/sub/c.md"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("doublestar.Glob docs/**/*.md = %v, want %v", got, want)
	}

	data, err := fs.ReadFile(fsys, "docs/sub/c.md")
	if err != nil {
		t.Fatalf("fs.ReadFile: %v", err)
	}
	if string(data) != "c" {
		t.Fatalf("fs.ReadFile = %q, want %q", data, "c")
	}
}

// TestMemWorkspaceFSSnapshotIsStable verifies an already-returned FS
// view is unaffected by later Set/Delete; the engine fetches a fresh
// FS per lint pass so edits land via Invalidate, not via mutating a
// live FS mid-walk.
func TestMemWorkspaceFSSnapshotIsStable(t *testing.T) {
	ws := NewMemWorkspace(map[string][]byte{"a.md": []byte("v1")})
	fsys := ws.FS()

	ws.Set("a.md", []byte("v2"))

	data, err := fs.ReadFile(fsys, "a.md")
	if err != nil {
		t.Fatalf("fs.ReadFile: %v", err)
	}
	if string(data) != "v1" {
		t.Fatalf("snapshot FS = %q, want %q (stable across Set)", data, "v1")
	}
}

// TestMemFSGlobDoublestar verifies the fs.GlobFS view honours
// doublestar `**` semantics, matching MemWorkspace.Glob and the rest of
// mdsmith rather than the weaker stdlib path.Match (which does not cross
// directory separators on `**`).
func TestMemFSGlobDoublestar(t *testing.T) {
	ws := NewMemWorkspace(map[string][]byte{
		"docs/guide/intro.md": []byte("a"),
		"docs/top.md":         []byte("b"),
		"other/x.md":          []byte("c"),
	})
	fsys, ok := ws.FS().(fs.GlobFS)
	if !ok {
		t.Fatal("MemWorkspace.FS() must implement fs.GlobFS")
	}

	got, err := fsys.Glob("docs/**/*.md")
	if err != nil {
		t.Fatalf("Glob: %v", err)
	}
	sort.Strings(got)
	// doublestar `**` matches zero-or-more segments, so it spans the
	// nested docs/guide/intro.md (which stdlib path.Match would miss) and
	// the top-level docs/top.md, but never escapes to other/x.md.
	want := []string{"docs/guide/intro.md", "docs/top.md"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		// ** must cross into docs/guide/ yet stay under docs/.
		t.Fatalf("memFS.Glob(%q) = %v, want %v", "docs/**/*.md", got, want)
	}
}

// --- indexSlash ---

// TestIndexSlash covers the four boundary cases: no slash, slash at
// index 0, slash in the middle, and slash at the last position.
func TestIndexSlash(t *testing.T) {
	cases := []struct {
		input string
		want  int
	}{
		{"noslash", -1},
		{"/leading", 0},
		{"mid/dle", 3},
		{"trailing/", 8},
	}
	for _, tc := range cases {
		got := indexSlash(tc.input)
		if got != tc.want {
			t.Errorf("indexSlash(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

// Workspace is satisfied by both implementations.
var (
	_ Workspace = OSWorkspace{}
	_ Workspace = (*MemWorkspace)(nil)
)
