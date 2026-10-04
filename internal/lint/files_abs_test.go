package lint

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jeduden/mdsmith/internal/gitignore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWalkDirGitignoreWorkingDirectoryFailure covers an unreadable
// working directory: an absolute root still honors its .gitignore, since
// resolving an absolute path never needed the working directory.
func TestWalkDirGitignoreWorkingDirectoryFailure(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, ".gitignore"), []byte("ignored.md\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "a.md"), []byte("# A\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "ignored.md"), []byte("# I\n"), 0o644))

	orig := getwdFn
	getwdFn = func() (string, error) { return "", errors.New("no cwd") }
	t.Cleanup(func() { getwdFn = orig })

	files, err := walkDir(root, true, false)
	require.NoError(t, err)
	assert.Equal(t, []string{filepath.Join(root, "a.md")}, files)
}

// TestIsGitignoredRelativePathUsesGivenCwd checks a relative path is
// matched against the supplied working directory, not the process's.
func TestIsGitignoredRelativePathUsesGivenCwd(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, ".gitignore"), []byte("skip.md\n"), 0o644))
	m := gitignore.NewMatcher(root)
	assert.True(t, isGitignored(m, root, "skip.md", false))
	assert.False(t, isGitignored(m, root, "keep.md", false))
}
