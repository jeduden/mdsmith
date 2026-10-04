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

// TestWalkDirRelativeRootWithoutWorkingDirectory covers a relative root
// whose working directory cannot be resolved: entries cannot be matched
// against .gitignore, so they are listed rather than dropped.
func TestWalkDirRelativeRootWithoutWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "a.md"), []byte("# A\n"), 0o644))
	t.Chdir(root)

	orig := getwdFn
	getwdFn = func() (string, error) { return "", errors.New("no cwd") }
	t.Cleanup(func() { getwdFn = orig })

	files, err := walkDir(".", true, false)
	require.NoError(t, err)
	assert.Equal(t, []string{"a.md"}, files)
}

// TestResolveFilesResolvesWorkingDirectoryOnce bounds the working-directory
// lookups for a relative root to a small constant per resolve call, not
// one per file.
func TestResolveFilesResolvesWorkingDirectoryOnce(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 20; i++ {
		name := filepath.Join(root, "f"+string(rune('a'+i))+".md")
		require.NoError(t, os.WriteFile(name, []byte("# T\n"), 0o644))
	}
	t.Chdir(root)

	calls := 0
	orig := getwdFn
	getwdFn = func() (string, error) { calls++; return orig() }
	t.Cleanup(func() { getwdFn = orig })

	files, err := ResolveFilesWithOpts([]string{"."}, DefaultResolveOpts())
	require.NoError(t, err)
	assert.Len(t, files, 20)
	assert.LessOrEqual(t, calls, 4, "working directory looked up per file")
}

// TestResolveFilesRelativePathWithoutWorkingDirectory covers a relative
// argument whose working directory cannot be resolved: the file is still
// listed, deduplicated by its given spelling.
func TestResolveFilesRelativePathWithoutWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "a.md"), []byte("# A\n"), 0o644))
	t.Chdir(root)

	orig := getwdFn
	getwdFn = func() (string, error) { return "", errors.New("no cwd") }
	t.Cleanup(func() { getwdFn = orig })

	files, err := ResolveFilesWithOpts([]string{"a.md", "a.md"}, DefaultResolveOpts())
	require.NoError(t, err)
	assert.Equal(t, []string{"a.md"}, files)
}

func TestAbsWithCwd_EmptyPathIsCwd(t *testing.T) {
	cwd := t.TempDir()
	assert.Equal(t, filepath.Clean(cwd), absWithCwd("", cwd))
}
