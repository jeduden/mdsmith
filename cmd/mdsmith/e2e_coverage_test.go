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

// =============================================================
// 1. queryFiles — verbose mode: "expression not satisfied"
// =============================================================

func TestE2E_Query_Verbose_ExpressionNotSatisfied(t *testing.T) {
	dir := t.TempDir()
	// File has front matter but does not match the expression.
	writeFixture(t, dir, "miss.md", "---\nstatus: \"🔲\"\n---\n# Miss\n\nContent here.\n")

	// Pass the file explicitly (not the dir) to avoid gitignore filtering.
	missPath := filepath.Join(dir, "miss.md")
	_, stderr, exitCode := runBinaryInDir(t, dir, "", "query", "-v", `status: "✅"`, missPath)
	assert.Equal(t, 1, exitCode, "expected exit 1 when no files match")
	assert.Contains(t, stderr, "expression not satisfied",
		"expected 'expression not satisfied' in verbose stderr, got: %s", stderr)
}

// =============================================================
// 2. runInit — extra positional arguments (exits 2)
// =============================================================

func TestE2E_Init_ExtraArgs_ExitsTwo(t *testing.T) {
	dir := t.TempDir()

	_, stderr, exitCode := runBinaryInDir(t, dir, "", "init", "extra-arg")
	assert.Equal(t, 2, exitCode, "expected exit code 2 for init with extra args, got %d", exitCode)
	assert.Contains(t, stderr, "no arguments",
		"expected 'no arguments' error, got: %s", stderr)
}

// =============================================================
// 4. printErrors — check on non-existent file triggers errors
// =============================================================

func TestE2E_Check_NonExistentFile_ExitsTwo(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist.md")
	_, stderr, exitCode := runBinary(t, "", "check", "--no-color", missing)
	assert.Equal(t, 2, exitCode, "expected exit code 2 for non-existent file, got %d", exitCode)
	assert.Contains(t, stderr, "mdsmith:",
		"expected error message in stderr, got: %s", stderr)
}

func TestE2E_Fix_NonExistentFile_ExitsTwo(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist.md")
	_, stderr, exitCode := runBinary(t, "", "fix", "--no-color", missing)
	assert.Equal(t, 2, exitCode, "expected exit code 2 for non-existent file, got %d", exitCode)
	assert.Contains(t, stderr, "mdsmith:",
		"expected error message in stderr, got: %s", stderr)
}

// =============================================================
// 5. checkFiles — bad config via --config
// =============================================================

func TestE2E_Check_BadConfig_ExitsTwo(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "test.md", "# Title\n\nContent here.\n")
	badConfig := writeFixture(t, dir, "bad.yml", "{{invalid yaml content")

	_, stderr, exitCode := runBinary(t, "", "check", "--config", badConfig, filepath.Join(dir, "test.md"))
	assert.Equal(t, 2, exitCode, "expected exit code 2 for bad config, got %d", exitCode)
	assert.Contains(t, stderr, "mdsmith:",
		"expected error in stderr for bad config, got: %s", stderr)
}

// (duplicate of TestE2E_Check_NonExistentFile_ExitsTwo removed)

// =============================================================
// 6. fixFiles — bad config via --config
// =============================================================

func TestE2E_Fix_BadConfig_ExitsTwo(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "test.md", "# Title\n\nContent here.\n")
	badConfig := writeFixture(t, dir, "bad.yml", "{{invalid yaml content")

	_, stderr, exitCode := runBinary(t, "", "fix", "--config", badConfig, filepath.Join(dir, "test.md"))
	assert.Equal(t, 2, exitCode, "expected exit code 2 for bad config, got %d", exitCode)
	assert.Contains(t, stderr, "mdsmith:",
		"expected error in stderr for bad config, got: %s", stderr)
}

// (duplicate of TestE2E_Fix_NonExistentFile_ExitsTwo removed)

// =============================================================
// 7. checkStdin — bad config path
// =============================================================

func TestE2E_Check_Stdin_BadConfig_ExitsTwo(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "# Title\n\nContent.\n",
		"check", "--config", "/no/such/config.yml", "-")
	assert.Equal(t, 2, exitCode,
		"expected exit code 2 for stdin with bad config, got %d", exitCode)
	assert.Contains(t, stderr, "mdsmith:",
		"expected error in stderr, got: %s", stderr)
}

// =============================================================
// 8. discoverFiles — bad config (discovery path)
// =============================================================

func TestE2E_Check_Discovered_BadConfig_ExitsTwo(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "test.md", "# Title\n\nContent here.\n")

	_, stderr, exitCode := runBinaryInDir(t, dir, "",
		"check", "--config", "/no/such/config.yml")
	assert.Equal(t, 2, exitCode,
		"expected exit code 2 for discovery with bad config, got %d", exitCode)
	assert.Contains(t, stderr, "mdsmith:",
		"expected error in stderr, got: %s", stderr)
}

func TestE2E_Fix_Discovered_BadConfig_ExitsTwo(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "test.md", "# Title\n\nContent here.\n")

	_, stderr, exitCode := runBinaryInDir(t, dir, "",
		"fix", "--config", "/no/such/config.yml")
	assert.Equal(t, 2, exitCode,
		"expected exit code 2 for discovery with bad config, got %d", exitCode)
	assert.Contains(t, stderr, "mdsmith:",
		"expected error in stderr, got: %s", stderr)
}

// =============================================================
// 10. rootDirFromConfig — empty cfgPath falls back to cwd
// =============================================================

func TestE2E_Check_NoCfgPath_UsesWorkingDir(t *testing.T) {
	// Running check in a directory with no config file exercises
	// the rootDirFromConfig fallback to cwd.
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	writeFixture(t, dir, "clean.md", "# Title\n\nContent here.\n")

	_, _, exitCode := runBinaryInDir(t, dir, "", "check", "--no-color", "clean.md")
	assert.Equal(t, 0, exitCode, "expected exit 0, got %d", exitCode)
}

// =============================================================
// 11. loadConfig — --config pointing at bad YAML
// =============================================================

// (duplicate of TestE2E_Check_BadConfig_ExitsTwo removed)

// =============================================================
// 14. runMetrics — no args prints usage; unknown subcommand
// =============================================================

func TestE2E_Metrics_NoArgs_PrintsUsage(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "", "metrics")
	assert.Equal(t, 0, exitCode, "expected exit 0, got %d", exitCode)
	assert.Contains(t, stderr, "Usage:", "expected usage in stderr, got: %s", stderr)
	assert.Contains(t, stderr, "list", "expected 'list' in usage, got: %s", stderr)
	assert.Contains(t, stderr, "rank", "expected 'rank' in usage, got: %s", stderr)
}

func TestE2E_Metrics_UnknownSubcommand_ExitsTwo(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "", "metrics", "bogus")
	assert.Equal(t, 2, exitCode, "expected exit 2, got %d", exitCode)
	assert.Contains(t, stderr, "unknown command",
		"expected 'unknown command' error, got: %s", stderr)
}

// =============================================================
// 15. runMetricsList — invalid scope, extra args, unknown format
// =============================================================

func TestE2E_MetricsList_InvalidScope_ExitsTwo(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "", "metrics", "list", "--scope", "bogus")
	assert.Equal(t, 2, exitCode, "expected exit 2, got %d", exitCode)
	assert.Contains(t, stderr, "mdsmith:",
		"expected error in stderr, got: %s", stderr)
}

func TestE2E_MetricsList_ExtraArgs_ExitsTwo(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "", "metrics", "list", "extra-arg")
	assert.Equal(t, 2, exitCode, "expected exit 2, got %d", exitCode)
	assert.Contains(t, stderr, "no file arguments",
		"expected error about file arguments, got: %s", stderr)
}

func TestE2E_MetricsList_UnknownFormat_ExitsTwo(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "", "metrics", "list", "--format", "xml")
	assert.Equal(t, 2, exitCode, "expected exit 2, got %d", exitCode)
	assert.Contains(t, stderr, "unknown format",
		"expected 'unknown format' error, got: %s", stderr)
}

// =============================================================
// 16-17. runMetricsRank — error paths
// =============================================================

func TestE2E_MetricsRank_NegativeTop_ExitsTwo(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "a.md", "# Title\n\nContent here.\n")

	_, stderr, exitCode := runBinaryInDir(t, dir, "", "metrics", "rank", "--top", "-1", ".")
	assert.Equal(t, 2, exitCode, "expected exit 2 for negative --top, got %d", exitCode)
	assert.Contains(t, stderr, "--top",
		"expected --top error in stderr, got: %s", stderr)
}

func TestE2E_MetricsRank_BadConfig_ExitsTwo(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "a.md", "# Title\n\nContent here.\n")

	_, stderr, exitCode := runBinaryInDir(t, dir, "",
		"metrics", "rank", "--config", "/no/such/config.yml", ".")
	assert.Equal(t, 2, exitCode, "expected exit 2 for bad config, got %d", exitCode)
	assert.Contains(t, stderr, "mdsmith:",
		"expected error in stderr, got: %s", stderr)
}

// =============================================================
// 18. executeMetricsRank — unknown format
// =============================================================

func TestE2E_MetricsRank_UnknownFormat_ExitsTwo(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "a.md", "# Title\n\nContent here.\n")

	_, stderr, exitCode := runBinaryInDir(t, dir, "",
		"metrics", "rank", "--format", "xml", ".")
	assert.Equal(t, 2, exitCode, "expected exit 2 for unknown format, got %d", exitCode)
	assert.Contains(t, stderr, "unknown format",
		"expected 'unknown format' error, got: %s", stderr)
}

// =============================================================
// 19. resolveRankSelection — --by not in --metrics; explicit --order
// =============================================================

func TestE2E_MetricsRank_ByNotInMetrics_ExitsTwo(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "a.md", "# Title\n\nContent here.\n")

	_, stderr, exitCode := runBinaryInDir(t, dir, "",
		"metrics", "rank", "--metrics", "bytes", "--by", "words", ".")
	assert.Equal(t, 2, exitCode,
		"expected exit 2 when --by metric not in --metrics, got %d", exitCode)
	assert.Contains(t, stderr, "must be included",
		"expected 'must be included' error, got: %s", stderr)
}

func TestE2E_MetricsRank_ExplicitOrderAsc(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "small.md", "# S\n\nSmall.\n")
	writeFixture(t, dir, "large.md", "# Large\n\nThis file has significantly more bytes than the small one.\n")

	stdout, stderr, exitCode := runBinaryInDir(t, dir, "",
		"metrics", "rank", "--by", "bytes", "--order", "asc", ".")
	require.Equal(t, 0, exitCode,
		"expected exit 0, got %d; stderr: %s", exitCode, stderr)

	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	require.GreaterOrEqual(t, len(lines), 3, "expected header + 2 rows, got: %s", stdout)
	// With asc order, smaller file should be first.
	assert.Contains(t, lines[1], "small.md",
		"expected smallest file first with --order asc, got: %s", lines[1])
}

func TestE2E_MetricsRank_ExplicitOrderDesc(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "small.md", "# S\n\nSmall.\n")
	writeFixture(t, dir, "large.md", "# Large\n\nThis file has significantly more bytes than the small one.\n")

	stdout, stderr, exitCode := runBinaryInDir(t, dir, "",
		"metrics", "rank", "--by", "bytes", "--order", "desc", ".")
	require.Equal(t, 0, exitCode,
		"expected exit 0, got %d; stderr: %s", exitCode, stderr)

	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	require.GreaterOrEqual(t, len(lines), 3, "expected header + 2 rows, got: %s", stdout)
	assert.Contains(t, lines[1], "large.md",
		"expected largest file first with --order desc, got: %s", lines[1])
}

func TestE2E_MetricsRank_InvalidOrder_ExitsTwo(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "a.md", "# Title\n\nContent here.\n")

	_, stderr, exitCode := runBinaryInDir(t, dir, "",
		"metrics", "rank", "--by", "bytes", "--order", "bogus", ".")
	assert.Equal(t, 2, exitCode,
		"expected exit 2 for invalid --order, got %d", exitCode)
	assert.Contains(t, stderr, "mdsmith:",
		"expected error in stderr, got: %s", stderr)
}

// =============================================================
// 22. writeRankOutput — JSON format for rank
// =============================================================

func TestE2E_MetricsRank_JSONFormat(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "a.md", "# Title\n\nSome text here.\n")

	stdout, stderr, exitCode := runBinaryInDir(t, dir, "",
		"metrics", "rank", "--by", "bytes", "--format", "json", ".")
	require.Equal(t, 0, exitCode,
		"expected exit 0, got %d; stderr: %s", exitCode, stderr)

	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &rows),
		"stdout is not valid JSON: %s", stdout)
	require.NotEmpty(t, rows, "expected non-empty rank rows")
	assert.Contains(t, rows[0], "path", "expected 'path' field in JSON row")
}

// =============================================================
// 24. showMetric — unknown metric lookup
// =============================================================

func TestE2E_HelpMetrics_UnknownMetric_ExitsTwo(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "", "help", "metrics", "not-a-metric")
	assert.Equal(t, 2, exitCode, "expected exit 2, got %d", exitCode)
	assert.Contains(t, stderr, "mdsmith:",
		"expected error in stderr, got: %s", stderr)
}

// =============================================================
// 27. runMergeDriverInstall — outside git repo
// =============================================================

func TestE2E_MergeDriver_Install_OutsideGitRepo_ExitsTwo(t *testing.T) {
	dir := t.TempDir()
	// Do NOT create .git marker — this dir is not a git repo.

	_, stderr, exitCode := runBinaryInDir(t, dir, "", "merge-driver", "install")
	assert.Equal(t, 2, exitCode,
		"expected exit 2 for install outside git repo, got %d", exitCode)
	assert.Contains(t, stderr, "not in a git repository",
		"expected 'not in a git repository' error, got: %s", stderr)
}

// =============================================================
// 29. check --help and fix --help (trigger Usage function)
// =============================================================

func TestE2E_Check_HelpFlag(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "", "check", "--help")
	assert.Equal(t, 2, exitCode, "expected exit 2 (pflag ContinueOnError), got %d", exitCode)
	assert.Contains(t, stderr, "Usage: mdsmith check",
		"expected check usage text, got: %s", stderr)
}

func TestE2E_Fix_HelpFlag(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "", "fix", "--help")
	assert.Equal(t, 2, exitCode, "expected exit 2 (pflag ContinueOnError), got %d", exitCode)
	assert.Contains(t, stderr, "Usage: mdsmith fix",
		"expected fix usage text, got: %s", stderr)
}

// =============================================================
// 30. check --quiet with violations — suppresses diagnostic output
// =============================================================

func TestE2E_Check_Quiet_SuppressesDiagnosticOutput(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "dirty.md", "# Title\n\nHello   \n")

	_, stderr, exitCode := runBinary(t, "", "check", "--quiet", "--no-color", path)
	assert.Equal(t, 1, exitCode, "expected exit 1, got %d", exitCode)
	// --quiet should suppress diagnostics and stats output.
	assert.NotContains(t, stderr, "MDS006",
		"expected no MDS006 output with --quiet, got: %s", stderr)
	assert.NotContains(t, stderr, "stats:",
		"expected no stats output with --quiet, got: %s", stderr)
}

// =============================================================
// 31. fix --quiet mode
// =============================================================

func TestE2E_Fix_Quiet_SuppressesOutput(t *testing.T) {
	dir := t.TempDir()
	isolateDir(t, dir)
	// Use a file with an unfixable issue so we get exit 1.
	writeFixture(t, dir, "dirty.md", "# Title!\n\nHello   \n")

	_, stderr, exitCode := runBinaryInDir(t, dir, "", "fix", "--quiet", "--no-color", "dirty.md")
	assert.Equal(t, 1, exitCode, "expected exit 1, got %d", exitCode)
	assert.NotContains(t, stderr, "MDS017",
		"expected no diagnostics with --quiet, got: %s", stderr)
	assert.NotContains(t, stderr, "stats:",
		"expected no stats output with --quiet, got: %s", stderr)
}

func TestE2E_Fix_Quiet_Clean(t *testing.T) {
	dir := t.TempDir()
	isolateDir(t, dir)
	writeFixture(t, dir, "fixme.md", "# Title\n\nHello   \n")

	_, stderr, exitCode := runBinaryInDir(t, dir, "", "fix", "--quiet", "--no-color", "fixme.md")
	assert.Equal(t, 0, exitCode, "expected exit 0, got %d; stderr: %s", exitCode, stderr)
	assert.NotContains(t, stderr, "stats:",
		"expected no stats output with --quiet, got: %s", stderr)
}

// =============================================================
// 35. query --verbose with expression not satisfied (different data)
// =============================================================

func TestE2E_Query_Verbose_MixedResults(t *testing.T) {
	dir := t.TempDir()
	// One file matches, one has FM but does not match, one has no FM.
	writeFixture(t, dir, "match.md", "---\nstatus: \"✅\"\n---\n# Match\n\nContent here.\n")
	writeFixture(t, dir, "miss.md", "---\nstatus: \"🔲\"\n---\n# Miss\n\nContent here.\n")
	writeFixture(t, dir, "plain.md", "# Plain\n\nNo front matter here.\n")

	// Pass files explicitly to avoid gitignore filtering on the directory.
	matchPath := filepath.Join(dir, "match.md")
	missPath := filepath.Join(dir, "miss.md")
	plainPath := filepath.Join(dir, "plain.md")
	stdout, stderr, exitCode := runBinaryInDir(t, dir, "", "query", "-v", `status: "✅"`, matchPath, missPath, plainPath)
	assert.Equal(t, 0, exitCode, "expected exit 0 (at least one match)")
	assert.Contains(t, stdout, "match.md", "expected match.md in stdout")
	assert.Contains(t, stderr, "expression not satisfied",
		"expected 'expression not satisfied' for miss.md, got: %s", stderr)
	assert.Contains(t, stderr, "no front matter",
		"expected 'no front matter' for plain.md, got: %s", stderr)
}

// =============================================================
// 36. Legacy no-follow-symlinks config key emits a deprecation
// =============================================================

// TestE2E_Check_LegacyNoFollowSymlinksConfig asserts that the old
// `no-follow-symlinks:` config key is still parsed (no hard error)
// and surfaces a deprecation warning. The new default-deny already
// achieves what the legacy key intended, so discovery results match
// what the user expects.
func TestE2E_Check_LegacyNoFollowSymlinksConfig(t *testing.T) {
	skipIfSymlinkUnsupported(t)
	dir := t.TempDir()
	writeFixture(t, dir, ".mdsmith.yml",
		"no-follow-symlinks:\n  - \"**\"\nrules:\n  no-trailing-spaces: true\n")

	subDir := filepath.Join(dir, "real")
	require.NoError(t, os.MkdirAll(subDir, 0o755))
	writeFixture(t, subDir, "dirty.md", "# Title\n\nHello   \n")

	link := filepath.Join(dir, "linked")
	require.NoError(t, os.Symlink(subDir, link))

	_, stderr, exitCode := runBinaryInDir(t, dir, "", "check", "--no-color")
	assert.Equal(t, 1, exitCode,
		"expected exit 1 (real dirty.md found), got %d; stderr: %s", exitCode, stderr)
	assert.NotContains(t, stderr, "linked/",
		"symlinked dir must be skipped under default-deny; stderr: %s", stderr)
	assert.Contains(t, stderr, "no-follow-symlinks",
		"expected deprecation warning, got stderr: %s", stderr)
	assert.Contains(t, stderr, "deprecated",
		"expected deprecation warning, got stderr: %s", stderr)
}

// =============================================================
// Additional: check --format json with --quiet
// =============================================================

// (duplicate quiet test removed — same behavior tested in TestE2E_Check_Quiet_SuppressesDiagnosticOutput)

// =============================================================
// Additional: fix --format json
// =============================================================

func TestE2E_Fix_JSONFormat_WithUnfixable(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "dirty.md", "# Title!\n\nHello   \n")

	_, stderr, exitCode := runBinary(t, "", "fix", "--no-color", "--format", "json", path)
	assert.Equal(t, 1, exitCode, "expected exit 1, got %d", exitCode)

	// Verify JSON array present in stderr.
	jsonStart := strings.Index(stderr, "[")
	jsonEnd := strings.LastIndex(stderr, "]")
	require.GreaterOrEqual(t, jsonStart, 0, "expected JSON array in stderr, got: %s", stderr)
	require.GreaterOrEqual(t, jsonEnd, jsonStart, "expected JSON array close in stderr, got: %s", stderr)
	jsonPart := stderr[jsonStart : jsonEnd+1]
	var diagnostics []map[string]any
	require.NoError(t, json.Unmarshal([]byte(jsonPart), &diagnostics),
		"JSON portion of stderr is not valid JSON: %s", jsonPart)
	require.NotEmpty(t, diagnostics, "expected at least one diagnostic")
}

// =============================================================
// Additional: metrics rank with --metrics selecting specific set
// =============================================================

func TestE2E_MetricsRank_ByWithDefaultMetrics(t *testing.T) {
	// When --by is set but --metrics is not, the by metric should
	// be included automatically and not fail.
	dir := t.TempDir()
	writeFixture(t, dir, "a.md", "# Title\n\nSome content here.\n")

	stdout, stderr, exitCode := runBinaryInDir(t, dir, "",
		"metrics", "rank", "--by", "words", ".")
	require.Equal(t, 0, exitCode,
		"expected exit 0, got %d; stderr: %s", exitCode, stderr)
	assert.NotEmpty(t, stdout, "expected non-empty rank output")
}

// =============================================================
// Additional: metrics rank with no files found
// =============================================================

func TestE2E_MetricsRank_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	stdout, stderr, exitCode := runBinaryInDir(t, dir, "",
		"metrics", "rank", "--by", "bytes", ".")
	assert.Equal(t, 0, exitCode,
		"expected exit 0 for empty dir, got %d; stderr: %s", exitCode, stderr)
	// Output should contain only the header line (no data rows).
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	assert.LessOrEqual(t, len(lines), 1,
		"expected at most 1 line (header) for empty dir, got: %s", stdout)
}

// =============================================================
// Additional: metrics list --scope file (explicit valid scope)
// =============================================================

func TestE2E_MetricsList_ScopeFile(t *testing.T) {
	stdout, _, exitCode := runBinary(t, "", "metrics", "list", "--scope", "file")
	assert.Equal(t, 0, exitCode, "expected exit 0, got %d", exitCode)
	assert.Contains(t, stdout, "MET001", "expected MET001 in output")
}

// =============================================================
// Additional: check unknown flag (pflag error)
// =============================================================

func TestE2E_Check_UnknownFlag_ExitsTwo(t *testing.T) {
	_, _, exitCode := runBinary(t, "", "check", "--bogus-flag")
	assert.Equal(t, 2, exitCode, "expected exit 2 for unknown flag, got %d", exitCode)
}

func TestE2E_Fix_UnknownFlag_ExitsTwo(t *testing.T) {
	_, _, exitCode := runBinary(t, "", "fix", "--bogus-flag")
	assert.Equal(t, 2, exitCode, "expected exit 2 for unknown flag, got %d", exitCode)
}

// =============================================================
// Additional: metrics rank --help
// =============================================================

func TestE2E_MetricsRank_HelpFlag(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "", "metrics", "rank", "--help")
	assert.Equal(t, 2, exitCode, "expected exit 2 (pflag ContinueOnError), got %d", exitCode)
	assert.Contains(t, stderr, "Usage: mdsmith metrics rank",
		"expected rank usage, got: %s", stderr)
}

func TestE2E_MetricsList_HelpFlag(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "", "metrics", "list", "--help")
	assert.Equal(t, 2, exitCode, "expected exit 2 (pflag ContinueOnError), got %d", exitCode)
	assert.Contains(t, stderr, "Usage: mdsmith metrics list",
		"expected list usage, got: %s", stderr)
}

// =============================================================
// Additional: check verbose with discovered files
// =============================================================

func TestE2E_Check_Verbose_Discovered(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "clean.md", "# Title\n\nContent here.\n")
	writeFixture(t, dir, ".mdsmith.yml", "rules:\n  no-trailing-spaces: true\n")

	_, stderr, exitCode := runBinaryInDir(t, dir, "", "check", "--verbose", "--no-color")
	assert.Equal(t, 0, exitCode, "expected exit 0, got %d; stderr: %s", exitCode, stderr)
	assert.Contains(t, stderr, "config:",
		"expected 'config:' in verbose discovered output, got: %s", stderr)
}

// =============================================================
// Additional: fix verbose with discovered files
// =============================================================

func TestE2E_Fix_Verbose_Discovered(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "fixme.md", "# Title\n\nHello   \n")
	writeFixture(t, dir, ".mdsmith.yml", "rules:\n  no-trailing-spaces: true\n")

	_, stderr, exitCode := runBinaryInDir(t, dir, "", "fix", "--verbose", "--no-color")
	assert.Equal(t, 0, exitCode, "expected exit 0, got %d; stderr: %s", exitCode, stderr)
	assert.Contains(t, stderr, "config:",
		"expected 'config:' in verbose discovered output, got: %s", stderr)
}

// =============================================================
// Additional: check with multiple file args
// =============================================================

func TestE2E_Check_MultipleFiles(t *testing.T) {
	dir := t.TempDir()
	p1 := writeFixture(t, dir, "a.md", "# A\n\nHello   \n")
	p2 := writeFixture(t, dir, "b.md", "# B\n\nWorld   \n")

	_, stderr, exitCode := runBinary(t, "", "check", "--no-color", p1, p2)
	assert.Equal(t, 1, exitCode, "expected exit 1, got %d", exitCode)
	// Both files should appear in diagnostics.
	assert.Contains(t, stderr, "a.md", "expected a.md in stderr, got: %s", stderr)
	assert.Contains(t, stderr, "b.md", "expected b.md in stderr, got: %s", stderr)
}

// =============================================================
// Additional: help metrics by ID
// =============================================================

func TestE2E_HelpMetrics_ByID(t *testing.T) {
	stdout, _, exitCode := runBinary(t, "", "help", "metrics", "MET001")
	assert.Equal(t, 0, exitCode, "expected exit 0, got %d", exitCode)
	assert.Contains(t, stdout, "MET001", "expected MET001 in output, got: %s", stdout)
}

// =============================================================
// Additional: init --help
// =============================================================

func TestE2E_Init_HelpFlag(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "", "init", "--help")
	assert.Equal(t, 2, exitCode, "expected exit 2 (pflag ContinueOnError), got %d", exitCode)
	assert.Contains(t, stderr, "Usage: mdsmith init",
		"expected init usage, got: %s", stderr)
}

// =============================================================
// Additional: metrics rank with --no-gitignore
// =============================================================

func TestE2E_MetricsRank_NoGitignore(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "vendor"), 0o755))
	writeFixture(t, dir, "a.md", "# Title\n\nContent here.\n")
	writeFixture(t, dir, "vendor/lib.md", "# Lib\n\nVendor content.\n")
	writeFixture(t, dir, ".gitignore", "vendor/\n")

	stdout, stderr, exitCode := runBinaryInDir(t, dir, "",
		"metrics", "rank", "--by", "bytes", "--no-gitignore", ".")
	require.Equal(t, 0, exitCode,
		"expected exit 0, got %d; stderr: %s", exitCode, stderr)
	assert.Contains(t, stdout, "lib.md",
		"expected vendor/lib.md included with --no-gitignore, got: %s", stdout)
}

// =============================================================
// Additional: fix with --format text (explicit)
// =============================================================

func TestE2E_Fix_TextFormat_WithUnfixable(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "dirty.md", "# Title!\n\nHello   \n")

	_, stderr, exitCode := runBinary(t, "", "fix", "--no-color", "--format", "text", path)
	assert.Equal(t, 1, exitCode, "expected exit 1, got %d", exitCode)
	assert.Contains(t, stderr, "MDS017",
		"expected MDS017 in text output, got: %s", stderr)
}

// =============================================================
// Additional: check stdin verbose
// =============================================================

func TestE2E_Check_Stdin_Verbose(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "# Hello\n\nWorld   \n",
		"check", "--verbose", "--no-color", "-")
	assert.Equal(t, 1, exitCode, "expected exit 1, got %d", exitCode)
	assert.Contains(t, stderr, "checked 1 files",
		"expected verbose summary, got: %s", stderr)
}

// =============================================================
// Additional: check stdin quiet
// =============================================================

func TestE2E_Check_Stdin_Quiet(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "# Hello\n\nWorld   \n",
		"check", "--quiet", "-")
	assert.Equal(t, 1, exitCode, "expected exit 1, got %d", exitCode)
	assert.NotContains(t, stderr, "MDS006",
		"expected no diagnostic output with --quiet stdin, got: %s", stderr)
}

// =============================================================
// fixDiscovered with unfixable diagnostics (non-quiet)
// =============================================================

func TestE2E_Fix_Discovered_UnfixableDiagnostic(t *testing.T) {
	dir := t.TempDir()
	isolateDir(t, dir)
	// trailing-punctuation in heading is unfixable; trailing spaces are fixable.
	// After fix, MDS017 remains → formatDiagnostics is called in fixDiscovered.
	writeFixture(t, dir, ".mdsmith.yml",
		"rules:\n  no-trailing-punctuation-in-heading: true\n  no-trailing-spaces: true\n")
	writeFixture(t, dir, "dirty.md", "# Title!\n\nHello   \n")

	_, stderr, exitCode := runBinaryInDir(t, dir, "", "fix", "--no-color")
	assert.Equal(t, 1, exitCode, "expected exit 1 (unfixable diagnostic), got %d", exitCode)
	assert.Contains(t, stderr, "MDS017",
		"expected MDS017 diagnostic in stderr, got: %s", stderr)
}
