//go:build !windows

package githooks

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteGitattributesFile_RejectsSymlink(t *testing.T) {
	// Call writeGitattributesFile directly to exercise its own Lstat guard
	// (defense-in-depth against TOCTOU between WriteGitattributes' initial
	// check and the actual write).
	dir := t.TempDir()
	target := filepath.Join(dir, "real.gitattributes")
	link := filepath.Join(dir, ".gitattributes")
	require.NoError(t, os.WriteFile(target, []byte("existing\n"), 0o644))
	require.NoError(t, os.Symlink(target, link))

	err := writeGitattributesFile(link, "new content\n")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a regular file")
}

func TestWriteGitattributes_RejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real.gitattributes")
	link := filepath.Join(dir, ".gitattributes")

	require.NoError(t, os.WriteFile(target, []byte("existing\n"), 0o644))
	require.NoError(t, os.Symlink(target, link))

	err := WriteGitattributes(link, Globs{Include: []string{"a.md"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a regular file")
}

func TestWriteGitattributes_ReturnsErrorForUnreadableExistingFile(t *testing.T) {
	// Mode 0000 only blocks reads for non-root users; root bypasses
	// file permission bits, so this assertion can't hold under uid 0.
	if os.Geteuid() == 0 {
		t.Skip("file permission bits don't restrict root")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitattributes")

	err := os.WriteFile(path, []byte("test"), 0000)
	require.NoError(t, err)

	err = WriteGitattributes(path, Globs{Include: []string{"a.md"}})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "reading")
}

func TestWriteGitattributes_PreservesExistingFileMode(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("file permission bits don't restrict root")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitattributes")

	// Write with a non-default mode to verify it is preserved.
	require.NoError(t, os.WriteFile(path, []byte("*.txt text\n"), 0o600))

	require.NoError(t, WriteGitattributes(path, Globs{Include: []string{"docs.md"}}))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(),
		"WriteGitattributes must not change the existing file's permission bits")
}

// TestBuildHookScript_StagesFixesWhenUnfixedRemain reproduces the
// merge-queue scenario where `mdsmith fix` modifies files in the
// working tree but also exits with code 1 because some diagnostics
// are not auto-fixable. The hook must still stage the modified
// files so the merge commit captures them.
//
// Regression for the case observed on
// merge-queue/batch-bisect-224-1777817057 (SHA b1ade018) where the
// catalog regeneration reached the working tree but never made it
// into the merge commit.
func TestBuildHookScript_StagesFixesWhenUnfixedRemain(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	dir := t.TempDir()

	// Stand up a fake mdsmith that:
	//   1. modifies a tracked file in the working tree (simulating
	//      mdsmith fix regenerating a catalog), and
	//   2. exits 1 to signal "diagnostics remain unfixed".
	fakeMdsmith := filepath.Join(dir, "fake-mdsmith")
	target := filepath.Join(dir, "PLAN.md")
	script := "#!/bin/sh\n" +
		"echo 'fixed by fake mdsmith' > " + shellQuote(target) + "\n" +
		"exit 1\n"
	require.NoError(t, os.WriteFile(fakeMdsmith, []byte(script), 0o755))

	// Initialise a git repo with one tracked file and a clean index.
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoErrorf(t, err, "git %s: %s", strings.Join(args, " "), out)
	}
	runGit("init", "-q", "-b", "main")
	runGit("config", "user.email", "test@test")
	runGit("config", "user.name", "test")
	runGit("config", "commit.gpgsign", "false")
	runGit("config", "tag.gpgsign", "false")
	require.NoError(t, os.WriteFile(target, []byte("original\n"), 0o644))
	runGit("add", "PLAN.md")
	runGit("commit", "-q", "-m", "init")

	// Install the canonical hook.
	hooksDir := filepath.Join(dir, ".git", "hooks")
	require.NoError(t, os.MkdirAll(hooksDir, 0o755))
	hookPath := filepath.Join(hooksDir, "pre-merge-commit")
	require.NoError(t, os.WriteFile(
		hookPath, []byte(BuildHookScript(fakeMdsmith)), 0o755))

	// Run the hook directly. This simulates merge-queue-action
	// invoking it after `git merge --no-commit`.
	cmd := exec.Command(hookPath)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "hook should not fail when fix exits 1: %s", out)

	// The fake mdsmith modified PLAN.md. The hook must stage that
	// change so the subsequent merge commit captures it. If the
	// hook exits early (e.g. because `! cmd` clobbers the exit
	// code that the script tries to inspect via $?), the working
	// tree change is left unstaged and the bug we hit on the
	// merge queue's bisect branch reproduces.
	staged := exec.Command("git", "diff", "--cached", "--name-only")
	staged.Dir = dir
	stagedOut, err := staged.Output()
	require.NoError(t, err)
	assert.Contains(t, string(stagedOut), "PLAN.md",
		"hook must stage files modified by `mdsmith fix .` even when "+
			"fix exits 1; got staged=%q", string(stagedOut))
}

// TestHookScript_MissingSetPlusE_FailsToStageOnExitOne proves that a hook
// without the `set +e` guard around the fix invocation does NOT stage files
// when `mdsmith fix` exits 1. This is the original merge-queue bug: under
// `set -e`, a non-zero exit aborts the shell before the staging loop runs.
// The test documents the defect so drift detection (HookMatchesCanonical) is
// shown to be meaningful — flagging the bad hook prevents silent data loss.
func TestHookScript_MissingSetPlusE_FailsToStageOnExitOne(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	dir := t.TempDir()

	fakeMdsmith := filepath.Join(dir, "fake-mdsmith")
	target := filepath.Join(dir, "PLAN.md")
	script := "#!/bin/sh\n" +
		"echo 'fixed by fake mdsmith' > " + shellQuote(target) + "\n" +
		"exit 1\n"
	require.NoError(t, os.WriteFile(fakeMdsmith, []byte(script), 0o755))

	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoErrorf(t, err, "git %s: %s", strings.Join(args, " "), out)
	}
	runGit("init", "-q", "-b", "main")
	runGit("config", "user.email", "test@test")
	runGit("config", "user.name", "test")
	runGit("config", "commit.gpgsign", "false")
	runGit("config", "tag.gpgsign", "false")
	require.NoError(t, os.WriteFile(target, []byte("original\n"), 0o644))
	runGit("add", "PLAN.md")
	runGit("commit", "-q", "-m", "init")

	// Read the bad hook golden file and substitute the real fake-mdsmith path.
	golden, err := os.ReadFile(filepath.Join("testdata", "hooks", "bad", "missing-set-plus-e.sh"))
	require.NoError(t, err)
	hookScript := strings.ReplaceAll(string(golden), "/usr/local/bin/mdsmith", fakeMdsmith)

	hooksDir := filepath.Join(dir, ".git", "hooks")
	require.NoError(t, os.MkdirAll(hooksDir, 0o755))
	hookPath := filepath.Join(hooksDir, "pre-merge-commit")
	require.NoError(t, os.WriteFile(hookPath, []byte(hookScript), 0o755))

	cmd := exec.Command(hookPath)
	cmd.Dir = dir
	// The hook exits non-zero because fix exits 1 and set -e propagates it.
	_ = cmd.Run()

	staged := exec.Command("git", "diff", "--cached", "--name-only")
	staged.Dir = dir
	stagedOut, err := staged.Output()
	require.NoError(t, err)
	assert.NotContains(t, string(stagedOut), "PLAN.md",
		"hook without set +e must NOT stage files when fix exits 1 — "+
			"this is the original merge-queue bug; got staged=%q", string(stagedOut))
}
