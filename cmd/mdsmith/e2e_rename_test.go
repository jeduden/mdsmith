package main_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupRenameWorkspace builds a workspace where b.md links to a
// heading in a.md by anchor and by a ref-def destination, so a
// heading rename has cross-file edits to make.
func setupRenameWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	wf := func(rel, body string) {
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(dir, rel)), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, rel), []byte(body), 0o644))
	}
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	wf(".mdsmith.yml", "files:\n  - \"**/*.md\"\nrules:\n  cross-file-reference-integrity: false\n")
	wf("a.md", "# Setup\n\nBody.\n")
	wf("b.md", "See [go](a.md#setup) and [the docs][docs].\n\n[docs]: https://x.example\n")
	return dir
}

func TestE2E_Rename_Heading_RewritesWorkspace(t *testing.T) {
	dir := setupRenameWorkspace(t)
	stdout, _, code := runBinaryInDir(t, dir, "", "rename", "a.md", "--heading", "Setup", "Install")
	require.Equal(t, 0, code, "stdout=%q", stdout)
	assert.Contains(t, stdout, "a.md: 1 edit(s)")
	assert.Contains(t, stdout, "b.md: 1 edit(s)")

	a, _ := os.ReadFile(filepath.Join(dir, "a.md"))
	assert.Contains(t, string(a), "# Install")
	b, _ := os.ReadFile(filepath.Join(dir, "b.md"))
	assert.Contains(t, string(b), "[go](a.md#install)")
}

func TestE2E_Rename_LinkRef_JSON(t *testing.T) {
	dir := setupRenameWorkspace(t)
	stdout, _, code := runBinaryInDir(t, dir, "", "rename",
		"b.md", "--link-ref", "--format", "json", "docs", "rfc")
	require.Equal(t, 0, code, "stdout=%q", stdout)
	assert.Contains(t, stdout, `"file": "b.md"`)
	b, _ := os.ReadFile(filepath.Join(dir, "b.md"))
	assert.Contains(t, string(b), "[the docs][rfc]")
	assert.Contains(t, string(b), "[rfc]: https://x.example")
}

func TestE2E_Rename_ExitCodes(t *testing.T) {
	dir := setupRenameWorkspace(t)
	// No matching heading → exit 1.
	_, _, code := runBinaryInDir(t, dir, "", "rename", "a.md", "--heading", "Ghost", "X")
	assert.Equal(t, 1, code)
	// Missing mode flag → exit 2.
	_, _, code = runBinaryInDir(t, dir, "", "rename", "a.md", "Setup", "Install")
	assert.Equal(t, 2, code)
}
