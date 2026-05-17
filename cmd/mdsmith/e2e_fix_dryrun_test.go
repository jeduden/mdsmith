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

// parseDryRunStats parses the stats line that includes would-fix from stderr.
// Expected format: stats: checked=N fixed=N failures=N unfixed=N would-fix=N
func parseDryRunStats(t *testing.T, stderr string) (checked, fixed, failures, unfixed, wouldFix int) {
	t.Helper()
	re := regexp.MustCompile(`stats: checked=(\d+) fixed=(\d+) failures=(\d+) unfixed=(\d+) would-fix=(\d+)`)
	m := re.FindStringSubmatch(stderr)
	require.Len(t, m, 6, "expected dry-run stats line in stderr, got: %s", stderr)

	vals := make([]int, 5)
	for i := 0; i < 5; i++ {
		v, err := strconv.Atoi(m[i+1])
		require.NoError(t, err)
		vals[i] = v
	}
	return vals[0], vals[1], vals[2], vals[3], vals[4]
}

// TestE2E_Fix_DryRun_WritesNothingToDisk verifies that --dry-run leaves every
// candidate file byte-identical after the run.
func TestE2E_Fix_DryRun_WritesNothingToDisk(t *testing.T) {
	dir := t.TempDir()
	isolateDir(t, dir)
	const original = "# Title\n\nHello   \n"
	path := writeFixture(t, dir, "fixme.md", original)

	_, _, exitCode := runBinaryInDir(t, dir, "", "fix", "--dry-run", "--no-color", "fixme.md")
	assert.Equal(t, 0, exitCode, "expected exit code 0 for fully-fixable file")

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, original, string(got), "dry-run must not modify the file on disk")
}

// TestE2E_Fix_DryRun_ExitCodeMatchesRealRun verifies that --dry-run returns
// the same exit code a real fix would return on the same input.
func TestE2E_Fix_DryRun_ExitCodeMatchesRealRun(t *testing.T) {
	// File with a non-fixable violation (MDS001: heading with punctuation stays
	// after fix, remaining as a diagnostic → exit 1).
	// "# Title!" triggers MDS003 heading-punctuation which is not auto-fixable.
	dir := t.TempDir()
	isolateDir(t, dir)
	const content = "# Title!\n\nHello   \n"
	writeFixture(t, dir, "partial.md", content)

	_, _, dryCode := runBinaryInDir(t, dir, "", "fix", "--dry-run", "--no-color", "partial.md")

	// Reset to original content before real run (dry-run must not have changed it).
	writeFixture(t, dir, "partial.md", content)

	_, _, realCode := runBinaryInDir(t, dir, "", "fix", "--no-color", "partial.md")

	assert.Equal(t, realCode, dryCode,
		"dry-run exit code (%d) must match real-run exit code (%d)", dryCode, realCode)
}

// TestE2E_Fix_DryRun_FixCountMatchesRealRun verifies that the would-fix count
// equals the number of violations a real run would auto-fix on the same input.
func TestE2E_Fix_DryRun_FixCountMatchesRealRun(t *testing.T) {
	dir := t.TempDir()
	isolateDir(t, dir)
	const content = "# Title\n\nHello   \n"
	writeFixture(t, dir, "fixme.md", content)

	_, dryStderr, _ := runBinaryInDir(t, dir, "", "fix", "--dry-run", "--no-color", "fixme.md")
	_, _, _, _, wouldFix := parseDryRunStats(t, dryStderr)

	// Reset to original (dry-run must not have changed the file).
	writeFixture(t, dir, "fixme.md", content)

	_, realStderr, _ := runBinaryInDir(t, dir, "", "fix", "--no-color", "fixme.md")
	_, realFixed, _, _ := parseStats(t, realStderr)

	assert.Equal(t, realFixed, wouldFix,
		"would-fix (%d) must equal real fixed (%d)", wouldFix, realFixed)
}

// TestE2E_Fix_DryRun_SummaryLine verifies the stats line on a dry run:
// - includes would-fix=N
// - fixed=0 (nothing written)
// - checked/failures/unfixed are present.
func TestE2E_Fix_DryRun_SummaryLine(t *testing.T) {
	dir := t.TempDir()
	isolateDir(t, dir)
	writeFixture(t, dir, "fixme.md", "# Title\n\nHello   \n")

	_, stderr, _ := runBinaryInDir(t, dir, "", "fix", "--dry-run", "--no-color", "fixme.md")

	checked, fixed, _, _, wouldFix := parseDryRunStats(t, stderr)
	assert.Equal(t, 1, checked, "checked must be 1")
	assert.Equal(t, 0, fixed, "fixed must be 0 on dry-run (nothing written)")
	assert.Greater(t, wouldFix, 0, "would-fix must be > 0 for a file with violations")
}

// TestE2E_Fix_DryRun_PerFileOutput verifies that --dry-run emits a
// "would fix N violations (RuleID ...)" line for files with fixable issues.
func TestE2E_Fix_DryRun_PerFileOutput(t *testing.T) {
	dir := t.TempDir()
	isolateDir(t, dir)
	writeFixture(t, dir, "fixme.md", "# Title\n\nHello   \n")

	_, stderr, _ := runBinaryInDir(t, dir, "", "fix", "--dry-run", "--no-color", "fixme.md")

	assert.Contains(t, stderr, "would fix", "expected 'would fix' in stderr output")
	assert.Contains(t, stderr, "MDS006", "expected rule ID MDS006 in per-file output")
}

// TestE2E_Fix_DryRun_CleanFile_NoOutput verifies that a file with no fixable
// violations produces no "would fix" line (and would-fix=0 in stats).
func TestE2E_Fix_DryRun_CleanFile_NoOutput(t *testing.T) {
	dir := t.TempDir()
	isolateDir(t, dir)
	writeFixture(t, dir, "clean.md", "# Title\n\nSome content here.\n")

	_, stderr, exitCode := runBinaryInDir(t, dir, "", "fix", "--dry-run", "--no-color", "clean.md")
	assert.Equal(t, 0, exitCode)

	assert.NotContains(t, stderr, "would fix", "clean file must not produce a 'would fix' line")
	_, _, _, _, wouldFix := parseDryRunStats(t, stderr)
	assert.Equal(t, 0, wouldFix, "would-fix must be 0 for clean file")
}

// TestE2E_Fix_DryRun_JSON_WouldFixAndRulesFields verifies that --format json
// exposes would_fix and rules per file.
func TestE2E_Fix_DryRun_JSON_WouldFixAndRulesFields(t *testing.T) {
	dir := t.TempDir()
	isolateDir(t, dir)
	writeFixture(t, dir, "fixme.md", "# Title\n\nHello   \n")

	stdout, _, exitCode := runBinaryInDir(t, dir, "", "fix", "--dry-run", "--format", "json", "fixme.md")
	assert.Equal(t, 0, exitCode)

	// JSON output for dry-run goes to stdout, one record per file with fixes.
	require.NotEmpty(t, strings.TrimSpace(stdout), "expected JSON output on stdout")

	var records []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &records),
		"stdout is not valid JSON: %s", stdout)
	require.NotEmpty(t, records, "expected at least one JSON record for the fixed file")

	rec := records[0]
	assert.Contains(t, rec, "path", "JSON record must have 'path' field")
	assert.Contains(t, rec, "would_fix", "JSON record must have 'would_fix' field")
	assert.Contains(t, rec, "rules", "JSON record must have 'rules' field")

	wouldFix, ok := rec["would_fix"].(float64)
	assert.True(t, ok, "would_fix must be a number")
	assert.Greater(t, wouldFix, float64(0), "would_fix must be > 0")

	rulesVal, ok := rec["rules"].([]any)
	assert.True(t, ok, "rules must be an array")
	assert.NotEmpty(t, rulesVal, "rules must not be empty")

	// MDS006 should appear in the rules list.
	found := false
	for _, r := range rulesVal {
		if s, ok := r.(string); ok && s == "MDS006" {
			found = true
		}
	}
	assert.True(t, found, "MDS006 must appear in the rules list")
}

// TestE2E_Fix_DryRun_RealRunSummaryLacksWouldFix verifies that a real (non-dry)
// run does NOT include the would-fix= field in its stats line.
func TestE2E_Fix_DryRun_RealRunSummaryLacksWouldFix(t *testing.T) {
	dir := t.TempDir()
	isolateDir(t, dir)
	writeFixture(t, dir, "fixme.md", "# Title\n\nHello   \n")

	_, stderr, _ := runBinaryInDir(t, dir, "", "fix", "--no-color", "fixme.md")

	assert.NotContains(t, stderr, "would-fix=",
		"real fix run must not emit would-fix= in stats")
}
