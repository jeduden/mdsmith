package githooksync

import (
	"os"
	"os/exec"
	"path/filepath"
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

func TestRule_Check_OncePerRepoAcrossClones(t *testing.T) {
	// Simulate the engine's clone-per-file path: when the rule is
	// enabled with a settings mapping (even an empty {}), the engine
	// clones the rule per file. The "at most one diagnostic per
	// repo" guarantee must still hold across clones.
	dir := t.TempDir()
	initRepoWithDriver(t, dir)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "test.md"),
		[]byte("# Test\n\n<?catalog?>\n<?/catalog?>\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"),
		[]byte("# README\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitattributes"),
		[]byte("README.md merge=mdsmith\n"), 0o644))

	t.Cleanup(resetReportedForTest)
	resetReportedForTest()

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
	assert.Len(t, diags1, 1, "first clone reports drift once")
	assert.Empty(t, diags2,
		"second clone in the same repo must not duplicate the diagnostic")
}

func TestRule_Check_OncePerRepo(t *testing.T) {
	dir := t.TempDir()
	initRepoWithDriver(t, dir)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "test.md"),
		[]byte("# Test\n\n<?catalog?>\n<?/catalog?>\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"),
		[]byte("# README\n\n<?include file=\"x.md\"?><?/include?>\n"), 0o644))
	// .gitattributes lists only README.md, so drift is real.
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitattributes"),
		[]byte("README.md merge=mdsmith\n"), 0o644))

	t.Cleanup(resetReportedForTest)
	resetReportedForTest()

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
	assert.Len(t, diags1, 1, "first file should report drift")
	assert.Empty(t, diags2, "second file in same repo should not duplicate the diagnostic")
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
