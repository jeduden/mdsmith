package main_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFixOutput_E2E runs the real binary and pins that fix routes its
// report exactly as check does: -o - to stdout, -o <path> to a file,
// and no flag to stderr. long.md keeps an unfixable line-length
// diagnostic, so every run has something left to report.
func TestFixOutput_E2E(t *testing.T) {
	t.Run("-o -", func(t *testing.T) {
		stdout, stderr, code := runBinaryInDir(t, outputWorkspace(t), "",
			"fix", "-o", "-", "-f", "json", "long.md")
		assert.Equal(t, 1, code)
		assert.Empty(t, stderr)
		assert.Equal(t, []string{"MDS001"}, jsonRules(t, stdout))
	})
	t.Run("-o path, text with stats", func(t *testing.T) {
		dir := outputWorkspace(t)
		stdout, stderr, code := runBinaryInDir(t, dir, "",
			"fix", "-o", "fix.txt", "long.md")
		assert.Equal(t, 1, code)
		assert.Empty(t, stdout)
		assert.Empty(t, stderr)
		report := readReport(t, filepath.Join(dir, "fix.txt"))
		assert.Contains(t, report, "long.md:3:31 MDS001 line too long")
		assert.True(t, strings.HasSuffix(report,
			"stats: checked=1 fixed=0 failures=1 unfixed=1\n"), "report=%q", report)
		assert.NotContains(t, report, "\033[", "a file is not a terminal")
	})
	t.Run("dry-run json, -o -", func(t *testing.T) {
		dir := outputWorkspace(t)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "ws.md"), []byte("# Hi  \n"), 0o644))
		stdout, stderr, code := runBinaryInDir(t, dir, "",
			"fix", "--dry-run", "-o", "-", "-f", "json", "ws.md")
		assert.Equal(t, 0, code)
		assert.Empty(t, stderr)
		assert.Contains(t, stdout, `"path": "ws.md"`)
		assert.Equal(t, "# Hi  \n", readReport(t, filepath.Join(dir, "ws.md")), "a dry run writes nothing")
	})
	t.Run("-q still fills an -o file", func(t *testing.T) {
		dir := outputWorkspace(t)
		stdout, stderr, code := runBinaryInDir(t, dir, "",
			"fix", "-q", "-o", "fix.json", "-f", "json", "long.md")
		assert.Equal(t, 1, code)
		assert.Empty(t, stdout)
		assert.Empty(t, stderr)
		assert.Equal(t, []string{"MDS001"}, jsonRules(t, readReport(t, filepath.Join(dir, "fix.json"))))
	})
	t.Run("default is stderr", func(t *testing.T) {
		stdout, stderr, code := runBinaryInDir(t, outputWorkspace(t), "",
			"fix", "-f", "json", "long.md")
		assert.Equal(t, 1, code)
		assert.Empty(t, stdout)
		assert.Equal(t, []string{"MDS001"}, jsonRules(t, stderr))
	})
}

// TestFixOutput_E2EClean pins `[]` on a clean json run on every route,
// including a run that resolves no Markdown file.
func TestFixOutput_E2EClean(t *testing.T) {
	t.Run("clean json, default route", func(t *testing.T) {
		stdout, stderr, code := runBinaryInDir(t, outputWorkspace(t), "",
			"fix", "-f", "json", "ok.md")
		assert.Equal(t, 0, code)
		assert.Empty(t, stdout)
		assert.Equal(t, "[]\n", stderr)
	})
	t.Run("no Markdown file, -o -", func(t *testing.T) {
		dir := outputWorkspace(t)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x\n"), 0o644))
		stdout, stderr, code := runBinaryInDir(t, dir, "",
			"fix", "-o", "-", "-f", "json", "notes.txt")
		assert.Equal(t, 0, code)
		assert.Equal(t, "[]\n", stdout)
		assert.Contains(t, stderr, `mdsmith: skipping "notes.txt": not a Markdown file`)
	})
	t.Run("discovery without Markdown, sarif", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
		stdout, stderr, code := runBinaryInDir(t, dir, "", "fix", "-o", "-", "-f", "sarif")
		assert.Equal(t, 0, code)
		assert.Contains(t, stdout, `"version": "2.1.0"`)
		assert.Empty(t, stderr)
	})
	t.Run("--build-only with no Markdown file creates no report", func(t *testing.T) {
		dir := outputWorkspace(t)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x\n"), 0o644))
		stdout, stderr, code := runBinaryInDir(t, dir, "",
			"fix", "--build-only", "-o", "r.json", "-f", "json", "notes.txt")
		assert.Equal(t, 0, code)
		assert.Empty(t, stdout)
		assert.Contains(t, stderr, `mdsmith: skipping "notes.txt"`)
		assert.NoFileExists(t, filepath.Join(dir, "r.json"))
	})
}

// TestFixOutput_E2EOutputErrors pins that an -o path the report cannot
// be created at is refused before any file is fixed, and that a report
// that fails later is a runtime error: the fixes are already on disk,
// the message is on stderr, and the exit code is 2.
func TestFixOutput_E2EOutputErrors(t *testing.T) {
	t.Run("report under a missing directory is refused before any fix", func(t *testing.T) {
		dir := outputWorkspace(t)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "ws.md"), []byte("# Hi  \n"), 0o644))
		out := filepath.Join("missing", "fix.txt")
		stdout, stderr, code := runBinaryInDir(t, dir, "", "fix", "-o", out, "ws.md")
		assert.Equal(t, 2, code)
		assert.Empty(t, stdout)
		assert.True(t, strings.HasPrefix(stderr, fmt.Sprintf("mdsmith: fix: cannot write --output %q: ", out)),
			"stderr=%q", stderr)
		assert.Equal(t, "# Hi  \n", readReport(t, filepath.Join(dir, "ws.md")), "no file is fixed")
	})
	t.Run("report at a directory is refused before any fix", func(t *testing.T) {
		dir := outputWorkspace(t)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "ws.md"), []byte("# Hi  \n"), 0o644))
		require.NoError(t, os.Mkdir(filepath.Join(dir, "out"), 0o755))
		_, stderr, code := runBinaryInDir(t, dir, "", "fix", "-o", "out", "ws.md")
		assert.Equal(t, 2, code)
		assert.Equal(t, "mdsmith: fix: cannot write --output \"out\": it is a directory\n", stderr)
		assert.Equal(t, "# Hi  \n", readReport(t, filepath.Join(dir, "ws.md")), "no file is fixed")
	})
	t.Run("unwritable report", func(t *testing.T) {
		if _, err := os.Stat("/dev/full"); err != nil {
			t.Skip("no /dev/full on this platform")
		}
		dir := outputWorkspace(t)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "ws.md"), []byte("# Hi  \n"), 0o644))
		stdout, stderr, code := runBinaryInDir(t, dir, "", "fix", "-o", "/dev/full", "ws.md")
		assert.Equal(t, 2, code)
		assert.Empty(t, stdout)
		assert.True(t, strings.HasPrefix(stderr, "mdsmith: error writing output: "), "stderr=%q", stderr)
		assert.Equal(t, "# Hi\n", readReport(t, filepath.Join(dir, "ws.md")), "the fix is written first")
	})
}
