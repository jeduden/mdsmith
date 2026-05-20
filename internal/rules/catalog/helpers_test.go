package catalog

import (
	"os"
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
// hostAbsDir there is no fsys-relative form to read, so the call
// returns nil without consulting the cache.
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

// TestIncludeTargetsOfAbs_OutsideHostDoesNotPoisonCache pins the
// correctness fix flagged in PR review: when host A reaches an
// absFilePath that lies outside its own hostAbsDir, the host-relative
// read failure must NOT be cached. Otherwise a later host B whose
// hostAbsDir does contain the same absFilePath would see the cached
// nil and silently skip parsing real include directives, producing
// order-dependent false negatives in cycle detection.
func TestIncludeTargetsOfAbs_OutsideHostDoesNotPoisonCache(t *testing.T) {
	cache := lint.NewRunCache()

	// Host A: hostAbsDir=/repo/a; the target lives at /repo/shared
	// which is OUTSIDE host A's directory. relToHost rejects the
	// translation, so includeTargetsOfAbs must return nil without
	// touching the cache.
	fsysA := fstest.MapFS{} // unused by the failing branch
	cfA := &lint.File{Path: "/repo/a/host.md", FS: fsysA, RunCache: cache}
	got := includeTargetsOfAbs(cfA, fsysA,
		"/repo/a", "/repo/shared/foo.md", 1024)
	require.Nil(t, got, "host A cannot read /repo/shared/foo.md "+
		"via its fsys; the helper must return nil")

	// Host B: hostAbsDir=/repo/shared; the SAME absolute target is
	// reachable. If host A polluted the cache with a nil, host B's
	// call would echo that nil and miss the include chain. Pin the
	// fix by giving host B a real include chain and asserting the
	// chain is parsed.
	body := "<?include\nfile: target.md\n?>\nbody\n<?/include?>\n"
	fsysB := fstest.MapFS{
		"foo.md":    &fstest.MapFile{Data: []byte(body)},
		"target.md": &fstest.MapFile{Data: []byte("# T\n")},
	}
	cfB := &lint.File{Path: "/repo/shared/host.md", FS: fsysB, RunCache: cache}
	gotB := includeTargetsOfAbs(cfB, fsysB,
		"/repo/shared", "/repo/shared/foo.md", 1024)
	require.Len(t, gotB, 1, "host B must parse foo.md fresh; a "+
		"cached nil from host A would silently skip the include "+
		"and miss real cycles in cross-directory catalogs")
	assert.Equal(t, "/repo/shared/target.md", gotB[0])
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

// TestScanFrameFor_LocalUsesHostFS pins that a local-FS-resolved
// catalog keeps the legacy frame: f.FS, base-filename target,
// hostAbsDir. This is the behavior every struct-literal test in
// memo_test.go and rule_test.go depends on.
func TestScanFrameFor_LocalUsesHostFS(t *testing.T) {
	fsys := fstest.MapFS{}
	f := &lint.File{Path: "/tmp/repo/host.md", FS: fsys}
	res := globResolution{rootRelative: false, fs: fsys}
	scanFS, target, abs, ok := scanFrameFor(f, res, f.Path)
	assert.True(t, ok)
	assert.Equal(t, fsys, scanFS, "local-FS catalog must scan through f.FS")
	assert.Equal(t, "host.md", target,
		"local-FS catalog target is the host file's basename")
	assert.Equal(t, "/tmp/repo", abs,
		"abs base is the host file's directory for local-FS scans")
}

// TestScanFrameFor_RootRelativeUsesRootFS pins the fix: a
// rootRelative catalog frames its scan against res.fs (f.RootFS),
// with a root-relative target path derived from res.fileDir. This
// is what lets the cycle scan see across directories.
func TestScanFrameFor_RootRelativeUsesRootFS(t *testing.T) {
	rootFS := fstest.MapFS{}
	f := &lint.File{Path: "/repo/sub/host.md", RootDir: "/repo"}
	res := globResolution{
		rootRelative: true,
		fs:           rootFS,
		fileDir:      "sub",
	}
	scanFS, target, abs, ok := scanFrameFor(f, res, f.Path)
	assert.True(t, ok)
	assert.Equal(t, rootFS, scanFS,
		"rootRelative catalog must scan through res.fs (f.RootFS), "+
			"not the host's os.DirFS")
	assert.Equal(t, "sub/host.md", target,
		"rootRelative target must be root-relative; otherwise an "+
			"include reaching back via \"../sub/host.md\" wouldn't "+
			"match a basename-only target")
	assert.Equal(t, "/repo", abs,
		"abs base for rootRelative scans is f.RootDir")
}

// TestScanFrameFor_RootRelativeAtRoot pins the host-at-root case:
// fileDir is empty, so the target stays the bare basename.
func TestScanFrameFor_RootRelativeAtRoot(t *testing.T) {
	rootFS := fstest.MapFS{}
	f := &lint.File{Path: "/repo/host.md", RootDir: "/repo"}
	res := globResolution{rootRelative: true, fs: rootFS, fileDir: ""}
	scanFS, target, _, ok := scanFrameFor(f, res, f.Path)
	assert.True(t, ok)
	assert.Equal(t, rootFS, scanFS)
	assert.Equal(t, "host.md", target,
		"host at root with fileDir=\"\" must produce a bare-basename "+
			"target, not \"/host.md\"")
}

// TestScanFrameFor_RootRelativeNoFSReturnsNil pins the safety
// branch: rootRelative without a resolved fs (an internally
// inconsistent globResolution) returns nil so the caller skips
// the scan rather than dereferencing res.fs.
func TestScanFrameFor_RootRelativeNoFSReturnsNil(t *testing.T) {
	f := &lint.File{Path: "/repo/host.md", RootDir: "/repo"}
	res := globResolution{rootRelative: true, fs: nil}
	scanFS, _, _, ok := scanFrameFor(f, res, f.Path)
	assert.Nil(t, scanFS)
	assert.False(t, ok)
}

// TestRootAbsDir_Set pins the success path.
func TestRootAbsDir_Set(t *testing.T) {
	abs, ok := rootAbsDir(&lint.File{RootDir: "/repo"})
	assert.True(t, ok)
	assert.Equal(t, "/repo", abs)
}

// TestRootAbsDir_EmptyReturnsFalse pins the empty-RootDir branch:
// when no project root is configured the cache cannot key
// consistently across hosts, so the helper opts out and the
// caller falls back to the legacy fsys-relative scan.
func TestRootAbsDir_EmptyReturnsFalse(t *testing.T) {
	_, ok := rootAbsDir(&lint.File{RootDir: ""})
	assert.False(t, ok)
}

// TestCheckCatalogIncludeCycle_RootRelativeDetectsCrossDirCycle pins
// the headline behavior change with a host file at root: a catalog
// resolves through f.RootFS via source-dir, lib/cycle.md
// back-includes ../host.md, and the scan reports the cycle. Note
// the *legacy* scan also catches this specific case (f.FS happens
// to equal f.RootFS when the host is at the project root) — the
// stronger regression test is in
// TestCheckCatalogIncludeCycle_RootRelativeFromSubdirHost below.
func TestCheckCatalogIncludeCycle_RootRelativeDetectsCrossDirCycle(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "lib"), 0o755))
	hostSrc := "<?catalog\n" +
		"source-dir: \"lib\"\n" +
		"glob: \"*.md\"\n" +
		"?>\n" +
		"<?/catalog?>\n"
	cycleSrc := "<?include\nfile: \"../host.md\"\n?>\nbody\n<?/include?>\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, "host.md"),
		[]byte(hostSrc), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "lib", "cycle.md"),
		[]byte(cycleSrc), 0o644))

	hostPath := filepath.Join(root, "host.md")
	src, err := os.ReadFile(hostPath)
	require.NoError(t, err)
	f, err := lint.NewFile(hostPath, src)
	require.NoError(t, err)
	f.FS = os.DirFS(root)
	f.SetRootDir(root)

	r := &Rule{}
	diags := r.Check(f)
	require.NotEmpty(t, diags,
		"lib/cycle.md back-includes ../host.md; the cycle scan must "+
			"detect this. Without the fix the scan uses f.FS = "+
			"os.DirFS(root) and tries to open the displayPath form "+
			"\"lib/cycle.md\", which actually works here — but for a "+
			"host file in a subdirectory the displayPath would be "+
			"\"../lib/cycle.md\" and os.DirFS rejects it. The fix "+
			"makes both cases route through f.RootFS with the "+
			"un-displayed root-relative path.")
	found := false
	for _, d := range diags {
		if d.RuleID == "MDS019" {
			found = true
			assert.Contains(t, d.Message, "cycle",
				"diagnostic must explain the cycle")
			break
		}
	}
	assert.True(t, found, "expected an MDS019 cycle diagnostic")
}

// TestCheckCatalogIncludeCycle_RootRelativeFromSubdirHost pins the
// case the legacy scan literally cannot detect: the host file lives
// in a subdirectory and its catalog reaches a sibling tree via
// "..". With the old scan, matchedPath was the displayPath form
// "../shared/cycle.md", which os.DirFS(f's directory) rejects on
// open. The fix scans through f.RootFS with root-relative
// "shared/cycle.md", so the back-include into sub/host.md is found.
func TestCheckCatalogIncludeCycle_RootRelativeFromSubdirHost(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "sub"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "shared"), 0o755))
	hostSrc := "<?catalog\n" +
		"glob: \"../shared/*.md\"\n" +
		"?>\n" +
		"<?/catalog?>\n"
	cycleSrc := "<?include\nfile: \"../sub/host.md\"\n?>\nbody\n<?/include?>\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, "sub", "host.md"),
		[]byte(hostSrc), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "shared", "cycle.md"),
		[]byte(cycleSrc), 0o644))

	hostPath := filepath.Join(root, "sub", "host.md")
	src, err := os.ReadFile(hostPath)
	require.NoError(t, err)
	f, err := lint.NewFile(hostPath, src)
	require.NoError(t, err)
	f.FS = os.DirFS(filepath.Join(root, "sub"))
	f.SetRootDir(root)

	r := &Rule{}
	diags := r.Check(f)
	require.NotEmpty(t, diags,
		"shared/cycle.md back-includes ../sub/host.md; the cycle "+
			"scan must walk through f.RootFS to see it. The legacy "+
			"scan tried displayPath \"../shared/cycle.md\" against "+
			"f.FS=os.DirFS(sub) which rejects the leading \"..\" — "+
			"a silent miss the fix closes.")
	for _, d := range diags {
		if d.RuleID == "MDS019" {
			assert.Contains(t, d.Message, "cycle")
			// The diagnostic must name the catalog target using the
			// root-relative path the scan actually used, not the
			// basename — a host at /repo/sub/host.md and another at
			// /repo/host.md are different targets and a basename-
			// only message would conflate them.
			assert.Contains(t, d.Message, "sub/host.md",
				"diagnostic must name the catalog target with its "+
					"root-relative path (\"sub/host.md\"); a bare "+
					"basename would misreport which host the cycle "+
					"reaches when multiple host.md files exist in "+
					"different subdirectories")
			return
		}
	}
	t.Fatalf("expected an MDS019 cycle diagnostic, got %v", diags)
}

// TestCheckCatalogIncludeCycle_NilScanFSReturnsNil pins the safety
// branch: when scanFrameFor returns a nil scanFS (rootRelative
// resolution without a resolved res.fs), checkCatalogIncludeCycle
// must short-circuit instead of opening through a nil filesystem.
func TestCheckCatalogIncludeCycle_NilScanFSReturnsNil(t *testing.T) {
	f := &lint.File{
		Path:    "/repo/host.md",
		RootDir: "/repo",
		FS:      fstest.MapFS{}, // non-nil so the outer guard doesn't fire
	}
	res := globResolution{rootRelative: true, fs: nil}
	diags := checkCatalogIncludeCycle(f, "/repo/host.md", 1,
		[]fileEntry{{fields: map[string]any{"filename": "x.md"}}}, res)
	assert.Nil(t, diags,
		"rootRelative resolution with no fs must short-circuit; "+
			"the scan has no filesystem to walk")
}

// TestCheckCatalogIncludeCycle_EntryWithoutMatchPath pins the
// legacy-fileEntry fallback: an entry constructed without matchPath
// (the way every struct-literal test in rule_test.go builds them)
// must still report cycles, by falling back to the display-form
// filename for the scan.
func TestCheckCatalogIncludeCycle_EntryWithoutMatchPath(t *testing.T) {
	body := "<?include\nfile: host.md\n?>\nbody\n<?/include?>\n"
	fsys := fstest.MapFS{
		"cycle.md": &fstest.MapFile{Data: []byte(body)},
	}
	f := &lint.File{Path: "host.md", FS: fsys}
	// matchPath intentionally empty so the fallback path runs.
	entries := []fileEntry{{fields: map[string]any{"filename": "cycle.md"}}}
	diags := checkCatalogIncludeCycle(f, "host.md", 1, entries, globResolution{})
	require.Len(t, diags, 1,
		"entry with no matchPath must fall back to fields[\"filename\"] "+
			"so legacy struct-literal callers still see the cycle")
	assert.Contains(t, diags[0].Message, "cycle")
}

// TestAbsBaseDir_AbsolutePathShortCircuits pins strategy 1: an
// absolute f.Path returns filepath.Dir directly without touching
// the process CWD or f.RootDir.
func TestAbsBaseDir_AbsolutePathShortCircuits(t *testing.T) {
	f := &lint.File{Path: "/abs/repo/sub/host.md", RootDir: "/wrong/root"}
	dir, ok := absBaseDir(f)
	assert.True(t, ok)
	assert.Equal(t, "/abs/repo/sub", dir,
		"absolute f.Path must short-circuit; f.RootDir must be ignored "+
			"when the file path is already absolute")
}

// TestAbsBaseDir_LSPShapeAnchorsAtRootDir pins strategy 2 — the
// case the PR review surfaced. The LSP sets f.Path to a
// workspace-relative path ("docs/host.md") and f.RootDir to the
// workspace's absolute path ("/abs/workspace"); the helper must
// anchor the base at RootDir + Dir(f.Path) so cache keys align
// with the absolute doc.path the LSP's invalidate seam uses.
//
// Without this strategy the helper would fall through to
// filepath.Abs("docs"), which anchors at the process CWD —
// producing /cwd/docs while the invalidation arrives as
// /abs/workspace/docs and the eviction silently misses.
func TestAbsBaseDir_LSPShapeAnchorsAtRootDir(t *testing.T) {
	f := &lint.File{Path: "docs/host.md", RootDir: "/abs/workspace"}
	dir, ok := absBaseDir(f)
	require.True(t, ok)
	assert.Equal(t, "/abs/workspace/docs", dir,
		"LSP-shaped File (workspace-relative Path + absolute RootDir) "+
			"must anchor at RootDir so cache keys match the LSP's "+
			"doc.path-based invalidation")
}

// TestAbsBaseDir_RelativePathNoRootDirFallsBackToCwd pins strategy
// 3: when no RootDir is configured the helper uses the process
// CWD, matching the legacy CLI behavior (and what os.DirFS opens
// against for a relative path).
func TestAbsBaseDir_RelativePathNoRootDirFallsBackToCwd(t *testing.T) {
	cwd := t.TempDir()
	t.Chdir(cwd)
	f := &lint.File{Path: "host.md"}
	dir, ok := absBaseDir(f)
	require.True(t, ok)
	want, err := filepath.EvalSymlinks(cwd)
	require.NoError(t, err)
	got, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	assert.Equal(t, want, got,
		"no RootDir + relative Path must anchor at CWD")
}

// TestAbsMatchedPath_LSPShapeAlignsWithDocPath is the end-to-end
// invariant for the PR review: the cache key absMatchedPath
// computes for a matched target inside the LSP's workspace must
// exactly match the absolute path Server.invalidateCachedRead is
// called with (== doc.path of an edited file). Otherwise a
// didChange / didSave on the target evicts a non-existent entry
// and the next runLint serves the stale cached body.
func TestAbsMatchedPath_LSPShapeAlignsWithDocPath(t *testing.T) {
	// LSP-shaped lint.File: workspace-relative Path, absolute
	// RootDir, RootFS rooted at the workspace.
	const workspace = "/abs/workspace"
	f := &lint.File{
		Path:    "docs/host.md",
		RootDir: workspace,
	}
	res := localFSResolution(f, nil, nil)

	// A target matched at "target.md" relative to res.fs (i.e.
	// f.FS, anchored at /abs/workspace/docs).
	abs, ok := absMatchedPath(res, "target.md")
	require.True(t, ok)
	// What the LSP would invalidate when the editor edits the
	// same file: dirname of the host's absolute doc.path joined
	// with the target's basename.
	docPathAbsTarget := filepath.Join(workspace, "docs", "target.md")
	assert.Equal(t, docPathAbsTarget, abs,
		"absMatchedPath under the LSP shape must produce the same "+
			"absolute path Server.invalidateCachedRead uses; a "+
			"mismatch means edits silently fail to evict cached reads")
}

// TestLocalFSResolution_LSPShapeAnchorsAtRootDir pins the
// happy-path contract under the LSP shape (workspace-relative
// f.Path + absolute f.RootDir): localFSResolution must produce
// a workspace-anchored absolute gitignoreBase, not a CWD-anchored
// one. This is the path that lets absMatchedPath build cache keys
// matching Server.invalidateCachedRead's absolute doc.path.
func TestLocalFSResolution_LSPShapeAnchorsAtRootDir(t *testing.T) {
	f := &lint.File{Path: "docs/host.md", RootDir: "/abs/workspace"}
	res := localFSResolution(f, nil, nil)
	assert.Equal(t, "/abs/workspace/docs", res.gitignoreBase,
		"LSP-shaped File must produce a workspace-anchored absolute "+
			"gitignoreBase, not a CWD-anchored one")
	assert.True(t, filepath.IsAbs(res.gitignoreBase),
		"gitignoreBase must be absolute — feeds absMatchedPath's "+
			"cache keys; relative leaks break LSP invalidation")
}

// TestLocalFSResolution_LeavesBaseEmptyForNonAbsResult pins the
// review-flagged safety branch: when absBaseDir cannot derive an
// absolute path, localFSResolution must leave gitignoreBase empty
// rather than letting a non-absolute "." or "" leak in. The full
// failure path (filepath.Abs returning err) requires a missing
// cwd, which is not portably reproducible in tests, so we drive
// the contract directly via a hand-built globResolution: an
// empty gitignoreBase must propagate to absMatchedPath as the
// "opt out" signal. Pinned end-to-end below in
// TestAbsMatchedPath_NoBaseReturnsFalse.
func TestLocalFSResolution_LeavesBaseEmptyForNonAbsResult(t *testing.T) {
	// The only public lever into the "empty base" outcome is a
	// resolution that was never built by absBaseDir. We assert the
	// downstream contract here — that absMatchedPath rejects an
	// empty base — and rely on localFSResolution honoring
	// absBaseDir's ok signal (a one-line guard) for the upstream
	// half. Reverting the guard to the pre-fix
	// `gitignoreBase: base` would leak "." into gitignoreBase but
	// not change the absMatchedPath contract here; the diagnostic
	// breakage would surface via the LSP integration tests.
	res := globResolution{gitignoreBase: ""}
	_, ok := absMatchedPath(res, "x.md")
	assert.False(t, ok,
		"absMatchedPath must short-circuit for an empty base so the "+
			"caller falls back to the per-Check memo instead of "+
			"keying the run cache at a non-absolute path")
}

// TestAbsMatchedPath_NoBaseReturnsFalse pins the contract
// absMatchedPath upholds when localFSResolution opts out of the
// run cache: an empty gitignoreBase must short-circuit to
// (_, false), keeping the caller on the per-Check memo path.
func TestAbsMatchedPath_NoBaseReturnsFalse(t *testing.T) {
	res := globResolution{}
	_, ok := absMatchedPath(res, "x.md")
	assert.False(t, ok,
		"empty gitignoreBase must opt out of the run cache; a "+
			"non-absolute key would break LSP invalidation")
}
