package main_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// refusal is the usage error check and fix print for an -o path that
// is, or would be, one of their inputs.
func refusal(cmd, output string) string {
	return fmt.Sprintf("mdsmith: %s: refusing --output %q: it is an input of this run, or would be once written\n",
		cmd, output)
}

// TestOutputGuard_E2E pins that check and fix refuse, with exit 2 and
// before any file is linted or fixed, an -o path that would overwrite
// one of their Markdown inputs or turn up as an input of the next run.
func TestOutputGuard_E2E(t *testing.T) {
	t.Run("check -o the file being checked", func(t *testing.T) {
		dir := outputWorkspace(t)
		before := readReport(t, filepath.Join(dir, "long.md"))
		stdout, stderr, code := runBinaryInDir(t, dir, "", "check", "-o", "long.md", "long.md")
		assert.Equal(t, 2, code)
		assert.Empty(t, stdout)
		assert.Equal(t, refusal("check", "long.md"), stderr)
		assert.Equal(t, before, readReport(t, filepath.Join(dir, "long.md")))
	})
	t.Run("check -o a new Markdown file in the linted directory", func(t *testing.T) {
		dir := outputWorkspace(t)
		require.NoError(t, os.Mkdir(filepath.Join(dir, "docs"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "docs", "a.md"), []byte("# A\n"), 0o644))
		out := filepath.Join("docs", "report.md")
		_, stderr, code := runBinaryInDir(t, dir, "", "check", "-o", out, "docs")
		assert.Equal(t, 2, code)
		assert.Equal(t, refusal("check", out), stderr)
		assert.NoFileExists(t, filepath.Join(dir, out))
	})
	t.Run("check discovery -o a Markdown file it would discover", func(t *testing.T) {
		dir := outputWorkspace(t)
		_, stderr, code := runBinaryInDir(t, dir, "", "check", "-o", "report.md")
		assert.Equal(t, 2, code)
		assert.Equal(t, refusal("check", "report.md"), stderr)
		assert.NoFileExists(t, filepath.Join(dir, "report.md"))
	})
	t.Run("check with no Markdown file yet", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
		_, stderr, code := runBinaryInDir(t, dir, "", "check", "-o", "report.md", ".")
		assert.Equal(t, 2, code)
		assert.Equal(t, refusal("check", "report.md"), stderr)

		_, stderr, code = runBinaryInDir(t, dir, "", "check", "-o", "report.md")
		assert.Equal(t, 2, code, "discovery with the default files: patterns")
		assert.Equal(t, refusal("check", "report.md"), stderr)
	})
	t.Run("a non-Markdown report beside the inputs is fine", func(t *testing.T) {
		dir := outputWorkspace(t)
		_, stderr, code := runBinaryInDir(t, dir, "", "check", "-o", "report.txt", ".")
		assert.Equal(t, 1, code)
		assert.Empty(t, stderr)
		assert.Contains(t, readReport(t, filepath.Join(dir, "report.txt")), "MDS001")
	})
	t.Run("fix -o a file being fixed changes nothing", func(t *testing.T) {
		dir := outputWorkspace(t)
		ws := filepath.Join(dir, "ws.md")
		require.NoError(t, os.WriteFile(ws, []byte("# Hi  \n"), 0o644))
		_, stderr, code := runBinaryInDir(t, dir, "", "fix", "-o", "ws.md", "ws.md")
		assert.Equal(t, 2, code)
		assert.Equal(t, refusal("fix", "ws.md"), stderr)
		assert.Equal(t, "# Hi  \n", readReport(t, ws), "refused before any fix")
	})
	t.Run("fix discovery -o a Markdown file it would discover", func(t *testing.T) {
		dir := outputWorkspace(t)
		_, stderr, code := runBinaryInDir(t, dir, "", "fix", "-o", "report.md")
		assert.Equal(t, 2, code)
		assert.Equal(t, refusal("fix", "report.md"), stderr)
	})
}

// TestExportOutputGuard_E2E pins that export refuses an -o path that
// is the file being exported, which export never modifies.
func TestExportOutputGuard_E2E(t *testing.T) {
	dir := outputWorkspace(t)
	src := filepath.Join(dir, "ok.md")
	before := readReport(t, src)
	stdout, stderr, code := runBinaryInDir(t, dir, "", "export", "-o", "./ok.md", "ok.md")
	assert.Equal(t, 2, code)
	assert.Empty(t, stdout)
	assert.Equal(t, "mdsmith: export: refusing --output \"./ok.md\": it is the file being exported\n", stderr)
	assert.Equal(t, before, readReport(t, src))

	_, _, code = runBinaryInDir(t, dir, "", "export", "-o", "copy.md", "ok.md")
	assert.Equal(t, 0, code)
	assert.Equal(t, before, readReport(t, filepath.Join(dir, "copy.md")))
}
