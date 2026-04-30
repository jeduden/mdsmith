package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- runPreMergeCommit dispatch ---

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
	require.NoError(t, exec.Command("git", "init", dir).Run())

	orig := executableFunc
	t.Cleanup(func() { executableFunc = orig })
	executableFunc = func() (string, error) { return "/usr/local/bin/mdsmith", nil }

	// Capture stderr during install.
	origStderr := os.Stderr
	_, w, _ := os.Pipe()
	os.Stderr = w
	t.Cleanup(func() {
		os.Stderr = origStderr
		_ = w.Close()
	})

	// Change to temp git repo so git commands work.
	origWd, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	code := runPreMergeCommitInstall([]string{"PLAN.md", "README.md"})
	assert.Equal(t, 0, code)

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
	assert.Contains(t, content, "'/usr/local/bin/mdsmith' fix --")
	assert.Contains(t, content, "'PLAN.md'")
	assert.Contains(t, content, "'README.md'")
	assert.Contains(t, content, "git add -- 'PLAN.md'")
	assert.Contains(t, content, "git add -- 'README.md'")
}

func TestPreMergeCommitUninstall_RemovesHook(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, exec.Command("git", "init", dir).Run())

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

	// Capture stderr.
	origStderr := os.Stderr
	_, w, _ := os.Pipe()
	os.Stderr = w
	t.Cleanup(func() {
		os.Stderr = origStderr
		_ = w.Close()
	})

	code := runPreMergeCommitUninstall(nil)
	assert.Equal(t, 0, code)

	// Hook should be removed.
	_, err := os.Stat(hookPath)
	assert.True(t, os.IsNotExist(err), "hook should be removed")
}

func TestPreMergeCommitUninstall_RefusesUserHook(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, exec.Command("git", "init", dir).Run())

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

	// Capture stderr.
	origStderr := os.Stderr
	_, w, _ := os.Pipe()
	os.Stderr = w
	t.Cleanup(func() {
		os.Stderr = origStderr
		_ = w.Close()
	})

	code := runPreMergeCommitUninstall(nil)
	assert.Equal(t, 2, code, "should fail with exit code 2")

	// Hook should still exist.
	data, err := os.ReadFile(hookPath)
	require.NoError(t, err)
	assert.Equal(t, userContent, string(data), "user hook must be untouched")
}

func TestPreMergeCommitStatus_NotInstalled(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, exec.Command("git", "init", dir).Run())

	// Change to temp git repo.
	origWd, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	// Capture stderr.
	origStderr := os.Stderr
	_, w, _ := os.Pipe()
	os.Stderr = w
	t.Cleanup(func() {
		os.Stderr = origStderr
		_ = w.Close()
	})

	code := runPreMergeCommitStatus(nil)
	assert.Equal(t, 1, code, "should exit 1 when not installed")
}

func TestPreMergeCommitStatus_Installed(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, exec.Command("git", "init", dir).Run())

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

	// Capture stderr.
	origStderr := os.Stderr
	_, w, _ := os.Pipe()
	os.Stderr = w
	t.Cleanup(func() {
		os.Stderr = origStderr
		_ = w.Close()
	})

	code := runPreMergeCommitStatus(nil)
	assert.Equal(t, 0, code)
}

// --- extractFilesFromHook ---

func TestExtractFilesFromHook_SingleFile(t *testing.T) {
	content := "#!/bin/sh\n" +
		"'/usr/bin/mdsmith' fix -- 'PLAN.md'\n" +
		"git add -- 'PLAN.md'\n"
	files := extractFilesFromHook(content)
	assert.Equal(t, []string{"PLAN.md"}, files)
}

func TestExtractFilesFromHook_MultipleFiles(t *testing.T) {
	content := "#!/bin/sh\n" +
		"'/usr/bin/mdsmith' fix -- 'PLAN.md'\n" +
		"git add -- 'PLAN.md'\n" +
		"'/usr/bin/mdsmith' fix -- 'README.md'\n" +
		"git add -- 'README.md'\n"
	files := extractFilesFromHook(content)
	assert.Equal(t, []string{"PLAN.md", "README.md"}, files)
}

func TestExtractFilesFromHook_NoFiles(t *testing.T) {
	content := "#!/bin/sh\necho test\n"
	files := extractFilesFromHook(content)
	assert.Nil(t, files)
}

func TestExtractFilesFromHook_WithConditionals(t *testing.T) {
	content := "#!/bin/sh\n" +
		"if [ -e 'PLAN.md' ]; then\n" +
		"  '/usr/bin/mdsmith' fix -- 'PLAN.md'\n" +
		"  git add -- 'PLAN.md'\n" +
		"fi\n"
	files := extractFilesFromHook(content)
	assert.Equal(t, []string{"PLAN.md"}, files)
}

// --- filesMatch ---

func TestFilesMatch_EmptyLists(t *testing.T) {
	assert.True(t, filesMatch(nil, nil))
	assert.True(t, filesMatch([]string{}, []string{}))
}

func TestFilesMatch_SameFiles(t *testing.T) {
	a := []string{"PLAN.md", "README.md"}
	b := []string{"PLAN.md", "README.md"}
	assert.True(t, filesMatch(a, b))
}

func TestFilesMatch_SameFilesDifferentOrder(t *testing.T) {
	a := []string{"PLAN.md", "README.md"}
	b := []string{"README.md", "PLAN.md"}
	assert.True(t, filesMatch(a, b))
}

func TestFilesMatch_DifferentLengths(t *testing.T) {
	a := []string{"PLAN.md"}
	b := []string{"PLAN.md", "README.md"}
	assert.False(t, filesMatch(a, b))
}

func TestFilesMatch_DifferentFiles(t *testing.T) {
	a := []string{"PLAN.md", "README.md"}
	b := []string{"PLAN.md", "CLAUDE.md"}
	assert.False(t, filesMatch(a, b))
}

func TestFilesMatch_OneEmpty(t *testing.T) {
	a := []string{"PLAN.md"}
	b := []string{}
	assert.False(t, filesMatch(a, b))
}

// --- sync detection integration ---

func TestPreMergeCommitStatus_ShowsWarningWhenOutOfSync(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, exec.Command("git", "init", dir).Run())

	orig := executableFunc
	t.Cleanup(func() { executableFunc = orig })
	executableFunc = func() (string, error) { return "/usr/local/bin/mdsmith", nil }

	// Create a file with generated content that will be discovered.
	testFile := filepath.Join(dir, "test.md")
	require.NoError(t, os.WriteFile(testFile, []byte("# Test\n\n<?catalog?>\n<?/catalog?>\n"), 0o644))

	// Install hook with different files (PLAN.md).
	hooksDir := resolveHooksDir(dir)
	require.NoError(t, os.MkdirAll(hooksDir, 0o755))
	hookPath := filepath.Join(hooksDir, "pre-merge-commit")
	hookContent := "#!/bin/sh\n" + preMergeCommitHookMarker + "\n" +
		"'/usr/local/bin/mdsmith' fix -- 'PLAN.md'\ngit add -- 'PLAN.md'\n"
	require.NoError(t, os.WriteFile(hookPath, []byte(hookContent), 0o755))

	// Create .mdsmith.yml to avoid config load errors.
	configPath := filepath.Join(dir, ".mdsmith.yml")
	require.NoError(t, os.WriteFile(configPath, []byte("max-input-bytes: 1048576\n"), 0o644))

	// Change to temp git repo.
	origWd, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	// Capture stderr.
	got := captureStderr(func() {
		code := runPreMergeCommitStatus(nil)
		assert.Equal(t, 0, code)
	})

	// Should show warning about out-of-sync files.
	assert.Contains(t, got, "Warning: hook files are out of sync")
	assert.Contains(t, got, "discovered files: test.md")
	assert.Contains(t, got, "Run 'mdsmith pre-merge-commit install' to update")
}

func TestPreMergeCommitStatus_NoWarningWhenInSync(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, exec.Command("git", "init", dir).Run())

	orig := executableFunc
	t.Cleanup(func() { executableFunc = orig })
	executableFunc = func() (string, error) { return "/usr/local/bin/mdsmith", nil }

	// Create a file with generated content.
	testFile := filepath.Join(dir, "test.md")
	require.NoError(t, os.WriteFile(testFile, []byte("# Test\n\n<?catalog?>\n<?/catalog?>\n"), 0o644))

	// Install hook with the same file that will be discovered.
	hooksDir := resolveHooksDir(dir)
	require.NoError(t, os.MkdirAll(hooksDir, 0o755))
	hookPath := filepath.Join(hooksDir, "pre-merge-commit")
	hookContent := "#!/bin/sh\n" + preMergeCommitHookMarker + "\n" +
		"'/usr/local/bin/mdsmith' fix -- 'test.md'\ngit add -- 'test.md'\n"
	require.NoError(t, os.WriteFile(hookPath, []byte(hookContent), 0o755))

	// Create .mdsmith.yml.
	configPath := filepath.Join(dir, ".mdsmith.yml")
	require.NoError(t, os.WriteFile(configPath, []byte("max-input-bytes: 1048576\n"), 0o644))

	// Change to temp git repo.
	origWd, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	// Capture stderr.
	got := captureStderr(func() {
		code := runPreMergeCommitStatus(nil)
		assert.Equal(t, 0, code)
	})

	// Should NOT show warning since files match.
	assert.NotContains(t, got, "Warning: hook files are out of sync")
	assert.Contains(t, got, "pre-merge-commit hook: installed")
}
