package githooksync

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/jeduden/mdsmith/internal/githooks"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	// Register the directive-bearing rules so anything that walks
	// fixtures with directive markers continues to work.
	_ "github.com/jeduden/mdsmith/internal/rules/catalog"
	_ "github.com/jeduden/mdsmith/internal/rules/include"
	_ "github.com/jeduden/mdsmith/internal/rules/toc"
)

// canonicalManagedBlock returns the .gitattributes managed block that
// the rule expects when no .mdsmith.yml is present (default include
// patterns, no exclusions).
func canonicalManagedBlock() string {
	g, _ := githooks.GlobsFromConfig(nil)
	return githooks.RenderManagedBlock(g)
}

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

// installCanonicalHook writes a canonical glob-based hook to dir's
// pre-merge-commit path so drift detection passes.
func installCanonicalHook(t *testing.T, dir string) {
	t.Helper()
	hooksDir := githooks.ResolveHooksDir(dir)
	require.NoError(t, os.MkdirAll(hooksDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(hooksDir, "pre-merge-commit"),
		[]byte(githooks.BuildHookScript("/usr/local/bin/mdsmith")),
		0o755,
	))
}

func TestRule_Check_SkipsWhenNoSourceOptedIn(t *testing.T) {
	// Neither the merge driver nor an mdsmith-managed
	// pre-merge-commit hook is installed. The rule must emit no
	// diagnostics.
	dir := t.TempDir()
	initTestRepo(t, dir)

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
	r := &Rule{}
	f := &lint.File{
		Path:          filepath.Join(dir, "README.md"),
		Source:        []byte("# Test\n"),
		MaxInputBytes: 1048576,
		FS:            os.DirFS(dir),
	}
	assert.Empty(t, r.Check(f))
}

func TestRule_Check_HooksInSync(t *testing.T) {
	dir := t.TempDir()
	initRepoWithDriver(t, dir)

	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitattributes"),
		[]byte(canonicalManagedBlock()), 0o644))
	installCanonicalHook(t, dir)

	r := &Rule{}
	f := &lint.File{
		Path:          filepath.Join(dir, "README.md"),
		Source:        []byte("# Test\n"),
		MaxInputBytes: 1048576,
		FS:            os.DirFS(dir),
	}
	assert.Empty(t, r.Check(f), "no diagnostics when both managed artifacts are canonical")
}

func TestRule_Check_HonorsConfigIgnorePatterns(t *testing.T) {
	// .mdsmith.yml ignore patterns become exclude lines in the
	// managed block.
	dir := t.TempDir()
	initRepoWithDriver(t, dir)

	require.NoError(t, os.WriteFile(filepath.Join(dir, ".mdsmith.yml"),
		[]byte("ignore:\n  - \"demo/**\"\n  - \"vendor/**\"\n"), 0o644))

	expectedBlock := githooks.RenderManagedBlock(githooks.Globs{
		Include: githooks.DefaultIncludes(),
		Exclude: []string{"demo/**", "vendor/**"},
	})
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitattributes"),
		[]byte(expectedBlock), 0o644))

	r := &Rule{}
	f := &lint.File{
		Path:          filepath.Join(dir, "README.md"),
		Source:        []byte("# Test\n"),
		MaxInputBytes: 1048576,
		FS:            os.DirFS(dir),
	}
	assert.Empty(t, r.Check(f),
		"managed block built from cfg.Ignore should be in sync")
}

func TestRule_Check_GitattributesOutOfSync(t *testing.T) {
	dir := t.TempDir()
	initRepoWithDriver(t, dir)

	// Old explicit-files content rather than the canonical glob block.
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitattributes"),
		[]byte("# BEGIN mdsmith merge-driver\nREADME.md merge=mdsmith\n# END mdsmith merge-driver\n"),
		0o644))

	r := &Rule{}
	f := &lint.File{
		Path:          filepath.Join(dir, "README.md"),
		Source:        []byte("# README\n"),
		MaxInputBytes: 1048576,
		FS:            os.DirFS(dir),
	}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message,
		".gitattributes managed block is out of sync")
	assert.Contains(t, diags[0].Message, "has include: README.md")
	assert.Contains(t, diags[0].Message, "should have include: *.md, *.markdown")
}

func TestRule_Check_DriverRegisteredButNoManagedBlock(t *testing.T) {
	dir := t.TempDir()
	initRepoWithDriver(t, dir)

	// .gitattributes exists but has no managed block at all.
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitattributes"),
		[]byte("*.txt text eol=lf\n"), 0o644))

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
		"merge.mdsmith.driver is registered but .gitattributes has no managed block")
}

func TestRule_Check_HookOutOfSync(t *testing.T) {
	dir := t.TempDir()
	initTestRepo(t, dir)

	// Old per-file hook content carrying our marker. Detected as
	// drift because it does not match the glob-based canonical
	// template.
	hooksDir := githooks.ResolveHooksDir(dir)
	require.NoError(t, os.MkdirAll(hooksDir, 0o755))
	hookContent := "#!/bin/sh\n" + githooks.PreMergeCommitMarker + "\n" +
		"set -e\nmdsmith fix -- 'PLAN.md'\ngit add -- 'PLAN.md'\n"
	require.NoError(t, os.WriteFile(
		filepath.Join(hooksDir, "pre-merge-commit"),
		[]byte(hookContent), 0o755,
	))

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
		"pre-merge-commit hook is out of sync with the glob-based template")
}

func TestRule_Check_HookWithoutMdsmithMarker(t *testing.T) {
	// User-authored hook lacking the mdsmith marker is ignored.
	dir := t.TempDir()
	initRepoWithDriver(t, dir)

	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitattributes"),
		[]byte(canonicalManagedBlock()), 0o644))

	hooksDir := githooks.ResolveHooksDir(dir)
	require.NoError(t, os.MkdirAll(hooksDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(hooksDir, "pre-merge-commit"),
		[]byte("#!/bin/sh\necho user hook\n"), 0o755))

	r := &Rule{}
	f := &lint.File{
		Path:          filepath.Join(dir, "README.md"),
		Source:        []byte("# Test\n"),
		MaxInputBytes: 1048576,
		FS:            os.DirFS(dir),
	}
	assert.Empty(t, r.Check(f))
}

func TestRule_Check_CombinesBothDriftSourcesIntoOneDiagnostic(t *testing.T) {
	dir := t.TempDir()
	initRepoWithDriver(t, dir)

	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitattributes"),
		[]byte("# BEGIN mdsmith merge-driver\nOTHER.md merge=mdsmith\n# END mdsmith merge-driver\n"),
		0o644))
	hooksDir := githooks.ResolveHooksDir(dir)
	require.NoError(t, os.MkdirAll(hooksDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(hooksDir, "pre-merge-commit"),
		[]byte("#!/bin/sh\n"+githooks.PreMergeCommitMarker+"\nmdsmith fix -- 'STALE.md'\n"),
		0o755))

	r := &Rule{}
	f := &lint.File{
		Path:          filepath.Join(dir, "README.md"),
		Source:        []byte("# Test\n"),
		MaxInputBytes: 1048576,
		FS:            os.DirFS(dir),
	}
	diags := r.Check(f)
	require.Len(t, diags, 1, "rule must emit at most one diagnostic per file")
	assert.Contains(t, diags[0].Message, ".gitattributes managed block is out of sync")
	assert.Contains(t, diags[0].Message, "pre-merge-commit hook is out of sync")
}

func TestRule_Check_HookReadErrorIsReported(t *testing.T) {
	dir := t.TempDir()
	initTestRepo(t, dir)

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

	require.NoError(t, os.RemoveAll(filepath.Join(dir, ".git")))

	second, err := r.resolveRepoRoot(dir)
	assert.NoError(t, err,
		"cached lookup must succeed even after .git is removed")
	assert.Equal(t, first, second,
		"second call must return the cached repo root, not re-run git")
}

func TestRule_Check_AnchorsDiagnosticToGitattributesPath(t *testing.T) {
	// The diagnostic File field points at the .gitattributes
	// artifact, not the markdown file the engine happened to be
	// linting. Drift is a repo-level concern, and anchoring the
	// report at the artifact lets downstream tooling collapse
	// duplicate reports by (file, line, ruleID).
	dir := t.TempDir()
	initRepoWithDriver(t, dir)

	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitattributes"),
		[]byte("# BEGIN mdsmith merge-driver\nREADME.md merge=mdsmith\n# END mdsmith merge-driver\n"),
		0o644))

	r := &Rule{}
	f := &lint.File{
		Path:          filepath.Join(dir, "README.md"),
		Source:        []byte("# README\n"),
		MaxInputBytes: 1048576,
		FS:            os.DirFS(dir),
	}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, filepath.Join(dir, ".gitattributes"), diags[0].File,
		"diagnostic should point at the artifact, not the linted markdown")
}

func TestRule_Metadata(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, "MDS048", r.ID())
	assert.Equal(t, "git-hook-sync", r.Name())
	assert.Equal(t, "structural", r.Category())
	assert.False(t, r.EnabledByDefault())
	assert.Equal(t, map[string]any{}, r.DefaultSettings())
}

func TestRule_ApplySettings(t *testing.T) {
	r := &Rule{}
	assert.NoError(t, r.ApplySettings(map[string]any{}))
}

func TestRule_ApplySettings_UnknownSetting(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"unknown": "value"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown setting")
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
	assert.Empty(t, r.Check(f))
}

func TestRule_Fix_SkipsWhenFSIsNil(t *testing.T) {
	r := &Rule{}
	f := &lint.File{FS: nil, Path: "<stdin>", Source: []byte("# Test\n")}
	assert.Equal(t, f.Source, r.Fix(f))
}

func TestRule_Fix_SkipsWhenNotInGitRepo(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "test.md")
	require.NoError(t, os.WriteFile(mdFile, []byte("# Test\n"), 0o644))

	r := &Rule{}
	f := &lint.File{
		FS:            os.DirFS(dir),
		Path:          mdFile,
		Source:        []byte("# Test\n"),
		MaxInputBytes: 10000,
	}
	assert.Equal(t, f.Source, r.Fix(f))
}

func TestRule_Fix_SkipsWhenDriverNotRegistered(t *testing.T) {
	dir := t.TempDir()
	initTestRepo(t, dir)

	mdFile := filepath.Join(dir, "test.md")
	require.NoError(t, os.WriteFile(mdFile, []byte("# Test\n"), 0o644))

	r := &Rule{}
	f := &lint.File{
		FS:            os.DirFS(dir),
		Path:          mdFile,
		Source:        []byte("# Test\n"),
		MaxInputBytes: 10000,
	}
	assert.Equal(t, f.Source, r.Fix(f))
}

func TestRule_Fix_RetriesAfterDriverBecomesRegistered(t *testing.T) {
	// An early Fix call (before the merge driver is registered)
	// returns without writing. A later call, once the driver is
	// registered, must still detect drift and write.
	dir := t.TempDir()
	initTestRepo(t, dir)

	mdFile := filepath.Join(dir, "test.md")
	require.NoError(t, os.WriteFile(mdFile, []byte("# Test\n"), 0o644))

	r := &Rule{}
	f := &lint.File{
		FS:            os.DirFS(dir),
		Path:          mdFile,
		Source:        []byte("# Test\n"),
		MaxInputBytes: 10000,
	}

	r.Fix(f)
	_, statErr := os.Stat(filepath.Join(dir, ".gitattributes"))
	require.True(t, os.IsNotExist(statErr),
		"Fix must not write .gitattributes when the driver is not registered")

	require.NoError(t, exec.Command(
		"git", "-C", dir, "config", "--local",
		"merge.mdsmith.driver", "mdsmith merge-driver %O %A %B %P",
	).Run())

	r.Fix(f)
	content, err := os.ReadFile(filepath.Join(dir, ".gitattributes"))
	require.NoError(t, err)
	assert.Equal(t, canonicalManagedBlock(), string(content),
		"Fix must write the canonical glob block once the driver is registered")
}

func TestRule_Fix_RegeneratesGitattributes(t *testing.T) {
	dir := t.TempDir()
	initRepoWithDriver(t, dir)

	mdFile := filepath.Join(dir, "test.md")
	require.NoError(t, os.WriteFile(mdFile, []byte("# Test\n"), 0o644))

	attrPath := filepath.Join(dir, ".gitattributes")
	initial := "# BEGIN mdsmith merge-driver\nold.md merge=mdsmith\n# END mdsmith merge-driver\n"
	require.NoError(t, os.WriteFile(attrPath, []byte(initial), 0o644))

	r := &Rule{}
	f := &lint.File{
		FS:            os.DirFS(dir),
		Path:          mdFile,
		Source:        []byte("# Test\n"),
		MaxInputBytes: 10000,
	}

	assert.Equal(t, f.Source, r.Fix(f),
		"Fix returns the markdown source unchanged; the fix is in .gitattributes")

	content, err := os.ReadFile(attrPath)
	require.NoError(t, err)
	assert.Equal(t, canonicalManagedBlock(), string(content))
}

func TestRule_Fix_DryRunSkipsGitattributesWrite(t *testing.T) {
	dir := t.TempDir()
	initRepoWithDriver(t, dir)

	mdFile := filepath.Join(dir, "test.md")
	require.NoError(t, os.WriteFile(mdFile, []byte("# Test\n"), 0o644))

	attrPath := filepath.Join(dir, ".gitattributes")
	initial := "# BEGIN mdsmith merge-driver\nold.md merge=mdsmith\n# END mdsmith merge-driver\n"
	require.NoError(t, os.WriteFile(attrPath, []byte(initial), 0o644))

	r := &Rule{}
	f := &lint.File{
		FS:            os.DirFS(dir),
		Path:          mdFile,
		Source:        []byte("# Test\n"),
		MaxInputBytes: 10000,
		DryRun:        true,
	}

	assert.Equal(t, f.Source, r.Fix(f),
		"Fix returns the markdown source unchanged in dry-run too")

	content, err := os.ReadFile(attrPath)
	require.NoError(t, err)
	assert.Equal(t, initial, string(content),
		".gitattributes must be untouched during dry-run")
}

func TestRule_Fix_EncodesConfigIgnoreAsExcludes(t *testing.T) {
	dir := t.TempDir()
	initRepoWithDriver(t, dir)

	require.NoError(t, os.WriteFile(filepath.Join(dir, ".mdsmith.yml"),
		[]byte("ignore:\n  - \"demo/**\"\n  - \"vendor/**\"\n"), 0o644))

	mdFile := filepath.Join(dir, "test.md")
	require.NoError(t, os.WriteFile(mdFile, []byte("# Test\n"), 0o644))

	r := &Rule{}
	f := &lint.File{
		FS:            os.DirFS(dir),
		Path:          mdFile,
		Source:        []byte("# Test\n"),
		MaxInputBytes: 10000,
	}
	r.Fix(f)

	content, err := os.ReadFile(filepath.Join(dir, ".gitattributes"))
	require.NoError(t, err)
	expected := githooks.RenderManagedBlock(githooks.Globs{
		Include: githooks.DefaultIncludes(),
		Exclude: []string{"demo/**", "vendor/**"},
	})
	assert.Equal(t, expected, string(content))
}

func TestRule_Fix_StagesGitattributes(t *testing.T) {
	// The pre-merge-commit hook flow stages only the markdown file
	// passed to `mdsmith fix`. To make sure regenerated .gitattributes
	// also lands in the merge commit, Fix runs `git add -- .gitattributes`
	// after writing.
	dir := t.TempDir()
	initRepoWithDriver(t, dir)

	mdFile := filepath.Join(dir, "test.md")
	require.NoError(t, os.WriteFile(mdFile, []byte("# Test\n"), 0o644))

	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitattributes"),
		[]byte("# BEGIN mdsmith merge-driver\nold.md merge=mdsmith\n# END mdsmith merge-driver\n"),
		0o644))

	r := &Rule{}
	f := &lint.File{
		FS:            os.DirFS(dir),
		Path:          mdFile,
		Source:        []byte("# Test\n"),
		MaxInputBytes: 10000,
	}
	r.Fix(f)

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
	initRepoWithDriver(t, dir)

	mdFile := filepath.Join(dir, "test.md")
	require.NoError(t, os.WriteFile(mdFile, []byte("# Test\n"), 0o644))

	attrPath := filepath.Join(dir, ".gitattributes")
	require.NoError(t, os.Mkdir(attrPath, 0o755))

	r := &Rule{}
	f := &lint.File{
		FS:            os.DirFS(dir),
		Path:          mdFile,
		Source:        []byte("# Test\n"),
		MaxInputBytes: 10000,
	}
	r.Fix(f)
	info, err := os.Stat(attrPath)
	require.NoError(t, err)
	require.True(t, info.IsDir(), "first call: write must fail (path is a directory)")

	require.NoError(t, os.Remove(attrPath))
	initial := "# BEGIN mdsmith merge-driver\nold.md merge=mdsmith\n# END mdsmith merge-driver\n"
	require.NoError(t, os.WriteFile(attrPath, []byte(initial), 0o644))

	r.Fix(f)
	content, err := os.ReadFile(attrPath)
	require.NoError(t, err)
	assert.Equal(t, canonicalManagedBlock(), string(content),
		"Fix must retry after a prior write failure within the same process")
}

// setupStagingFailureRepo sets up a repo whose `git add` fails because
// .git/index.lock is held. Returns the dir, the markdown file path,
// and the lock-file path so the caller can release it to retry.
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
	require.NoError(t, os.WriteFile(mdFile, []byte("# Test\n"), 0o644))

	attrPath := filepath.Join(dir, ".gitattributes")
	initial := "# BEGIN mdsmith merge-driver\nold.md merge=mdsmith\n# END mdsmith merge-driver\n"
	require.NoError(t, os.WriteFile(attrPath, []byte(initial), 0o644))

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

	r := &Rule{}
	f := &lint.File{
		FS:            os.DirFS(dir),
		Path:          mdFile,
		Source:        []byte("# Test\n"),
		MaxInputBytes: 10000,
	}

	r.Fix(f)
	require.Error(t, stagingError(dir),
		"failed staging must be recorded so it can be surfaced and retried")

	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "git add` failed",
		"Check must report a pending staging failure")

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
	dir := t.TempDir()
	initRepoWithDriver(t, dir)

	mdFile := filepath.Join(dir, "test.md")
	require.NoError(t, os.WriteFile(mdFile, []byte("# Test\n"), 0o644))

	attrPath := filepath.Join(dir, ".gitattributes")
	require.NoError(t, os.WriteFile(attrPath, []byte(canonicalManagedBlock()), 0o644))

	infoBefore, err := os.Stat(attrPath)
	require.NoError(t, err)

	r := &Rule{}
	f := &lint.File{
		FS:            os.DirFS(dir),
		Path:          mdFile,
		Source:        []byte("# Test\n"),
		MaxInputBytes: 10000,
	}
	r.Fix(f)

	infoAfter, err := os.Stat(attrPath)
	require.NoError(t, err)
	assert.Equal(t, infoBefore.ModTime(), infoAfter.ModTime(),
		"Fix must not re-write when .gitattributes is already in sync")
}

func TestRule_Fix_ReturnsOriginalOnWriteError(t *testing.T) {
	dir := t.TempDir()
	initRepoWithDriver(t, dir)

	mdFile := filepath.Join(dir, "test.md")
	require.NoError(t, os.WriteFile(mdFile, []byte("# Test\n"), 0o644))

	attrPath := filepath.Join(dir, ".gitattributes")
	require.NoError(t, os.Mkdir(attrPath, 0o755))

	r := &Rule{}
	f := &lint.File{
		FS:            os.DirFS(dir),
		Path:          mdFile,
		Source:        []byte("# Test\n"),
		MaxInputBytes: 10000,
	}
	assert.Equal(t, f.Source, r.Fix(f))
}

func TestRule_ResolveRepoRoot_NotInRepo(t *testing.T) {
	// A directory outside any git repo must produce a cached error
	// entry so subsequent lookups skip the subprocess.
	dir := t.TempDir()
	r := &Rule{}
	repoRootMu.Lock()
	delete(repoRootCache, dir)
	repoRootMu.Unlock()

	_, err := r.resolveRepoRoot(dir)
	require.Error(t, err)

	repoRootMu.Lock()
	entry, ok := repoRootCache[dir]
	repoRootMu.Unlock()
	require.True(t, ok, "non-repo lookup must populate the cache")
	assert.Error(t, entry.err)

	// Second lookup hits the cached error branch.
	_, err = r.resolveRepoRoot(dir)
	assert.Error(t, err)
}

func TestRule_Check_CachesDriftPartsPerRepo(t *testing.T) {
	// Drift inspection is expensive (git config subprocess, hook
	// read, .mdsmith.yml parse). The rule caches the result per
	// repo so per-file Check calls don't repeat it. We probe the
	// cache directly to confirm a Check call populates it and a
	// second call reuses it.
	dir := t.TempDir()
	initRepoWithDriver(t, dir)

	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitattributes"),
		[]byte(canonicalManagedBlock()), 0o644))

	driftMu.Lock()
	delete(driftCache, dir)
	driftMu.Unlock()

	r := &Rule{}
	f := &lint.File{
		Path:          filepath.Join(dir, "README.md"),
		Source:        []byte("# Test\n"),
		MaxInputBytes: 1048576,
		FS:            os.DirFS(dir),
	}
	r.Check(f)

	driftMu.Lock()
	_, populated := driftCache[dir]
	driftMu.Unlock()
	assert.True(t, populated, "first Check call must populate the drift cache")

	// Make .gitattributes drift on disk; the cached entry should
	// suppress the new computation, so Check still returns no diag.
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitattributes"),
		[]byte("# BEGIN mdsmith merge-driver\nstale.md merge=mdsmith\n# END mdsmith merge-driver\n"),
		0o644))
	assert.Empty(t, r.Check(f),
		"cached in-sync result is reused even when on-disk state changes")
}

func TestRule_Fix_InvalidatesDriftCache(t *testing.T) {
	// Fix changes the on-disk state, so the cached drift entry is
	// invalidated. Subsequent Check calls re-read the repo.
	dir := t.TempDir()
	initRepoWithDriver(t, dir)

	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitattributes"),
		[]byte("# BEGIN mdsmith merge-driver\nold.md merge=mdsmith\n# END mdsmith merge-driver\n"),
		0o644))

	driftMu.Lock()
	delete(driftCache, dir)
	driftMu.Unlock()

	r := &Rule{}
	f := &lint.File{
		Path:          filepath.Join(dir, "test.md"),
		Source:        []byte("# Test\n"),
		MaxInputBytes: 1048576,
		FS:            os.DirFS(dir),
	}
	// First Check populates the cache with the drift entry.
	require.Len(t, r.Check(f), 1)
	driftMu.Lock()
	_, populated := driftCache[dir]
	driftMu.Unlock()
	require.True(t, populated)

	// Fix runs and clears the cached entry.
	r.Fix(f)
	driftMu.Lock()
	_, stillPopulated := driftCache[dir]
	driftMu.Unlock()
	assert.False(t, stillPopulated, "Fix must invalidate the drift cache")
}

func TestRule_RepoScopedDiagnostics(t *testing.T) {
	assert.True(t, (&Rule{}).RepoScopedDiagnostics(),
		"git-hook-sync must declare itself repo-scoped: its diagnostics "+
			"are anchored to .gitattributes, not the linted host file")
}
