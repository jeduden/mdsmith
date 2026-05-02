package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jeduden/mdsmith/internal/githooks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRun_DispatchesPreMergeCommit covers the `pre-merge-commit`
// case in main.go's run() dispatch.
func TestRun_DispatchesPreMergeCommit(t *testing.T) {
	origArgs := os.Args
	t.Cleanup(func() { os.Args = origArgs })
	os.Args = []string{"mdsmith", "pre-merge-commit", "--help"}
	captureStderr(func() {
		assert.Equal(t, 0, run())
	})
}

// chdirToNonRepo changes the working directory to a fresh temp dir
// that is not inside any git repository, so commands under test
// exercise their "not in a git repo" branch.
func chdirToNonRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	// Plant an empty .git so `git rev-parse --show-toplevel` fails.
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".git"), []byte("not a real gitdir"), 0o644))
	origWd, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })
	return dir
}

// initTestRepo runs `git init` on dir and pins core.hooksPath to
// dir/.git/hooks in the repo-local config. The pin makes the test
// hermetic: a developer with a non-default core.hooksPath set
// globally cannot have install/uninstall/status commands write into
// or remove files outside the temp repo.
func initTestRepo(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, exec.Command("git", "init", dir).Run())
	hooksDir := filepath.Join(dir, ".git", "hooks")
	require.NoError(t, exec.Command(
		"git", "-C", dir, "config", "core.hooksPath", hooksDir,
	).Run())
}

// --- runPreMergeCommit dispatch ---

func TestRunPreMergeCommit_DispatchInstall(t *testing.T) {
	chdirToNonRepo(t)
	captureStderr(func() {
		// install dispatched but bails out on "not in git repo".
		assert.Equal(t, 2, runPreMergeCommit([]string{"install"}))
	})
}

func TestRunPreMergeCommit_DispatchUninstall(t *testing.T) {
	chdirToNonRepo(t)
	captureStderr(func() {
		assert.Equal(t, 2, runPreMergeCommit([]string{"uninstall"}))
	})
}

func TestRunPreMergeCommit_DispatchStatus(t *testing.T) {
	chdirToNonRepo(t)
	captureStderr(func() {
		assert.Equal(t, 2, runPreMergeCommit([]string{"status"}))
	})
}

func TestRunPreMergeCommitInstall_NotInRepo(t *testing.T) {
	chdirToNonRepo(t)
	got := captureStderr(func() {
		assert.Equal(t, 2, runPreMergeCommitInstall(nil))
	})
	assert.Contains(t, got, "not in a git repository")
}

func TestRunPreMergeCommitUninstall_NotInRepo(t *testing.T) {
	chdirToNonRepo(t)
	got := captureStderr(func() {
		assert.Equal(t, 2, runPreMergeCommitUninstall(nil))
	})
	assert.Contains(t, got, "not in a git repository")
}

func TestRunPreMergeCommitStatus_NotInRepo(t *testing.T) {
	chdirToNonRepo(t)
	got := captureStderr(func() {
		assert.Equal(t, 2, runPreMergeCommitStatus(nil))
	})
	assert.Contains(t, got, "not in a git repository")
}

// TestRunPreMergeCommitUninstall_ReadError makes hookPath a directory
// so os.ReadFile returns a non-IsNotExist error, exercising the
// read-error branch.
func TestRunPreMergeCommitUninstall_ReadError(t *testing.T) {
	dir := t.TempDir()
	initTestRepo(t, dir)
	hooksDir := resolveHooksDir(dir)
	require.NoError(t, os.MkdirAll(filepath.Join(hooksDir, "pre-merge-commit"), 0o755))

	origWd, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	got := captureStderr(func() {
		assert.Equal(t, 2, runPreMergeCommitUninstall(nil))
	})
	assert.Contains(t, got, "reading hook")
}

// TestRunPreMergeCommitStatus_ReadError exercises the same branch in
// the status command.
func TestRunPreMergeCommitStatus_ReadError(t *testing.T) {
	dir := t.TempDir()
	initTestRepo(t, dir)
	hooksDir := resolveHooksDir(dir)
	require.NoError(t, os.MkdirAll(filepath.Join(hooksDir, "pre-merge-commit"), 0o755))

	origWd, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	got := captureStderr(func() {
		assert.Equal(t, 2, runPreMergeCommitStatus(nil))
	})
	assert.Contains(t, got, "reading hook")
}

// TestRunPreMergeCommitUninstall_RemoveError creates an
// mdsmith-marked hook file, then drops write permission from the
// hooks directory so os.Remove fails with EACCES. This exercises
// the remove-error branch in runPreMergeCommitUninstall.
//
// The denial is permission-based, not ENOTEMPTY: as root, chmod is
// bypassed entirely, so the test self-skips when a probe rename
// against the locked directory still succeeds.
func TestRunPreMergeCommitUninstall_RemoveError(t *testing.T) {
	dir := t.TempDir()
	initTestRepo(t, dir)
	hooksDir := resolveHooksDir(dir)
	hookPath := filepath.Join(hooksDir, "pre-merge-commit")
	require.NoError(t, os.MkdirAll(hooksDir, 0o755))
	require.NoError(t, os.WriteFile(hookPath,
		[]byte("#!/bin/sh\n"+preMergeCommitHookMarker+"\n"), 0o755))

	require.NoError(t, os.Chmod(hooksDir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(hooksDir, 0o755) })

	if removeWillSucceed(hookPath) {
		t.Skip("filesystem permissions did not block remove (likely running as root); skip remove-error branch")
	}

	origWd, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	got := captureStderr(func() {
		assert.Equal(t, 2, runPreMergeCommitUninstall(nil))
	})
	assert.Contains(t, got, "removing hook")
}

// removeWillSucceed reports whether the current process can remove
// path. Used by tests that need to skip when root-mode bypasses the
// chmod-based denial they rely on.
func removeWillSucceed(path string) bool {
	tmp := path + ".probe"
	if err := os.Rename(path, tmp); err != nil {
		return false
	}
	_ = os.Rename(tmp, path)
	return true
}

func TestRunPreMergeCommitUninstall_HookNotPresent(t *testing.T) {
	dir := t.TempDir()
	initTestRepo(t, dir)
	origWd, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	got := captureStderr(func() {
		assert.Equal(t, 0, runPreMergeCommitUninstall(nil))
	})
	assert.Contains(t, got, "no pre-merge-commit hook found")
}

func TestPreMergeCommitInstall_RejectsAnyExplicitArgs(t *testing.T) {
	// The glob-based hook does not embed a per-file list, so explicit
	// args are no longer meaningful. The install command must reject
	// them with a clear hint to edit `.mdsmith.yml` instead.
	dir := t.TempDir()
	initTestRepo(t, dir)

	origWd, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	got := captureStderr(func() {
		assert.Equal(t, 2, runPreMergeCommitInstall([]string{"bad name.md"}))
	})
	assert.Contains(t, got, "no longer accepts explicit files")
}

func TestPreMergeCommitInstall_RefusesUserHook(t *testing.T) {
	dir := t.TempDir()
	initTestRepo(t, dir)

	orig := executableFunc
	t.Cleanup(func() { executableFunc = orig })
	executableFunc = func() (string, error) { return "/usr/local/bin/mdsmith", nil }

	hooksDir := resolveHooksDir(dir)
	require.NoError(t, os.MkdirAll(hooksDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(hooksDir, "pre-merge-commit"),
		[]byte("#!/bin/sh\necho user hook\n"), 0o755))

	origWd, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	got := captureStderr(func() {
		assert.Equal(t, 2, runPreMergeCommitInstall(nil))
	})
	assert.Contains(t, got, "installing pre-merge-commit hook")
}

func TestPreMergeCommitInstall_NoArgsInstallsCanonicalHook(t *testing.T) {
	dir := t.TempDir()
	initTestRepo(t, dir)

	orig := executableFunc
	t.Cleanup(func() { executableFunc = orig })
	executableFunc = func() (string, error) { return "/usr/local/bin/mdsmith", nil }

	origWd, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	got := captureStderr(func() {
		assert.Equal(t, 0, runPreMergeCommitInstall(nil))
	})
	assert.Contains(t, got, "pre-merge-commit hook installed")
	hookData, err := os.ReadFile(filepath.Join(resolveHooksDir(dir), "pre-merge-commit"))
	require.NoError(t, err)
	assert.Contains(t, string(hookData), "fix .; then",
		"installed hook must use the glob-based template")
}

func TestPreMergeCommitStatus_UnmanagedHook(t *testing.T) {
	dir := t.TempDir()
	initTestRepo(t, dir)

	hooksDir := resolveHooksDir(dir)
	require.NoError(t, os.MkdirAll(hooksDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(hooksDir, "pre-merge-commit"),
		[]byte("#!/bin/sh\necho user hook\n"), 0o755))

	origWd, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	got := captureStderr(func() {
		assert.Equal(t, 0, runPreMergeCommitStatus(nil))
	})
	assert.Contains(t, got, "managed by: user")
	assert.Contains(t, got, "not installed by mdsmith")
}

func TestRunPreMergeCommit_NoArgs_ExitsZero(t *testing.T) {
	captureStderr(func() {
		code := runPreMergeCommit(nil)
		assert.Equal(t, 0, code)
	})
}

func TestRunPreMergeCommit_HelpLong_ExitsZero(t *testing.T) {
	captureStderr(func() {
		code := runPreMergeCommit([]string{"--help"})
		assert.Equal(t, 0, code)
	})
}

func TestRunPreMergeCommit_HelpShort_ExitsZero(t *testing.T) {
	captureStderr(func() {
		code := runPreMergeCommit([]string{"-h"})
		assert.Equal(t, 0, code)
	})
}

func TestRunPreMergeCommit_UnknownSubcommand_ExitsTwo(t *testing.T) {
	got := captureStderr(func() {
		code := runPreMergeCommit([]string{"unknown"})
		assert.Equal(t, 2, code)
	})
	assert.Contains(t, got, "unknown subcommand")
}

func TestRunPreMergeCommitInstall_HelpFlag_ExitsZero(t *testing.T) {
	captureStderr(func() {
		code := runPreMergeCommitInstall([]string{"--help"})
		assert.Equal(t, 0, code)
	})
}

func TestRunPreMergeCommitUninstall_HelpFlag_ExitsZero(t *testing.T) {
	captureStderr(func() {
		code := runPreMergeCommitUninstall([]string{"--help"})
		assert.Equal(t, 0, code)
	})
}

func TestRunPreMergeCommitStatus_HelpFlag_ExitsZero(t *testing.T) {
	captureStderr(func() {
		code := runPreMergeCommitStatus([]string{"--help"})
		assert.Equal(t, 0, code)
	})
}

// --- install/uninstall/status integration ---

func TestPreMergeCommitInstall_CreatesHook(t *testing.T) {
	dir := t.TempDir()
	initTestRepo(t, dir)

	orig := executableFunc
	t.Cleanup(func() { executableFunc = orig })
	executableFunc = func() (string, error) { return "/usr/local/bin/mdsmith", nil }

	// Change to temp git repo so git commands work.
	origWd, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	// captureStderr wraps the call so the pipe's read end is
	// drained — an unconsumed os.Pipe could block writes once the
	// kernel buffer fills, and would leak FDs on test cleanup.
	captureStderr(func() {
		assert.Equal(t, 0, runPreMergeCommitInstall(nil))
	})

	hookPath := filepath.Join(resolveHooksDir(dir), "pre-merge-commit")
	info, err := os.Stat(hookPath)
	require.NoError(t, err, "hook must exist at %s", hookPath)
	if runtime.GOOS != "windows" {
		assert.NotZero(t, info.Mode()&0o111, "hook must be executable")
	}

	data, err := os.ReadFile(hookPath)
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, preMergeCommitHookMarker)
	assert.Contains(t, content, "if ! '/usr/local/bin/mdsmith' fix .; then")
	assert.Contains(t, content, "git diff --name-only -- '*.md' '*.markdown'")
}

func TestPreMergeCommitUninstall_RemovesHook(t *testing.T) {
	dir := t.TempDir()
	initTestRepo(t, dir)

	orig := executableFunc
	t.Cleanup(func() { executableFunc = orig })
	executableFunc = func() (string, error) { return "/usr/local/bin/mdsmith", nil }

	// Install hook first.
	hooksDir := resolveHooksDir(dir)
	require.NoError(t, os.MkdirAll(hooksDir, 0o755))
	hookPath := filepath.Join(hooksDir, "pre-merge-commit")
	hookContent := "#!/bin/sh\n" + preMergeCommitHookMarker + "\necho test\n"
	require.NoError(t, os.WriteFile(hookPath, []byte(hookContent), 0o755))

	// Change to temp git repo.
	origWd, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	captureStderr(func() {
		assert.Equal(t, 0, runPreMergeCommitUninstall(nil))
	})

	// Hook should be removed.
	_, err := os.Stat(hookPath)
	assert.True(t, os.IsNotExist(err), "hook should be removed")
}

func TestPreMergeCommitUninstall_RefusesUserHook(t *testing.T) {
	dir := t.TempDir()
	initTestRepo(t, dir)

	// Create a user hook without our marker.
	hooksDir := resolveHooksDir(dir)
	require.NoError(t, os.MkdirAll(hooksDir, 0o755))
	hookPath := filepath.Join(hooksDir, "pre-merge-commit")
	userContent := "#!/bin/sh\necho user hook\n"
	require.NoError(t, os.WriteFile(hookPath, []byte(userContent), 0o755))

	// Change to temp git repo.
	origWd, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	captureStderr(func() {
		assert.Equal(t, 2, runPreMergeCommitUninstall(nil),
			"should fail with exit code 2")
	})

	// Hook should still exist.
	data, err := os.ReadFile(hookPath)
	require.NoError(t, err)
	assert.Equal(t, userContent, string(data), "user hook must be untouched")
}

func TestPreMergeCommitStatus_NotInstalled(t *testing.T) {
	dir := t.TempDir()
	initTestRepo(t, dir)

	// Change to temp git repo.
	origWd, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	captureStderr(func() {
		assert.Equal(t, 1, runPreMergeCommitStatus(nil),
			"should exit 1 when not installed")
	})
}

func TestPreMergeCommitStatus_Installed(t *testing.T) {
	dir := t.TempDir()
	initTestRepo(t, dir)

	orig := executableFunc
	t.Cleanup(func() { executableFunc = orig })
	executableFunc = func() (string, error) { return "/usr/local/bin/mdsmith", nil }

	// Install hook.
	hooksDir := resolveHooksDir(dir)
	require.NoError(t, os.MkdirAll(hooksDir, 0o755))
	hookPath := filepath.Join(hooksDir, "pre-merge-commit")
	hookContent := "#!/bin/sh\n" + preMergeCommitHookMarker + "\n" +
		"'/usr/local/bin/mdsmith' fix -- 'PLAN.md'\ngit add -- 'PLAN.md'\n"
	require.NoError(t, os.WriteFile(hookPath, []byte(hookContent), 0o755))

	// Change to temp git repo.
	origWd, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	captureStderr(func() {
		assert.Equal(t, 0, runPreMergeCommitStatus(nil))
	})
}

// --- glob-based hook drift status ---

func TestPreMergeCommitStatus_NoWarningWhenCanonical(t *testing.T) {
	dir := t.TempDir()
	initTestRepo(t, dir)

	orig := executableFunc
	t.Cleanup(func() { executableFunc = orig })
	executableFunc = func() (string, error) { return "/usr/local/bin/mdsmith", nil }

	hooksDir := resolveHooksDir(dir)
	require.NoError(t, os.MkdirAll(hooksDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(hooksDir, "pre-merge-commit"),
		[]byte(githooks.BuildHookScript("/usr/local/bin/mdsmith")),
		0o755,
	))

	origWd, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	got := captureStderr(func() {
		assert.Equal(t, 0, runPreMergeCommitStatus(nil))
	})
	assert.Contains(t, got, "managed by: mdsmith")
	assert.NotContains(t, got, "out of sync with the glob-based template")
}

func TestPreMergeCommitStatus_WarnsWhenLegacyHookInstalled(t *testing.T) {
	dir := t.TempDir()
	initTestRepo(t, dir)

	orig := executableFunc
	t.Cleanup(func() { executableFunc = orig })
	executableFunc = func() (string, error) { return "/usr/local/bin/mdsmith", nil }

	// Old per-file hook still bears the marker but no longer matches
	// the canonical glob template.
	hooksDir := resolveHooksDir(dir)
	require.NoError(t, os.MkdirAll(hooksDir, 0o755))
	hookContent := "#!/bin/sh\n" + preMergeCommitHookMarker + "\n" +
		"set -e\n'/usr/local/bin/mdsmith' fix -- 'PLAN.md'\ngit add -- 'PLAN.md'\n"
	require.NoError(t, os.WriteFile(
		filepath.Join(hooksDir, "pre-merge-commit"),
		[]byte(hookContent), 0o755,
	))

	origWd, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	got := captureStderr(func() {
		assert.Equal(t, 0, runPreMergeCommitStatus(nil))
	})
	assert.Contains(t, got, "out of sync with the glob-based template")
}

func TestPreMergeCommitInstall_RejectsExplicitFiles(t *testing.T) {
	dir := t.TempDir()
	initTestRepo(t, dir)

	origWd, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	got := captureStderr(func() {
		assert.Equal(t, 2, runPreMergeCommitInstall([]string{"PLAN.md"}))
	})
	assert.Contains(t, got, "no longer accepts explicit files")
}
