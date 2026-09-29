package main_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stdoutWorkspace writes a workspace with one clean file (ok.md, 5
// bytes) and one file with a line-length diagnostic (long.md).
func stdoutWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".mdsmith.yml"),
		[]byte("rules:\n  line-length:\n    max: 30\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ok.md"), []byte("# Hi\n"), 0o644))
	long := "# Title\n\n" + strings.Repeat("x", 60) + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "long.md"), []byte(long), 0o644))
	return dir
}

// jsonRules decodes a `-f json` document and returns each diagnostic's
// rule ID. It fails the test when out is not a JSON array.
func jsonRules(t *testing.T, out string) []string {
	t.Helper()
	var diags []struct {
		Rule string `json:"rule"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &diags), "stdout=%q", out)
	rules := make([]string, 0, len(diags))
	for _, d := range diags {
		rules = append(rules, d.Rule)
	}
	return rules
}

// TestCheckStdout_E2E runs the real binary with --stdout through each
// check entry point (file args, stdin, config discovery) and pins
// which stream each kind of output reaches.
func TestCheckStdout_E2E(t *testing.T) {
	t.Run("file args json on stdout", func(t *testing.T) {
		stdout, stderr, code := runBinaryInDir(t, stdoutWorkspace(t), "",
			"check", "--stdout", "-f", "json", "long.md")
		assert.Equal(t, 1, code)
		assert.Empty(t, stderr)
		assert.Equal(t, []string{"MDS001"}, jsonRules(t, stdout))
	})
	t.Run("clean json is an empty array", func(t *testing.T) {
		stdout, stderr, code := runBinaryInDir(t, stdoutWorkspace(t), "",
			"check", "--stdout", "-f", "json", "ok.md")
		assert.Equal(t, 0, code)
		assert.Empty(t, stderr)
		assert.Equal(t, "[]\n", stdout)
	})
	t.Run("stdin json on stdout", func(t *testing.T) {
		stdout, stderr, code := runBinaryInDir(t, stdoutWorkspace(t),
			"# Title\n\n"+strings.Repeat("y", 60)+"\n",
			"check", "--stdout", "-f", "json", "-")
		assert.Equal(t, 1, code)
		assert.Empty(t, stderr)
		assert.Equal(t, []string{"MDS001"}, jsonRules(t, stdout))
	})
	t.Run("discovery json on stdout", func(t *testing.T) {
		stdout, stderr, code := runBinaryInDir(t, stdoutWorkspace(t), "",
			"check", "--stdout", "-f", "json")
		assert.Equal(t, 1, code)
		assert.Empty(t, stderr)
		assert.Equal(t, []string{"MDS001"}, jsonRules(t, stdout))
	})
	t.Run("runtime error stays on stderr", func(t *testing.T) {
		stdout, stderr, code := runBinaryInDir(t, stdoutWorkspace(t), "",
			"check", "--stdout", "-f", "json", "--max-input-size", "10", "ok.md", "long.md")
		assert.Equal(t, 2, code)
		assert.Equal(t, "[]\n", stdout)
		assert.Contains(t, stderr, `mdsmith: reading "long.md": file too large`)
	})
	t.Run("text diagnostics and stats on stdout", func(t *testing.T) {
		stdout, stderr, code := runBinaryInDir(t, stdoutWorkspace(t), "",
			"check", "--stdout", "--no-color", "long.md")
		assert.Equal(t, 1, code)
		assert.Empty(t, stderr)
		assert.Contains(t, stdout, "long.md:3:31 MDS001 line too long")
		assert.True(t, strings.HasSuffix(stdout,
			"stats: checked=1 fixed=0 failures=1 unfixed=1\n"), "stdout=%q", stdout)
	})
	t.Run("quiet writes nothing", func(t *testing.T) {
		stdout, stderr, code := runBinaryInDir(t, stdoutWorkspace(t), "",
			"check", "--stdout", "-q", "-f", "json", "long.md")
		assert.Equal(t, 1, code)
		assert.Empty(t, stdout)
		assert.Empty(t, stderr)
	})
	t.Run("without the flag json stays on stderr", func(t *testing.T) {
		stdout, stderr, code := runBinaryInDir(t, stdoutWorkspace(t), "",
			"check", "-f", "json", "long.md")
		assert.Equal(t, 1, code)
		assert.Empty(t, stdout)
		assert.Equal(t, []string{"MDS001"}, jsonRules(t, stderr))
	})
}
