package main_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var binaryPath string
var coverDir string

func TestMain(m *testing.M) {
	// Build the binary once for all e2e tests.
	// go test runs from the package directory (cmd/mdsmith/),
	// so "go build ." builds the main package in this directory.
	tmp, err := os.MkdirTemp("", "mdsmith-e2e-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create temp dir: %v\n", err)
		os.Exit(1)
	}

	// Create a shared directory for coverage data from all e2e runs.
	coverDir, err = os.MkdirTemp("", "mdsmith-e2e-cover-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create cover dir: %v\n", err)
		_ = os.RemoveAll(tmp)
		os.Exit(1)
	}

	binaryPath = filepath.Join(tmp, "mdsmith")
	cmd := exec.Command("go", "build", "-cover", "-covermode=atomic", "-o", binaryPath, ".")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to build binary: %v\n", err)
		_ = os.RemoveAll(tmp)
		_ = os.RemoveAll(coverDir)
		os.Exit(1)
	}

	code := m.Run()

	// Merge e2e coverage data into a text profile if E2E_COVERDIR is
	// set by the caller, so it can be combined with unit-test coverage.
	if outDir := os.Getenv("E2E_COVERDIR"); outDir != "" {
		if err := os.MkdirAll(outDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "E2E_COVERDIR: cannot create %s: %v\n", outDir, err)
			code = 1
		} else {
			mergeCmd := exec.Command("go", "tool", "covdata", "textfmt",
				"-i="+coverDir, "-o="+filepath.Join(outDir, "e2e_coverage.txt"))
			mergeCmd.Stderr = os.Stderr
			if err := mergeCmd.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "E2E_COVERDIR: failed to export coverage: %v\n", err)
				code = 1
			}
		}
	}

	_ = os.RemoveAll(tmp)
	_ = os.RemoveAll(coverDir)
	os.Exit(code)
}

// runBinary runs the mdsmith binary with the given args and optional stdin.
// It returns stdout, stderr, and the exit code.
func runBinary(t *testing.T, stdin string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()

	cmd := exec.Command(binaryPath, args...)
	cmd.Env = envWithCoverDir(coverDir)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}

	err := cmd.Run()
	exitCode = 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("unexpected error running binary: %v", err)
		}
	}

	return outBuf.String(), errBuf.String(), exitCode
}

// runBinaryInDir runs the mdsmith binary with the given args in the given directory.
func runBinaryInDir(t *testing.T, dir, stdin string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()

	cmd := exec.Command(binaryPath, args...)
	cmd.Dir = dir
	cmd.Env = envWithCoverDir(coverDir)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}

	err := cmd.Run()
	exitCode = 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("unexpected error running binary: %v", err)
		}
	}

	return outBuf.String(), errBuf.String(), exitCode
}

// envWithCoverDir returns os.Environ() with any existing GOCOVERDIR removed
// and the given dir set as GOCOVERDIR.
func envWithCoverDir(dir string) []string {
	var env []string
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "GOCOVERDIR=") {
			env = append(env, e)
		}
	}
	return append(env, "GOCOVERDIR="+dir)
}

// writeFixture creates a file with the given content in the given directory.
func writeFixture(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writing fixture %s: %v", path, err)
	}
	return path
}

func parseStats(t *testing.T, stderr string) (checked, fixed, failures, unfixed int) {
	t.Helper()
	re := regexp.MustCompile(`stats: checked=(\d+) fixed=(\d+) failures=(\d+) unfixed=(\d+)`)
	m := re.FindStringSubmatch(stderr)
	require.Len(t, m, 5, "expected stats line in stderr, got: %s", stderr)

	values := make([]int, 4)
	for i := 0; i < 4; i++ {
		v, err := strconv.Atoi(m[i+1])
		require.NoError(t, err, "parsing stats value %q: %v", m[i+1], err)
		values[i] = v
	}

	return values[0], values[1], values[2], values[3]
}

// --- Coverage instrumentation test ---

func TestE2E_CoverageInstrumentation(t *testing.T) {
	tmpCoverDir := t.TempDir()
	cmd := exec.Command(binaryPath, "version")
	cmd.Env = envWithCoverDir(tmpCoverDir)
	require.NoError(t, cmd.Run(), "unexpected error running binary")
	entries, err := os.ReadDir(tmpCoverDir)
	require.NoError(t, err, "reading cover dir")
	require.NotEmpty(t, entries, "binary was not built with -cover: no coverage data written to GOCOVERDIR")
}

// --- Top-level behavior tests ---

func TestE2E_NoArgs_PrintsUsage_ExitsZero(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "")
	assert.Equal(t, 0, exitCode, "expected exit code 0, got %d", exitCode)
	assert.Contains(t, stderr, "Usage:", "expected usage text in stderr, got: %s", stderr)
	assert.Contains(t, stderr, "check", "expected 'check' subcommand in usage, got: %s", stderr)
}

func TestE2E_Help_PrintsUsage_ExitsZero(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "", "--help")
	assert.Equal(t, 0, exitCode, "expected exit code 0, got %d", exitCode)
	assert.Contains(t, stderr, "Usage:", "expected usage text in stderr, got: %s", stderr)
}

func TestE2E_HelpShorthand_PrintsUsage_ExitsZero(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "", "-h")
	assert.Equal(t, 0, exitCode, "expected exit code 0, got %d", exitCode)
	assert.Contains(t, stderr, "Usage:", "expected usage text in stderr, got: %s", stderr)
}

func TestE2E_VersionSubcommand(t *testing.T) {
	stdout, _, exitCode := runBinary(t, "", "version")
	assert.Equal(t, 0, exitCode, "expected exit code 0, got %d", exitCode)
	assert.True(t, strings.HasPrefix(stdout, "mdsmith "),
		"expected version output to start with 'mdsmith ', got: %s", stdout)
}

func TestE2E_VersionFlag_NotRecognized(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "", "--version")
	assert.Equal(t, 2, exitCode, "expected exit code 2, got %d", exitCode)
	assert.Contains(t, stderr, "unknown command", "expected 'unknown command' in stderr, got: %s", stderr)
}

func TestE2E_VersionShortFlag_NotRecognized(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "", "-v")
	assert.Equal(t, 2, exitCode, "expected exit code 2, got %d", exitCode)
	assert.Contains(t, stderr, "unknown command", "expected 'unknown command' in stderr, got: %s", stderr)
}

func TestE2E_UnknownCommand_ExitsTwo(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "", "bogus")
	assert.Equal(t, 2, exitCode, "expected exit code 2, got %d", exitCode)
	assert.Contains(t, stderr, "unknown command", "expected 'unknown command' in stderr, got: %s", stderr)
}

func TestE2E_FilePathWithoutSubcommand_ExitsTwo(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "test.md", "# Title\n\nHello   \n")

	// Passing a file path without a subcommand should exit 2.
	_, stderr, exitCode := runBinary(t, "", path)
	assert.Equal(t, 2, exitCode, "expected exit code 2, got %d", exitCode)
	assert.Contains(t, stderr, "unknown command", "expected 'unknown command' in stderr, got: %s", stderr)
}

func TestE2E_LegacyFixFlag_ExitsTwo(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "test.md", "# Title\n\nHello   \n")

	// Passing --fix without a subcommand should exit 2.
	_, stderr, exitCode := runBinary(t, "", "--fix", path)
	assert.Equal(t, 2, exitCode, "expected exit code 2, got %d", exitCode)
	assert.Contains(t, stderr, "unknown command", "expected 'unknown command' in stderr, got: %s", stderr)
}

// --- Check subcommand tests ---

func TestE2E_Check_CleanFile_ExitsZero(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "clean.md", "# Title\n\nSome content here.\n")

	_, _, exitCode := runBinary(t, "", "check", "--no-color", path)
	assert.Equal(t, 0, exitCode, "expected exit code 0 for clean file, got %d", exitCode)
}

func TestE2E_Check_PrintsStats(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "clean.md", "# Title\n\nSome content here.\n")

	_, stderr, exitCode := runBinary(t, "", "check", "--no-color", path)
	require.Equal(t, 0, exitCode, "expected exit code 0, got %d; stderr: %s", exitCode, stderr)

	checked, fixed, failures, unfixed := parseStats(t, stderr)
	assert.Equal(t, 1, checked, "expected checked=1, got %d", checked)
	assert.Equal(t, 0, fixed, "expected fixed=0 for check, got %d", fixed)
	assert.Equal(t, 0, failures, "expected failures=0, got %d", failures)
	assert.Equal(t, 0, unfixed, "expected unfixed=0, got %d", unfixed)
}

func TestE2E_Check_Violations_ExitsOne(t *testing.T) {
	dir := t.TempDir()
	// Trailing spaces on lines should trigger MDS006.
	path := writeFixture(t, dir, "dirty.md", "# Title\n\nHello   \n")

	_, stderr, exitCode := runBinary(t, "", "check", "--no-color", path)
	assert.Equal(t, 1, exitCode, "expected exit code 1, got %d", exitCode)
	assert.Contains(t, stderr, "MDS006", "expected stderr to contain MDS006, got: %s", stderr)
	assert.Contains(t, stderr, "trailing whitespace",
		"expected stderr to contain 'trailing whitespace', got: %s", stderr)
}

func TestE2E_Check_JSONFormat(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "dirty.md", "# Title\n\nHello   \n")

	_, stderr, exitCode := runBinary(t, "", "check", "--no-color", "--format", "json", path)
	assert.Equal(t, 1, exitCode, "expected exit code 1, got %d", exitCode)

	// Validate JSON output.
	var diagnostics []map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(stderr), &diagnostics), "stderr is not valid JSON: %s", stderr)
	require.NotEmpty(t, diagnostics, "expected at least one diagnostic in JSON output")

	// Check the JSON schema has required fields.
	d := diagnostics[0]
	requiredFields := []string{"file", "line", "column", "rule", "name", "severity", "message"}
	for _, field := range requiredFields {
		assert.Contains(t, d, field, "JSON diagnostic missing required field %q", field)
	}

	// Check that the file field points to our fixture.
	fileVal, _ := d["file"].(string)
	assert.True(t, strings.HasSuffix(fileVal, "dirty.md"),
		"expected file field to end with dirty.md, got %q", fileVal)
}

func TestE2E_Check_Stdin_Clean(t *testing.T) {
	_, _, exitCode := runBinary(t, "# Hello\n\nWorld.\n", "check", "-")
	assert.Equal(t, 0, exitCode, "expected exit code 0 for clean stdin, got %d", exitCode)
}

func TestE2E_Check_Stdin_Violations(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "# Hello\n\nWorld   \n", "check", "--no-color", "-")
	assert.Equal(t, 1, exitCode, "expected exit code 1 for stdin with violations, got %d", exitCode)
	assert.Contains(t, stderr, "<stdin>", "expected diagnostics to use <stdin> as file name, got: %s", stderr)
	assert.Contains(t, stderr, "MDS006", "expected MDS006 in stderr, got: %s", stderr)
}

func TestE2E_Check_Stdin_JSONFormat(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "# Hello\n\nWorld   \n", "check", "--no-color", "--format", "json", "-")
	assert.Equal(t, 1, exitCode, "expected exit code 1, got %d", exitCode)

	var diagnostics []map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(stderr), &diagnostics), "stderr is not valid JSON: %s", stderr)
	require.NotEmpty(t, diagnostics, "expected at least one diagnostic")

	fileVal, _ := diagnostics[0]["file"].(string)
	assert.Equal(t, "<stdin>", fileVal, "expected file to be \"<stdin>\", got %q", fileVal)
}

func TestE2E_Check_CustomConfig(t *testing.T) {
	dir := t.TempDir()

	// Create a file that violates no-trailing-spaces (MDS006).
	path := writeFixture(t, dir, "test.md", "# Title\n\nHello   \n")

	// Create a config that disables no-trailing-spaces.
	configContent := "rules:\n  no-trailing-spaces: false\n"
	configPath := writeFixture(t, dir, ".mdsmith.yml", configContent)

	// Run with the custom config; the violation should be suppressed.
	_, stderr, exitCode := runBinary(t, "", "check", "--no-color", "--config", configPath, path)
	assert.NotContains(t, stderr, "MDS006",
		"expected MDS006 to be suppressed by config, but found in stderr: %s", stderr)
	assert.Equal(t, 0, exitCode, "expected exit code 0 with rule disabled, got %d", exitCode)
}

func TestE2E_Check_Gitignore_SkipsIgnoredDirectory(t *testing.T) {
	dir := t.TempDir()
	ignoredDir := filepath.Join(dir, "ignored")
	require.NoError(t, os.MkdirAll(ignoredDir, 0o755))

	// Place a clean file at root and a file with violations in ignored/.
	writeFixture(t, dir, "clean.md", "# Title\n\nSome content here.\n")
	writeFixture(t, ignoredDir, "dirty.md", "# Title\n\nHello   \n")

	// Create .gitignore that excludes the ignored directory.
	writeFixture(t, dir, ".gitignore", "ignored/\n")

	// Run mdsmith on the directory -- the ignored file should be skipped.
	_, stderr, exitCode := runBinary(t, "", "check", "--no-color", dir)
	assert.Equal(t, 0, exitCode, "expected exit code 0 (ignored file skipped), got %d; stderr: %s", exitCode, stderr)
}

func TestE2E_Check_NoGitignore_IncludesIgnoredDirectory(t *testing.T) {
	dir := t.TempDir()
	ignoredDir := filepath.Join(dir, "ignored")
	require.NoError(t, os.MkdirAll(ignoredDir, 0o755))

	// Place a clean file at root and a file with violations in ignored/.
	writeFixture(t, dir, "clean.md", "# Title\n\nSome content here.\n")
	writeFixture(t, ignoredDir, "dirty.md", "# Title\n\nHello   \n")

	// Create .gitignore that excludes the ignored directory.
	writeFixture(t, dir, ".gitignore", "ignored/\n")

	// Run with --no-gitignore -- the violated file should be found.
	_, stderr, exitCode := runBinary(t, "", "check", "--no-color", "--no-gitignore", dir)
	assert.Equal(t, 1, exitCode,
		"expected exit code 1 (violations found with --no-gitignore), got %d; stderr: %s",
		exitCode, stderr)
	assert.Contains(t, stderr, "MDS006", "expected MDS006 in stderr, got: %s", stderr)
}

// --- Fix subcommand tests ---

func TestE2E_Fix_FixableFile(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "fixme.md", "# Title\n\nHello   \n")

	// Run fix subcommand.
	_, _, exitCode := runBinary(t, "", "fix", "--no-color", path)
	assert.Equal(t, 0, exitCode, "expected exit code 0 after fix, got %d", exitCode)

	// Read the file back and check that trailing spaces are removed.
	content, err := os.ReadFile(path)
	require.NoError(t, err, "reading fixed file: %v", err)
	assert.NotContains(t, string(content), "Hello   ", "file still contains trailing spaces after fix")
	assert.Contains(t, string(content), "Hello", "file does not contain expected content after fix")

	// Re-run check; should exit 0 now.
	_, _, exitCode = runBinary(t, "", "check", "--no-color", path)
	assert.Equal(t, 0, exitCode, "expected exit code 0 on re-lint after fix, got %d", exitCode)
}

func TestE2E_Fix_PreservesFrontMatter(t *testing.T) {
	dir := t.TempDir()
	content := "---\ntitle: hello\n---\n# Title\n\nHello   \n"
	path := writeFixture(t, dir, "fm.md", content)

	// Run fix subcommand.
	_, _, exitCode := runBinary(t, "", "fix", "--no-color", path)
	assert.Equal(t, 0, exitCode, "expected exit code 0 after fix, got %d", exitCode)

	// Read the file back.
	got, err := os.ReadFile(path)
	require.NoError(t, err, "reading fixed file: %v", err)

	// Frontmatter should be preserved intact.
	expectedFM := "---\ntitle: hello\n---\n"
	assert.True(t, strings.HasPrefix(string(got), expectedFM),
		"frontmatter not preserved; got prefix %q, want %q",
		string(got[:len(expectedFM)]), expectedFM)

	// Content should be fixed (trailing spaces removed).
	body := string(got[len(expectedFM):])
	assert.NotContains(t, body, "Hello   ", "file still contains trailing spaces after fix")
	assert.Contains(t, body, "Hello", "file does not contain expected content after fix")
}

func TestE2E_Fix_Stdin_Rejected(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "# Hello\n\nWorld   \n", "fix", "-")
	assert.Equal(t, 2, exitCode, "expected exit code 2 for fix with stdin, got %d", exitCode)
	assert.Contains(t, stderr, "cannot fix stdin in place", "expected error message about stdin fix, got: %s", stderr)
}

func TestE2E_Fix_PrintsStatsWithUnfixedFailures(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "partially-fixable.md", "# Title!\n\nHello   \n")

	_, stderr, exitCode := runBinary(t, "", "fix", "--no-color", path)
	require.Equal(t, 1, exitCode,
		"expected exit code 1 with remaining non-fixable issue, got %d; stderr: %s",
		exitCode, stderr)

	checked, fixed, failures, unfixed := parseStats(t, stderr)
	assert.Equal(t, 1, checked, "expected checked=1, got %d", checked)
	assert.Equal(t, 1, fixed, "expected fixed=1, got %d", fixed)
	assert.GreaterOrEqual(t, failures, 1, "expected failures >= 1, got %d", failures)
	assert.GreaterOrEqual(t, unfixed, 1, "expected unfixed >= 1, got %d", unfixed)
	assert.GreaterOrEqual(t, failures, unfixed,
		"expected failures >= unfixed, got failures=%d unfixed=%d", failures, unfixed)
}

// --- Init subcommand tests ---

func TestE2E_Init_CreatesConfig(t *testing.T) {
	dir := t.TempDir()

	_, stderr, exitCode := runBinaryInDir(t, dir, "", "init")
	assert.Equal(t, 0, exitCode, "expected exit code 0, got %d; stderr: %s", exitCode, stderr)
	assert.Contains(t, stderr, "created .mdsmith.yml", "expected confirmation message, got: %s", stderr)

	// Check that the file was created.
	configPath := filepath.Join(dir, ".mdsmith.yml")
	content, err := os.ReadFile(configPath)
	require.NoError(t, err, "reading config file: %v", err)

	// Verify it contains some expected content.
	s := string(content)
	assert.Contains(t, s, "rules:", "config file should contain 'rules:', got: %s", s)
	assert.Contains(t, s, "front-matter:", "config file should contain 'front-matter:', got: %s", s)
	assert.Contains(t, s, "line-length", "config file should contain 'line-length', got: %s", s)
}

func TestE2E_Init_RefusesIfExists(t *testing.T) {
	dir := t.TempDir()

	// Create an existing config file.
	writeFixture(t, dir, ".mdsmith.yml", "rules: {}\n")

	_, stderr, exitCode := runBinaryInDir(t, dir, "", "init")
	assert.Equal(t, 2, exitCode, "expected exit code 2, got %d", exitCode)
	assert.Contains(t, stderr, "already exists", "expected 'already exists' error, got: %s", stderr)
}

// --- Stdin frontmatter and Configurable settings tests ---

func TestE2E_Check_Stdin_FrontMatterLineOffset(t *testing.T) {
	// Pipe content with YAML front matter followed by a line with trailing
	// spaces. The reported line number should reflect the original file
	// (including front matter lines), not the stripped content.
	input := "---\ntitle: hello\n---\n# Title\n\nHello   \n"
	// "Hello   " is on line 6 of the original.
	_, stderr, exitCode := runBinary(t, input, "check", "--no-color", "-")
	assert.Equal(t, 1, exitCode, "expected exit code 1, got %d; stderr: %s", exitCode, stderr)
	assert.Contains(t, stderr, "MDS006", "expected MDS006 in stderr, got: %s", stderr)
	// Verify the line number is 6 (original file), not 3 (stripped content).
	assert.Contains(t, stderr, "<stdin>:6:", "expected line 6 in diagnostic, got: %s", stderr)
}

func TestE2E_Check_Stdin_ConfigurableSettingsApplied(t *testing.T) {
	// Pipe a file with 101-char lines through stdin. With a config that
	// sets line-length max to 120, no MDS001 diagnostic should fire.
	dir := t.TempDir()
	line101 := strings.Repeat("a", 101)
	input := "# Title\n\n" + line101 + "\n"

	// Create a config file that increases max line length.
	configContent := "rules:\n  line-length:\n    max: 120\n"
	configPath := writeFixture(t, dir, ".mdsmith.yml", configContent)

	_, stderr, exitCode := runBinary(t, input, "check", "--no-color", "--config", configPath, "-")
	assert.NotContains(t, stderr, "MDS001",
		"expected MDS001 to be suppressed by max=120 setting, but found in stderr: %s", stderr)
	assert.Equal(t, 0, exitCode,
		"expected exit code 0 with max=120 for 101-char line, got %d; stderr: %s",
		exitCode, stderr)
}

// --- Help rule subcommand tests ---

func TestE2E_HelpRule_ByID(t *testing.T) {
	stdout, _, exitCode := runBinary(t, "", "help", "rule", "MDS001")
	assert.Equal(t, 0, exitCode, "expected exit code 0, got %d", exitCode)
	assert.Contains(t, stdout, "MDS001", "expected stdout to contain MDS001, got: %s", stdout)
	assert.Contains(t, stdout, "line-length", "expected stdout to contain 'line-length', got: %s", stdout)
}

func TestE2E_HelpRule_ByName(t *testing.T) {
	stdout, _, exitCode := runBinary(t, "", "help", "rule", "line-length")
	assert.Equal(t, 0, exitCode, "expected exit code 0, got %d", exitCode)
	assert.Contains(t, stdout, "MDS001", "expected stdout to contain MDS001, got: %s", stdout)
}

func TestE2E_HelpRule_UnknownRule_ExitsTwo(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "", "help", "rule", "MDSXXX")
	assert.Equal(t, 2, exitCode, "expected exit code 2, got %d", exitCode)
	assert.Contains(t, stderr, "unknown rule", "expected 'unknown rule' in stderr, got: %s", stderr)
}

func TestE2E_HelpRule_ListAll(t *testing.T) {
	stdout, _, exitCode := runBinary(t, "", "help", "rule")
	assert.Equal(t, 0, exitCode, "expected exit code 0, got %d", exitCode)
	assert.Contains(t, stdout, "MDS001", "expected stdout to contain MDS001, got: %s", stdout)
	assert.Contains(t, stdout, "line-length", "expected stdout to contain 'line-length', got: %s", stdout)
	// Should also include other rules
	assert.Contains(t, stdout, "MDS002", "expected stdout to contain MDS002, got: %s", stdout)
}

func TestE2E_Help_NoArgs_PrintsHelpUsage(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "", "help")
	assert.Equal(t, 0, exitCode, "expected exit code 0, got %d", exitCode)
	assert.Contains(t, stderr, "rule", "expected help usage to mention 'rule', got: %s", stderr)
}

func TestE2E_Help_UnknownTopic_ExitsTwo(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "", "help", "bogus")
	assert.Equal(t, 2, exitCode, "expected exit code 2, got %d", exitCode)
	assert.Contains(t, stderr, "unknown topic", "expected 'unknown topic' in stderr, got: %s", stderr)
}

// --- Metrics command tests ---

func TestE2E_MetricsList_Text(t *testing.T) {
	stdout, _, exitCode := runBinary(t, "", "metrics", "list")
	assert.Equal(t, 0, exitCode, "expected exit code 0, got %d", exitCode)
	assert.Contains(t, stdout, "MET001", "expected MET001 in output, got: %s", stdout)
	assert.Contains(t, stdout, "bytes", "expected bytes metric in output, got: %s", stdout)
}

func TestE2E_MetricsList_JSON(t *testing.T) {
	stdout, _, exitCode := runBinary(t, "", "metrics", "list", "--format", "json")
	assert.Equal(t, 0, exitCode, "expected exit code 0, got %d", exitCode)

	var items []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &items), "stdout is not valid JSON: %s", stdout)
	require.NotEmpty(t, items, "expected non-empty metric list")
}

func TestE2E_HelpMetrics_ListAndLookup(t *testing.T) {
	stdout, _, exitCode := runBinary(t, "", "help", "metrics")
	assert.Equal(t, 0, exitCode, "expected exit code 0, got %d", exitCode)
	assert.Contains(t, stdout, "MET001", "expected MET001 in output, got: %s", stdout)

	stdout, _, exitCode = runBinary(t, "", "help", "metrics", "conciseness")
	assert.Equal(t, 0, exitCode, "expected exit code 0, got %d", exitCode)
	assert.Contains(t, stdout, "MET006", "expected MET006 content, got: %s", stdout)
	assert.Contains(t, stdout, "conciseness", "expected conciseness content, got: %s", stdout)
}

func TestE2E_MetricsRank_ByBytesTop(t *testing.T) {
	dir := t.TempDir()
	_ = writeFixture(t, dir, "small.md", "# S\n\nsmall\n")
	_ = writeFixture(
		t,
		dir,
		"large.md",
		"# Large\n\nThis file has more words and bytes than small.md.\n",
	)

	stdout, stderr, exitCode := runBinaryInDir(
		t,
		dir,
		"",
		"metrics",
		"rank",
		"--by",
		"bytes",
		"--top",
		"1",
		".",
	)
	require.Equal(t, 0, exitCode, "expected exit code 0, got %d; stderr: %s", exitCode, stderr)

	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	require.GreaterOrEqual(t, len(lines), 2, "expected header and one data row, got: %s", stdout)
	require.Contains(t, lines[1], "large.md", "expected top row to include large.md, got row: %s", lines[1])
}

func TestE2E_MetricsRank_ConcisenessDefaultOrder(t *testing.T) {
	dir := t.TempDir()
	_ = writeFixture(
		t,
		dir,
		"verbose.md",
		"In order to make sure we are on the same page, it is important to note that we might adjust this later.\n",
	)
	_ = writeFixture(
		t,
		dir,
		"dense.md",
		"The synchronization algorithm enforces linearizability via monotonic commit indices.\n",
	)

	stdout, stderr, exitCode := runBinaryInDir(
		t,
		dir,
		"",
		"metrics",
		"rank",
		"--by",
		"conciseness",
		"--top",
		"1",
		".",
	)
	require.Equal(t, 0, exitCode, "expected exit code 0, got %d; stderr: %s", exitCode, stderr)

	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	require.GreaterOrEqual(t, len(lines), 2, "expected header and one data row, got: %s", stdout)
	require.Contains(t, lines[1], "verbose.md", "expected least concise file first, got row: %s", lines[1])
}

func TestE2E_MetricsRank_SelectedColumns(t *testing.T) {
	dir := t.TempDir()
	_ = writeFixture(t, dir, "a.md", "# Title\n\nsome text\n")

	stdout, stderr, exitCode := runBinaryInDir(
		t,
		dir,
		"",
		"metrics",
		"rank",
		"--metrics",
		"bytes,lines,words",
		"--by",
		"bytes",
		".",
	)
	require.Equal(t, 0, exitCode, "expected exit code 0, got %d; stderr: %s", exitCode, stderr)

	header := strings.Split(strings.TrimSpace(stdout), "\n")[0]
	require.Contains(t, header, "BYTES", "unexpected header: %s", header)
	require.Contains(t, header, "LINES", "unexpected header: %s", header)
	require.Contains(t, header, "WORDS", "unexpected header: %s", header)
	require.NotContains(t, header, "HEADINGS", "unexpected HEADINGS column in header: %s", header)
}

func TestE2E_MetricsRank_JSONDeterministicTieBreak(t *testing.T) {
	dir := t.TempDir()
	_ = writeFixture(t, dir, "a.md", "same bytes\n")
	_ = writeFixture(t, dir, "b.md", "same bytes\n")

	stdout, stderr, exitCode := runBinaryInDir(
		t,
		dir,
		"",
		"metrics",
		"rank",
		"--metrics",
		"bytes",
		"--by",
		"bytes",
		"--format",
		"json",
		".",
	)
	require.Equal(t, 0, exitCode, "expected exit code 0, got %d; stderr: %s", exitCode, stderr)

	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &rows), "stdout is not valid JSON: %s", stdout)
	require.Len(t, rows, 2, "expected 2 rows, got %d", len(rows))

	firstPath, _ := rows[0]["path"].(string)
	secondPath, _ := rows[1]["path"].(string)
	assert.Contains(t, firstPath, "a.md",
		"expected path tie-break order a.md, b.md; got %q then %q", firstPath, secondPath)
	assert.Contains(t, secondPath, "b.md",
		"expected path tie-break order a.md, b.md; got %q then %q", firstPath, secondPath)
}

func TestE2E_MetricsRank_UnknownMetric_ExitsTwo(t *testing.T) {
	dir := t.TempDir()
	_ = writeFixture(t, dir, "a.md", "# Title\n")

	_, stderr, exitCode := runBinaryInDir(
		t,
		dir,
		"",
		"metrics",
		"rank",
		"--by",
		"not-a-metric",
		".",
	)
	require.Equal(t, 2, exitCode, "expected exit code 2, got %d", exitCode)
	require.Contains(t, stderr, "unknown metric", "expected unknown metric error, got: %s", stderr)
}

func TestE2E_Check_Stdin_ConfigurableSettingsViolation(t *testing.T) {
	// Pipe a file with 130-char lines through stdin. Even with max=120,
	// the 130-char line should still fire MDS001.
	dir := t.TempDir()
	line130 := strings.Repeat("a", 130)
	input := "# Title\n\n" + line130 + "\n"

	configContent := "rules:\n  line-length:\n    max: 120\n"
	configPath := writeFixture(t, dir, ".mdsmith.yml", configContent)

	_, stderr, exitCode := runBinary(t, input, "check", "--no-color", "--config", configPath, "-")
	assert.Equal(t, 1, exitCode,
		"expected exit code 1 for 130-char line with max=120, got %d; stderr: %s",
		exitCode, stderr)
	assert.Contains(t, stderr, "MDS001", "expected MDS001 in stderr, got: %s", stderr)
}

// --- Verbose mode tests ---

func TestE2E_Check_Verbose_ShowsConfigAndFile(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "clean.md", "# Title\n\nSome content here.\n")
	configPath := writeFixture(t, dir, ".mdsmith.yml", "rules:\n  line-length: true\n")

	_, stderr, exitCode := runBinary(t, "", "check", "--verbose", "--no-color", "--config", configPath, path)
	assert.Equal(t, 0, exitCode, "expected exit code 0, got %d; stderr: %s", exitCode, stderr)
	assert.Contains(t, stderr, "config: ", "expected 'config: ' in verbose stderr, got: %s", stderr)
	assert.Contains(t, stderr, "file: ", "expected 'file: ' in verbose stderr, got: %s", stderr)
	assert.Contains(t, stderr, "rule: ", "expected 'rule: ' in verbose stderr, got: %s", stderr)
}

func TestE2E_Check_Verbose_ShortFlag(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "clean.md", "# Title\n\nSome content here.\n")

	_, stderr, exitCode := runBinary(t, "", "check", "-v", "--no-color", path)
	assert.Equal(t, 0, exitCode, "expected exit code 0, got %d; stderr: %s", exitCode, stderr)
	assert.Contains(t, stderr, "file: ", "expected 'file: ' in verbose stderr with -v, got: %s", stderr)
}

func TestE2E_Check_Verbose_SummaryLine(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "dirty.md", "# Title\n\nHello   \n")

	_, stderr, exitCode := runBinary(t, "", "check", "--verbose", "--no-color", path)
	assert.Equal(t, 1, exitCode, "expected exit code 1, got %d", exitCode)
	assert.Contains(t, stderr, "checked 1 files", "expected summary line in verbose output, got: %s", stderr)
	assert.Contains(t, stderr, "issues found", "expected 'issues found' in summary, got: %s", stderr)
}

func TestE2E_Check_QuietSuppressesVerbose(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "dirty.md", "# Title\n\nHello   \n")

	_, stderr, exitCode := runBinary(t, "", "check", "--quiet", "--verbose", "--no-color", path)
	assert.Equal(t, 1, exitCode, "expected exit code 1, got %d", exitCode)
	assert.NotContains(t, stderr, "config:", "expected no verbose output with --quiet, got: %s", stderr)
	assert.NotContains(t, stderr, "file:", "expected no verbose output with --quiet, got: %s", stderr)
	assert.NotContains(t, stderr, "rule:", "expected no verbose output with --quiet, got: %s", stderr)
	assert.NotContains(t, stderr, "checked", "expected no verbose summary with --quiet, got: %s", stderr)
}

func TestE2E_Check_Verbose_JSONStdoutClean(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "dirty.md", "# Title\n\nHello   \n")

	_, stderr, exitCode := runBinary(t, "", "check", "--verbose", "--no-color", "--format", "json", path)
	assert.Equal(t, 1, exitCode, "expected exit code 1, got %d", exitCode)

	// Verbose output should be on stderr, not mixed into JSON.
	// Find the JSON array in stderr (it starts with [ and ends with ]).
	jsonStart := strings.Index(stderr, "[")
	jsonEnd := strings.LastIndex(stderr, "]")
	require.GreaterOrEqual(t, jsonStart, 0, "expected JSON array in stderr, got: %s", stderr)
	require.GreaterOrEqual(t, jsonEnd, 0, "expected JSON array in stderr, got: %s", stderr)
	jsonPart := stderr[jsonStart : jsonEnd+1]

	var diagnostics []map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(jsonPart), &diagnostics),
		"JSON portion of stderr is not valid JSON: %s", jsonPart)

	// Verbose lines should appear somewhere in stderr.
	assert.Contains(t, stderr, "file: ", "expected verbose 'file: ' in stderr, got: %s", stderr)
}

func TestE2E_Fix_Verbose_ShowsFixPasses(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "fixme.md", "# Title\n\nHello   \n")

	_, stderr, exitCode := runBinary(t, "", "fix", "--verbose", "--no-color", path)
	assert.Equal(t, 0, exitCode, "expected exit code 0 after fix, got %d; stderr: %s", exitCode, stderr)
	assert.Contains(t, stderr, "file: ", "expected 'file: ' in verbose stderr, got: %s", stderr)
	assert.Contains(t, stderr, "fix: pass", "expected 'fix: pass' in verbose stderr, got: %s", stderr)
	assert.Contains(t, stderr, "stable after", "expected 'stable after' in verbose stderr, got: %s", stderr)
}

// --- File discovery tests ---

func TestE2E_Check_NoArgs_DiscoversFiles(t *testing.T) {
	dir := t.TempDir()

	// Create a dirty file in the directory.
	writeFixture(t, dir, "dirty.md", "# Title\n\nHello   \n")

	// Create a config with default file patterns.
	writeFixture(t, dir, ".mdsmith.yml", "rules:\n  no-trailing-spaces: true\n")

	// Run check with no file args - should discover and lint dirty.md.
	_, stderr, exitCode := runBinaryInDir(t, dir, "", "check", "--no-color")
	assert.Equal(t, 1, exitCode,
		"expected exit code 1 (violations found via discovery), got %d; stderr: %s",
		exitCode, stderr)
	assert.Contains(t, stderr, "MDS006", "expected MDS006 in stderr, got: %s", stderr)
}

func TestE2E_Check_NoArgs_CleanDirectory(t *testing.T) {
	dir := t.TempDir()

	// Create a clean file.
	writeFixture(t, dir, "clean.md", "# Title\n\nSome content here.\n")

	// Create config.
	writeFixture(t, dir, ".mdsmith.yml", "rules:\n  no-trailing-spaces: true\n")

	_, _, exitCode := runBinaryInDir(t, dir, "", "check", "--no-color")
	assert.Equal(t, 0, exitCode, "expected exit code 0 for clean discovered files, got %d", exitCode)
}

func TestE2E_Check_NoArgs_EmptyFilesConfig(t *testing.T) {
	dir := t.TempDir()

	// Create a dirty file that should not be discovered.
	writeFixture(t, dir, "dirty.md", "# Title\n\nHello   \n")

	// Create config with empty files list.
	writeFixture(t, dir, ".mdsmith.yml", "files: []\nrules:\n  no-trailing-spaces: true\n")

	_, _, exitCode := runBinaryInDir(t, dir, "", "check", "--no-color")
	assert.Equal(t, 0, exitCode, "expected exit code 0 (empty files list means no discovery), got %d", exitCode)
}

func TestE2E_Check_NoArgs_CustomFilesPattern(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "docs"), 0o755))

	// Create files in different directories.
	writeFixture(t, dir, "docs/guide.md", "# Title\n\nHello   \n")
	writeFixture(t, dir, "README.md", "# Title\n\nHello   \n")

	// Config that only discovers files in docs/.
	writeFixture(t, dir, ".mdsmith.yml", "files:\n  - \"docs/**/*.md\"\nrules:\n  no-trailing-spaces: true\n")

	_, stderr, exitCode := runBinaryInDir(t, dir, "", "check", "--no-color")
	assert.Equal(t, 1, exitCode, "expected exit code 1, got %d; stderr: %s", exitCode, stderr)
	// Only docs/guide.md should be discovered.
	assert.Contains(t, stderr, "guide.md", "expected guide.md in stderr, got: %s", stderr)
	assert.NotContains(t, stderr, "README.md", "README.md should not be in results (not in docs/), stderr: %s", stderr)
}

func TestE2E_Check_StdinExplicitDash(t *testing.T) {
	// Passing - reads from stdin.
	_, stderr, exitCode := runBinary(t, "# Hello\n\nWorld   \n", "check", "--no-color", "-")
	assert.Equal(t, 1, exitCode, "expected exit code 1 for stdin with -, got %d", exitCode)
	assert.Contains(t, stderr, "<stdin>", "expected <stdin> in diagnostics, got: %s", stderr)
}

func TestE2E_Fix_NoArgs_DiscoversAndFixes(t *testing.T) {
	dir := t.TempDir()

	// Create a fixable file.
	writeFixture(t, dir, "fixme.md", "# Title\n\nHello   \n")

	// Create config.
	writeFixture(t, dir, ".mdsmith.yml", "rules:\n  no-trailing-spaces: true\n")

	// Run fix with no file args.
	_, _, exitCode := runBinaryInDir(t, dir, "", "fix", "--no-color")
	assert.Equal(t, 0, exitCode, "expected exit code 0 after fix, got %d", exitCode)

	// Verify file was fixed.
	content, err := os.ReadFile(filepath.Join(dir, "fixme.md"))
	require.NoError(t, err, "reading fixed file: %v", err)
	assert.NotContains(t, string(content), "Hello   ", "file still contains trailing spaces after fix")
}

func TestE2E_Fix_StdinDash_Rejected(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "# Hello\n\nWorld   \n", "fix", "-")
	assert.Equal(t, 2, exitCode, "expected exit code 2 for fix with -, got %d", exitCode)
	assert.Contains(t, stderr, "cannot fix stdin in place", "expected error message about stdin fix, got: %s", stderr)
}

func TestE2E_Check_NoArgs_GitignoreRespected(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "vendor"), 0o755))

	// Create a dirty file in an ignored directory.
	writeFixture(t, dir, "vendor/lib.md", "# Title\n\nHello   \n")

	// Create a clean file.
	writeFixture(t, dir, "clean.md", "# Title\n\nSome content here.\n")

	// Create .gitignore.
	writeFixture(t, dir, ".gitignore", "vendor/\n")

	// Create config.
	writeFixture(t, dir, ".mdsmith.yml", "rules:\n  no-trailing-spaces: true\n")

	_, stderr, exitCode := runBinaryInDir(t, dir, "", "check", "--no-color")
	assert.Equal(t, 0, exitCode,
		"expected exit code 0 (vendor ignored via gitignore), got %d; stderr: %s", exitCode, stderr)
}

func TestE2E_Check_NoArgs_NoGitignoreIncludesAll(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "vendor"), 0o755))

	// Create a dirty file in an ignored directory.
	writeFixture(t, dir, "vendor/lib.md", "# Title\n\nHello   \n")

	// Create a clean file.
	writeFixture(t, dir, "clean.md", "# Title\n\nSome content here.\n")

	// Create .gitignore.
	writeFixture(t, dir, ".gitignore", "vendor/\n")

	// Create config.
	writeFixture(t, dir, ".mdsmith.yml", "rules:\n  no-trailing-spaces: true\n")

	_, stderr, exitCode := runBinaryInDir(t, dir, "", "check", "--no-color", "--no-gitignore")
	assert.Equal(t, 1, exitCode,
		"expected exit code 1 (vendor included with --no-gitignore), got %d; stderr: %s",
		exitCode, stderr)
	assert.Contains(t, stderr, "MDS006", "expected MDS006 in stderr, got: %s", stderr)
}

func TestE2E_Check_NoArgs_NoConfig_ExitsZero(t *testing.T) {
	dir := t.TempDir()

	// Empty directory with no config - uses defaults, finds no md files.
	// Create .git boundary so config discovery stops.
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))

	_, _, exitCode := runBinaryInDir(t, dir, "", "check", "--no-color")
	assert.Equal(t, 0, exitCode, "expected exit code 0 for empty dir with no files, got %d", exitCode)
}

// --- no-duplicate-output tests (issue #39) ---

func TestE2E_Check_NoDuplicateOutput(t *testing.T) {
	dir := t.TempDir()
	// Trailing spaces on a line should trigger MDS006.
	path := writeFixture(t, dir, "dirty.md", "# Title\n\nHello   \n")

	_, stderr, exitCode := runBinary(t, "", "check", "--no-color", path)
	require.Equal(t, 1, exitCode, "expected exit code 1, got %d; stderr: %s", exitCode, stderr)

	assert.Equal(t, 1, strings.Count(stderr, "MDS006"),
		"expected MDS006 exactly once; stderr:\n%s", stderr)
	assert.Equal(t, 1, strings.Count(stderr, "stats:"),
		"expected stats line exactly once; stderr:\n%s", stderr)
}

func TestE2E_Check_NoDuplicateOutput_Discovered(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "dirty.md", "# Title\n\nHello   \n")
	writeFixture(t, dir, ".mdsmith.yml", "files:\n  - \"**/*.md\"\n")

	_, stderr, exitCode := runBinaryInDir(t, dir, "", "check", "--no-color")
	require.Equal(t, 1, exitCode, "expected exit code 1, got %d; stderr: %s", exitCode, stderr)

	assert.Equal(t, 1, strings.Count(stderr, "MDS006"),
		"expected MDS006 exactly once; stderr:\n%s", stderr)
	assert.Equal(t, 1, strings.Count(stderr, "stats:"),
		"expected stats line exactly once; stderr:\n%s", stderr)
}

func TestE2E_Check_NoDuplicateOutput_Stdin(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "# Title\n\nHello   \n", "check", "--no-color", "-")
	require.Equal(t, 1, exitCode, "expected exit code 1, got %d; stderr: %s", exitCode, stderr)

	assert.Equal(t, 1, strings.Count(stderr, "MDS006"),
		"expected MDS006 exactly once; stderr:\n%s", stderr)
	assert.Equal(t, 1, strings.Count(stderr, "stats:"),
		"expected stats line exactly once; stderr:\n%s", stderr)
}

func TestE2E_Fix_NoDuplicateOutput_Discovered(t *testing.T) {
	dir := t.TempDir()
	// Trailing punctuation in heading (MDS017) is not auto-fixable,
	// so fix will report it as an unfixed diagnostic.
	writeFixture(t, dir, "dirty.md", "# Title!\n\nHello   \n")
	writeFixture(t, dir, ".mdsmith.yml", "files:\n  - \"**/*.md\"\n")

	_, stderr, exitCode := runBinaryInDir(t, dir, "", "fix", "--no-color")
	require.Equal(t, 1, exitCode, "expected exit code 1, got %d; stderr: %s", exitCode, stderr)

	assert.Equal(t, 1, strings.Count(stderr, "MDS017"),
		"expected MDS017 exactly once; stderr:\n%s", stderr)
	assert.Equal(t, 1, strings.Count(stderr, "stats:"),
		"expected stats line exactly once; stderr:\n%s", stderr)
}

// --- merge-driver tests ---

func TestE2E_MergeDriver_Help(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "", "merge-driver", "--help")
	assert.Equal(t, 0, exitCode, "expected exit 0, got %d", exitCode)
	assert.Contains(t, stderr, "merge-driver", "expected help text, got: %s", stderr)
}

func TestE2E_MergeDriver_NoArgs(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "", "merge-driver")
	assert.Equal(t, 0, exitCode, "expected exit 0, got %d", exitCode)
	assert.Contains(t, stderr, "Usage:", "expected usage in stderr, got: %s", stderr)
}

func TestE2E_MergeDriver_UnknownSubcommand(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "", "merge-driver", "bogus")
	assert.Equal(t, 2, exitCode, "expected exit 2, got %d", exitCode)
	assert.Contains(t, stderr, "unknown subcommand", "expected unknown subcommand error, got: %s", stderr)
}

func TestE2E_MergeDriver_Run_TooFewArgs(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "", "merge-driver", "run", "a", "b")
	assert.Equal(t, 2, exitCode, "expected exit 2, got %d", exitCode)
	assert.Contains(t, stderr, "requires 4 arguments", "expected usage error, got: %s", stderr)
}

func TestE2E_MergeDriver_Run_Help(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "", "merge-driver", "run", "--help")
	assert.Equal(t, 0, exitCode, "expected exit 0, got %d", exitCode)
	assert.Contains(t, stderr, "merge-driver", "expected help text, got: %s", stderr)
}

func TestE2E_MergeDriver_Install_Help(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "", "merge-driver", "install", "--help")
	assert.Equal(t, 0, exitCode, "expected exit 0, got %d", exitCode)
	assert.Contains(t, stderr, "merge-driver", "expected help text, got: %s", stderr)
}

func TestE2E_MergeDriver_CleanMerge(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))

	// Non-overlapping changes: ours edits line 3, theirs edits line 5.
	base := writeFixture(t, dir, "base.md", "# Plans\n\nline a\n\nline b\n")
	ours := writeFixture(t, dir, "ours.md", "# Plans\n\nline a changed\n\nline b\n")
	theirs := writeFixture(t, dir, "theirs.md", "# Plans\n\nline a\n\nline b changed\n")
	pathname := writeFixture(t, dir, "PLAN.md", "# Plans\n\nline a changed\n\nline b\n")

	_, stderr, exitCode := runBinaryInDir(t, dir, "", "merge-driver", "run", base, ours, theirs, pathname)
	assert.Equal(t, 0, exitCode, "expected exit 0 (clean merge), got %d; stderr: %s", exitCode, stderr)

	result, _ := os.ReadFile(ours)
	assert.NotContains(t, string(result), "<<<<<<<", "expected no conflict markers in result")
}

func TestE2E_MergeDriver_CatalogConflict_Resolved(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))

	// Minimal config so fix runs.
	writeFixture(t, dir, ".mdsmith.yml", "rules:\n  catalog: true\n")

	catalogBase := "# Doc\n\n<?catalog\nglob: \"plans/*.md\"\nsort: title\n" +
		"header: |\n  | Title |\n  |-------|\nrow: \"| [{{.title}}]({{.filename}}) |\"\n?>\n" +
		"| Title       |\n|-------------|\n| [Alpha](plans/alpha.md) |\n<?/catalog?>\n"

	// Ours adds beta.
	catalogOurs := "# Doc\n\n<?catalog\nglob: \"plans/*.md\"\nsort: title\n" +
		"header: |\n  | Title |\n  |-------|\nrow: \"| [{{.title}}]({{.filename}}) |\"\n?>\n" +
		"| Title       |\n|-------------|\n| [Alpha](plans/alpha.md) |\n| [Beta](plans/beta.md) |\n<?/catalog?>\n"

	// Theirs adds gamma.
	catalogTheirs := "# Doc\n\n<?catalog\nglob: \"plans/*.md\"\nsort: title\n" +
		"header: |\n  | Title |\n  |-------|\nrow: \"| [{{.title}}]({{.filename}}) |\"\n?>\n" +
		"| Title       |\n|-------------|\n| [Alpha](plans/alpha.md) |\n" +
		"| [Gamma](plans/gamma.md) |\n<?/catalog?>\n"

	base := writeFixture(t, dir, "base.md", catalogBase)
	ours := writeFixture(t, dir, "ours.md", catalogOurs)
	theirs := writeFixture(t, dir, "theirs.md", catalogTheirs)
	pathname := writeFixture(t, dir, "CATALOG.md", catalogOurs)

	// Create plan files so catalog regeneration has source data.
	plansDir := filepath.Join(dir, "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	writeFixture(t, dir, "plans/alpha.md", "---\ntitle: Alpha\n---\n\n# Alpha\n\nContent.\n")
	writeFixture(t, dir, "plans/beta.md", "---\ntitle: Beta\n---\n\n# Beta\n\nContent.\n")
	writeFixture(t, dir, "plans/gamma.md", "---\ntitle: Gamma\n---\n\n# Gamma\n\nContent.\n")

	_, stderr, exitCode := runBinaryInDir(t, dir, "", "merge-driver", "run", base, ours, theirs, pathname)
	assert.Equal(t, 0, exitCode, "expected exit 0 (catalog conflict resolved), got %d; stderr: %s", exitCode, stderr)

	result, _ := os.ReadFile(ours)
	content := string(result)
	assert.NotContains(t, content, "<<<<<<<", "expected no conflict markers after merge-driver")
}

func TestE2E_MergeDriver_NonCatalogConflict_ExitsOne(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	writeFixture(t, dir, ".mdsmith.yml", "rules: {}\n")

	// A conflict outside any catalog section cannot be resolved.
	base := writeFixture(t, dir, "base.md", "# Doc\n\noriginal line\n")
	ours := writeFixture(t, dir, "ours.md", "# Doc\n\nours change\n")
	theirs := writeFixture(t, dir, "theirs.md", "# Doc\n\ntheirs change\n")
	pathname := writeFixture(t, dir, "README.md", "# Doc\n\nours change\n")

	_, _, exitCode := runBinaryInDir(t, dir, "", "merge-driver", "run", base, ours, theirs, pathname)
	assert.Equal(t, 1, exitCode, "expected exit 1 (unresolved conflict), got %d", exitCode)

	result, _ := os.ReadFile(ours)
	assert.Contains(t, string(result), "<<<<<<<", "expected conflict markers preserved for non-catalog conflict")
}

func TestE2E_MergeDriver_Install(t *testing.T) {
	dir := t.TempDir()

	// Initialize a git repo.
	cmd := exec.Command("git", "init", dir)
	require.NoError(t, cmd.Run(), "git init")

	_, stderr, exitCode := runBinaryInDir(t, dir, "", "merge-driver", "install")
	assert.Equal(t, 0, exitCode, "expected exit 0, got %d; stderr: %s", exitCode, stderr)

	// Verify git config.
	out, err := exec.Command("git", "-C", dir, "config", "merge.mdsmith.driver").Output()
	require.NoError(t, err, "reading git config: %v", err)
	driver := strings.TrimSpace(string(out))
	assert.Contains(t, driver, "mdsmith merge-driver run", "expected merge driver config with 'run', got: %s", driver)

	// Verify .gitattributes.
	attrs, err := os.ReadFile(filepath.Join(dir, ".gitattributes"))
	require.NoError(t, err, "reading .gitattributes: %v", err)
	content := string(attrs)
	assert.Contains(t, content, "PLAN.md merge=mdsmith", "expected PLAN.md entry in .gitattributes")
	assert.Contains(t, content, "README.md merge=mdsmith", "expected README.md entry in .gitattributes")
}

func TestE2E_MergeDriver_Install_Idempotent(t *testing.T) {
	dir := t.TempDir()

	cmd := exec.Command("git", "init", dir)
	require.NoError(t, cmd.Run(), "git init")

	// Run install twice.
	runBinaryInDir(t, dir, "", "merge-driver", "install")
	_, stderr, exitCode := runBinaryInDir(t, dir, "", "merge-driver", "install")
	assert.Equal(t, 0, exitCode, "expected exit 0 on second install, got %d; stderr: %s", exitCode, stderr)

	// .gitattributes should not have duplicates.
	attrs, _ := os.ReadFile(filepath.Join(dir, ".gitattributes"))
	count := strings.Count(string(attrs), "PLAN.md merge=mdsmith")
	assert.Equal(t, 1, count, "expected 1 PLAN.md entry, got %d; content:\n%s", count, attrs)
}

func TestE2E_MergeDriver_Install_CustomFiles(t *testing.T) {
	dir := t.TempDir()

	cmd := exec.Command("git", "init", dir)
	require.NoError(t, cmd.Run(), "git init")

	_, stderr, exitCode := runBinaryInDir(t, dir, "",
		"merge-driver", "install", "CHANGELOG.md", "docs/INDEX.md")
	assert.Equal(t, 0, exitCode, "expected exit 0, got %d; stderr: %s", exitCode, stderr)

	// Verify git config.
	out, err := exec.Command("git", "-C", dir, "config", "merge.mdsmith.driver").Output()
	require.NoError(t, err, "reading git config: %v", err)
	driver := strings.TrimSpace(string(out))
	assert.Contains(t, driver, "mdsmith merge-driver run", "expected merge driver config, got: %s", driver)

	// Verify .gitattributes has custom files, not defaults.
	attrs, err := os.ReadFile(filepath.Join(dir, ".gitattributes"))
	require.NoError(t, err, "reading .gitattributes: %v", err)
	content := string(attrs)
	assert.Contains(t, content, "CHANGELOG.md merge=mdsmith", "expected CHANGELOG.md entry in .gitattributes")
	assert.Contains(t, content, "docs/INDEX.md merge=mdsmith", "expected docs/INDEX.md entry in .gitattributes")
	assert.NotContains(t, content, "PLAN.md", "default PLAN.md should not appear when custom files given")
	assert.NotContains(t, content, "README.md", "default README.md should not appear when custom files given")
}

func TestE2E_MergeDriver_IncludeConflict_Resolved(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))

	// Minimal config so fix runs.
	writeFixture(t, dir, ".mdsmith.yml", "rules:\n  include: true\n")

	// Create the included source file.
	writeFixture(t, dir, "snippet.md", "included content\n")

	includeBase := "# Doc\n\n<?include\nfile: snippet.md\n?>\nold content\n<?/include?>\n"
	includeOurs := "# Doc\n\n<?include\nfile: snippet.md\n?>\nours content\n<?/include?>\n"
	includeTheirs := "# Doc\n\n<?include\nfile: snippet.md\n?>\ntheirs content\n<?/include?>\n"

	base := writeFixture(t, dir, "base.md", includeBase)
	ours := writeFixture(t, dir, "ours.md", includeOurs)
	theirs := writeFixture(t, dir, "theirs.md", includeTheirs)
	pathname := writeFixture(t, dir, "DOC.md", includeOurs)

	_, stderr, exitCode := runBinaryInDir(t, dir, "",
		"merge-driver", "run", base, ours, theirs, pathname)
	assert.Equal(t, 0, exitCode,
		"expected exit 0 (include conflict resolved), got %d; stderr: %s",
		exitCode, stderr)

	result, _ := os.ReadFile(ours)
	content := string(result)
	assert.NotContains(t, content, "<<<<<<<", "expected no conflict markers after merge-driver")
	assert.Contains(t, content, "included content", "expected include section to contain regenerated content")
}

func TestE2E_MergeDriver_SetextInSection_Preserved(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))

	writeFixture(t, dir, ".mdsmith.yml", "rules:\n  include: true\n  heading-style: false\n")

	// The included file uses a setext heading (=======) that must
	// not be stripped as a conflict separator.
	writeFixture(t, dir, "snippet.md", "Title\n=======\n\nBody text.\n")

	includeBase := "# Doc\n\n<?include\nfile: snippet.md\n?>\nold\n<?/include?>\n"
	includeOurs := "# Doc\n\n<?include\nfile: snippet.md\n?>\nours\n<?/include?>\n"
	includeTheirs := "# Doc\n\n<?include\nfile: snippet.md\n?>\ntheirs\n<?/include?>\n"

	base := writeFixture(t, dir, "base.md", includeBase)
	ours := writeFixture(t, dir, "ours.md", includeOurs)
	theirs := writeFixture(t, dir, "theirs.md", includeTheirs)
	pathname := writeFixture(t, dir, "DOC.md", includeOurs)

	_, stderr, exitCode := runBinaryInDir(t, dir, "",
		"merge-driver", "run", base, ours, theirs, pathname)
	assert.Equal(t, 0, exitCode, "expected exit 0, got %d; stderr: %s", exitCode, stderr)

	result, _ := os.ReadFile(ours)
	content := string(result)
	assert.Contains(t, content, "=======", "setext heading underline (=======) was incorrectly stripped")
	assert.Contains(t, content, "Title", "expected regenerated include content with Title")
}

func TestE2E_MergeDriver_ConflictStraddlesSection_Preserved(t *testing.T) {
	// A conflict that opens before a section and closes inside it
	// must not have its close marker stripped.
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	writeFixture(t, dir, ".mdsmith.yml", "rules: {}\n")

	// Simulate: ours has a conflict that starts outside and ends
	// inside an include section. We pre-build the conflicted file
	// instead of relying on git merge-file to produce this layout.
	conflicted := "# Doc\n\n<<<<<<< ours\nours text\n=======\n" +
		"<?include\nfile: snippet.md\n?>\ntheirs content\n>>>>>>> theirs\n" +
		"<?/include?>\n"

	base := writeFixture(t, dir, "base.md", "# Doc\n\noriginal\n")
	ours := writeFixture(t, dir, "ours.md", conflicted)
	theirs := writeFixture(t, dir, "theirs.md", "# Doc\n\ntheirs\n")
	pathname := writeFixture(t, dir, "DOC.md", conflicted)

	_, _, exitCode := runBinaryInDir(t, dir, "",
		"merge-driver", "run", base, ours, theirs, pathname)

	// The driver should exit 1 because unresolved conflict markers
	// remain (the conflict straddles a section boundary and can't
	// be auto-resolved).
	assert.Equal(t, 1, exitCode, "expected exit 1 (unresolved conflict), got %d", exitCode)

	result, _ := os.ReadFile(ours)
	content := string(result)
	assert.Contains(t, content, "<<<<<<<", "expected <<<<<<< marker preserved")
	assert.Contains(t, content, ">>>>>>>", "expected >>>>>>> marker preserved (conflict close inside section)")
}

func TestE2E_MergeDriver_SectionMarkersInsideConflict_Preserved(t *testing.T) {
	// When one branch adds a section and another doesn't, the
	// section markers themselves are inside the conflict. All
	// conflict markers must be preserved.
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	writeFixture(t, dir, ".mdsmith.yml", "rules: {}\n")

	conflicted := "# Doc\n\n<<<<<<< ours\n<?include\nfile: snippet.md\n?>\n" +
		"content\n<?/include?>\n=======\nno section\n>>>>>>> theirs\n"

	base := writeFixture(t, dir, "base.md", "# Doc\n\noriginal\n")
	ours := writeFixture(t, dir, "ours.md", conflicted)
	theirs := writeFixture(t, dir, "theirs.md", "# Doc\n\nno section\n")
	pathname := writeFixture(t, dir, "DOC.md", conflicted)

	_, _, exitCode := runBinaryInDir(t, dir, "",
		"merge-driver", "run", base, ours, theirs, pathname)
	assert.Equal(t, 1, exitCode, "expected exit 1 (unresolved conflict), got %d", exitCode)

	result, _ := os.ReadFile(ours)
	content := string(result)
	assert.Contains(t, content, "<<<<<<<", "expected <<<<<<< marker preserved")
	assert.Contains(t, content, "=======", "expected ======= separator preserved")
	assert.Contains(t, content, ">>>>>>>", "expected >>>>>>> marker preserved")
}
