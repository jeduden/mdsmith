package main_test

import (
	"encoding/json"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// parseDryRunStats extracts the dry-run stats line from stderr.
// The line has the form:
//
//	stats: checked=N fixed=0 failures=N unfixed=N would-fix=N
func parseDryRunStats(t *testing.T, stderr string) (checked, fixed, failures, unfixed, wouldFix int) {
	t.Helper()
	re := regexp.MustCompile(
		`stats: checked=(\d+) fixed=(\d+) failures=(\d+) unfixed=(\d+) would-fix=(\d+)`,
	)
	m := re.FindStringSubmatch(stderr)
	require.Len(t, m, 6, "expected dry-run stats line in stderr, got: %s", stderr)

	vals := make([]int, 5)
	for i := range vals {
		v, err := strconv.Atoi(m[i+1])
		require.NoError(t, err)
		vals[i] = v
	}
	return vals[0], vals[1], vals[2], vals[3], vals[4]
}

func TestE2E_Fix_DryRun_WritesNothingToDisk(t *testing.T) {
	dir := t.TempDir()
	isolateDir(t, dir)
	original := "# Title\n\nHello   \n"
	path := writeFixture(t, dir, "test.md", original)

	_, _, exitCode := runBinaryInDir(t, dir, "", "fix", "--dry-run", "--no-color", "test.md")
	assert.Equal(t, 0, exitCode, "expected exit code 0, got %d", exitCode)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, original, string(got), "dry-run must not modify file on disk")
}

func TestE2E_Fix_DryRun_StatsLine(t *testing.T) {
	dir := t.TempDir()
	isolateDir(t, dir)
	writeFixture(t, dir, "test.md", "# Title\n\nHello   \n")

	_, stderr, exitCode := runBinaryInDir(t, dir, "", "fix", "--dry-run", "--no-color", "test.md")
	assert.Equal(t, 0, exitCode, "expected exit code 0, got %d; stderr: %s", exitCode, stderr)

	checked, fixed, _, _, wouldFix := parseDryRunStats(t, stderr)
	assert.Equal(t, 1, checked, "expected checked=1")
	assert.Equal(t, 0, fixed, "expected fixed=0 on dry-run")
	assert.Greater(t, wouldFix, 0, "expected would-fix > 0")
}

func TestE2E_Fix_DryRun_PerFileTextOutput(t *testing.T) {
	dir := t.TempDir()
	isolateDir(t, dir)
	writeFixture(t, dir, "test.md", "# Title\n\nHello   \n")

	_, stderr, exitCode := runBinaryInDir(t, dir, "", "fix", "--dry-run", "--no-color", "test.md")
	assert.Equal(t, 0, exitCode, "got %d; stderr: %s", exitCode, stderr)

	assert.Contains(t, stderr, "would fix", "expected per-file 'would fix' line in stderr")
	assert.Contains(t, stderr, "test.md", "expected filename in per-file output")
}

func TestE2E_Fix_DryRun_ExitCodeMatchesRealRun(t *testing.T) {
	// A file with both fixable and non-fixable issues: real run exits 1 (unfixable
	// remain); dry-run must also exit 1.
	dir := t.TempDir()
	isolateDir(t, dir)
	// "# Title!" has a trailing punctuation (non-fixable); trailing spaces are fixable.
	writeFixture(t, dir, "test.md", "# Title!\n\nHello   \n")

	_, _, realExit := runBinaryInDir(t, dir, "", "fix", "--no-color", "test.md")
	// Reset the file because real run modified it.
	writeFixture(t, dir, "test.md", "# Title!\n\nHello   \n")

	_, _, dryExit := runBinaryInDir(t, dir, "", "fix", "--dry-run", "--no-color", "test.md")

	assert.Equal(t, realExit, dryExit,
		"dry-run exit code %d should match real-run exit code %d", dryExit, realExit)
}

func TestE2E_Fix_DryRun_JSONOutput(t *testing.T) {
	dir := t.TempDir()
	isolateDir(t, dir)
	writeFixture(t, dir, "test.md", "# Title\n\nHello   \n")

	stdout, _, exitCode := runBinaryInDir(
		t, dir, "",
		"fix", "--dry-run", "--format", "json", "test.md",
	)
	assert.Equal(t, 0, exitCode, "expected exit code 0, got %d; stdout: %s", exitCode, stdout)

	// Stdout must be a JSON array.
	var records []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &records),
		"stdout must be valid JSON array; got: %s", stdout)
	require.Len(t, records, 1, "expected 1 JSON record for 1 file; got: %v", records)

	rec := records[0]
	_, hasPath := rec["path"]
	assert.True(t, hasPath, "record missing 'path' field: %v", rec)
	wouldFix, ok := rec["would_fix"]
	assert.True(t, ok, "record missing 'would_fix' field: %v", rec)
	assert.Greater(t, wouldFix.(float64), float64(0), "expected would_fix > 0; got: %v", wouldFix)
	rules, hasRules := rec["rules"]
	assert.True(t, hasRules, "record missing 'rules' field: %v", rec)
	rulesSlice, ok := rules.([]any)
	assert.True(t, ok, "expected 'rules' to be an array; got: %T", rules)
	assert.NotEmpty(t, rulesSlice, "expected at least one rule in 'rules'; got empty")
	_, hasDiags := rec["diagnostics"]
	assert.True(t, hasDiags, "record missing 'diagnostics' field: %v", rec)
}

func TestE2E_Fix_DryRun_CleanFileNotReported(t *testing.T) {
	dir := t.TempDir()
	isolateDir(t, dir)
	writeFixture(t, dir, "clean.md", "# Title\n\nSome content here.\n")

	_, stderr, exitCode := runBinaryInDir(t, dir, "", "fix", "--dry-run", "--no-color", "clean.md")
	assert.Equal(t, 0, exitCode, "expected exit code 0, got %d", exitCode)
	assert.NotContains(t, stderr, "would fix", "clean file should not appear in per-file output")

	checked, fixed, _, _, wouldFix := parseDryRunStats(t, stderr)
	assert.Equal(t, 1, checked, "expected checked=1")
	assert.Equal(t, 0, fixed, "expected fixed=0")
	assert.Equal(t, 0, wouldFix, "expected would-fix=0 for a clean file")
}

func TestE2E_Fix_DryRun_PerFileRuleNames(t *testing.T) {
	dir := t.TempDir()
	isolateDir(t, dir)
	// Trailing spaces trigger a fixable rule.
	writeFixture(t, dir, "test.md", "# Title\n\nHello   \n")

	_, stderr, _ := runBinaryInDir(t, dir, "", "fix", "--dry-run", "--no-color", "test.md")

	// The per-file line should name at least one rule ID (MDS-style).
	assert.Regexp(t, `MDS\d+`, stderr,
		"expected at least one rule ID in per-file output; stderr: %s", stderr)
}

func TestE2E_Fix_NonDryRun_NoWouldFixInStats(t *testing.T) {
	dir := t.TempDir()
	isolateDir(t, dir)
	writeFixture(t, dir, "test.md", "# Title\n\nHello   \n")

	_, stderr, _ := runBinaryInDir(t, dir, "", "fix", "--no-color", "test.md")

	assert.NotContains(t, stderr, "would-fix",
		"real run stats should not include 'would-fix'; stderr: %s", stderr)

	// The real-run line still has the four standard fields.
	re := regexp.MustCompile(`stats: checked=\d+ fixed=\d+ failures=\d+ unfixed=\d+`)
	assert.Regexp(t, re, stderr, "expected standard stats line; stderr: %s", stderr)
}

func TestE2E_Fix_DryRun_JSONEmptyArrayForCleanFiles(t *testing.T) {
	dir := t.TempDir()
	isolateDir(t, dir)
	writeFixture(t, dir, "clean.md", "# Title\n\nSome content here.\n")

	stdout, _, exitCode := runBinaryInDir(
		t, dir, "",
		"fix", "--dry-run", "--format", "json", "clean.md",
	)
	assert.Equal(t, 0, exitCode)

	// When there are no would-fix files, stdout should be an empty JSON array.
	trimmed := strings.TrimSpace(stdout)
	assert.Equal(t, "[]", trimmed,
		"expected empty JSON array for clean file; got: %s", stdout)
}
