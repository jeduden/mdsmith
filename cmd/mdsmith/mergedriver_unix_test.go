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

func TestFixAtRealPath_PathnameIsSymlink_ExitsTwo(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.md")
	link := filepath.Join(dir, "pathname.md")
	require.NoError(t, os.WriteFile(target, []byte("# content\n"), 0o644))
	require.NoError(t, os.Symlink(target, link))

	ours := filepath.Join(dir, "ours.md")
	require.NoError(t, os.WriteFile(ours, []byte("# content\n"), 0o644))

	_, code := fixAtRealPath([]byte("# content\n"), ours, link, 1<<20)
	assert.Equal(t, 2, code)
}

func TestFixAtRealPath_OursIsSymlink_ExitsTwo(t *testing.T) {
	dir := t.TempDir()
	pathname := filepath.Join(dir, "pathname.md")
	require.NoError(t, os.WriteFile(pathname, []byte("# content\n"), 0o644))

	oursTarget := filepath.Join(dir, "ours-target.md")
	oursLink := filepath.Join(dir, "ours.md")
	require.NoError(t, os.WriteFile(oursTarget, []byte("# content\n"), 0o644))
	require.NoError(t, os.Symlink(oursTarget, oursLink))

	_, code := fixAtRealPath([]byte("# content\n"), oursLink, pathname, 1<<20)
	assert.Equal(t, 2, code)
}
