//go:build !wasm

package refactor

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gitInit creates a committed Git repo in dir with one tracked file so
// the move executor's git path can be exercised. It skips the test when
// no git binary is available.
func gitInit(t *testing.T, dir string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "t@example.com"},
		{"config", "user.name", "t"},
	} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		require.NoError(t, cmd.Run(), "git %v", args)
	}
}

func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
	return string(out)
}

func TestFileOpExecute_GitTrackedStagesRename(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.md"), []byte("# A\n"), 0o644))
	gitRun(t, dir, "add", "a.md")
	gitRun(t, dir, "commit", "-q", "-m", "init")

	err := FileOp{From: "a.md", To: "docs/b.md"}.Execute(dir)
	require.NoError(t, err)

	assert.NoFileExists(t, filepath.Join(dir, "a.md"))
	assert.FileExists(t, filepath.Join(dir, "docs", "b.md"))
	// The rename is staged: git tracks the new path, not the old.
	tracked := gitRun(t, dir, "ls-files")
	assert.Contains(t, tracked, "docs/b.md")
	assert.NotContains(t, tracked, "a.md\n")
}

func TestFileOpExecute_UntrackedFileFallsBackToRename(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	// Written but never `git add`ed — untracked.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.md"), []byte("# A\n"), 0o644))

	err := FileOp{From: "a.md", To: "b.md"}.Execute(dir)
	require.NoError(t, err)

	assert.NoFileExists(t, filepath.Join(dir, "a.md"))
	assert.FileExists(t, filepath.Join(dir, "b.md"))
}

func TestFileOpExecute_NoRepoUsesPlainRename(t *testing.T) {
	dir := t.TempDir() // no git init
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.md"), []byte("# A\n"), 0o644))

	err := FileOp{From: "a.md", To: "sub/dir/b.md"}.Execute(dir)
	require.NoError(t, err)

	assert.NoFileExists(t, filepath.Join(dir, "a.md"))
	assert.FileExists(t, filepath.Join(dir, "sub", "dir", "b.md"))
}

func TestFileOpExecute_GitMvFailureAbortsNoHalfMove(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.md"), []byte("# A\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.md"), []byte("# B\n"), 0o644))
	gitRun(t, dir, "add", "a.md", "b.md")
	gitRun(t, dir, "commit", "-q", "-m", "init")

	// b.md already exists and is tracked, so `git mv a.md b.md` fails
	// without -f. The source must stay put.
	err := FileOp{From: "a.md", To: "b.md"}.Execute(dir)
	require.Error(t, err)
	assert.FileExists(t, filepath.Join(dir, "a.md"))
}
