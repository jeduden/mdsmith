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

// parseDryRunStats parses the stats line produced by fix --dry-run.
// Expected format:
//
//	stats: checked=N fixed=N failures=N unfixed=N would-fix=N
func parseDryRunStats(t *testing.T, stderr string) (checked, fixed, failures, unfixed, wouldFix int) {
	t.Helper()
	re := regexp.MustCompile(
		`stats: checked=(\d+) fixed=(\d+) failures=(\d+) unfixed=(\d+) would-fix=(\d+)`)
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

// TestE2E_Fix_DryRun_WritesNothing verifies that --dry-run leaves files byte-identical.
func TestE2E_Fix_DryRun_WritesNothing(t *testing.T) {
	dir := t.TempDir()
	isolateDir(t, dir)

	original := "# Title\n\nHello   \n"
	path := writeFixture(t, dir, "fixme.md", original)

	_, _, exitCode := runBinaryInDir(t, dir, "", "fix", "--dry-run", "--no-color", "fixme.md")
	assert.Equal(t, 0, exitCode, "expected exit code 0, got %d", exitCode)

	// File must be byte-identical to original.
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, original, string(got), "dry-run must not modify files on disk")
}

// TestE2E_Fix_DryRun_ReportsSameCountAsRealRun verifies that the would-fix
// count in dry-run equals the fixed count reported by a real run.
func TestE2E_Fix_DryRun_ReportsSameCountAsRealRun(t *testing.T) {
	dir := t.TempDir()
	isolateDir(t, dir)
	writeFixture(t, dir, "fixme.md", "# Title\n\nHello   \n")

	// Dry-run: capture would-fix count.
	_, dryStderr, dryCode := runBinaryInDir(t, dir, "", "fix", "--dry-run", "--no-color", "fixme.md")
	assert.Equal(t, 0, dryCode, "dry-run exit code; stderr: %s", dryStderr)
	_, _, _, _, wouldFix := parseDryRunStats(t, dryStderr)

	// Make a fresh copy and do a real fix to compare.
	dir2 := t.TempDir()
	isolateDir(t, dir2)
	writeFixture(t, dir2, "fixme.md", "# Title\n\nHello   \n")

	_, realStderr, realCode := runBinaryInDir(t, dir2, "", "fix", "--no-color", "fixme.md")
	assert.Equal(t, 0, realCode, "real-fix exit code; stderr: %s", realStderr)
	_, realFixed, _, _ := parseStats(t, realStderr)

	// would-fix from dry-run must equal fixed from real run for identical input.
	assert.Equal(t, realFixed, wouldFix,
		"dry-run would-fix=%d should equal real-run fixed=%d", wouldFix, realFixed)
}

// TestE2E_Fix_DryRun_ExitCodeMatchesRealRun verifies that exit codes match.
func TestE2E_Fix_DryRun_ExitCodeMatchesRealRun(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"all fixable", "# Title\n\nHello   \n"},
		{"has unfixable", "# Title!\n\nHello   \n"},
		{"clean", "# Title\n\nHello\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Dry-run exit code.
			dir1 := t.TempDir()
			isolateDir(t, dir1)
			writeFixture(t, dir1, "file.md", tc.content)
			_, _, dryCode := runBinaryInDir(t, dir1, "", "fix", "--dry-run", "--no-color", "file.md")

			// Real-run exit code (on separate copy so dry-run hasn't mutated it).
			dir2 := t.TempDir()
			isolateDir(t, dir2)
			writeFixture(t, dir2, "file.md", tc.content)
			_, _, realCode := runBinaryInDir(t, dir2, "", "fix", "--no-color", "file.md")

			assert.Equal(t, realCode, dryCode,
				"dry-run exit code %d should match real-run exit code %d", dryCode, realCode)
		})
	}
}

// TestE2E_Fix_DryRun_SummaryLine verifies the stats line has fixed=0 and would-fix=N.
func TestE2E_Fix_DryRun_SummaryLine(t *testing.T) {
	dir := t.TempDir()
	isolateDir(t, dir)
	writeFixture(t, dir, "fixme.md", "# Title\n\nHello   \n")

	_, stderr, exitCode := runBinaryInDir(t, dir, "", "fix", "--dry-run", "--no-color", "fixme.md")
	assert.Equal(t, 0, exitCode, "exit code; stderr: %s", stderr)

	checked, fixed, _, _, wouldFix := parseDryRunStats(t, stderr)
	assert.Equal(t, 1, checked, "checked=1")
	assert.Equal(t, 0, fixed, "fixed must be 0 on dry-run (nothing written)")
	assert.Greater(t, wouldFix, 0, "would-fix must be positive when file has fixable issues")
}

// TestE2E_Fix_DryRun_PerFileOutput verifies per-file "would fix N violations" line.
func TestE2E_Fix_DryRun_PerFileOutput(t *testing.T) {
	dir := t.TempDir()
	isolateDir(t, dir)
	writeFixture(t, dir, "fixme.md", "# Title\n\nHello   \n")

	_, stderr, exitCode := runBinaryInDir(t, dir, "", "fix", "--dry-run", "--no-color", "fixme.md")
	assert.Equal(t, 0, exitCode, "exit code; stderr: %s", stderr)

	assert.Contains(t, stderr, "would fix", "expected per-file 'would fix' line; stderr: %s", stderr)
}

// TestE2E_Fix_DryRun_JSONOutput_ExposesWouldFixAndRules verifies JSON output shape.
func TestE2E_Fix_DryRun_JSONOutput_ExposesWouldFixAndRules(t *testing.T) {
	dir := t.TempDir()
	isolateDir(t, dir)
	writeFixture(t, dir, "fixme.md", "# Title\n\nHello   \n")

	stdout, stderr, exitCode := runBinaryInDir(
		t, dir, "", "fix", "--dry-run", "--format", "json", "fixme.md")
	assert.Equal(t, 0, exitCode, "exit code; stderr: %s", stderr)

	// JSON output should be a valid array with at least one record
	// that has would_fix and rules fields.
	var records []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &records),
		"could not parse JSON output: %s", stdout)
	require.NotEmpty(t, records, "expected at least one JSON record")

	// Find the record for fixme.md.
	var rec map[string]any
	for _, r := range records {
		if path, ok := r["path"].(string); ok && strings.HasSuffix(path, "fixme.md") {
			rec = r
			break
		}
	}
	require.NotNil(t, rec, "no record for fixme.md in JSON output: %s", stdout)

	wouldFix, ok := rec["would_fix"].(float64)
	assert.True(t, ok, "expected numeric would_fix field; got %T: %v", rec["would_fix"], rec["would_fix"])
	assert.Greater(t, wouldFix, float64(0), "would_fix must be > 0")

	rules, ok := rec["rules"].([]any)
	assert.True(t, ok, "expected array rules field; got %T: %v", rec["rules"], rec["rules"])
	assert.NotEmpty(t, rules, "rules must be non-empty when would_fix > 0")
}

// TestE2E_Fix_DryRun_CleanFile_NoOutput verifies that clean files don't appear in output.
func TestE2E_Fix_DryRun_CleanFile_NoOutput(t *testing.T) {
	dir := t.TempDir()
	isolateDir(t, dir)
	writeFixture(t, dir, "clean.md", "# Title\n\nHello\n")

	_, stderr, exitCode := runBinaryInDir(t, dir, "", "fix", "--dry-run", "--no-color", "clean.md")
	assert.Equal(t, 0, exitCode, "exit code; stderr: %s", stderr)

	assert.NotContains(t, stderr, "would fix",
		"clean file should not appear in dry-run output; stderr: %s", stderr)
}

// TestE2E_Fix_DryRun_JSON_CleanFile_NoWouldFix verifies JSON for clean files.
func TestE2E_Fix_DryRun_JSON_CleanFile_NoWouldFix(t *testing.T) {
	dir := t.TempDir()
	isolateDir(t, dir)
	writeFixture(t, dir, "clean.md", "# Title\n\nHello\n")

	stdout, _, _ := runBinaryInDir(
		t, dir, "", "fix", "--dry-run", "--format", "json", "clean.md")

	var records []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &records),
		"could not parse JSON: %s", stdout)

	// Either empty array or record with would_fix=0.
	for _, r := range records {
		if path, ok := r["path"].(string); ok && strings.HasSuffix(path, "clean.md") {
			wouldFix := r["would_fix"]
			if wf, ok := wouldFix.(float64); ok {
				assert.Equal(t, float64(0), wf, "clean file should have would_fix=0")
			}
		}
	}
}
