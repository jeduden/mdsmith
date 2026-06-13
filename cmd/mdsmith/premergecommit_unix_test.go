//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunPreMergeCommitUninstall_SymlinkAtHookPath_ReturnsError places a
// symlink at the hook path and asserts runPreMergeCommitUninstall returns
// exit code 2 without removing the external target.
func TestRunPreMergeCommitUninstall_SymlinkAtHookPath_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	external := t.TempDir()
	initTestRepo(t, dir)
	hooksDir := resolveHooksDir(dir)
	require.NoError(t, os.MkdirAll(hooksDir, 0o755))

	// Create an external file to act as the symlink target.
	target := filepath.Join(external, "external-hook")
	require.NoError(t, os.WriteFile(target, []byte("#!/bin/sh\n"), 0o644))

	hookPath := filepath.Join(hooksDir, "pre-merge-commit")
	require.NoError(t, os.Symlink(target, hookPath))

	origWd, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	got := captureStderr(func() {
		assert.Equal(t, 2, runPreMergeCommitUninstall(nil))
	})
	assert.Contains(t, got, "not a regular file")

	// Confirm the external target was not removed.
	_, statErr := os.Stat(target)
	assert.NoError(t, statErr, "external target must not be removed through the symlink")
}

// TestRunPreMergeCommitStatus_SymlinkAtHookPath_ReturnsError places a
// symlink at the hook path and asserts runPreMergeCommitStatus returns
// exit code 2 without reading through the link.
func TestRunPreMergeCommitStatus_SymlinkAtHookPath_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	external := t.TempDir()
	initTestRepo(t, dir)
	hooksDir := resolveHooksDir(dir)
	require.NoError(t, os.MkdirAll(hooksDir, 0o755))

	// Create an external file to act as the symlink target.
	target := filepath.Join(external, "external-hook")
	require.NoError(t, os.WriteFile(target, []byte("#!/bin/sh\n"), 0o644))

	hookPath := filepath.Join(hooksDir, "pre-merge-commit")
	require.NoError(t, os.Symlink(target, hookPath))

	origWd, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	got := captureStderr(func() {
		assert.Equal(t, 2, runPreMergeCommitStatus(nil))
	})
	assert.Contains(t, got, "not a regular file")
}

// TestRunPreMergeCommitUninstall_ReadFileFails places a mode-000 file at
// the hook path so os.ReadFile returns a non-ENOENT error after the lstat
// guard passes. Skips as root (which bypasses permission checks).
func TestRunPreMergeCommitUninstall_ReadFileFails(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root: permission checks don't apply")
	}
	dir := t.TempDir()
	initTestRepo(t, dir)
	hooksDir := resolveHooksDir(dir)
	require.NoError(t, os.MkdirAll(hooksDir, 0o755))
	hookPath := filepath.Join(hooksDir, "pre-merge-commit")
	require.NoError(t, os.WriteFile(hookPath, []byte("#!/bin/sh\n"), 0o644))
	require.NoError(t, os.Chmod(hookPath, 0o000))
	t.Cleanup(func() { _ = os.Chmod(hookPath, 0o644) })

	origWd, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	got := captureStderr(func() {
		assert.Equal(t, 2, runPreMergeCommitUninstall(nil))
	})
	assert.Contains(t, got, "reading hook")
}

// TestRunPreMergeCommitStatus_ReadFileFails places a mode-000 file at the
// hook path so os.ReadFile returns a non-ENOENT error after the lstat guard
// passes. Skips as root (which bypasses permission checks).
func TestRunPreMergeCommitStatus_ReadFileFails(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root: permission checks don't apply")
	}
	dir := t.TempDir()
	initTestRepo(t, dir)
	hooksDir := resolveHooksDir(dir)
	require.NoError(t, os.MkdirAll(hooksDir, 0o755))
	hookPath := filepath.Join(hooksDir, "pre-merge-commit")
	require.NoError(t, os.WriteFile(hookPath, []byte("#!/bin/sh\n"), 0o644))
	require.NoError(t, os.Chmod(hookPath, 0o000))
	t.Cleanup(func() { _ = os.Chmod(hookPath, 0o644) })

	origWd, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	got := captureStderr(func() {
		assert.Equal(t, 2, runPreMergeCommitStatus(nil))
	})
	assert.Contains(t, got, "reading hook")
}
