package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// feedStdin replaces os.Stdin with a pipe carrying s for the
// duration of the test, restoring the real stdin on cleanup.
func feedStdin(t *testing.T, s string) {
	t.Helper()
	old := os.Stdin
	r, w, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() { os.Stdin = old })
	os.Stdin = r
	go func() {
		_, _ = io.WriteString(w, s)
		_ = w.Close()
	}()
}

// writeMiniModule lays down a one-package module whose single unit
// test matches the miniStream event below.
func writeMiniModule(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"),
		[]byte("module example.com/m\n\ngo 1.24\n"), 0o644))
	foo := filepath.Join(root, "foo")
	require.NoError(t, os.MkdirAll(foo, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(foo, "foo_test.go"),
		[]byte("package foo\nimport \"testing\"\nfunc TestU(t *testing.T) {}\n"), 0o644))
	return root
}

const miniStream = `{"Action":"pass","Package":"example.com/m/foo","Test":"TestU"}` + "\n"

// chdir changes into dir for the test, restoring the prior working
// directory on cleanup.
func chdir(t *testing.T, dir string) {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(wd) })
	require.NoError(t, os.Chdir(dir))
}

// TestRunTestSummaryWritesStepSummary dispatches `run test-summary`
// with a json stream on stdin and GITHUB_STEP_SUMMARY set, asserting
// the count table lands in the summary file.
func TestRunTestSummaryWritesStepSummary(t *testing.T) {
	chdir(t, writeMiniModule(t))
	summary := filepath.Join(t.TempDir(), "summary.md")
	t.Setenv("GITHUB_STEP_SUMMARY", summary)
	feedStdin(t, miniStream)

	assert.Equal(t, 0, run([]string{"test-summary"}))

	data, err := os.ReadFile(summary)
	require.NoError(t, err)
	assert.Contains(t, string(data), "## Test suite")
	assert.Contains(t, string(data), "| Unit | 1 | 1 |")
}

// TestRunTestSummaryPrintsTableWithoutCI covers the branch taken when
// GITHUB_STEP_SUMMARY is unset: the table is printed to stdout.
func TestRunTestSummaryPrintsTableWithoutCI(t *testing.T) {
	chdir(t, writeMiniModule(t))
	t.Setenv("GITHUB_STEP_SUMMARY", "")
	feedStdin(t, miniStream)

	old := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	code := run([]string{"test-summary"})
	_ = w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)

	assert.Equal(t, 0, code)
	assert.Contains(t, buf.String(), "## Test suite")
	assert.Contains(t, buf.String(), "| Unit | 1 | 1 |")
}

// TestRunTestSummaryRejectsArgs covers the arity guard (positional
// args are not accepted; input arrives on stdin).
func TestRunTestSummaryRejectsArgs(t *testing.T) {
	assert.Equal(t, 2, run([]string{"test-summary", "extra"}))
}

// TestRunTestSummaryReportsError covers the reportError branch: with
// no go.mod in cwd the source scan fails and the subcommand exits 1.
func TestRunTestSummaryReportsError(t *testing.T) {
	chdir(t, t.TempDir())
	t.Setenv("GITHUB_STEP_SUMMARY", "")
	feedStdin(t, miniStream)
	assert.Equal(t, 1, run([]string{"test-summary"}))
}

// TestRunTestSummaryAppendError covers the appendFile error branch:
// GITHUB_STEP_SUMMARY points under a regular file, so the summary
// cannot be written and the subcommand exits 1.
func TestRunTestSummaryAppendError(t *testing.T) {
	chdir(t, writeMiniModule(t))
	blocker := filepath.Join(t.TempDir(), "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o644))
	t.Setenv("GITHUB_STEP_SUMMARY", filepath.Join(blocker, "sub", "s.md"))
	feedStdin(t, miniStream)
	assert.Equal(t, 1, run([]string{"test-summary"}))
}

func TestAppendFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f.md")
	require.NoError(t, appendFile(p, "a"))
	require.NoError(t, appendFile(p, "b"))
	data, err := os.ReadFile(p)
	require.NoError(t, err)
	assert.Equal(t, "ab", string(data))

	// A path that cannot be created surfaces an error.
	blocker := filepath.Join(t.TempDir(), "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o644))
	assert.Error(t, appendFile(filepath.Join(blocker, "sub", "f.md"), "data"))
}
