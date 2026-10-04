package lint

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAbsWithinMatchesFilepathAbs pins absWithin to filepath.Abs so the
// gitignore walk can resolve the working directory once per walk.
func TestAbsWithinMatchesFilepathAbs(t *testing.T) {
	cwd := t.TempDir()
	t.Chdir(cwd)
	real, err := filepath.Abs(".")
	require.NoError(t, err)
	for _, p := range []string{".", "docs", "docs/a.md", "./x/../y.md", real, real + "/z/../w.md"} {
		want, err := filepath.Abs(p)
		require.NoError(t, err)
		assert.Equal(t, want, absWithin(real, p), p)
	}
}

// TestWalkDirGitignoreWithoutWorkingDirectory covers the unreadable
// working directory: with gitignore on, walkDir keeps listing files
// (it cannot match ignore rules, as filepath.Abs failing did before).
func TestWalkDirGitignoreWithoutWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "a.md"), []byte("# A\n"), 0o644))
	gone := t.TempDir()
	t.Chdir(gone)
	require.NoError(t, os.Remove(gone))
	t.Setenv("PWD", "")
	if _, err := os.Getwd(); err == nil {
		t.Skip("working directory still resolvable on this platform")
	}

	files, err := walkDir(root, true, false)
	require.NoError(t, err)
	assert.Equal(t, []string{filepath.Join(root, "a.md")}, files)
}
