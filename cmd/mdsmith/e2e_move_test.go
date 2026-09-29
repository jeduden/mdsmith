package main_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupMoveRepo builds a real Git repo where b.md links to a heading in
// a.md, so a move has a cross-file reference to rewrite and `check` can
// confirm no link broke. It skips when git is unavailable.
func setupMoveRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	wf := func(rel, body string) {
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(dir, rel)), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, rel), []byte(body), 0o644))
	}
	wf(".mdsmith.yml", "files:\n  - \"**/*.md\"\n")
	wf("a.md", "# Setup\n\nBody text here.\n")
	wf("b.md", "# B\n\nSee [go](a.md#setup) for details.\n")
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "t@example.com"},
		{"config", "user.name", "t"},
		{"add", "-A"},
		{"commit", "-q", "-m", "init"},
	} {
		require.NoError(t, exec.Command("git", append([]string{"-C", dir}, args...)...).Run(), "git %v", args)
	}
	return dir
}

func TestE2E_Move_RewritesReferencesAndStagesRename(t *testing.T) {
	dir := setupMoveRepo(t)
	stdout, stderr, code := runBinaryInDir(t, dir, "", "move", "a.md", "docs/a.md")
	require.Equal(t, 0, code, "stdout=%q stderr=%q", stdout, stderr)
	assert.Contains(t, stdout, "b.md: 1 edit(s)")
	assert.Contains(t, stdout, "moved a.md -> docs/a.md")

	assert.NoFileExists(t, filepath.Join(dir, "a.md"))
	assert.FileExists(t, filepath.Join(dir, "docs", "a.md"))
	b, _ := os.ReadFile(filepath.Join(dir, "b.md"))
	assert.Contains(t, string(b), "[go](docs/a.md#setup)")

	// The rename is staged in git.
	tracked, err := exec.Command("git", "-C", dir, "ls-files").Output()
	require.NoError(t, err)
	assert.Contains(t, string(tracked), "docs/a.md")

	// No link broke: check reports no MDS027.
	cout, cerr, ccode := runBinaryInDir(t, dir, "", "check", ".")
	assert.Equal(t, 0, ccode, "check stdout=%q stderr=%q", cout, cerr)
	assert.NotContains(t, cout+cerr, "MDS027")
}

func TestE2E_Move_DryRunChangesNothing(t *testing.T) {
	dir := setupMoveRepo(t)
	before, _ := os.ReadFile(filepath.Join(dir, "b.md"))

	stdout, _, code := runBinaryInDir(t, dir, "", "move", "a.md", "docs/a.md", "--dry-run")
	require.Equal(t, 0, code)
	assert.Contains(t, stdout, "would move a.md -> docs/a.md")
	assert.Contains(t, stdout, "b.md: 1 edit(s)")

	assert.FileExists(t, filepath.Join(dir, "a.md"))
	after, _ := os.ReadFile(filepath.Join(dir, "b.md"))
	assert.Equal(t, string(before), string(after))
}

func TestE2E_Move_JSON(t *testing.T) {
	dir := setupMoveRepo(t)
	stdout, _, code := runBinaryInDir(t, dir, "", "move", "a.md", "docs/a.md", "--format", "json")
	require.Equal(t, 0, code)
	assert.Contains(t, stdout, `"from": "a.md"`)
	assert.Contains(t, stdout, `"to": "docs/a.md"`)
	assert.Contains(t, stdout, `"file": "b.md"`)
}

func TestE2E_Move_ExitCodes(t *testing.T) {
	dir := setupMoveRepo(t)
	// Destination already exists → exit 2, nothing moved.
	_, _, code := runBinaryInDir(t, dir, "", "move", "a.md", "b.md")
	assert.Equal(t, 2, code)
	assert.FileExists(t, filepath.Join(dir, "a.md"))

	// Missing source → exit 1.
	_, _, code = runBinaryInDir(t, dir, "", "move", "ghost.md", "x.md")
	assert.Equal(t, 1, code)

	// Traversal destination → exit 2.
	_, _, code = runBinaryInDir(t, dir, "", "move", "a.md", "../evil.md")
	assert.Equal(t, 2, code)
}
