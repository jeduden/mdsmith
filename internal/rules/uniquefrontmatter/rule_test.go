package uniquefrontmatter

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/testsymlink"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func file(t *testing.T, path string, fsys fstest.MapFS) *lint.File {
	t.Helper()
	f, err := lint.NewFile(path, []byte("# Title\n"))
	require.NoError(t, err)
	f.FS = fsys
	return f
}

func planFS() fstest.MapFS {
	return fstest.MapFS{
		"plan/a.md":     {Data: []byte("---\nid: 7\ntitle: A\n---\n# A\n")},
		"plan/b.md":     {Data: []byte("---\nid: 7\ntitle: B\n---\n# B\n")},
		"plan/c.md":     {Data: []byte("---\nid: 9\ntitle: C\n---\n# C\n")},
		"plan/notes.md": {Data: []byte("---\ntitle: no id here\n---\n# N\n")},
		"plan/proto.md": {Data: []byte("---\nid: 'int & >=1'\n---\n# P\n")},
		"other/d.md":    {Data: []byte("---\nid: 7\n---\n# D\n")},
	}
}

// planRule builds the shared test rule through ApplySettings so
// the interned scope key (the production path) is exercised.
func planRule() *Rule {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"field":   "id",
		"include": []any{"plan/*.md"},
		"exclude": []any{"plan/proto.md"},
	})
	if err != nil {
		panic(err)
	}
	return r
}

func TestDuplicateFlagsLaterPathOnly(t *testing.T) {
	fsys := planFS()
	r := planRule()

	require.Nil(t, r.Check(file(t, "plan/a.md", fsys)),
		"first holder in path order stays clean")

	diags := r.Check(file(t, "plan/b.md", fsys))
	require.Len(t, diags, 1)
	d := diags[0]
	assert.Equal(t, "plan/b.md", d.File)
	assert.Equal(t, 2, d.Line, "diagnostic anchors at the id: line")
	assert.Equal(t, 1, d.Column)
	assert.Equal(t, "MDS069", d.RuleID)
	assert.Equal(t,
		`front-matter "id": value 7 already used by plan/a.md`,
		d.Message)
}

func TestDistinctValuesPass(t *testing.T) {
	fsys := planFS()
	r := planRule()
	assert.Nil(t, r.Check(file(t, "plan/c.md", fsys)))
}

func TestMissingFieldSkips(t *testing.T) {
	fsys := planFS()
	r := planRule()
	assert.Nil(t, r.Check(file(t, "plan/notes.md", fsys)))
}

func TestExcludedAndOutOfScopeSkip(t *testing.T) {
	fsys := planFS()
	r := planRule()
	assert.Nil(t, r.Check(file(t, "plan/proto.md", fsys)),
		"excluded file is never flagged")
	assert.Nil(t, r.Check(file(t, "other/d.md", fsys)),
		"file outside the include globs is never flagged")
}

func TestUnconfiguredIsInert(t *testing.T) {
	fsys := planFS()
	assert.Nil(t, (&Rule{}).Check(file(t, "plan/b.md", fsys)))
	assert.Nil(t, (&Rule{Field: "id"}).Check(file(t, "plan/b.md", fsys)),
		"empty include list disables the rule")
}

func TestIntAndStringValuesCollide(t *testing.T) {
	fsys := fstest.MapFS{
		"plan/a.md": {Data: []byte("---\nid: 7\n---\n# A\n")},
		"plan/b.md": {Data: []byte("---\nid: \"7\"\n---\n# B\n")},
	}
	r := &Rule{Field: "id", Include: []string{"plan/*.md"}}
	diags := r.Check(file(t, "plan/b.md", fsys))
	require.Len(t, diags, 1, "quoted and bare scalars compare by value")
}

func TestThreeWayDuplicateFlagsEachLaterFile(t *testing.T) {
	fsys := fstest.MapFS{
		"plan/a.md": {Data: []byte("---\nid: 7\n---\n# A\n")},
		"plan/b.md": {Data: []byte("---\nid: 7\n---\n# B\n")},
		"plan/c.md": {Data: []byte("---\nid: 7\n---\n# C\n")},
	}
	r := &Rule{Field: "id", Include: []string{"plan/*.md"}}
	assert.Nil(t, r.Check(file(t, "plan/a.md", fsys)))
	for _, p := range []string{"plan/b.md", "plan/c.md"} {
		diags := r.Check(file(t, p, fsys))
		require.Len(t, diags, 1, p)
		assert.Contains(t, diags[0].Message, "plan/a.md",
			"every later file names the first holder")
	}
}

func TestRunCacheSharesOneIndex(t *testing.T) {
	fsys := planFS()
	r := planRule()
	rc := lint.NewRunCache()

	fa := file(t, "plan/a.md", fsys)
	fb := file(t, "plan/b.md", fsys)
	fa.RunCache = rc
	fb.RunCache = rc

	require.Nil(t, r.Check(fa))
	diags := r.Check(fb)
	require.Len(t, diags, 1)

	ia := r.index(fa)
	ib := r.index(fb)
	assert.Same(t, ia, ib, "both hosts read the same cached index")

	rc.Invalidate("plan/a.md")
	assert.NotSame(t, ia, r.index(fa),
		"Invalidate drops the index so the next Check rebuilds")
}

func TestApplySettings(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"field":   "id",
		"include": []any{"plan/*.md"},
		"exclude": []any{"plan/proto.md"},
	})
	require.NoError(t, err)
	assert.Equal(t, "id", r.Field)
	assert.Equal(t, []string{"plan/*.md"}, r.Include)
	assert.Equal(t, []string{"plan/proto.md"}, r.Exclude)
}

func TestApplySettingsRejectsBadInput(t *testing.T) {
	assert.Error(t, (&Rule{}).ApplySettings(map[string]any{"field": 7}),
		"non-string field")
	assert.Error(t, (&Rule{}).ApplySettings(map[string]any{"include": "x"}),
		"non-list include")
	assert.Error(t, (&Rule{}).ApplySettings(map[string]any{"nope": true}),
		"unknown key")
	assert.Error(t, (&Rule{}).ApplySettings(map[string]any{
		"field": "id", "include": []any{"plan/[*.md"},
	}), "invalid glob pattern")
}

// TestDiagnosticLineSurvivesLineOffset pins the body-coordinate
// contract: the rule emits e.line - f.LineOffset so the engine's
// AdjustDiagnostics shift lands the diagnostic back on the raw
// file line of the field (the MDS020 pattern).
func TestDiagnosticLineSurvivesLineOffset(t *testing.T) {
	fsys := planFS()
	r := planRule()

	src := []byte("---\nid: 7\ntitle: B\n---\n# B\n")
	f, err := lint.NewFileFromSource("plan/b.md", src, true)
	require.NoError(t, err)
	f.FS = fsys

	diags := r.Check(f)
	require.Len(t, diags, 1)
	f.AdjustDiagnostics(diags)
	assert.Equal(t, 2, diags[0].Line,
		"diagnostic anchors at the raw file line of id:")
}

// TestAbsolutePathHostStillMatches pins the lookup-key
// normalization: the CLI sets f.Path to the absolute argument
// path, while the index keys are workspace-relative glob output.
// Without anchoring to RootDir the rule was silently inert for
// absolute-path invocations.
func TestAbsolutePathHostStillMatches(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "plan"), 0o755))
	write := func(name, body string) {
		require.NoError(t, os.WriteFile(
			filepath.Join(root, "plan", name), []byte(body), 0o644))
	}
	write("a.md", "---\nid: 7\n---\n# A\n")
	write("b.md", "---\nid: 7\n---\n# B\n")

	r := &Rule{Field: "id", Include: []string{"plan/*.md"}}
	abs := filepath.Join(root, "plan", "b.md")
	f, err := lint.NewFile(abs, []byte("# Title\n"))
	require.NoError(t, err)
	f.RootDir = root
	f.RootFS = os.DirFS(root)

	diags := r.Check(f)
	require.Len(t, diags, 1,
		"absolute host path must anchor to RootDir and match")
	assert.Contains(t, diags[0].Message, "plan/a.md")
}

// TestSymlinkedFilesAreSkipped pins the plan-84 posture: a symlink
// planted in the scope must not pull outside front matter into the
// uniqueness namespace, neither to flag a real file nor to claim
// first-holder.
func TestSymlinkedFilesAreSkipped(t *testing.T) {
	testsymlink.SkipIfSymlinkUnsupported(t)

	root := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "plan"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(outside, "outside.md"),
		[]byte("---\nid: 7\n---\n# Outside\n"), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "plan", "zzz.md"),
		[]byte("---\nid: 7\n---\n# Real\n"), 0o644))
	require.NoError(t, os.Symlink(
		filepath.Join(outside, "outside.md"),
		filepath.Join(root, "plan", "aaa.md")))

	r := &Rule{Field: "id", Include: []string{"plan/*.md"}}
	f, err := lint.NewFile("plan/zzz.md", []byte("# Title\n"))
	require.NoError(t, err)
	f.RootFS = os.DirFS(root)

	assert.Nil(t, r.Check(f),
		"a symlinked first-holder must not flag the real file")
}

// TestScopeIndexMatchesInvalidatedPath pins the targeted
// invalidation matcher the LSP relies on.
func TestScopeIndexMatchesInvalidatedPath(t *testing.T) {
	s := &scopeIndex{
		rootDir: filepath.FromSlash("/ws"),
		include: []string{"plan/*.md"},
		exclude: []string{"plan/proto.md"},
	}
	abs := func(p string) string { return filepath.FromSlash(p) }

	assert.True(t, s.MatchesInvalidatedPath(abs("/ws/plan/a.md")),
		"in-scope edit must drop the index")
	assert.False(t, s.MatchesInvalidatedPath(abs("/ws/docs/a.md")),
		"out-of-scope edit must keep the index")
	assert.False(t, s.MatchesInvalidatedPath(abs("/ws/plan/proto.md")),
		"excluded path must keep the index")
	assert.True(t, s.MatchesInvalidatedPath(abs("/elsewhere/plan/a.md")),
		"unrelatable paths drop the index — stale verdicts cost more "+
			"than a rebuild (e.g. macOS /tmp vs /private/tmp roots)")
	assert.True(t,
		(&scopeIndex{}).MatchesInvalidatedPath(abs("/anything")),
		"no root recorded: match everything, drop conservatively")
}

// TestSubdirDiscoveryShapeStillMatches pins the CWD-relative host
// shape: discovery run from a subdirectory hands the rule paths
// relative to the working directory, not the workspace root, while
// the index globs the root. The absolute key space reconciles the
// two.
func TestSubdirDiscoveryShapeStillMatches(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "plan"), 0o755))
	write := func(name, body string) {
		require.NoError(t, os.WriteFile(
			filepath.Join(root, "plan", name), []byte(body), 0o644))
	}
	write("a.md", "---\nid: 7\n---\n# A\n")
	write("b.md", "---\nid: 7\n---\n# B\n")
	t.Chdir(filepath.Join(root, "plan"))

	r := &Rule{Field: "id", Include: []string{"plan/*.md"}}
	f, err := lint.NewFile("b.md", []byte("# Title\n"))
	require.NoError(t, err)
	f.RootDir = root
	f.RootFS = os.DirFS(root)

	diags := r.Check(f)
	require.Len(t, diags, 1,
		"CWD-relative host path must resolve against the cwd")
	assert.Contains(t, diags[0].Message, "plan/a.md")
}

// TestTargetedInvalidationWithRootDir drives the real scopeIndex
// through RunCache.Invalidate with absolute paths: an in-root edit
// outside the globs keeps the index, an in-scope edit drops it.
func TestTargetedInvalidationWithRootDir(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "plan"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "plan", "a.md"),
		[]byte("---\nid: 7\n---\n# A\n"), 0o644))

	r := &Rule{Field: "id", Include: []string{"plan/*.md"}}
	rc := lint.NewRunCache()
	f, err := lint.NewFile(filepath.Join(root, "plan", "a.md"),
		[]byte("# Title\n"))
	require.NoError(t, err)
	f.RootDir = root
	f.RootFS = os.DirFS(root)
	f.RunCache = rc

	require.Nil(t, r.Check(f))
	warm := r.index(f)

	rc.Invalidate(filepath.Join(root, "docs", "x.md"))
	assert.Same(t, warm, r.index(f),
		"in-root edit outside the globs keeps the index")

	rc.Invalidate(filepath.Join(root, "plan", "a.md"))
	assert.NotSame(t, warm, r.index(f),
		"in-scope edit drops the index")
}

// TestSpellingVariantsCollide pins canonical scalar comparison:
// int, bool, and float spellings of one value share a key.
func TestSpellingVariantsCollide(t *testing.T) {
	fsys := fstest.MapFS{
		"plan/a.md": {Data: []byte("---\nid: 16\n---\n# A\n")},
		"plan/b.md": {Data: []byte("---\nid: 0x10\n---\n# B\n")},
	}
	r := &Rule{Field: "id", Include: []string{"plan/*.md"}}
	diags := r.Check(file(t, "plan/b.md", fsys))
	require.Len(t, diags, 1, "hex and decimal spellings collide")
	assert.Contains(t, diags[0].Message, "value 16")
}

func TestScopePathsOrderingAndExclusion(t *testing.T) {
	fsys := planFS()
	r := planRule()
	assert.Equal(t,
		[]string{"plan/a.md", "plan/b.md", "plan/c.md", "plan/notes.md"},
		r.scopePaths(fsys),
		"ascending order, excludes dropped, out-of-glob dropped")
}

func TestRuleMetadata(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, "MDS069", r.ID())
	assert.Equal(t, "unique-frontmatter", r.Name())
	assert.Equal(t, "structural", r.Category())
}

func TestDefaultSettingsZeroValues(t *testing.T) {
	assert.Equal(t, map[string]any{
		"field":   "",
		"include": []string{},
		"exclude": []string{},
	}, (&Rule{}).DefaultSettings())
}

func TestHostKeyWithoutCwdMissesIndex(t *testing.T) {
	s := &scopeIndex{rootDir: filepath.FromSlash("/ws")}
	f, err := lint.NewFile("plan/b.md", []byte("# T\n"))
	require.NoError(t, err)
	assert.Equal(t, "", s.hostKey(f),
		"relative host without a captured cwd cannot resolve")
}

func TestNoWorkspaceFSYieldsEmptyIndex(t *testing.T) {
	r := planRule()
	f, err := lint.NewFile("plan/b.md", []byte("# T\n"))
	require.NoError(t, err)
	assert.Nil(t, r.Check(f),
		"a File with neither FS nor RootFS has no scope to index")
}

func TestOverlappingIncludesDeduplicate(t *testing.T) {
	fsys := fstest.MapFS{
		"plan/a.md": {Data: []byte("---\nid: 7\n---\n# A\n")},
		"plan/b.md": {Data: []byte("---\nid: 7\n---\n# B\n")},
	}
	r := &Rule{Field: "id", Include: []string{"plan/*.md", "**/*.md"}}
	diags := r.Check(file(t, "plan/b.md", fsys))
	require.Len(t, diags, 1,
		"a file matched by two patterns joins the scope once")
}

func TestUnparseableParticipantsAreSkipped(t *testing.T) {
	fsys := fstest.MapFS{
		"plan/alias.md":   {Data: []byte("---\nid: &a 7\n---\n# A\n")},
		"plan/badyaml.md": {Data: []byte("---\nid: [unclosed\n---\n# B\n")},
		"plan/nofm.md":    {Data: []byte("# No front matter\n")},
		"plan/ok.md":      {Data: []byte("---\nid: 7\n---\n# O\n")},
		"plan/zz.md":      {Data: []byte("---\nid: 7\n---\n# Z\n")},
	}
	r := &Rule{Field: "id", Include: []string{"plan/*.md"}}
	diags := r.Check(file(t, "plan/zz.md", fsys))
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "plan/ok.md",
		"files without front matter or with unparseable YAML never participate")
}

func TestOversizeParticipantsAreSkipped(t *testing.T) {
	fsys := planFS()
	r := planRule()
	f := file(t, "plan/b.md", fsys)
	f.MaxInputBytes = 1
	assert.Nil(t, r.Check(f),
		"reads beyond the byte limit drop the file from the scope")
}
