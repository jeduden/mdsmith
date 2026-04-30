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
// dir/.git/hooks in the dir-local config. The pin keeps tests
// hermetic: a developer with a non-default core.hooksPath set
// globally cannot have the rule (or its tests) read or write to
// files outside the temp dir.
func initTestRepo(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, exec.Command("git", "init", dir).Run())
	require.NoError(t, exec.Command(
		"git", "-C", dir, "config", "core.hooksPath",
		filepath.Join(dir, ".git", "hooks"),
	).Run())
}

// initRepoWithDriver initialises a git dir at dir, pins
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
	// the dir-wide discovery walk and must emit no diagnostics.
	dir := t.TempDir()
	initTestRepo(t, dir)

	// Put a directive file in the dir to confirm the rule does
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
	// must short-circuit so it does not scan whatever git dir
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
	require.Len(t, diags, 1, "rule must emit at most one diagnostic per dir")
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
	// dir" guarantee must still hold across clones.
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
	assert.Empty(t, diags2, "second clone in the same dir does not report (once-per-repo guard)")
}

func TestRule_Check_MultipleFilesReportDrift(t *testing.T) {
	dir := t.TempDir()
	initRepoWithDriver(t, dir)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "test.md"),
		[]byte("# Test\n\n<?catalog?>\n<?/catalog?>\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"),
		[]byte("# README\n\n<?include file=\"x.md\"?><?/include?>\n"), 0o644))
	// .gitattributes lists only README.md, so drift is real.
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
	assert.Empty(t, diags2, "second file does not report (once-per-repo guard)")
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

func TestRule_Fix_RegeneratesGitattributes(t *testing.T) {
	dir := t.TempDir()
	initTestRepo(t, dir)

	// Register merge driver
	cmd := exec.Command("git", "-C", dir, "config", "--local", "merge.mdsmith.driver", "mdsmith merge-driver %O %A %B %P")
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

func TestRule_Fix_OnlyFixesOncePerRepo(t *testing.T) {
	dir := t.TempDir()
	initTestRepo(t, dir)

	// Register merge driver
	cmd := exec.Command("git", "-C", dir, "config", "--local", "merge.mdsmith.driver", "mdsmith merge-driver %O %A %B %P")
	require.NoError(t, cmd.Run())

	// Create two markdown files with directives
	mdFile1 := filepath.Join(dir, "a.md")
	mdFile2 := filepath.Join(dir, "b.md")
	mdContent := "# Test\n<?catalog glob=\"*.md\"?>\n<?/catalog?>\n"
	require.NoError(t, os.WriteFile(mdFile1, []byte(mdContent), 0644))
	require.NoError(t, os.WriteFile(mdFile2, []byte(mdContent), 0644))

	// Create .gitattributes with wrong content
	attrPath := filepath.Join(dir, ".gitattributes")
	initial := "# BEGIN mdsmith merge-driver\nold.md merge=mdsmith\n# END mdsmith merge-driver\n"
	require.NoError(t, os.WriteFile(attrPath, []byte(initial), 0644))

	r := &Rule{}
	f1 := &lint.File{
		FS:            os.DirFS(dir),
		Path:          mdFile1,
		Source:        []byte(mdContent),
		MaxInputBytes: 10000,
	}
	f2 := &lint.File{
		FS:            os.DirFS(dir),
		Path:          mdFile2,
		Source:        []byte(mdContent),
		MaxInputBytes: 10000,
	}

	// First fix should update .gitattributes
	r.Fix(f1)
	content1, err := os.ReadFile(attrPath)
	require.NoError(t, err)
	expected := "# BEGIN mdsmith merge-driver\na.md merge=mdsmith\nb.md merge=mdsmith\n# END mdsmith merge-driver\n"
	assert.Equal(t, expected, string(content1))

	// Corrupt .gitattributes again
	require.NoError(t, os.WriteFile(attrPath, []byte(initial), 0644))

	// Second fix should NOT update it (once per dir)
	r.Fix(f2)
	content2, err := os.ReadFile(attrPath)
	require.NoError(t, err)
	assert.Equal(t, initial, string(content2)) // Should still be corrupted
}

func TestRule_Fix_SkipsWhenAlreadyInSync(t *testing.T) {
	dir := t.TempDir()
	initTestRepo(t, dir)

	// Register merge driver
	cmd := exec.Command("git", "-C", dir, "config", "--local", "merge.mdsmith.driver", "mdsmith merge-driver %O %A %B %P")
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
	cmd := exec.Command("git", "-C", dir, "config", "--local", "merge.mdsmith.driver", "mdsmith merge-driver %O %A %B %P")
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
