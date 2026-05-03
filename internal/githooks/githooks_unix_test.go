//go:build !windows

package githooks

import (
	"os"
	"path/filepath"
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
