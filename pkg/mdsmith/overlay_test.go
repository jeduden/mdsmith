package mdsmith

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/bytelimit"
)

// TestOverlayWorkspaceReadFilePrefersOverlay verifies an
// OverlayWorkspace returns the overlaid (open-buffer) bytes for a
// shadowed path and falls through to disk for everything else. This is
// the LSP's workspace: unsaved-buffer bytes shadow the on-disk file.
func TestOverlayWorkspaceReadFilePrefersOverlay(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.md"), []byte("disk-a"), 0o600); err != nil {
		t.Fatalf("WriteFile a: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "b.md"), []byte("disk-b"), 0o600); err != nil {
		t.Fatalf("WriteFile b: %v", err)
	}

	ws := NewOverlayWorkspace(root)
	ws.Set("a.md", []byte("buffer-a"))

	gotA, err := ws.ReadFile("a.md")
	if err != nil {
		t.Fatalf("ReadFile a: %v", err)
	}
	if string(gotA) != "buffer-a" {
		t.Fatalf("ReadFile a = %q, want overlay bytes buffer-a", gotA)
	}
	gotB, err := ws.ReadFile("b.md")
	if err != nil {
		t.Fatalf("ReadFile b: %v", err)
	}
	if string(gotB) != "disk-b" {
		t.Fatalf("ReadFile b = %q, want disk fall-through disk-b", gotB)
	}
}

// TestOverlayWorkspaceFSSymlinkEscapeRefused verifies that the disk
// fall-through layer of OverlayWorkspace.FS() is os.OpenRoot-contained,
// so a within-workspace symlink pointing outside the root cannot be read
// through the editor (LSP) path. An unshadowed include/catalog target
// reads through this disk layer, so it must enforce the same RESOLVE_BENEATH
// containment the CLI engine gained in OSWorkspace.FS(). Guards CWE-73.
func TestOverlayWorkspaceFSSymlinkEscapeRefused(t *testing.T) {
	root := t.TempDir()

	// Symlink inside the workspace pointing outside the root.
	symlinkPath := filepath.Join(root, "escape.md")
	if err := os.Symlink("/etc/passwd", symlinkPath); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	// Confirm os.DirFS can read through the symlink (establishes the
	// baseline vulnerability that the overlay disk layer must close).
	if _, errDirFS := fs.ReadFile(os.DirFS(root), "escape.md"); errDirFS != nil {
		t.Skip("test invariant: os.DirFS must be able to read the symlink target; skipping on this platform")
	}

	// The escaping symlink is not shadowed by an overlay buffer, so the
	// read falls through to the disk layer — which must refuse it.
	fsys := NewOverlayWorkspace(root).FS()
	if _, err := fsys.Open("escape.md"); err == nil {
		t.Fatal("overlayFS.Open(escape.md) succeeded; expected containment error for symlink escaping root")
	}
	if _, err := fs.ReadFile(fsys, "escape.md"); err == nil {
		t.Fatal("fs.ReadFile(overlayFS, escape.md) succeeded; expected containment error for symlink escaping root")
	}
}

// TestOverlayWorkspaceReadFileSymlinkEscapeRefused verifies that
// OverlayWorkspace.ReadFile's disk fall-through shares FS()'s
// os.OpenRoot containment, so an unshadowed within-workspace symlink
// pointing outside the root is refused. Guards CWE-73.
func TestOverlayWorkspaceReadFileSymlinkEscapeRefused(t *testing.T) {
	root := t.TempDir()

	symlinkPath := filepath.Join(root, "escape.md")
	if err := os.Symlink("/etc/passwd", symlinkPath); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	if _, errDirFS := os.ReadFile(symlinkPath); errDirFS != nil {
		t.Skip("test invariant: the symlink target must be readable; skipping on this platform")
	}

	ws := NewOverlayWorkspace(root)
	if _, err := ws.ReadFile("escape.md"); err == nil {
		t.Fatal("OverlayWorkspace.ReadFile(escape.md) succeeded; expected containment error for symlink escaping root")
	}
}

// TestOverlayWorkspaceReadFileDiskFallthroughSkipsOverlaySnapshot verifies
// that ReadFile's disk fall-through does not pay FS()'s per-call overlay-map
// clone (see FS's doc comment): with many open buffers, a disk-fallback read
// must stay O(1), not O(len(overlay)).
func TestOverlayWorkspaceReadFileDiskFallthroughSkipsOverlaySnapshot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "disk.md"), []byte("disk"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	ws := NewOverlayWorkspace(root)
	const openBuffers = 500
	for i := range openBuffers {
		ws.Set(strconv.Itoa(i)+".md", []byte("x"))
	}
	if _, err := ws.ReadFile("disk.md"); err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	allocs := testing.AllocsPerRun(50, func() {
		_, _ = ws.ReadFile("disk.md")
	})
	if allocs > 20 {
		t.Fatalf("ReadFile disk fall-through allocated %.0f times per call with %d open buffers; "+
			"expected O(1), not O(overlay size) from cloning the overlay map", allocs, openBuffers)
	}
}

// TestOverlayWorkspaceFSPrefersOverlay verifies the fs.FS view also
// shadows disk with overlay bytes, so cross-file rules (catalog,
// include) reading through FS see the open-buffer content (footgun 3).
func TestOverlayWorkspaceFSPrefersOverlay(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "a.md"), []byte("disk"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ws := NewOverlayWorkspace(root)
	ws.Set("docs/a.md", []byte("overlay"))

	got, err := fs.ReadFile(ws.FS(), "docs/a.md")
	if err != nil {
		t.Fatalf("fs.ReadFile: %v", err)
	}
	if string(got) != "overlay" {
		t.Fatalf("FS ReadFile = %q, want overlay", got)
	}

	// A path with no overlay falls through to disk.
	if err := os.WriteFile(filepath.Join(root, "docs", "c.md"), []byte("only-disk"), 0o600); err != nil {
		t.Fatalf("WriteFile c: %v", err)
	}
	gotC, err := fs.ReadFile(ws.FS(), "docs/c.md")
	if err != nil {
		t.Fatalf("fs.ReadFile c: %v", err)
	}
	if string(gotC) != "only-disk" {
		t.Fatalf("FS ReadFile c = %q, want disk fall-through", gotC)
	}
}

// TestOverlayWorkspaceFSReflectsLatestSet verifies a Set after an FS
// view was taken is visible to a freshly fetched FS — the engine
// fetches a new FS per lint pass, so the overlay edit lands on the next
// Check.
func TestOverlayWorkspaceFSReflectsLatestSet(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.md"), []byte("disk"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	ws := NewOverlayWorkspace(root)

	ws.Set("a.md", []byte("v1"))
	if got, _ := fs.ReadFile(ws.FS(), "a.md"); string(got) != "v1" {
		t.Fatalf("FS after Set v1 = %q, want v1", got)
	}
	ws.Set("a.md", []byte("v2"))
	if got, _ := fs.ReadFile(ws.FS(), "a.md"); string(got) != "v2" {
		t.Fatalf("FS after Set v2 = %q, want v2", got)
	}
	ws.Delete("a.md")
	if got, _ := fs.ReadFile(ws.FS(), "a.md"); string(got) != "disk" {
		t.Fatalf("FS after Delete = %q, want disk fall-through", got)
	}
}

// TestOverlayWorkspaceSatisfiesMutable confirms OverlayWorkspace
// satisfies the mutable overlay interface Session.Invalidate uses, so
// the LSP's buffer bytes reach cross-file rules.
func TestOverlayWorkspaceSatisfiesMutable(t *testing.T) {
	var _ mutableWorkspace = NewOverlayWorkspace(t.TempDir())
	var _ Workspace = NewOverlayWorkspace(t.TempDir())
}

// TestOverlayWorkspaceGlob verifies OverlayWorkspace.Glob expands a
// doublestar pattern against the on-disk tree rooted at Root and returns
// sorted, root-relative-resolved matches. Open buffers exist on disk, so
// globbing defers to disk; the overlay only shadows content on read.
func TestOverlayWorkspaceGlob(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	for _, name := range []string{"docs/b.md", "docs/a.md", "docs/c.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("x"), 0o600); err != nil {
			t.Fatalf("WriteFile %s: %v", name, err)
		}
	}

	ws := NewOverlayWorkspace(root)
	// An overlaid buffer must not add or remove a glob match: globbing
	// reads disk, not the overlay.
	ws.Set("docs/a.md", []byte("buffer"))

	got, err := ws.Glob("docs/*.md")
	if err != nil {
		t.Fatalf("Glob: %v", err)
	}
	want := []string{filepath.Join(root, "docs", "a.md"), filepath.Join(root, "docs", "b.md")}
	if !slices.Equal(got, want) {
		t.Fatalf("Glob(docs/*.md) = %v, want %v (sorted, disk-backed)", got, want)
	}
}

// TestOverlayWorkspaceGlobBadPattern verifies a malformed doublestar
// pattern surfaces an error rather than being silently swallowed,
// matching OSWorkspace.Glob's error contract.
func TestOverlayWorkspaceGlobBadPattern(t *testing.T) {
	ws := NewOverlayWorkspace(t.TempDir())
	if _, err := ws.Glob("[bad"); err == nil {
		t.Fatal("Glob([bad): expected an error for a malformed pattern, got nil")
	}
}

// TestOverlayWorkspaceReadFileAbsolutePath covers ReadFile's
// absolute-path passthrough branch: an absolute path is read unchanged
// (not re-rooted under Root), mirroring OSWorkspace so an absolute-path
// caller reaches the file it named.
func TestOverlayWorkspaceReadFileAbsolutePath(t *testing.T) {
	root := t.TempDir()
	// A file OUTSIDE Root, addressed by its absolute path. If the
	// passthrough branch wrongly joined it under Root, this read would miss.
	other := t.TempDir()
	abs := filepath.Join(other, "elsewhere.md")
	if err := os.WriteFile(abs, []byte("absolute"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ws := NewOverlayWorkspace(root)
	got, err := ws.ReadFile(abs)
	if err != nil {
		t.Fatalf("ReadFile(abs): %v", err)
	}
	if string(got) != "absolute" {
		t.Fatalf("ReadFile(abs) = %q, want disk bytes read at the absolute path", got)
	}
}

// TestOverlayFSGlob verifies the fs.FS view's Glob (fs.GlobFS) expands a
// doublestar pattern against disk, so doublestar.Glob and the
// catalog/include rules globbing through the FS view honour `**` and
// brace alternatives just like the other glob surfaces in mdsmith.
func TestOverlayFSGlob(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs", "sub"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "a.md"), []byte("a"), 0o600); err != nil {
		t.Fatalf("WriteFile a: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "sub", "b.md"), []byte("b"), 0o600); err != nil {
		t.Fatalf("WriteFile b: %v", err)
	}

	fsys, ok := NewOverlayWorkspace(root).FS().(fs.GlobFS)
	if !ok {
		t.Fatal("OverlayWorkspace.FS() must implement fs.GlobFS")
	}
	got, err := fsys.Glob("docs/**/*.md")
	if err != nil {
		t.Fatalf("FS().Glob: %v", err)
	}
	want := []string{"docs/a.md", "docs/sub/b.md"}
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Fatalf("FS().Glob(docs/**/*.md) = %v, want %v", got, want)
	}
}

// TestSessionOverlayAnchorsRootDir verifies a Session built over an
// OverlayWorkspace (the LSP's workspace) anchors its rootDir at the
// overlay's project root, not the empty string. The empty value would
// flip the cross-file RunCache from absolute keys (the CLI's OSWorkspace
// behaviour) to relative keys — an unintended asymmetry between the CLI
// and the LSP. absPath, which Invalidate keys cache evictions by, must
// then resolve the same absolute path Runner.RootDir keys cache writes
// by, so the two stay consistent.
func TestSessionOverlayAnchorsRootDir(t *testing.T) {
	root := t.TempDir()
	ws := NewOverlayWorkspace(root)
	s, err := NewSession(SessionOptions{Workspace: ws, Config: ConfigYAML("")})
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer s.Dispose()

	if s.rootDir != root {
		t.Fatalf("session rootDir = %q, want overlay root %q", s.rootDir, root)
	}
	// absPath (cache-eviction keying) must produce an absolute path under
	// the root for a workspace-relative uri, matching how the runner's
	// RootDir anchors cross-file cache writes. A relative result would
	// mean Invalidate evicts a key the rules never wrote.
	if got := s.absPath("docs/a.md"); got != filepath.Join(root, "docs", "a.md") {
		t.Fatalf("absPath(docs/a.md) = %q, want %q", got, filepath.Join(root, "docs", "a.md"))
	}
}

// TestSessionOverlayInvalidateDropsStaleCrossFileRead is the
// invalidation-consistency acceptance for the corrected absolute
// keying: a cross-file Fix warms the RunCache for a neighbour (keyed by
// the absolute path the runner's RootDir produces), then Invalidate
// (keyed by absPath) must drop that exact entry so the next Fix reads
// the changed bytes. If the write key and the eviction key disagreed —
// the asymmetry an empty rootDir introduces — the second Fix would
// project the stale summary from the cached cross-file read. Fix is
// used (not a hand-written catalog body) so the canonical generated
// formatting is produced by the engine, not guessed.
func TestSessionOverlayInvalidateDropsStaleCrossFileRead(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "one.md"),
		[]byte("---\nsummary: First\n---\n# One\n\nBody paragraph.\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ws := NewOverlayWorkspace(root)
	s, err := NewSession(SessionOptions{Workspace: ws, Config: ConfigYAML("")})
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer s.Dispose()

	index := []byte("# Index\n\n<?catalog\nglob:\n  - \"docs/*.md\"\n" +
		"row: \"- [{summary}](docs/{filename})\"\n?>\n<?/catalog?>\n")

	// First Fix reads docs/one.md through the RunCache and projects its
	// saved summary, warming the absolute-keyed cross-file read.
	res1, err := s.Fix("index.md", index)
	if err != nil {
		t.Fatalf("Fix 1: %v", err)
	}
	if !strings.Contains(res1.Source, "First") {
		t.Fatalf("Fix 1: catalog should project the saved summary First:\n%s", res1.Source)
	}

	// Change the neighbour's summary through the overlay. Invalidate keys
	// the eviction by absPath; the next Fix's cross-file read keys the
	// lookup by the runner's RootDir-anchored absolute path. They must be
	// the same key, or the cache returns the stale "First".
	s.Invalidate("docs/one.md", []byte("---\nsummary: Second\n---\n# One\n\nBody paragraph.\n"))

	res2, err := s.Fix("index.md", index)
	if err != nil {
		t.Fatalf("Fix 2: %v", err)
	}
	if !strings.Contains(res2.Source, "Second") {
		t.Fatalf("Fix 2: expected the new summary Second, proving Invalidate "+
			"dropped the absolute-keyed cross-file read:\n%s", res2.Source)
	}
	if strings.Contains(res2.Source, "First") {
		t.Fatalf("Fix 2: stale summary First survived; the eviction key did "+
			"not match the cross-file read key:\n%s", res2.Source)
	}
}

// TestSessionOverlayBufferReachesCrossFileRule is the end-to-end
// footgun-3 acceptance for the LSP scenario: a Session over an
// OverlayWorkspace catalogs a file whose unsaved-buffer summary differs
// from disk. After Invalidate pushes the buffer bytes, the index's
// catalog projects the buffer summary, not the saved one — the open
// document reached the cross-file rule through the session.
func TestSessionOverlayBufferReachesCrossFileRule(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "one.md"),
		[]byte("---\nsummary: Saved\n---\n# One\n\nBody paragraph.\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ws := NewOverlayWorkspace(root)
	s, err := NewSession(SessionOptions{Workspace: ws, Config: ConfigYAML("")})
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer s.Dispose()

	index := []byte("# Index\n\n<?catalog\nglob:\n  - \"docs/*.md\"\n" +
		"row: \"- [{summary}](docs/{filename})\"\n?>\n<?/catalog?>\n")

	// Saved state: the catalog projects "Saved".
	res1, err := s.Fix("index.md", index)
	if err != nil {
		t.Fatalf("Fix 1: %v", err)
	}
	if !strings.Contains(res1.Source, "Saved") {
		t.Fatalf("Fix 1: catalog should project the saved summary:\n%s", res1.Source)
	}

	// Push an unsaved buffer edit through Invalidate (the LSP didChange
	// path), then re-fix: the catalog must pick up the buffer summary.
	s.Invalidate("docs/one.md", []byte("---\nsummary: Buffered\n---\n# One\n\nBody paragraph.\n"))
	res2, err := s.Fix("index.md", index)
	if err != nil {
		t.Fatalf("Fix 2: %v", err)
	}
	if !strings.Contains(res2.Source, "Buffered") {
		t.Fatalf("Fix 2: open-buffer summary did not reach the catalog rule:\n%s", res2.Source)
	}
}

// TestOverlayWorkspaceReadFileLimitedOverlayHit verifies readFileLimited
// applies the same cap to an overlaid buffer as it does to a disk read,
// and returns the buffer bytes unchanged when under the cap.
func TestOverlayWorkspaceReadFileLimitedOverlayHit(t *testing.T) {
	ws := NewOverlayWorkspace(t.TempDir())
	ws.Set("a.md", []byte("buffer-a"))

	got, err := ws.readFileLimited("a.md", 100)
	if err != nil {
		t.Fatalf("readFileLimited within limit: %v", err)
	}
	if string(got) != "buffer-a" {
		t.Fatalf("readFileLimited = %q, want %q", got, "buffer-a")
	}

	if _, err := ws.readFileLimited("a.md", 3); err == nil {
		t.Fatal("readFileLimited over limit on overlaid buffer: want error, got nil")
	} else if !errors.Is(err, bytelimit.ErrFileTooLarge) {
		t.Fatalf("readFileLimited over limit: got err %v, want it to wrap bytelimit.ErrFileTooLarge", err)
	}
}

// TestOverlayWorkspaceReadFileLimitedDiskFallthrough verifies
// readFileLimited rejects an oversized on-disk file (the non-overlaid
// fall-through path) instead of reading it fully into memory.
func TestOverlayWorkspaceReadFileLimitedDiskFallthrough(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "huge.md"), make([]byte, 100), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	ws := NewOverlayWorkspace(root)

	if _, err := ws.readFileLimited("huge.md", 50); err == nil {
		t.Fatal("readFileLimited over limit on disk fall-through: want error, got nil")
	} else if !errors.Is(err, bytelimit.ErrFileTooLarge) {
		t.Fatalf("readFileLimited over limit: got err %v, want it to wrap bytelimit.ErrFileTooLarge", err)
	}

	if err := os.WriteFile(filepath.Join(root, "small.md"), []byte("ok"), 0o600); err != nil {
		t.Fatalf("WriteFile small: %v", err)
	}
	got, err := ws.readFileLimited("small.md", 50)
	if err != nil {
		t.Fatalf("readFileLimited within limit: %v", err)
	}
	if string(got) != "ok" {
		t.Fatalf("readFileLimited = %q, want %q", got, "ok")
	}
}

// TestOverlayWorkspaceReadFileLimitedAbsolutePathIgnoresRoot verifies
// readFileLimited's absolute-path branch: an absolute path bypasses the
// rooted disk FS the same way ReadFile's does, even when Root is set,
// and still enforces the size cap via bytelimit.ReadFileLimited.
func TestOverlayWorkspaceReadFileLimitedAbsolutePathIgnoresRoot(t *testing.T) {
	root := t.TempDir()
	other := t.TempDir()
	abs := filepath.Join(other, "x.md")
	if err := os.WriteFile(abs, []byte("absolute"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	ws := NewOverlayWorkspace(root)

	got, err := ws.readFileLimited(abs, 50)
	if err != nil {
		t.Fatalf("readFileLimited(abs) within limit: %v", err)
	}
	if string(got) != "absolute" {
		t.Fatalf("readFileLimited(abs) = %q, want %q", got, "absolute")
	}

	if _, err := ws.readFileLimited(abs, 3); err == nil {
		t.Fatal("readFileLimited(abs) over limit: want error, got nil")
	} else if !errors.Is(err, bytelimit.ErrFileTooLarge) {
		t.Fatalf("readFileLimited(abs) over limit: got err %v, want it to wrap bytelimit.ErrFileTooLarge", err)
	}
}
