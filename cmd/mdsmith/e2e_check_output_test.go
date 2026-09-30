package main_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// outputWorkspace writes a workspace with one clean file (ok.md, 5
// bytes) and one file with a line-length diagnostic (long.md).
func outputWorkspace(t *testing.T) string {
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
	require.NoError(t, json.Unmarshal([]byte(out), &diags), "report=%q", out)
	rules := make([]string, 0, len(diags))
	for _, d := range diags {
		rules = append(rules, d.Rule)
	}
	return rules
}

// readReport returns the content of a report file the binary wrote.
func readReport(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(b)
}

// TestCheckOutput_E2E runs the real binary with -o through each check
// entry point (file args, stdin, config discovery) and pins where the
// JSON document lands: -o - on stdout, -o <path> in the file, and the
// default on stderr.
func TestCheckOutput_E2E(t *testing.T) {
	t.Run("file args, -o -", func(t *testing.T) {
		stdout, stderr, code := runBinaryInDir(t, outputWorkspace(t), "",
			"check", "-o", "-", "-f", "json", "long.md")
		assert.Equal(t, 1, code)
		assert.Empty(t, stderr)
		assert.Equal(t, []string{"MDS001"}, jsonRules(t, stdout))
	})
	t.Run("stdin, -o -", func(t *testing.T) {
		stdout, stderr, code := runBinaryInDir(t, outputWorkspace(t),
			"# Title\n\n"+strings.Repeat("y", 60)+"\n",
			"check", "--output", "-", "-f", "json", "-")
		assert.Equal(t, 1, code)
		assert.Empty(t, stderr)
		assert.Equal(t, []string{"MDS001"}, jsonRules(t, stdout))
	})
	t.Run("discovery, -o path", func(t *testing.T) {
		dir := outputWorkspace(t)
		stdout, stderr, code := runBinaryInDir(t, dir, "",
			"check", "-o", "report.json", "-f", "json")
		assert.Equal(t, 1, code)
		assert.Empty(t, stdout)
		assert.Empty(t, stderr)
		assert.Equal(t, []string{"MDS001"}, jsonRules(t, readReport(t, filepath.Join(dir, "report.json"))))
	})
	t.Run("default is stderr", func(t *testing.T) {
		stdout, stderr, code := runBinaryInDir(t, outputWorkspace(t), "",
			"check", "-f", "json", "long.md")
		assert.Equal(t, 1, code)
		assert.Empty(t, stdout)
		assert.Equal(t, []string{"MDS001"}, jsonRules(t, stderr))
	})
	t.Run("--stdout is gone", func(t *testing.T) {
		_, stderr, code := runBinaryInDir(t, outputWorkspace(t), "",
			"check", "--stdout", "long.md")
		assert.Equal(t, 2, code)
		assert.Contains(t, stderr, "unknown flag: --stdout")
	})
}

// TestCheckOutput_E2ECleanRun pins that a clean json run writes `[]`
// on every route, and that -o <path> truncates an existing file.
func TestCheckOutput_E2ECleanRun(t *testing.T) {
	t.Run("default route", func(t *testing.T) {
		stdout, stderr, code := runBinaryInDir(t, outputWorkspace(t), "",
			"check", "-f", "json", "ok.md")
		assert.Equal(t, 0, code)
		assert.Empty(t, stdout)
		assert.Equal(t, "[]\n", stderr)
	})
	t.Run("-o -", func(t *testing.T) {
		stdout, stderr, code := runBinaryInDir(t, outputWorkspace(t), "",
			"check", "-o", "-", "-f", "json", "ok.md")
		assert.Equal(t, 0, code)
		assert.Empty(t, stderr)
		assert.Equal(t, "[]\n", stdout)
	})
	t.Run("-o path truncates", func(t *testing.T) {
		dir := outputWorkspace(t)
		report := filepath.Join(dir, "report.json")
		require.NoError(t, os.WriteFile(report, []byte(strings.Repeat("stale\n", 20)), 0o644))
		stdout, stderr, code := runBinaryInDir(t, dir, "",
			"check", "-o", "report.json", "-f", "json", "ok.md")
		assert.Equal(t, 0, code)
		assert.Empty(t, stdout)
		assert.Empty(t, stderr)
		assert.Equal(t, "[]\n", readReport(t, report))
	})
	t.Run("sarif default route", func(t *testing.T) {
		_, stderr, code := runBinaryInDir(t, outputWorkspace(t), "",
			"check", "-f", "sarif", "ok.md")
		assert.Equal(t, 0, code)
		assert.Contains(t, stderr, `"version": "2.1.0"`)
	})
}

// TestCheckOutput_E2EStreams pins which stream each kind of output
// reaches off the default route: runtime errors, the text stats line,
// the non-Markdown skip warning, and -q.
func TestCheckOutput_E2EStreams(t *testing.T) {
	t.Run("runtime error stays on stderr", func(t *testing.T) {
		stdout, stderr, code := runBinaryInDir(t, outputWorkspace(t), "",
			"check", "-o", "-", "-f", "json", "--max-input-size", "10", "ok.md", "long.md")
		assert.Equal(t, 2, code)
		assert.Equal(t, "[]\n", stdout)
		assert.Contains(t, stderr, `mdsmith: reading "long.md": file too large`)
	})
	t.Run("text diagnostics and stats on stdout", func(t *testing.T) {
		stdout, stderr, code := runBinaryInDir(t, outputWorkspace(t), "",
			"check", "-o", "-", "long.md")
		assert.Equal(t, 1, code)
		assert.Empty(t, stderr)
		assert.Contains(t, stdout, "long.md:3:31 MDS001 line too long")
		assert.True(t, strings.HasSuffix(stdout,
			"stats: checked=1 fixed=0 failures=1 unfixed=1\n"), "stdout=%q", stdout)
	})
	t.Run("skip warning shows once json leaves stderr", func(t *testing.T) {
		dir := outputWorkspace(t)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x\n"), 0o644))
		stdout, stderr, code := runBinaryInDir(t, dir, "",
			"check", "-o", "-", "-f", "json", "ok.md", "notes.txt")
		assert.Equal(t, 0, code)
		assert.Equal(t, "[]\n", stdout)
		assert.Contains(t, stderr, `mdsmith: skipping "notes.txt": not a Markdown file`)

		_, stderr, code = runBinaryInDir(t, dir, "", "check", "-f", "json", "ok.md", "notes.txt")
		assert.Equal(t, 0, code)
		assert.Equal(t, "[]\n", stderr, "no prose warning inside json on stderr")

		// An unknown -f value renders text, so the warning shows.
		_, stderr, code = runBinaryInDir(t, dir, "", "check", "-f", "xml", "ok.md", "notes.txt")
		assert.Equal(t, 0, code)
		assert.Contains(t, stderr, `mdsmith: skipping "notes.txt": not a Markdown file`)
	})
	t.Run("quiet silences the terminal, not an -o file", func(t *testing.T) {
		dir := outputWorkspace(t)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x\n"), 0o644))
		stdout, stderr, code := runBinaryInDir(t, dir, "",
			"check", "-o", "-", "-q", "-f", "json", "long.md")
		assert.Equal(t, 1, code)
		assert.Empty(t, stdout)
		assert.Empty(t, stderr)

		stdout, stderr, code = runBinaryInDir(t, dir, "",
			"check", "-o", "report.txt", "-q", "long.md", "notes.txt")
		assert.Equal(t, 1, code)
		assert.Empty(t, stdout)
		assert.Empty(t, stderr, "-q still drops the skip warning")
		report := readReport(t, filepath.Join(dir, "report.txt"))
		assert.Contains(t, report, "long.md:3:31 MDS001 line too long")
		assert.True(t, strings.HasSuffix(report,
			"stats: checked=1 fixed=0 failures=1 unfixed=1\n"), "report=%q", report)
	})
}

// TestCheckOutput_E2EWriteErrors pins that an -o path the report
// cannot be created at is a usage error before any file is linted,
// and that a report that cannot be written is a runtime error: a
// message on stderr, exit 2.
func TestCheckOutput_E2EWriteErrors(t *testing.T) {
	t.Run("missing directory", func(t *testing.T) {
		out := filepath.Join("missing", "report.json")
		stdout, stderr, code := runBinaryInDir(t, outputWorkspace(t), "", "check", "-o", out, "long.md")
		assert.Equal(t, 2, code)
		assert.Empty(t, stdout)
		assert.True(t, strings.HasPrefix(stderr, fmt.Sprintf("mdsmith: check: cannot write --output %q: ", out)),
			"stderr=%q", stderr)
		assert.NotContains(t, stderr, "MDS001", "refused before linting")
	})
	t.Run("path is a directory", func(t *testing.T) {
		dir := outputWorkspace(t)
		require.NoError(t, os.Mkdir(filepath.Join(dir, "out"), 0o755))
		_, stderr, code := runBinaryInDir(t, dir, "", "check", "-o", "out", "ok.md")
		assert.Equal(t, 2, code)
		assert.Equal(t, "mdsmith: check: cannot write --output \"out\": it is a directory\n", stderr)
	})
	t.Run("stdin, missing directory", func(t *testing.T) {
		out := filepath.Join("missing", "report.json")
		_, stderr, code := runBinaryInDir(t, outputWorkspace(t), "# Hi\n", "check", "-o", out, "-")
		assert.Equal(t, 2, code)
		assert.True(t, strings.HasPrefix(stderr, fmt.Sprintf("mdsmith: check: cannot write --output %q: ", out)),
			"stderr=%q", stderr)
	})
	t.Run("empty path", func(t *testing.T) {
		_, stderr, code := runBinaryInDir(t, outputWorkspace(t), "", "check", "-o", "", "ok.md")
		assert.Equal(t, 2, code)
		assert.Equal(t, "mdsmith: check: --output needs a path, or - for stdout\n", stderr)
	})
	t.Run("full device", func(t *testing.T) {
		if _, err := os.Stat("/dev/full"); err != nil {
			t.Skip("no /dev/full on this platform")
		}
		_, stderr, code := runBinaryInDir(t, outputWorkspace(t), "",
			"check", "-o", "/dev/full", "-f", "json", "ok.md")
		assert.Equal(t, 2, code)
		assert.Equal(t, "mdsmith: error writing output: write /dev/full: no space left on device\n", stderr)
	})
}

// TestCheckOutput_E2ENoFiles pins the clean runs that resolve no
// Markdown file at all: a named non-Markdown file, a directory with no
// Markdown in it, and config discovery that finds nothing. They exit 0
// before any file is linted, and json or sarif still writes a valid
// document on every route.
func TestCheckOutput_E2ENoFiles(t *testing.T) {
	noMarkdown := func(t *testing.T) string {
		t.Helper()
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "empty"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x\n"), 0o644))
		return dir
	}
	t.Run("named non-Markdown file, -o -", func(t *testing.T) {
		stdout, stderr, code := runBinaryInDir(t, noMarkdown(t), "",
			"check", "-o", "-", "-f", "json", "notes.txt")
		assert.Equal(t, 0, code)
		assert.Equal(t, "[]\n", stdout)
		assert.Contains(t, stderr, `mdsmith: skipping "notes.txt": not a Markdown file`)
	})
	t.Run("directory without Markdown, default route", func(t *testing.T) {
		stdout, stderr, code := runBinaryInDir(t, noMarkdown(t), "",
			"check", "-f", "json", "empty")
		assert.Equal(t, 0, code)
		assert.Empty(t, stdout)
		assert.Equal(t, "[]\n", stderr)
	})
	t.Run("discovery without Markdown, sarif to a file", func(t *testing.T) {
		dir := noMarkdown(t)
		stdout, stderr, code := runBinaryInDir(t, dir, "",
			"check", "-o", "report.sarif", "-f", "sarif")
		assert.Equal(t, 0, code)
		assert.Empty(t, stdout)
		assert.Empty(t, stderr)
		assert.Contains(t, readReport(t, filepath.Join(dir, "report.sarif")), `"version": "2.1.0"`)
	})
	t.Run("text writes nothing", func(t *testing.T) {
		stdout, stderr, code := runBinaryInDir(t, noMarkdown(t), "",
			"check", "-o", "-", "empty")
		assert.Equal(t, 0, code)
		assert.Empty(t, stdout)
		assert.Empty(t, stderr)
	})
}

// TestCheckOutput_E2ENoColorOffTerminal pins that text output carries
// no ANSI color when its destination is not a terminal, with no
// --no-color: here a pipe (the default stderr route) and a file. The
// terminal case is in e2e_color_linux_test.go.
func TestCheckOutput_E2ENoColorOffTerminal(t *testing.T) {
	dir := outputWorkspace(t)
	_, stderr, code := runBinaryInDir(t, dir, "", "check", "long.md")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "MDS001")
	assert.NotContains(t, stderr, "\033[")

	_, _, code = runBinaryInDir(t, dir, "", "check", "-o", "report.txt", "long.md")
	assert.Equal(t, 1, code)
	report := readReport(t, filepath.Join(dir, "report.txt"))
	assert.Contains(t, report, "MDS001")
	assert.NotContains(t, report, "\033[")
}
