//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGuardRegularFile_Symlink_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.md")
	link := filepath.Join(dir, "link.md")
	require.NoError(t, os.WriteFile(target, []byte("content"), 0o644))
	require.NoError(t, os.Symlink(target, link))

	err := guardRegularFile(link)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a regular file")
}

func TestMergeAndClean_OursIsSymlink_ExitsTwo(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.md")
	link := filepath.Join(dir, "ours.md")
	require.NoError(t, os.WriteFile(target, []byte("# content\n"), 0o644))
	require.NoError(t, os.Symlink(target, link))

	base := filepath.Join(dir, "base.md")
	theirs := filepath.Join(dir, "theirs.md")
	require.NoError(t, os.WriteFile(base, []byte("# content\n"), 0o644))
	require.NoError(t, os.WriteFile(theirs, []byte("# content\n"), 0o644))

	_, code := mergeAndClean(base, link, theirs, 1<<20)
	assert.Equal(t, 2, code)
}

func TestMergeAndClean_BaseIsSymlink_ExitsTwo(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.md")
	link := filepath.Join(dir, "base.md")
	require.NoError(t, os.WriteFile(target, []byte("# content\n"), 0o644))
	require.NoError(t, os.Symlink(target, link))

	ours := filepath.Join(dir, "ours.md")
	theirs := filepath.Join(dir, "theirs.md")
	require.NoError(t, os.WriteFile(ours, []byte("# content\n"), 0o644))
	require.NoError(t, os.WriteFile(theirs, []byte("# content\n"), 0o644))

	_, code := mergeAndClean(link, ours, theirs, 1<<20)
	assert.Equal(t, 2, code)
}

func TestMergeAndClean_TheirsIsSymlink_ExitsTwo(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.md")
	link := filepath.Join(dir, "theirs.md")
	require.NoError(t, os.WriteFile(target, []byte("# content\n"), 0o644))
	require.NoError(t, os.Symlink(target, link))

	base := filepath.Join(dir, "base.md")
	ours := filepath.Join(dir, "ours.md")
	require.NoError(t, os.WriteFile(base, []byte("# content\n"), 0o644))
	require.NoError(t, os.WriteFile(ours, []byte("# content\n"), 0o644))

	_, code := mergeAndClean(base, ours, link, 1<<20)
	assert.Equal(t, 2, code)
}

func TestFixMergedContent_PathnameIsSymlink_Harmless(t *testing.T) {
	// The driver never dereferences the worktree path (%P) — it is
	// only a label for config-glob matching — so a symlinked %P
	// cannot pull content from outside the worktree and is not an
	// error. The link itself must be left untouched.
	dir := t.TempDir()
	target := filepath.Join(dir, "target.md")
	link := filepath.Join(dir, "pathname.md")
	require.NoError(t, os.WriteFile(target, []byte("# content\n"), 0o644))
	require.NoError(t, os.Symlink(target, link))

	ours := filepath.Join(dir, "ours.md")
	require.NoError(t, os.WriteFile(ours, []byte("# content\n"), 0o644))

	origFix := fixSourceFn
	t.Cleanup(func() { fixSourceFn = origFix })
	fixSourceFn = func(_ string, src []byte, _ int64) ([]byte, error) {
		return src, nil
	}

	_, code := fixMergedContent([]byte("# content\n"), ours, link, 1<<20)
	assert.Equal(t, 0, code)

	data, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "# content\n", string(data),
		"symlink target must be untouched")
}

func TestFixMergedContent_OursIsSymlink_ExitsTwo(t *testing.T) {
	dir := t.TempDir()
	oursTarget := filepath.Join(dir, "ours-target.md")
	oursLink := filepath.Join(dir, "ours.md")
	require.NoError(t, os.WriteFile(oursTarget, []byte("# content\n"), 0o644))
	require.NoError(t, os.Symlink(oursTarget, oursLink))

	origFix := fixSourceFn
	t.Cleanup(func() { fixSourceFn = origFix })
	fixSourceFn = func(_ string, src []byte, _ int64) ([]byte, error) {
		return src, nil
	}

	_, code := fixMergedContent([]byte("# content\n"), oursLink, "pathname.md", 1<<20)
	assert.Equal(t, 2, code)
}

// TestEnsurePreMergeCommitHook_SymlinkAtHookPath_ReturnsError places a symlink
// at the hook path and asserts ensurePreMergeCommitHook returns an error
// instead of following the link.
func TestEnsurePreMergeCommitHook_SymlinkAtHookPath_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	external := t.TempDir()
	hooksDir := filepath.Join(dir, ".git", "hooks")
	require.NoError(t, os.MkdirAll(hooksDir, 0o755))

	// Create the symlink target outside the repo.
	target := filepath.Join(external, "external-hook")
	require.NoError(t, os.WriteFile(target, []byte("#!/bin/sh\n"), 0o644))

	hookPath := filepath.Join(hooksDir, "pre-merge-commit")
	require.NoError(t, os.Symlink(target, hookPath))

	orig := executableFunc
	t.Cleanup(func() { executableFunc = orig })
	executableFunc = func() (string, error) { return "/usr/local/bin/mdsmith", nil }

	err := ensurePreMergeCommitHook(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a regular file")

	// Confirm the external target was not modified.
	got, readErr := os.ReadFile(target)
	require.NoError(t, readErr)
	assert.Equal(t, "#!/bin/sh\n", string(got),
		"external target must not be rewritten through the symlink")
}
