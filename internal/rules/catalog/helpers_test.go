package catalog

import (
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/jeduden/mdsmith/internal/lint"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHostAbsDir_AbsolutePath pins the success path: an absolute
// f.Path turns into the cleaned absolute directory and ok=true.
func TestHostAbsDir_AbsolutePath(t *testing.T) {
	f := &lint.File{Path: "/tmp/repo/host.md"}
	dir, ok := hostAbsDir(f)
	assert.True(t, ok)
	assert.Equal(t, "/tmp/repo", dir)
}

// TestHostAbsDir_RelativePathAnchorsAtCwd pins that a relative
// f.Path resolves against the test's cwd. We chdir into a temp dir
// so the assertion is deterministic across hosts.
func TestHostAbsDir_RelativePathAnchorsAtCwd(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)
	f := &lint.File{Path: "host.md"}
	dir, ok := hostAbsDir(f)
	require.True(t, ok)
	// macOS resolves t.TempDir to a path that may or may not have
	// /private prefixed; compare via EvalSymlinks-style cleanup.
	want, err := filepath.EvalSymlinks(tmp)
	require.NoError(t, err)
	got, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

// TestRelToHost_SameDir pins the trivial case: a target inside
// hostAbsDir returns the same path with the host prefix stripped.
func TestRelToHost_SameDir(t *testing.T) {
	rel, ok := relToHost("/a/b", "/a/b/c.md")
	assert.True(t, ok)
	assert.Equal(t, "c.md", rel)
}

// TestRelToHost_Subdir pins nested resolution: the result uses
// forward slashes so it can feed fs.FS.Open directly on any OS.
func TestRelToHost_Subdir(t *testing.T) {
	rel, ok := relToHost("/a/b", filepath.Join("/a/b", "sub", "c.md"))
	assert.True(t, ok)
	assert.Equal(t, "sub/c.md", rel)
}

// TestRelToHost_EscapesHostReturnsFalse pins the safety check: a
// path that would escape hostAbsDir (e.g. an include reaching above
// the host directory) is rejected so we don't open files outside
// the host's os.DirFS root.
func TestRelToHost_EscapesHostReturnsFalse(t *testing.T) {
	_, ok := relToHost("/a/b", "/a/other.md")
	assert.False(t, ok, "../other.md escapes /a/b; must be rejected")

	_, ok = relToHost("/a/b", "/a")
	assert.False(t, ok, "..  escapes /a/b; must be rejected")
}

// TestRelToHost_RelError pins the filepath.Rel error branch: a
// relative hostAbsDir paired with an absolute target makes Rel
// reject the pair, which the helper translates to (_, false).
func TestRelToHost_RelError(t *testing.T) {
	_, ok := relToHost("rel/base", "/abs/target.md")
	assert.False(t, ok,
		"filepath.Rel of a relative base + absolute target must error; "+
			"helper must surface that as (_, false) so the caller falls "+
			"back to the legacy scan")
}

// TestScanIncludeTargets_ReadError pins the missing-file branch:
// when fsys.Open fails the helper returns nil instead of panicking.
func TestScanIncludeTargets_ReadError(t *testing.T) {
	fsys := fstest.MapFS{}
	got := scanIncludeTargets(fsys, "missing.md", 1024)
	assert.Nil(t, got)
}

// TestScanIncludeTargets_NoDirectives pins the no-includes branch:
// a file with prose but no <?include?> markers returns nil.
func TestScanIncludeTargets_NoDirectives(t *testing.T) {
	fsys := fstest.MapFS{
		"a.md": &fstest.MapFile{Data: []byte("# Heading\n\nProse only.\n")},
	}
	got := scanIncludeTargets(fsys, "a.md", 1024)
	assert.Nil(t, got)
}

// TestScanIncludeTargets_EmptyFileParamSkipped pins the
// empty-file continue branch: an <?include?> with no `file:` value
// parses cleanly but resolves to no target, so the helper must not
// append `path.Dir(filePath)` as a bogus include.
func TestScanIncludeTargets_EmptyFileParamSkipped(t *testing.T) {
	body := "<?include\n?>\nbody\n<?/include?>\n"
	fsys := fstest.MapFS{"a.md": &fstest.MapFile{Data: []byte(body)}}
	got := scanIncludeTargets(fsys, "a.md", 1024)
	assert.Empty(t, got,
		"directive with no `file:` must be skipped, not resolved as "+
			"an empty path")
}

// TestScanIncludeTargets_MalformedYAMLSkipped pins the
// `dir == nil || len(diags) > 0` continue branch: a directive with
// invalid YAML in its body produces (nil, diags) from ParseDirective;
// the helper must skip it instead of dereferencing dir.
func TestScanIncludeTargets_MalformedYAMLSkipped(t *testing.T) {
	// Unclosed YAML flow sequence makes ParseYAMLBody return diags
	// and dir=nil, hitting the `dir == nil` branch in the scan.
	body := "<?include\nfile: [unclosed\n?>\nbody\n<?/include?>\n"
	fsys := fstest.MapFS{"a.md": &fstest.MapFile{Data: []byte(body)}}
	got := scanIncludeTargets(fsys, "a.md", 1024)
	assert.Empty(t, got,
		"directive whose YAML body fails to parse must be skipped, "+
			"not dereferenced")
}

// TestScanIncludeTargets_DirectiveResolvesTarget pins the success
// path: a well-formed <?include?> appends one path-cleaned target
// joined with the source file's directory.
func TestScanIncludeTargets_DirectiveResolvesTarget(t *testing.T) {
	body := "<?include\nfile: target.md\n?>\nbody\n<?/include?>\n"
	fsys := fstest.MapFS{
		"sub/a.md":      &fstest.MapFile{Data: []byte(body)},
		"sub/target.md": &fstest.MapFile{Data: []byte("# T\n")},
	}
	got := scanIncludeTargets(fsys, "sub/a.md", 1024)
	assert.Equal(t, []string{"sub/target.md"}, got)
}

// TestIncludeTargetsOfAbs_NoIncludesReturnsNil pins the early-return
// branch: a file without <?include?> directives produces nil and
// the cache caches that nil (no rebuild on repeat calls).
func TestIncludeTargetsOfAbs_NoIncludesReturnsNil(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)
	fsys := fstest.MapFS{
		"a.md": &fstest.MapFile{Data: []byte("# A\n")},
	}
	cf := &lint.File{Path: "host.md", FS: fsys, RunCache: lint.NewRunCache()}

	got := includeTargetsOfAbs(cf, fsys, tmp, filepath.Join(tmp, "a.md"), 1024)
	assert.Nil(t, got)
}

// TestIncludeTargetsOfAbs_BuildsAbsoluteTargets pins the success
// path: include adjacency is returned as absolute paths so two host
// files whose f.FS roots differ can still share the cached value.
func TestIncludeTargetsOfAbs_BuildsAbsoluteTargets(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)
	body := "<?include\nfile: target.md\n?>\nbody\n<?/include?>\n"
	fsys := fstest.MapFS{
		"sub/a.md":      &fstest.MapFile{Data: []byte(body)},
		"sub/target.md": &fstest.MapFile{Data: []byte("# T\n")},
	}
	cf := &lint.File{Path: "host.md", FS: fsys, RunCache: lint.NewRunCache()}

	absA := filepath.Join(tmp, "sub", "a.md")
	got := includeTargetsOfAbs(cf, fsys, tmp, absA, 1024)
	require.Len(t, got, 1)
	assert.Equal(t, filepath.Join(tmp, "sub", "target.md"), got[0])
}

// TestIncludeTargetsOfAbs_PathOutsideHostReturnsNil pins the
// relToHost rejection branch: when the absolute file path escapes
// hostAbsDir there is no fsys-relative form to read, so the build
// closure returns nil.
func TestIncludeTargetsOfAbs_PathOutsideHostReturnsNil(t *testing.T) {
	fsys := fstest.MapFS{
		"a.md": &fstest.MapFile{Data: []byte("# A\n")},
	}
	cf := &lint.File{Path: "/host/host.md", FS: fsys, RunCache: lint.NewRunCache()}

	got := includeTargetsOfAbs(cf, fsys, "/host", "/elsewhere/a.md", 1024)
	assert.Nil(t, got,
		"absFilePath outside hostAbsDir must return nil — the read "+
			"would otherwise escape the host's os.DirFS root")
}

// TestScanIncludesForTargetAbs_DepthLimit pins the depth guard: an
// initial depth above maxIncludeDepth returns false without reading
// any file (matches the legacy scanIncludesForTarget contract).
func TestScanIncludesForTargetAbs_DepthLimit(t *testing.T) {
	fsys := fstest.MapFS{
		"a.md": &fstest.MapFile{Data: []byte("# A\n")},
	}
	cf := &lint.File{Path: "host.md", FS: fsys, RunCache: lint.NewRunCache()}
	visited := map[string]bool{}
	got := scanIncludesForTargetAbs(cf, fsys, "/host",
		"/host/a.md", "/host/target.md",
		visited, maxIncludeDepth+1, 1024)
	assert.False(t, got)
}

// TestScanIncludesForTargetAbs_DirectMatch pins the success path:
// a single include hitting absTarget returns true.
func TestScanIncludesForTargetAbs_DirectMatch(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)
	body := "<?include\nfile: target.md\n?>\nbody\n<?/include?>\n"
	fsys := fstest.MapFS{
		"a.md":      &fstest.MapFile{Data: []byte(body)},
		"target.md": &fstest.MapFile{Data: []byte("# T\n")},
	}
	cf := &lint.File{Path: "host.md", FS: fsys, RunCache: lint.NewRunCache()}
	visited := map[string]bool{filepath.Join(tmp, "a.md"): true}
	got := scanIncludesForTargetAbs(cf, fsys, tmp,
		filepath.Join(tmp, "a.md"),
		filepath.Join(tmp, "target.md"),
		visited, 0, 1024)
	assert.True(t, got)
}

// TestScanIncludesForTargetAbs_VisitedCycleSkipped pins the
// visited-set short-circuit: an already-visited target is skipped
// rather than re-entered (no infinite recursion).
func TestScanIncludesForTargetAbs_VisitedCycleSkipped(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)
	body := "<?include\nfile: b.md\n?>\ncontent\n<?/include?>\n"
	fsys := fstest.MapFS{
		"a.md": &fstest.MapFile{Data: []byte(body)},
	}
	cf := &lint.File{Path: "host.md", FS: fsys, RunCache: lint.NewRunCache()}
	absA := filepath.Join(tmp, "a.md")
	absB := filepath.Join(tmp, "b.md")
	absTarget := filepath.Join(tmp, "target.md")
	visited := map[string]bool{absA: true, absB: true}
	got := scanIncludesForTargetAbs(cf, fsys, tmp,
		absA, absTarget, visited, 0, 1024)
	assert.False(t, got,
		"b.md is pre-visited and not target; the visited check must "+
			"prevent re-entry and the scan must return false")
}

// TestScanIncludesForTargetAbs_IndirectMatch pins the recursive
// path: a.md -> b.md -> target.md resolves via two recursion levels
// and reports the cycle.
func TestScanIncludesForTargetAbs_IndirectMatch(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)
	aBody := "<?include\nfile: b.md\n?>\ncontent\n<?/include?>\n"
	bBody := "<?include\nfile: target.md\n?>\ncontent\n<?/include?>\n"
	fsys := fstest.MapFS{
		"a.md":      &fstest.MapFile{Data: []byte(aBody)},
		"b.md":      &fstest.MapFile{Data: []byte(bBody)},
		"target.md": &fstest.MapFile{Data: []byte("# T\n")},
	}
	cf := &lint.File{Path: "host.md", FS: fsys, RunCache: lint.NewRunCache()}
	visited := map[string]bool{filepath.Join(tmp, "a.md"): true}
	got := scanIncludesForTargetAbs(cf, fsys, tmp,
		filepath.Join(tmp, "a.md"),
		filepath.Join(tmp, "target.md"),
		visited, 0, 1024)
	assert.True(t, got)
}
