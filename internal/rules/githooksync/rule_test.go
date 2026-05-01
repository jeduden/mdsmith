package githooksync

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/githooks"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	// Register the directive-bearing rules so DiscoverFiles can find
	// real catalog/include/toc markers in test fixtures.
	_ "github.com/jeduden/mdsmith/internal/rules/catalog"
	_ "github.com/jeduden/mdsmith/internal/rules/include"
	_ "github.com/jeduden/mdsmith/internal/rules/toc"
)

// initTestRepo runs `git init` on dir and pins core.hooksPath to
// dir/.git/hooks in the repo-local config. The pin keeps tests
// hermetic: a developer with a non-default core.hooksPath set
// globally cannot have the rule (or its tests) read or write to
// files outside the temp repo.
func initTestRepo(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, exec.Command("git", "init", dir).Run())
	require.NoError(t, exec.Command(
		"git", "-C", dir, "config", "core.hooksPath",
		filepath.Join(dir, ".git", "hooks"),
	).Run())
}

// initRepoWithDriver initialises a git repo at dir, pins
// core.hooksPath, and registers the mdsmith merge driver in its
// local config so the rule will read .gitattributes (the real
// source of truth).
func initRepoWithDriver(t *testing.T, dir string) {
	t.Helper()
	initTestRepo(t, dir)
	require.NoError(t, exec.Command(
		"git", "-C", dir, "config", "merge.mdsmith.driver",
		"mdsmith merge-driver run %O %A %B %P",
	).Run())
}

func TestRule_Check_SkipsWhenNoSourceOptedIn(t *testing.T) {
	// Neither the merge driver nor an mdsmith-managed
	// pre-merge-commit hook is installed. The rule must not run
	// the repo-wide discovery walk and must emit no diagnostics.
	dir := t.TempDir()
	initTestRepo(t, dir)

	// Put a directive file in the repo to confirm the rule does
	// not attempt drift comparison against it.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"),
		[]byte("# Test\n\n<?catalog?>\n<?/catalog?>\n"), 0o644))

	r := &Rule{}
	f := &lint.File{
		Path:          filepath.Join(dir, "README.md"),
		Source:        []byte("# Test\n"),
		MaxInputBytes: 1048576,
		FS:            os.DirFS(dir),
	}
	assert.Empty(t, r.Check(f))
}

func TestRule_Check_SkipsWhenFSIsNil(t *testing.T) {
	// stdin and other in-memory inputs leave f.FS == nil. The rule
	// must short-circuit so it does not scan whatever git repo
	// happens to be the process working directory.
	r := &Rule{}
	f := &lint.File{
		Path:          "<stdin>",
		Source:        []byte("# Test\n"),
		MaxInputBytes: 1048576,
	}
	assert.Empty(t, r.Check(f))
}

func TestRule_Check_NotInGitRepo(t *testing.T) {
	dir := t.TempDir()
	// No git init - the directory is not inside a git repository.

	r := &Rule{}
	f := &lint.File{
		Path:          filepath.Join(dir, "README.md"),
		Source:        []byte("# Test\n"),
		MaxInputBytes: 1048576,
		FS:            os.DirFS(dir),
	}

	diags := r.Check(f)
	assert.Empty(t, diags)
}

func TestRule_Check_HooksInSync(t *testing.T) {
	dir := t.TempDir()
	initRepoWithDriver(t, dir)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"),
		[]byte("# Test\n\n<?catalog?>\n<?/catalog?>\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "PLAN.md"),
		[]byte("# Plan\n\n<?include file=\"test.md\"?><?/include?>\n"), 0o644))

	// .gitattributes assigns both files to the mdsmith merge driver.
	gitattrs := "PLAN.md merge=mdsmith\nREADME.md merge=mdsmith\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitattributes"),
		[]byte(gitattrs), 0o644))

	// pre-merge-commit hook lists both files.
	hooksDir := githooks.ResolveHooksDir(dir)
	require.NoError(t, os.MkdirAll(hooksDir, 0o755))
	hookContent := "#!/bin/sh\n" + githooks.PreMergeCommitMarker + "\n" +
		"mdsmith fix -- 'PLAN.md'\ngit add -- 'PLAN.md'\n" +
		"mdsmith fix -- 'README.md'\ngit add -- 'README.md'\n"
	require.NoError(t, os.WriteFile(filepath.Join(hooksDir, "pre-merge-commit"),
		[]byte(hookContent), 0o755))

	r := &Rule{}
	f := &lint.File{
		Path:          filepath.Join(dir, "README.md"),
		Source:        []byte("# Test\n\n<?catalog?>\n<?/catalog?>\n"),
		MaxInputBytes: 1048576,
		FS:            os.DirFS(dir),
	}

	diags := r.Check(f)
	assert.Empty(t, diags, "Should have no diagnostics when hooks are in sync")
}

func TestRule_Check_PreMergeCommitOutOfSync(t *testing.T) {
	dir := t.TempDir()
	initRepoWithDriver(t, dir)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "test.md"),
		[]byte("# Test\n\n<?catalog?>\nlist\n<?/catalog?>\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"),
		[]byte("# README\n\n<?include file=\"test.md\"?><?/include?>\n"), 0o644))

	// .gitattributes is in sync with discovery so only the hook drifts.
	gitattrs := "README.md merge=mdsmith\ntest.md merge=mdsmith\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitattributes"),
		[]byte(gitattrs), 0o644))

	hooksDir := githooks.ResolveHooksDir(dir)
	require.NoError(t, os.MkdirAll(hooksDir, 0o755))
	hookContent := "#!/bin/sh\n" + githooks.PreMergeCommitMarker + "\n" +
		"mdsmith fix -- 'README.md'\ngit add -- 'README.md'\n"
	require.NoError(t, os.WriteFile(filepath.Join(hooksDir, "pre-merge-commit"),
		[]byte(hookContent), 0o755))

	r := &Rule{}
	f := &lint.File{
		Path:          filepath.Join(dir, "README.md"),
		Source:        []byte("# README\n\n<?include file=\"test.md\"?><?/include?>\n"),
		MaxInputBytes: 1048576,
		FS:            os.DirFS(dir),
	}

	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "pre-merge-commit hook is out of sync")
	assert.Contains(t, diags[0].Message, "has: README.md")
	assert.Contains(t, diags[0].Message, "should have:")
}

func TestRule_Check_GitattributesOutOfSync(t *testing.T) {
	dir := t.TempDir()
	initRepoWithDriver(t, dir)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "test.md"),
		[]byte("# Test\n\n<?catalog?>\nlist\n<?/catalog?>\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"),
		[]byte("# README\n\n<?include file=\"test.md\"?><?/include?>\n"), 0o644))

	// .gitattributes only lists README.md but discovery will find both.
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitattributes"),
		[]byte("README.md merge=mdsmith\n"), 0o644))

	r := &Rule{}
	f := &lint.File{
		Path:          filepath.Join(dir, "README.md"),
		Source:        []byte("# README\n\n<?include file=\"test.md\"?><?/include?>\n"),
		MaxInputBytes: 1048576,
		FS:            os.DirFS(dir),
	}

	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "merge-driver assignments in .gitattributes are out of sync")
	assert.Contains(t, diags[0].Message, "has: README.md")
}

func TestRule_Check_DriverRegisteredButNoGitattributes(t *testing.T) {
	dir := t.TempDir()
	initRepoWithDriver(t, dir)

	// File with a directive exists but .gitattributes has no
	// merge=mdsmith entries, which means the registered driver will
	// not run for any file. The rule should warn about that.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"),
		[]byte("# Test\n\n<?catalog?>\n<?/catalog?>\n"), 0o644))

	r := &Rule{}
	f := &lint.File{
		Path:          filepath.Join(dir, "README.md"),
		Source:        []byte("# Test\n\n<?catalog?>\n<?/catalog?>\n"),
		MaxInputBytes: 1048576,
		FS:            os.DirFS(dir),
	}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message,
		"merge.mdsmith.driver is registered but .gitattributes has no merge=mdsmith entries")
}

func TestRule_Check_HookWithoutMdsmithMarker(t *testing.T) {
	dir := t.TempDir()
	initRepoWithDriver(t, dir)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"),
		[]byte("# Test\n\n<?catalog?>\n<?/catalog?>\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitattributes"),
		[]byte("README.md merge=mdsmith\n"), 0o644))

	// User-authored hook lacking the mdsmith marker — must be ignored.
	hooksDir := githooks.ResolveHooksDir(dir)
	require.NoError(t, os.MkdirAll(hooksDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(hooksDir, "pre-merge-commit"),
		[]byte("#!/bin/sh\necho user hook\n"), 0o755))

	r := &Rule{}
	f := &lint.File{
		Path:          filepath.Join(dir, "README.md"),
		Source:        []byte("# Test\n\n<?catalog?>\n<?/catalog?>\n"),
		MaxInputBytes: 1048576,
		FS:            os.DirFS(dir),
	}
	assert.Empty(t, r.Check(f))
}

func TestRule_Check_CombinesBothDriftSourcesIntoOneDiagnostic(t *testing.T) {
	dir := t.TempDir()
	initRepoWithDriver(t, dir)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"),
		[]byte("# Test\n\n<?catalog?>\n<?/catalog?>\n"), 0o644))
	// Both sources drift simultaneously: .gitattributes lists a
	// different file and the hook lists yet another file.
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitattributes"),
		[]byte("OTHER.md merge=mdsmith\n"), 0o644))
	hooksDir := githooks.ResolveHooksDir(dir)
	require.NoError(t, os.MkdirAll(hooksDir, 0o755))
	hookContent := "#!/bin/sh\n" + githooks.PreMergeCommitMarker + "\n" +
		"mdsmith fix -- 'STALE.md'\n"
	require.NoError(t, os.WriteFile(filepath.Join(hooksDir, "pre-merge-commit"),
		[]byte(hookContent), 0o755))

	r := &Rule{}
	f := &lint.File{
		Path:          filepath.Join(dir, "README.md"),
		Source:        []byte("# Test\n"),
		MaxInputBytes: 1048576,
		FS:            os.DirFS(dir),
	}
	diags := r.Check(f)
	require.Len(t, diags, 1, "rule must emit at most one diagnostic per repo")
	assert.Contains(t, diags[0].Message, "merge-driver assignments in .gitattributes")
	assert.Contains(t, diags[0].Message, "pre-merge-commit hook is out of sync")
}

func TestRule_Check_HookListsNoFilesRendersNone(t *testing.T) {
	dir := t.TempDir()
	initTestRepo(t, dir)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"),
		[]byte("# Test\n\n<?catalog?>\n<?/catalog?>\n"), 0o644))

	// Hook bears the mdsmith marker but has no `fix --` lines, so
	// ExtractHookFiles returns an empty slice. The drift message
	// should render `(none)` rather than a blank list.
	hooksDir := githooks.ResolveHooksDir(dir)
	require.NoError(t, os.MkdirAll(hooksDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(hooksDir, "pre-merge-commit"),
		[]byte("#!/bin/sh\n"+githooks.PreMergeCommitMarker+"\n"), 0o755))

	r := &Rule{}
	f := &lint.File{
		Path:          filepath.Join(dir, "README.md"),
		Source:        []byte("# Test\n"),
		MaxInputBytes: 1048576,
		FS:            os.DirFS(dir),
	}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "has: (none)")
}

func TestRule_Check_GitattributesListsFilesButRepoHasNoDirectives(t *testing.T) {
	// Driver registered + .gitattributes lists a file + repo has no
	// files with directives → mergeDriverDrift renders
	// `should have: (none)`.
	dir := t.TempDir()
	initRepoWithDriver(t, dir)

	// A markdown file without any directive: discovery returns empty.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "plain.md"),
		[]byte("# Plain\nno directives here\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitattributes"),
		[]byte("stale.md merge=mdsmith\n"), 0o644))

	r := &Rule{}
	f := &lint.File{
		Path:          filepath.Join(dir, "plain.md"),
		Source:        []byte("# Plain\n"),
		MaxInputBytes: 1048576,
		FS:            os.DirFS(dir),
	}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "has: stale.md")
	assert.Contains(t, diags[0].Message, "should have: (none)")
}

func TestRule_Check_HookListsFilesButRepoHasNoDirectives(t *testing.T) {
	// Hook lists a file + repo has no files with directives →
	// preMergeCommitHookDrift renders `should have: (none)`.
	dir := t.TempDir()
	initTestRepo(t, dir)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "plain.md"),
		[]byte("# Plain\n"), 0o644))

	hooksDir := githooks.ResolveHooksDir(dir)
	require.NoError(t, os.MkdirAll(hooksDir, 0o755))
	hookContent := "#!/bin/sh\n" + githooks.PreMergeCommitMarker + "\n" +
		"mdsmith fix -- 'stale.md' && git add -- 'stale.md'\n"
	require.NoError(t, os.WriteFile(
		filepath.Join(hooksDir, "pre-merge-commit"), []byte(hookContent), 0o755))

	r := &Rule{}
	f := &lint.File{
		Path:          filepath.Join(dir, "plain.md"),
		Source:        []byte("# Plain\n"),
		MaxInputBytes: 1048576,
		FS:            os.DirFS(dir),
	}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "has: stale.md")
	assert.Contains(t, diags[0].Message, "should have: (none)")
}

func TestRule_ResolveRepoRootIsCached(t *testing.T) {
	// resolveRepoRoot must memoise the GitRepoRoot lookup so per-file
	// Check/Fix calls don't respawn `git rev-parse` for every linted
	// file. We verify by deleting the .git directory between calls
	// and asserting the cached value is still returned.
	dir := t.TempDir()
	initTestRepo(t, dir)

	r := &Rule{}
	repoRootMu.Lock()
	delete(repoRootCache, dir)
	repoRootMu.Unlock()

	first, err := r.resolveRepoRoot(dir)
	require.NoError(t, err)
	require.NotEmpty(t, first)

	// Remove .git so a fresh GitRepoRoot lookup would now fail.
	require.NoError(t, os.RemoveAll(filepath.Join(dir, ".git")))

	second, err := r.resolveRepoRoot(dir)
	assert.NoError(t, err,
		"cached lookup must succeed even after .git is removed")
	assert.Equal(t, first, second,
		"second call must return the cached repo root, not re-run git")
}

func TestRule_GetDiscoveredCacheHit(t *testing.T) {
	// Two consecutive Check() calls in the same repo must hit the
	// discoveredCache rather than re-scan. We verify by deleting the
	// repo's directive-bearing file between calls and confirming the
	// rule still observes the cached discovery (i.e. drift logic
	// continues to see the file as discovered).
	dir := t.TempDir()
	initRepoWithDriver(t, dir)

	mdFile := filepath.Join(dir, "test.md")
	require.NoError(t, os.WriteFile(mdFile,
		[]byte("# Test\n\n<?catalog?>\n<?/catalog?>\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitattributes"),
		[]byte("# BEGIN mdsmith merge-driver\ntest.md merge=mdsmith\n# END mdsmith merge-driver\n"), 0o644))

	r := &Rule{}
	// Prime the cache via a Check call.
	_ = r.Check(&lint.File{
		Path:          mdFile,
		Source:        []byte("# Test\n"),
		MaxInputBytes: 1048576,
		FS:            os.DirFS(dir),
	})

	// Direct getDiscovered call: must return the cached slice without
	// invoking DiscoverFiles again. We assert it returns the original
	// file list even though the file was deleted, proving the cache
	// hit branch is exercised.
	require.NoError(t, os.Remove(mdFile))
	got := r.getDiscovered(dir, 1048576)
	assert.Equal(t, []string{"test.md"}, got,
		"second call must return cached discovery, not rescan")
}

func TestRule_GetDiscoveredCacheKeyIncludesMaxBytes(t *testing.T) {
	// DiscoverFiles uses maxBytes when reading each candidate file.
	// A different limit can change which files qualify, so the cache
	// must key on (repoRoot, maxBytes), not repoRoot alone — otherwise
	// the second caller would receive a slice computed under the
	// first caller's limit and could miss real drift.
	dir := t.TempDir()
	initRepoWithDriver(t, dir)

	// File contains the directive token only beyond byte 50.
	mdFile := filepath.Join(dir, "deep.md")
	pad := strings.Repeat("x", 200)
	content := "# Test\n" + pad + "\n<?catalog?>\n<?/catalog?>\n"
	require.NoError(t, os.WriteFile(mdFile, []byte(content), 0o644))

	r := &Rule{}

	// With a tight read budget, DiscoverFiles cannot see the
	// directive: deep.md is not discovered.
	gotSmall := r.getDiscovered(dir, 50)
	assert.Empty(t, gotSmall,
		"with a tight maxBytes budget the directive past byte 50 is not visible")

	// With a generous budget, the same repo MUST be re-scanned and
	// deep.md must show up. If the cache keyed only on repoRoot, the
	// previous empty result would be reused.
	gotLarge := r.getDiscovered(dir, 1<<20)
	assert.Equal(t, []string{"deep.md"}, gotLarge,
		"a different maxBytes must trigger rediscovery, not reuse the small-budget cache")
}

func TestRule_Check_HookReadErrorIsReported(t *testing.T) {
	dir := t.TempDir()
	initTestRepo(t, dir)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"),
		[]byte("# Test\n\n<?catalog?>\n<?/catalog?>\n"), 0o644))

	// Make hookPath a directory so os.ReadFile returns a non-ENOENT
	// error. The rule should surface that as drift instead of
	// silently passing.
	hooksDir := githooks.ResolveHooksDir(dir)
	require.NoError(t, os.MkdirAll(filepath.Join(hooksDir, "pre-merge-commit"), 0o755))

	r := &Rule{}
	f := &lint.File{
		Path:          filepath.Join(dir, "README.md"),
		Source:        []byte("# Test\n"),
		MaxInputBytes: 1048576,
		FS:            os.DirFS(dir),
	}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message,
		"cannot verify pre-merge-commit hook")
}

func TestRule_Check_UnreadableHookSkipsDiscovery(t *testing.T) {
	// When the merge driver is not registered and the hook file is
	// merely unreadable, Check emits an IO diagnostic without ever
	// needing the discovered set. Discovery (a repo-wide walk) must
	// not run in that path. We verify by leaving the cache primed
	// only after the first eager call would have populated it: a
	// post-Check getDiscovered observation finds the cache empty,
	// proving Check didn't trigger discovery.
	dir := t.TempDir()
	initTestRepo(t, dir)

	// Markdown file that, if discovered, would carry a directive.
	mdPath := filepath.Join(dir, "README.md")
	require.NoError(t, os.WriteFile(mdPath,
		[]byte("# Test\n\n<?catalog?>\n<?/catalog?>\n"), 0o644))

	// Make the hook unreadable (a directory at the hook path).
	hooksDir := githooks.ResolveHooksDir(dir)
	require.NoError(t, os.MkdirAll(filepath.Join(hooksDir, "pre-merge-commit"), 0o755))

	r := &Rule{}
	f := &lint.File{
		Path:          mdPath,
		Source:        []byte("# Test\n"),
		MaxInputBytes: 1048576,
		FS:            os.DirFS(dir),
	}

	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message,
		"cannot verify pre-merge-commit hook")

	// Probe the cache: if Check had triggered discovery, the cache
	// would already hold an entry for (dir, MaxInputBytes). Since
	// the unreadable-only path must skip discovery, the cache is
	// empty and only this probe populates it.
	discoveredMu.Lock()
	_, populated := discoveredCache[fmt.Sprintf("%s:%d", dir, int64(1048576))]
	discoveredMu.Unlock()
	assert.False(t, populated,
		"Check must not run the repo-wide DiscoverFiles walk when only the unreadable-hook IO diagnostic can fire")
}

func TestRule_Check_GitattributesReadErrorIsReported(t *testing.T) {
	dir := t.TempDir()
	initRepoWithDriver(t, dir)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"),
		[]byte("# Test\n\n<?catalog?>\n<?/catalog?>\n"), 0o644))

	// Make .gitattributes a directory so reading it returns a
	// non-ENOENT error. With merge.mdsmith.driver registered, the
	// rule must surface that as drift.
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".gitattributes"), 0o755))

	r := &Rule{}
	f := &lint.File{
		Path:          filepath.Join(dir, "README.md"),
		Source:        []byte("# Test\n"),
		MaxInputBytes: 1048576,
		FS:            os.DirFS(dir),
	}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message,
		"cannot verify merge-driver assignments")
}

func TestRule_Check_ReportsConsistentlyAcrossClones(t *testing.T) {
	// The engine clones the rule per file when configured with a
	// settings mapping. Each clone must observe drift independently:
	// suppressing duplicate diagnostics here would prevent the fixer
	// pipeline (which calls Check before deciding whether to run Fix)
	// from triggering Fix on subsequent files.
	dir := t.TempDir()
	initRepoWithDriver(t, dir)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "test.md"),
		[]byte("# Test\n\n<?catalog?>\n<?/catalog?>\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"),
		[]byte("# README\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitattributes"),
		[]byte("README.md merge=mdsmith\n"), 0o644))

	clone1 := rule.CloneRule(&Rule{}).(*Rule)
	clone2 := rule.CloneRule(&Rule{}).(*Rule)

	f1 := &lint.File{
		Path:          filepath.Join(dir, "README.md"),
		Source:        []byte("# README\n"),
		MaxInputBytes: 1048576,
		FS:            os.DirFS(dir),
	}
	f2 := &lint.File{
		Path:          filepath.Join(dir, "test.md"),
		Source:        []byte("# Test\n"),
		MaxInputBytes: 1048576,
		FS:            os.DirFS(dir),
	}

	diags1 := clone1.Check(f1)
	diags2 := clone2.Check(f2)
	assert.Len(t, diags1, 1, "first clone reports drift")
	assert.Len(t, diags2, 1, "second clone also reports so Fix can trigger")
}

func TestRule_Check_ReportsForEachFileWhileDriftExists(t *testing.T) {
	// The fixer pipeline calls Check before each fix pass. Until
	// Fix runs and brings .gitattributes in sync, Check must keep
	// returning a diagnostic for every file in the repo.
	dir := t.TempDir()
	initRepoWithDriver(t, dir)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "test.md"),
		[]byte("# Test\n\n<?catalog?>\n<?/catalog?>\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"),
		[]byte("# README\n\n<?include file=\"x.md\"?><?/include?>\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitattributes"),
		[]byte("README.md merge=mdsmith\n"), 0o644))

	r := &Rule{}
	f1 := &lint.File{
		Path:          filepath.Join(dir, "README.md"),
		Source:        []byte("# README\n"),
		MaxInputBytes: 1048576,
		FS:            os.DirFS(dir),
	}
	f2 := &lint.File{
		Path:          filepath.Join(dir, "test.md"),
		Source:        []byte("# Test\n"),
		MaxInputBytes: 1048576,
		FS:            os.DirFS(dir),
	}

	diags1 := r.Check(f1)
	diags2 := r.Check(f2)
	assert.Len(t, diags1, 1, "first file reports drift")
	assert.Len(t, diags2, 1, "second file also reports so Fix can trigger per-file")
}

func TestRule_Metadata(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, "MDS048", r.ID())
	assert.Equal(t, "git-hook-sync", r.Name())
	assert.Equal(t, "meta", r.Category())
	assert.False(t, r.EnabledByDefault())
	assert.Equal(t, map[string]any{}, r.DefaultSettings())
}

func TestRule_ApplySettings(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{})
	assert.NoError(t, err)
}

func TestRule_Check_RunnableInZeroState(t *testing.T) {
	// The rule must run when enabled via the bool form
	// (`git-hook-sync: true`), which doesn't call ApplySettings.
	dir := t.TempDir()
	initTestRepo(t, dir)

	r := &Rule{}
	f := &lint.File{
		Path:          filepath.Join(dir, "README.md"),
		Source:        []byte("# Test\n"),
		MaxInputBytes: 1048576,
		FS:            os.DirFS(dir),
	}
	// No diagnostics yet (no .gitattributes / no hook installed),
	// but the call must reach the body of Check rather than bailing
	// on a "not configured" gate.
	assert.Empty(t, r.Check(f))
}

func TestRule_ApplySettings_UnknownSetting(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"unknown": "value"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown setting")
}

func TestRule_Fix_SkipsWhenFSIsNil(t *testing.T) {
	r := &Rule{}
	f := &lint.File{
		FS:     nil,
		Path:   "<stdin>",
		Source: []byte("# Test\n"),
	}
	result := r.Fix(f)
	assert.Equal(t, f.Source, result)
}

func TestRule_Fix_SkipsWhenNotInGitRepo(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "test.md")
	require.NoError(t, os.WriteFile(mdFile, []byte("# Test\n"), 0644))

	r := &Rule{}
	f := &lint.File{
		FS:            os.DirFS(dir),
		Path:          mdFile,
		Source:        []byte("# Test\n"),
		MaxInputBytes: 10000,
	}

	result := r.Fix(f)
	assert.Equal(t, f.Source, result)
}

func TestRule_Fix_SkipsWhenDriverNotRegistered(t *testing.T) {
	dir := t.TempDir()
	initTestRepo(t, dir)

	mdFile := filepath.Join(dir, "test.md")
	require.NoError(t, os.WriteFile(mdFile, []byte("# Test\n"), 0644))

	r := &Rule{}
	f := &lint.File{
		FS:            os.DirFS(dir),
		Path:          mdFile,
		Source:        []byte("# Test\n"),
		MaxInputBytes: 10000,
	}

	result := r.Fix(f)
	assert.Equal(t, f.Source, result)
}

func TestRule_Fix_RetriesAfterDriverBecomesRegistered(t *testing.T) {
	// An early Fix call (before the merge driver is registered)
	// returns without writing. A later call, once the driver is
	// registered, must still be able to detect drift and write —
	// nothing in the rule should poison repo-level state on an
	// early-returned call.
	dir := t.TempDir()
	initTestRepo(t, dir)

	mdFile := filepath.Join(dir, "test.md")
	mdContent := "# Test\n<?catalog glob=\"*.md\"?>\n<?/catalog?>\n"
	require.NoError(t, os.WriteFile(mdFile, []byte(mdContent), 0644))

	r := &Rule{}
	f := &lint.File{
		FS:            os.DirFS(dir),
		Path:          mdFile,
		Source:        []byte(mdContent),
		MaxInputBytes: 10000,
	}

	// Fix #1: driver not registered → early return, no markFixed.
	r.Fix(f)
	_, statErr := os.Stat(filepath.Join(dir, ".gitattributes"))
	require.True(t, os.IsNotExist(statErr),
		"Fix must not write .gitattributes when the driver is not registered")

	// Now register the merge driver.
	cmd := exec.Command(
		"git", "-C", dir, "config", "--local",
		"merge.mdsmith.driver", "mdsmith merge-driver %O %A %B %P",
	)
	require.NoError(t, cmd.Run())

	// Fix #2: driver is registered, drift exists, Fix must write.
	r.Fix(f)
	content, err := os.ReadFile(filepath.Join(dir, ".gitattributes"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "test.md merge=mdsmith",
		"Fix must write .gitattributes once the driver is registered, even after a prior no-op call")
}

func TestRule_Fix_RegeneratesGitattributes(t *testing.T) {
	dir := t.TempDir()
	initTestRepo(t, dir)

	// Register merge driver
	cmd := exec.Command(
		"git", "-C", dir, "config", "--local",
		"merge.mdsmith.driver", "mdsmith merge-driver %O %A %B %P",
	)
	require.NoError(t, cmd.Run())

	// Create markdown file with directive
	mdFile := filepath.Join(dir, "test.md")
	mdContent := "# Test\n<?catalog glob=\"*.md\"?>\n<?/catalog?>\n"
	require.NoError(t, os.WriteFile(mdFile, []byte(mdContent), 0644))

	// Create .gitattributes with wrong content
	attrPath := filepath.Join(dir, ".gitattributes")
	initial := "# BEGIN mdsmith merge-driver\nold.md merge=mdsmith\n# END mdsmith merge-driver\n"
	require.NoError(t, os.WriteFile(attrPath, []byte(initial), 0644))

	r := &Rule{}
	f := &lint.File{
		FS:            os.DirFS(dir),
		Path:          mdFile,
		Source:        []byte(mdContent),
		MaxInputBytes: 10000,
	}

	result := r.Fix(f)
	assert.Equal(t, f.Source, result) // Fix doesn't change the markdown file

	// Verify .gitattributes was updated
	content, err := os.ReadFile(attrPath)
	require.NoError(t, err)

	expected := "# BEGIN mdsmith merge-driver\ntest.md merge=mdsmith\n# END mdsmith merge-driver\n"
	assert.Equal(t, expected, string(content))
}

func TestRule_Fix_StagesGitattributes(t *testing.T) {
	// The pre-merge-commit hook flow stages only the markdown file
	// passed to `mdsmith fix`. To make sure regenerated .gitattributes
	// also lands in the merge commit, Fix runs `git add -- .gitattributes`
	// after writing.
	dir := t.TempDir()
	initTestRepo(t, dir)

	cmd := exec.Command(
		"git", "-C", dir, "config", "--local",
		"merge.mdsmith.driver", "mdsmith merge-driver %O %A %B %P",
	)
	require.NoError(t, cmd.Run())
	mdFile := filepath.Join(dir, "test.md")
	mdContent := "# Test\n<?catalog glob=\"*.md\"?>\n<?/catalog?>\n"
	require.NoError(t, os.WriteFile(mdFile, []byte(mdContent), 0644))

	// Pre-existing .gitattributes that is out of sync.
	attrPath := filepath.Join(dir, ".gitattributes")
	initial := "# BEGIN mdsmith merge-driver\nold.md merge=mdsmith\n# END mdsmith merge-driver\n"
	require.NoError(t, os.WriteFile(attrPath, []byte(initial), 0644))

	r := &Rule{}
	f := &lint.File{
		FS:            os.DirFS(dir),
		Path:          mdFile,
		Source:        []byte(mdContent),
		MaxInputBytes: 10000,
	}
	r.Fix(f)

	// `git ls-files --stage` lists the index contents. After Fix, the
	// regenerated .gitattributes must be present in the index.
	staged, err := exec.Command(
		"git", "-C", dir, "ls-files", "--stage", "--", ".gitattributes",
	).Output()
	require.NoError(t, err)
	assert.Contains(t, string(staged), ".gitattributes",
		"Fix must stage the regenerated .gitattributes")
}

func TestRule_Fix_RetriesAfterTransientWriteFailure(t *testing.T) {
	// A failed WriteGitattributes leaves no lasting state, so the
	// next Fix call within the same process can retry. Once the
	// underlying problem clears, the second call must write.
	dir := t.TempDir()
	initTestRepo(t, dir)

	cmd := exec.Command(
		"git", "-C", dir, "config", "--local",
		"merge.mdsmith.driver", "mdsmith merge-driver %O %A %B %P",
	)
	require.NoError(t, cmd.Run())

	mdFile := filepath.Join(dir, "test.md")
	mdContent := "# Test\n<?catalog glob=\"*.md\"?>\n<?/catalog?>\n"
	require.NoError(t, os.WriteFile(mdFile, []byte(mdContent), 0644))

	attrPath := filepath.Join(dir, ".gitattributes")
	// Make .gitattributes a directory so the first WriteGitattributes
	// call fails. Discovery still runs, drift is detected, but the
	// write returns an error.
	require.NoError(t, os.Mkdir(attrPath, 0755))

	r := &Rule{}
	f := &lint.File{
		FS:            os.DirFS(dir),
		Path:          mdFile,
		Source:        []byte(mdContent),
		MaxInputBytes: 10000,
	}
	r.Fix(f)
	info, err := os.Stat(attrPath)
	require.NoError(t, err)
	require.True(t, info.IsDir(), "first call: write must fail (path is a directory)")

	// Replace the directory with a regular (out-of-sync) file so the
	// next write can succeed.
	require.NoError(t, os.Remove(attrPath))
	initial := "# BEGIN mdsmith merge-driver\nold.md merge=mdsmith\n# END mdsmith merge-driver\n"
	require.NoError(t, os.WriteFile(attrPath, []byte(initial), 0644))

	// Second Fix must retry: the prior failure must not have locked
	// fixedRepos[dir] = true.
	r.Fix(f)
	content, err := os.ReadFile(attrPath)
	require.NoError(t, err)
	expected := "# BEGIN mdsmith merge-driver\ntest.md merge=mdsmith\n# END mdsmith merge-driver\n"
	assert.Equal(t, expected, string(content),
		"Fix must retry after a prior write failure within the same process")
}

// setupStagingFailureRepo sets up a repo whose `git add` fails because
// .git/index.lock is held. Returns the dir, the markdown file path, and
// the lock-file path so the caller can release it to retry.
func setupStagingFailureRepo(t *testing.T) (dir, mdFile, lockPath string) {
	t.Helper()
	dir = t.TempDir()
	require.NoError(t, exec.Command("git", "init", dir).Run())
	require.NoError(t, exec.Command(
		"git", "-C", dir, "config", "core.hooksPath",
		filepath.Join(dir, ".git", "hooks"),
	).Run())
	require.NoError(t, exec.Command(
		"git", "-C", dir, "config", "--local",
		"merge.mdsmith.driver", "mdsmith merge-driver run %O %A %B %P",
	).Run())

	mdFile = filepath.Join(dir, "test.md")
	mdContent := "# Test\n<?catalog glob=\"*.md\"?>\n<?/catalog?>\n"
	require.NoError(t, os.WriteFile(mdFile, []byte(mdContent), 0o644))

	attrPath := filepath.Join(dir, ".gitattributes")
	initial := "# BEGIN mdsmith merge-driver\nold.md merge=mdsmith\n# END mdsmith merge-driver\n"
	require.NoError(t, os.WriteFile(attrPath, []byte(initial), 0o644))

	// Holding index.lock makes git refuse to modify the index.
	lockPath = filepath.Join(dir, ".git", "index.lock")
	require.NoError(t, os.WriteFile(lockPath, nil, 0o644))
	t.Cleanup(func() {
		stagingMu.Lock()
		delete(stagingErrors, dir)
		stagingMu.Unlock()
	})
	return dir, mdFile, lockPath
}

func TestRule_StagingFailure_SurfacedAndRetried(t *testing.T) {
	// A failed `git add -- .gitattributes` (simulated via index.lock)
	// must (1) be recorded so Check keeps reporting it even though
	// the on-disk file is in sync and (2) be cleared by a subsequent
	// Fix call once staging can succeed.
	dir, mdFile, lockPath := setupStagingFailureRepo(t)
	mdContent := "# Test\n<?catalog glob=\"*.md\"?>\n<?/catalog?>\n"

	r := &Rule{}
	f := &lint.File{
		FS:            os.DirFS(dir),
		Path:          mdFile,
		Source:        []byte(mdContent),
		MaxInputBytes: 10000,
	}

	r.Fix(f)
	require.Error(t, stagingError(dir),
		"failed staging must be recorded so it can be surfaced and retried")

	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "git add` failed",
		"Check must report a pending staging failure")

	// Release the lock; the next Fix call must retry and succeed.
	require.NoError(t, os.Remove(lockPath))
	r.Fix(f)
	assert.NoError(t, stagingError(dir),
		"successful staging must clear the recorded error")

	staged, err := exec.Command(
		"git", "-C", dir, "ls-files", "--stage", "--", ".gitattributes",
	).Output()
	require.NoError(t, err)
	assert.Contains(t, string(staged), ".gitattributes",
		"retry must actually stage .gitattributes")

	assert.Empty(t, r.Check(f),
		"Check must stop reporting once staging succeeds and drift is gone")
}

func TestRule_Fix_DoesNotReWriteWhenAlreadyInSync(t *testing.T) {
	// After a successful first fix, a second Fix call in the same
	// process must not redundantly re-write .gitattributes. The
	// FilesMatch short-circuit should detect that the on-disk state
	// already matches the discovered set and return without writing
	// or re-staging.
	dir := t.TempDir()
	initTestRepo(t, dir)

	cmd := exec.Command(
		"git", "-C", dir, "config", "--local",
		"merge.mdsmith.driver", "mdsmith merge-driver %O %A %B %P",
	)
	require.NoError(t, cmd.Run())

	mdFile := filepath.Join(dir, "test.md")
	mdContent := "# Test\n<?catalog glob=\"*.md\"?>\n<?/catalog?>\n"
	require.NoError(t, os.WriteFile(mdFile, []byte(mdContent), 0644))

	attrPath := filepath.Join(dir, ".gitattributes")
	initial := "# BEGIN mdsmith merge-driver\nold.md merge=mdsmith\n# END mdsmith merge-driver\n"
	require.NoError(t, os.WriteFile(attrPath, []byte(initial), 0644))

	r := &Rule{}
	f := &lint.File{
		FS:            os.DirFS(dir),
		Path:          mdFile,
		Source:        []byte(mdContent),
		MaxInputBytes: 10000,
	}

	r.Fix(f)
	// Capture mtime after the first write.
	infoBefore, err := os.Stat(attrPath)
	require.NoError(t, err)
	mtimeBefore := infoBefore.ModTime()

	// Second call: .gitattributes is already in sync so FilesMatch
	// returns true and Fix returns before the writer runs.
	r.Fix(f)
	infoAfter, err := os.Stat(attrPath)
	require.NoError(t, err)
	assert.Equal(t, mtimeBefore, infoAfter.ModTime(),
		"second Fix call must not re-write when .gitattributes is already in sync")
}

func TestRule_Fix_RewritesAndStagesWhenDriftReappears(t *testing.T) {
	// If something corrupts .gitattributes after a first successful
	// fix (e.g. a stray write, a different MaxInputBytes triggers
	// rediscovery, or a sibling tool overwrites it), the next Fix
	// call must rewrite *and* stage the corrected content. Without
	// this guarantee the working tree could be repaired while the
	// index keeps pointing at the stale version.
	dir := t.TempDir()
	initTestRepo(t, dir)

	cmd := exec.Command(
		"git", "-C", dir, "config", "--local",
		"merge.mdsmith.driver", "mdsmith merge-driver %O %A %B %P",
	)
	require.NoError(t, cmd.Run())

	mdFile := filepath.Join(dir, "test.md")
	mdContent := "# Test\n<?catalog glob=\"*.md\"?>\n<?/catalog?>\n"
	require.NoError(t, os.WriteFile(mdFile, []byte(mdContent), 0644))

	attrPath := filepath.Join(dir, ".gitattributes")
	initial := "# BEGIN mdsmith merge-driver\nold.md merge=mdsmith\n# END mdsmith merge-driver\n"
	require.NoError(t, os.WriteFile(attrPath, []byte(initial), 0644))

	r := &Rule{}
	f := &lint.File{
		FS:            os.DirFS(dir),
		Path:          mdFile,
		Source:        []byte(mdContent),
		MaxInputBytes: 10000,
	}

	// First call: writes test.md and stages.
	r.Fix(f)
	stagedFirst, err := exec.Command(
		"git", "-C", dir, "ls-files", "--stage", "--", ".gitattributes",
	).Output()
	require.NoError(t, err)
	require.Contains(t, string(stagedFirst), ".gitattributes")

	// Simulate external corruption — index now disagrees with the
	// working tree's intended (test.md) content.
	require.NoError(t, os.WriteFile(attrPath, []byte(initial), 0644))

	// Second call must observe the drift, rewrite, and re-stage.
	r.Fix(f)

	expected := "# BEGIN mdsmith merge-driver\ntest.md merge=mdsmith\n# END mdsmith merge-driver\n"
	got, err := os.ReadFile(attrPath)
	require.NoError(t, err)
	assert.Equal(t, expected, string(got),
		"a second Fix call must rewrite .gitattributes when drift reappears")

	// The index must hold the regenerated content. Read the staged
	// blob via ls-files / cat-file and assert it matches the rewrite.
	assert.Contains(t, stagedGitattributesBlob(t, dir), "test.md merge=mdsmith",
		"rewrite must also be staged so the index matches the working tree")
}

// stagedGitattributesBlob returns the contents of the .gitattributes
// blob currently in the index, fetched via `git ls-files -s` plus
// `git cat-file -p`. Tests use it to assert the index matches the
// working tree without depending on a HEAD commit.
func stagedGitattributesBlob(t *testing.T, dir string) string {
	t.Helper()
	staged, err := exec.Command(
		"git", "-C", dir, "ls-files", "-s", "--", ".gitattributes",
	).Output()
	require.NoError(t, err)
	fields := strings.Fields(string(staged))
	require.GreaterOrEqual(t, len(fields), 2,
		"`git ls-files -s` must report a staged blob hash")
	blob, err := exec.Command(
		"git", "-C", dir, "cat-file", "-p", fields[1],
	).Output()
	require.NoError(t, err)
	return string(blob)
}

func TestRule_Fix_SkipsWhenAlreadyInSync(t *testing.T) {
	dir := t.TempDir()
	initTestRepo(t, dir)

	// Register merge driver
	cmd := exec.Command(
		"git", "-C", dir, "config", "--local",
		"merge.mdsmith.driver", "mdsmith merge-driver %O %A %B %P",
	)
	require.NoError(t, cmd.Run())

	// Create markdown file with directive
	mdFile := filepath.Join(dir, "test.md")
	mdContent := "# Test\n<?catalog glob=\"*.md\"?>\n<?/catalog?>\n"
	require.NoError(t, os.WriteFile(mdFile, []byte(mdContent), 0644))

	// Create .gitattributes with correct content
	attrPath := filepath.Join(dir, ".gitattributes")
	correct := "# BEGIN mdsmith merge-driver\ntest.md merge=mdsmith\n# END mdsmith merge-driver\n"
	require.NoError(t, os.WriteFile(attrPath, []byte(correct), 0644))

	// Record modification time
	info, err := os.Stat(attrPath)
	require.NoError(t, err)
	modTimeBefore := info.ModTime()

	r := &Rule{}
	f := &lint.File{
		FS:            os.DirFS(dir),
		Path:          mdFile,
		Source:        []byte(mdContent),
		MaxInputBytes: 10000,
	}

	r.Fix(f)

	// Verify .gitattributes wasn't modified
	info, err = os.Stat(attrPath)
	require.NoError(t, err)
	assert.Equal(t, modTimeBefore, info.ModTime())
}

func TestRule_Fix_ReturnsOriginalOnWriteError(t *testing.T) {
	dir := t.TempDir()
	initTestRepo(t, dir)

	// Register merge driver
	cmd := exec.Command(
		"git", "-C", dir, "config", "--local",
		"merge.mdsmith.driver", "mdsmith merge-driver %O %A %B %P",
	)
	require.NoError(t, cmd.Run())

	// Create markdown file with directive
	mdFile := filepath.Join(dir, "test.md")
	mdContent := "# Test\n<?catalog glob=\"*.md\"?>\n<?/catalog?>\n"
	require.NoError(t, os.WriteFile(mdFile, []byte(mdContent), 0644))

	// Create .gitattributes as a directory (cannot write to it)
	attrPath := filepath.Join(dir, ".gitattributes")
	require.NoError(t, os.Mkdir(attrPath, 0755))

	r := &Rule{}
	f := &lint.File{
		FS:            os.DirFS(dir),
		Path:          mdFile,
		Source:        []byte(mdContent),
		MaxInputBytes: 10000,
	}

	result := r.Fix(f)
	// Should return original source when write fails
	assert.Equal(t, f.Source, result)
}
