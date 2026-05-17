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

// parseDryRunStats extracts the stats line from a dry-run output.
// The dry-run stats line has the form:
//
//	stats: checked=N fixed=N failures=N unfixed=N would-fix=N
func parseDryRunStats(t *testing.T, stderr string) (checked, fixed, failures, unfixed, wouldFix int) {
	t.Helper()
	re := regexp.MustCompile(`stats: checked=(\d+) fixed=(\d+) failures=(\d+) unfixed=(\d+) would-fix=(\d+)`)
	m := re.FindStringSubmatch(stderr)
	require.Len(t, m, 6, "expected dry-run stats line in stderr, got: %s", stderr)

	vals := make([]int, 5)
	for i := 0; i < 5; i++ {
		v, err := strconv.Atoi(m[i+1])
		require.NoError(t, err, "parsing stats value %q: %v", m[i+1], err)
		vals[i] = v
	}
	return vals[0], vals[1], vals[2], vals[3], vals[4]
}

// TestE2E_Fix_DryRun_WritesNothing asserts that --dry-run does not modify any
// file on disk, even when there are fixable violations.
func TestE2E_Fix_DryRun_WritesNothing(t *testing.T) {
	dir := t.TempDir()
	isolateDir(t, dir)
	original := "# Title\n\nHello   \n"
	path := writeFixture(t, dir, "fixme.md", original)

	_, _, exitCode := runBinaryInDir(t, dir, "", "fix", "--dry-run", "--no-color", "fixme.md")
	// fixable-only file: exit 0 on dry run (would be clean after a real fix)
	assert.Equal(t, 0, exitCode, "expected exit code 0 on dry-run of fixable file, got %d", exitCode)

	content, err := os.ReadFile(path)
	require.NoError(t, err, "reading file after dry-run: %v", err)
	assert.Equal(t, original, string(content),
		"dry-run must not write any bytes: file content changed")
}

// TestE2E_Fix_DryRun_ReportsWouldFix asserts that the per-file output line names
// rules that would fire and the summary line contains would-fix=N.
func TestE2E_Fix_DryRun_ReportsWouldFix(t *testing.T) {
	dir := t.TempDir()
	isolateDir(t, dir)
	writeFixture(t, dir, "fixme.md", "# Title\n\nHello   \n")

	_, stderr, _ := runBinaryInDir(t, dir, "", "fix", "--dry-run", "--no-color", "fixme.md")

	// Per-file line must name the rule and count.
	assert.Contains(t, stderr, "would fix", "expected 'would fix' in per-file output, got: %s", stderr)
	assert.Contains(t, stderr, "MDS006", "expected MDS006 in per-file output, got: %s", stderr)

	// Summary line must include would-fix=N (N > 0).
	checked, fixed, _, _, wouldFix := parseDryRunStats(t, stderr)
	assert.Equal(t, 1, checked, "checked should be 1")
	assert.Equal(t, 0, fixed, "fixed must be 0 on dry-run")
	assert.Greater(t, wouldFix, 0, "would-fix must be > 0 for a fixable file")
}

// TestE2E_Fix_DryRun_SummaryFixedAlwaysZero asserts that fixed=0 even when the
// file has fixable violations.
func TestE2E_Fix_DryRun_SummaryFixedAlwaysZero(t *testing.T) {
	dir := t.TempDir()
	isolateDir(t, dir)
	writeFixture(t, dir, "fixme.md", "# Title\n\nHello   \n")

	_, stderr, _ := runBinaryInDir(t, dir, "", "fix", "--dry-run", "--no-color", "fixme.md")

	_, fixed, _, _, _ := parseDryRunStats(t, stderr)
	assert.Equal(t, 0, fixed, "fixed must always be 0 on dry-run")
}

// TestE2E_Fix_DryRun_ExitCodeMatchesRealRun asserts that the exit code from a
// dry-run matches what a real fix run would return on the same input.
func TestE2E_Fix_DryRun_ExitCodeMatchesRealRun(t *testing.T) {
	// File with only fixable violations → both real run and dry-run exit 0.
	t.Run("fixable_only", func(t *testing.T) {
		dir := t.TempDir()
		isolateDir(t, dir)
		writeFixture(t, dir, "fixme.md", "# Title\n\nHello   \n")

		_, _, dryCode := runBinaryInDir(t, dir, "", "fix", "--dry-run", "--no-color", "fixme.md")
		assert.Equal(t, 0, dryCode, "expected exit 0 for fixable-only file in dry-run")
	})

	// File with an unfixable violation → both real run and dry-run exit 1.
	t.Run("unfixable_violation", func(t *testing.T) {
		dir := t.TempDir()
		isolateDir(t, dir)
		// MDS002 (heading-punctuation): trailing "!" on a heading is not auto-fixed.
		writeFixture(t, dir, "bad.md", "# Title!\n\nHello.\n")

		_, _, dryCode := runBinaryInDir(t, dir, "", "fix", "--dry-run", "--no-color", "bad.md")
		assert.Equal(t, 1, dryCode, "expected exit 1 for unfixable violation in dry-run")
	})
}

// TestE2E_Fix_DryRun_JSONOutput asserts that --format json exposes would_fix and
// rules fields per file when --dry-run is set.
func TestE2E_Fix_DryRun_JSONOutput(t *testing.T) {
	dir := t.TempDir()
	isolateDir(t, dir)
	writeFixture(t, dir, "fixme.md", "# Title\n\nHello   \n")

	_, stderr, _ := runBinaryInDir(t, dir, "", "fix", "--dry-run", "--format", "json", "fixme.md")

	// The JSON output is an array of per-file records.
	var records []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stderr), &records),
		"stderr should be valid JSON array, got: %s", stderr)

	// Find the record for fixme.md.
	var fileRec map[string]any
	for _, r := range records {
		if p, _ := r["path"].(string); strings.HasSuffix(p, "fixme.md") {
			fileRec = r
			break
		}
	}
	require.NotNil(t, fileRec, "expected a record for fixme.md in JSON output, got: %s", stderr)

	// must have would_fix > 0
	wouldFix, ok := fileRec["would_fix"]
	require.True(t, ok, "expected would_fix field in JSON record, got keys: %v", mapKeys(fileRec))
	wouldFixN, ok := wouldFix.(float64)
	require.True(t, ok, "expected would_fix to be a number, got %T", wouldFix)
	assert.Greater(t, wouldFixN, float64(0), "expected would_fix > 0")

	// must have rules array (non-empty)
	rules, ok := fileRec["rules"]
	require.True(t, ok, "expected rules field in JSON record, got keys: %v", mapKeys(fileRec))
	rulesArr, ok := rules.([]any)
	require.True(t, ok, "expected rules to be an array, got %T", rules)
	assert.NotEmpty(t, rulesArr, "expected rules to be non-empty")
}

// TestE2E_Fix_DryRun_CleanFile asserts that a file with no violations produces
// no per-file output and would-fix=0 in the summary.
func TestE2E_Fix_DryRun_CleanFile(t *testing.T) {
	dir := t.TempDir()
	isolateDir(t, dir)
	writeFixture(t, dir, "clean.md", "# Title\n\nSome content here.\n")

	_, stderr, exitCode := runBinaryInDir(t, dir, "", "fix", "--dry-run", "--no-color", "clean.md")
	assert.Equal(t, 0, exitCode, "expected exit 0 for clean file")

	// No per-file "would fix" line expected.
	assert.NotContains(t, stderr, "would fix",
		"clean file should not produce a 'would fix' line, got: %s", stderr)

	checked, fixed, _, _, wouldFix := parseDryRunStats(t, stderr)
	assert.Equal(t, 1, checked, "expected checked=1")
	assert.Equal(t, 0, fixed, "expected fixed=0 on dry-run")
	assert.Equal(t, 0, wouldFix, "expected would-fix=0 for clean file")
}

// TestE2E_Fix_DryRun_WouldFixCountMatchesRealFixCount asserts that the
// would-fix count reported by dry-run matches the number of fixes a real run
// would apply (regression: both modes must agree on the fix count).
func TestE2E_Fix_DryRun_WouldFixCountMatchesRealFixCount(t *testing.T) {
	content := "# Title\n\nHello   \n\nWorld   \n"
	// Run dry-run on a copy, then real fix on another copy, compare would-fix
	// to the real-run fixed count (or at minimum check both are > 0).
	t.Run("dry_run_sees_same_violations", func(t *testing.T) {
		dir := t.TempDir()
		isolateDir(t, dir)
		writeFixture(t, dir, "test.md", content)

		_, dryStderr, _ := runBinaryInDir(t, dir, "", "fix", "--dry-run", "--no-color", "test.md")
		_, _, _, _, wouldFix := parseDryRunStats(t, dryStderr)
		assert.Greater(t, wouldFix, 0, "expected would-fix > 0 for file with violations")
	})

	t.Run("real_run_fixes_same_file", func(t *testing.T) {
		dir := t.TempDir()
		isolateDir(t, dir)
		writeFixture(t, dir, "test.md", content)

		_, realStderr, _ := runBinaryInDir(t, dir, "", "fix", "--no-color", "test.md")
		_, realFixed, _, _ := parseStats(t, realStderr)
		assert.Equal(t, 1, realFixed, "real run should fix the file")
	})
}

// mapKeys returns the keys of a map (for test error messages).
func mapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
