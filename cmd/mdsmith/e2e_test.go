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
)

var binaryPath string

func TestMain(m *testing.M) {
	// Build the binary once for all e2e tests.
	// go test runs from the package directory (cmd/mdsmith/),
	// so "go build ." builds the main package in this directory.
	tmp, err := os.MkdirTemp("", "mdsmith-e2e-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create temp dir: %v\n", err)
		os.Exit(1)
	}

	binaryPath = filepath.Join(tmp, "mdsmith")
	cmd := exec.Command("go", "build", "-o", binaryPath, ".")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to build binary: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()

	_ = os.RemoveAll(tmp)
	os.Exit(code)
}

// runBinary runs the mdsmith binary with the given args and optional stdin.
// It returns stdout, stderr, and the exit code.
func runBinary(t *testing.T, stdin string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()

	cmd := exec.Command(binaryPath, args...)
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
	if len(m) != 5 {
		t.Fatalf("expected stats line in stderr, got: %s", stderr)
	}

	values := make([]int, 4)
	for i := 0; i < 4; i++ {
		v, err := strconv.Atoi(m[i+1])
		if err != nil {
			t.Fatalf("parsing stats value %q: %v", m[i+1], err)
		}
		values[i] = v
	}

	return values[0], values[1], values[2], values[3]
}

// --- Top-level behavior tests ---

func TestE2E_NoArgs_PrintsUsage_ExitsZero(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "")
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(stderr, "Usage:") {
		t.Errorf("expected usage text in stderr, got: %s", stderr)
	}
	if !strings.Contains(stderr, "check") {
		t.Errorf("expected 'check' subcommand in usage, got: %s", stderr)
	}
}

func TestE2E_Help_PrintsUsage_ExitsZero(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "", "--help")
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(stderr, "Usage:") {
		t.Errorf("expected usage text in stderr, got: %s", stderr)
	}
}

func TestE2E_HelpShorthand_PrintsUsage_ExitsZero(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "", "-h")
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(stderr, "Usage:") {
		t.Errorf("expected usage text in stderr, got: %s", stderr)
	}
}

func TestE2E_VersionSubcommand(t *testing.T) {
	stdout, _, exitCode := runBinary(t, "", "version")
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
	if !strings.HasPrefix(stdout, "mdsmith ") {
		t.Errorf("expected version output to start with 'mdsmith ', got: %s", stdout)
	}
}

func TestE2E_VersionFlag_NotRecognized(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "", "--version")
	if exitCode != 2 {
		t.Errorf("expected exit code 2, got %d", exitCode)
	}
	if !strings.Contains(stderr, "unknown command") {
		t.Errorf("expected 'unknown command' in stderr, got: %s", stderr)
	}
}

func TestE2E_VersionShortFlag_NotRecognized(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "", "-v")
	if exitCode != 2 {
		t.Errorf("expected exit code 2, got %d", exitCode)
	}
	if !strings.Contains(stderr, "unknown command") {
		t.Errorf("expected 'unknown command' in stderr, got: %s", stderr)
	}
}

func TestE2E_UnknownCommand_ExitsTwo(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "", "bogus")
	if exitCode != 2 {
		t.Errorf("expected exit code 2, got %d", exitCode)
	}
	if !strings.Contains(stderr, "unknown command") {
		t.Errorf("expected 'unknown command' in stderr, got: %s", stderr)
	}
}

func TestE2E_FilePathWithoutSubcommand_ExitsTwo(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "test.md", "# Title\n\nHello   \n")

	// Passing a file path without a subcommand should exit 2.
	_, stderr, exitCode := runBinary(t, "", path)
	if exitCode != 2 {
		t.Errorf("expected exit code 2, got %d", exitCode)
	}
	if !strings.Contains(stderr, "unknown command") {
		t.Errorf("expected 'unknown command' in stderr, got: %s", stderr)
	}
}

func TestE2E_LegacyFixFlag_ExitsTwo(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "test.md", "# Title\n\nHello   \n")

	// Passing --fix without a subcommand should exit 2.
	_, stderr, exitCode := runBinary(t, "", "--fix", path)
	if exitCode != 2 {
		t.Errorf("expected exit code 2, got %d", exitCode)
	}
	if !strings.Contains(stderr, "unknown command") {
		t.Errorf("expected 'unknown command' in stderr, got: %s", stderr)
	}
}

// --- Check subcommand tests ---

func TestE2E_Check_CleanFile_ExitsZero(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "clean.md", "# Title\n\nSome content here.\n")

	_, _, exitCode := runBinary(t, "", "check", "--no-color", path)
	if exitCode != 0 {
		t.Errorf("expected exit code 0 for clean file, got %d", exitCode)
	}
}

func TestE2E_Check_PrintsStats(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "clean.md", "# Title\n\nSome content here.\n")

	_, stderr, exitCode := runBinary(t, "", "check", "--no-color", path)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", exitCode, stderr)
	}

	checked, fixed, failures, unfixed := parseStats(t, stderr)
	if checked != 1 {
		t.Errorf("expected checked=1, got %d", checked)
	}
	if fixed != 0 {
		t.Errorf("expected fixed=0 for check, got %d", fixed)
	}
	if failures != 0 {
		t.Errorf("expected failures=0, got %d", failures)
	}
	if unfixed != 0 {
		t.Errorf("expected unfixed=0, got %d", unfixed)
	}
}

func TestE2E_Check_Violations_ExitsOne(t *testing.T) {
	dir := t.TempDir()
	// Trailing spaces on lines should trigger MDS006.
	path := writeFixture(t, dir, "dirty.md", "# Title\n\nHello   \n")

	_, stderr, exitCode := runBinary(t, "", "check", "--no-color", path)
	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}
	if !strings.Contains(stderr, "MDS006") {
		t.Errorf("expected stderr to contain MDS006, got: %s", stderr)
	}
	if !strings.Contains(stderr, "trailing whitespace") {
		t.Errorf("expected stderr to contain 'trailing whitespace', got: %s", stderr)
	}
}

func TestE2E_Check_JSONFormat(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "dirty.md", "# Title\n\nHello   \n")

	_, stderr, exitCode := runBinary(t, "", "check", "--no-color", "--format", "json", path)
	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}

	// Validate JSON output.
	var diagnostics []map[string]interface{}
	if err := json.Unmarshal([]byte(stderr), &diagnostics); err != nil {
		t.Fatalf("stderr is not valid JSON: %v\nstderr: %s", err, stderr)
	}

	if len(diagnostics) == 0 {
		t.Fatal("expected at least one diagnostic in JSON output")
	}

	// Check the JSON schema has required fields.
	d := diagnostics[0]
	requiredFields := []string{"file", "line", "column", "rule", "name", "severity", "message"}
	for _, field := range requiredFields {
		if _, ok := d[field]; !ok {
			t.Errorf("JSON diagnostic missing required field %q", field)
		}
	}

	// Check that the file field points to our fixture.
	fileVal, _ := d["file"].(string)
	if !strings.HasSuffix(fileVal, "dirty.md") {
		t.Errorf("expected file field to end with dirty.md, got %q", fileVal)
	}
}

func TestE2E_Check_Stdin_Clean(t *testing.T) {
	_, _, exitCode := runBinary(t, "# Hello\n\nWorld.\n", "check", "-")
	if exitCode != 0 {
		t.Errorf("expected exit code 0 for clean stdin, got %d", exitCode)
	}
}

func TestE2E_Check_Stdin_Violations(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "# Hello\n\nWorld   \n", "check", "--no-color", "-")
	if exitCode != 1 {
		t.Errorf("expected exit code 1 for stdin with violations, got %d", exitCode)
	}
	if !strings.Contains(stderr, "<stdin>") {
		t.Errorf("expected diagnostics to use <stdin> as file name, got: %s", stderr)
	}
	if !strings.Contains(stderr, "MDS006") {
		t.Errorf("expected MDS006 in stderr, got: %s", stderr)
	}
}

func TestE2E_Check_Stdin_JSONFormat(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "# Hello\n\nWorld   \n", "check", "--no-color", "--format", "json", "-")
	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}

	var diagnostics []map[string]interface{}
	if err := json.Unmarshal([]byte(stderr), &diagnostics); err != nil {
		t.Fatalf("stderr is not valid JSON: %v\nstderr: %s", err, stderr)
	}

	if len(diagnostics) == 0 {
		t.Fatal("expected at least one diagnostic")
	}

	fileVal, _ := diagnostics[0]["file"].(string)
	if fileVal != "<stdin>" {
		t.Errorf("expected file to be \"<stdin>\", got %q", fileVal)
	}
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
	if strings.Contains(stderr, "MDS006") {
		t.Errorf("expected MDS006 to be suppressed by config, but found in stderr: %s", stderr)
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0 with rule disabled, got %d", exitCode)
	}
}

func TestE2E_Check_Gitignore_SkipsIgnoredDirectory(t *testing.T) {
	dir := t.TempDir()
	ignoredDir := filepath.Join(dir, "ignored")
	if err := os.MkdirAll(ignoredDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Place a clean file at root and a file with violations in ignored/.
	writeFixture(t, dir, "clean.md", "# Title\n\nSome content here.\n")
	writeFixture(t, ignoredDir, "dirty.md", "# Title\n\nHello   \n")

	// Create .gitignore that excludes the ignored directory.
	writeFixture(t, dir, ".gitignore", "ignored/\n")

	// Run mdsmith on the directory -- the ignored file should be skipped.
	_, stderr, exitCode := runBinary(t, "", "check", "--no-color", dir)
	if exitCode != 0 {
		t.Errorf("expected exit code 0 (ignored file skipped), got %d; stderr: %s", exitCode, stderr)
	}
}

func TestE2E_Check_NoGitignore_IncludesIgnoredDirectory(t *testing.T) {
	dir := t.TempDir()
	ignoredDir := filepath.Join(dir, "ignored")
	if err := os.MkdirAll(ignoredDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Place a clean file at root and a file with violations in ignored/.
	writeFixture(t, dir, "clean.md", "# Title\n\nSome content here.\n")
	writeFixture(t, ignoredDir, "dirty.md", "# Title\n\nHello   \n")

	// Create .gitignore that excludes the ignored directory.
	writeFixture(t, dir, ".gitignore", "ignored/\n")

	// Run with --no-gitignore -- the violated file should be found.
	_, stderr, exitCode := runBinary(t, "", "check", "--no-color", "--no-gitignore", dir)
	if exitCode != 1 {
		t.Errorf("expected exit code 1 (violations found with --no-gitignore), got %d; stderr: %s", exitCode, stderr)
	}
	if !strings.Contains(stderr, "MDS006") {
		t.Errorf("expected MDS006 in stderr, got: %s", stderr)
	}
}

// --- Fix subcommand tests ---

func TestE2E_Fix_FixableFile(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "fixme.md", "# Title\n\nHello   \n")

	// Run fix subcommand.
	_, _, exitCode := runBinary(t, "", "fix", "--no-color", path)
	if exitCode != 0 {
		t.Errorf("expected exit code 0 after fix, got %d", exitCode)
	}

	// Read the file back and check that trailing spaces are removed.
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading fixed file: %v", err)
	}
	if strings.Contains(string(content), "Hello   ") {
		t.Error("file still contains trailing spaces after fix")
	}
	if !strings.Contains(string(content), "Hello") {
		t.Error("file does not contain expected content after fix")
	}

	// Re-run check; should exit 0 now.
	_, _, exitCode = runBinary(t, "", "check", "--no-color", path)
	if exitCode != 0 {
		t.Errorf("expected exit code 0 on re-lint after fix, got %d", exitCode)
	}
}

func TestE2E_Fix_PreservesFrontMatter(t *testing.T) {
	dir := t.TempDir()
	content := "---\ntitle: hello\n---\n# Title\n\nHello   \n"
	path := writeFixture(t, dir, "fm.md", content)

	// Run fix subcommand.
	_, _, exitCode := runBinary(t, "", "fix", "--no-color", path)
	if exitCode != 0 {
		t.Errorf("expected exit code 0 after fix, got %d", exitCode)
	}

	// Read the file back.
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading fixed file: %v", err)
	}

	// Frontmatter should be preserved intact.
	expectedFM := "---\ntitle: hello\n---\n"
	if !strings.HasPrefix(string(got), expectedFM) {
		t.Errorf("frontmatter not preserved; got prefix %q, want %q",
			string(got[:len(expectedFM)]), expectedFM)
	}

	// Content should be fixed (trailing spaces removed).
	body := string(got[len(expectedFM):])
	if strings.Contains(body, "Hello   ") {
		t.Error("file still contains trailing spaces after fix")
	}
	if !strings.Contains(body, "Hello") {
		t.Error("file does not contain expected content after fix")
	}
}

func TestE2E_Fix_Stdin_Rejected(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "# Hello\n\nWorld   \n", "fix", "-")
	if exitCode != 2 {
		t.Errorf("expected exit code 2 for fix with stdin, got %d", exitCode)
	}
	if !strings.Contains(stderr, "cannot fix stdin in place") {
		t.Errorf("expected error message about stdin fix, got: %s", stderr)
	}
}

func TestE2E_Fix_PrintsStatsWithUnfixedFailures(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "partially-fixable.md", "# Title!\n\nHello   \n")

	_, stderr, exitCode := runBinary(t, "", "fix", "--no-color", path)
	if exitCode != 1 {
		t.Fatalf("expected exit code 1 with remaining non-fixable issue, got %d; stderr: %s", exitCode, stderr)
	}

	checked, fixed, failures, unfixed := parseStats(t, stderr)
	if checked != 1 {
		t.Errorf("expected checked=1, got %d", checked)
	}
	if fixed != 1 {
		t.Errorf("expected fixed=1, got %d", fixed)
	}
	if failures < 1 {
		t.Errorf("expected failures >= 1, got %d", failures)
	}
	if unfixed < 1 {
		t.Errorf("expected unfixed >= 1, got %d", unfixed)
	}
	if failures < unfixed {
		t.Errorf("expected failures >= unfixed, got failures=%d unfixed=%d", failures, unfixed)
	}
}

// --- Init subcommand tests ---

func TestE2E_Init_CreatesConfig(t *testing.T) {
	dir := t.TempDir()

	_, stderr, exitCode := runBinaryInDir(t, dir, "", "init")
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d; stderr: %s", exitCode, stderr)
	}
	if !strings.Contains(stderr, "created .mdsmith.yml") {
		t.Errorf("expected confirmation message, got: %s", stderr)
	}

	// Check that the file was created.
	configPath := filepath.Join(dir, ".mdsmith.yml")
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("reading config file: %v", err)
	}

	// Verify it contains some expected content.
	s := string(content)
	if !strings.Contains(s, "rules:") {
		t.Errorf("config file should contain 'rules:', got: %s", s)
	}
	if !strings.Contains(s, "front-matter:") {
		t.Errorf("config file should contain 'front-matter:', got: %s", s)
	}
	if !strings.Contains(s, "line-length") {
		t.Errorf("config file should contain 'line-length', got: %s", s)
	}
}

func TestE2E_Init_RefusesIfExists(t *testing.T) {
	dir := t.TempDir()

	// Create an existing config file.
	writeFixture(t, dir, ".mdsmith.yml", "rules: {}\n")

	_, stderr, exitCode := runBinaryInDir(t, dir, "", "init")
	if exitCode != 2 {
		t.Errorf("expected exit code 2, got %d", exitCode)
	}
	if !strings.Contains(stderr, "already exists") {
		t.Errorf("expected 'already exists' error, got: %s", stderr)
	}
}

// --- Stdin frontmatter and Configurable settings tests ---

func TestE2E_Check_Stdin_FrontMatterLineOffset(t *testing.T) {
	// Pipe content with YAML front matter followed by a line with trailing
	// spaces. The reported line number should reflect the original file
	// (including front matter lines), not the stripped content.
	input := "---\ntitle: hello\n---\n# Title\n\nHello   \n"
	// "Hello   " is on line 6 of the original.
	_, stderr, exitCode := runBinary(t, input, "check", "--no-color", "-")
	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d; stderr: %s", exitCode, stderr)
	}
	if !strings.Contains(stderr, "MDS006") {
		t.Errorf("expected MDS006 in stderr, got: %s", stderr)
	}
	// Verify the line number is 6 (original file), not 3 (stripped content).
	if !strings.Contains(stderr, "<stdin>:6:") {
		t.Errorf("expected line 6 in diagnostic, got: %s", stderr)
	}
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
	if strings.Contains(stderr, "MDS001") {
		t.Errorf("expected MDS001 to be suppressed by max=120 setting, but found in stderr: %s", stderr)
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0 with max=120 for 101-char line, got %d; stderr: %s", exitCode, stderr)
	}
}

// --- Help rule subcommand tests ---

func TestE2E_HelpRule_ByID(t *testing.T) {
	stdout, _, exitCode := runBinary(t, "", "help", "rule", "MDS001")
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(stdout, "MDS001") {
		t.Errorf("expected stdout to contain MDS001, got: %s", stdout)
	}
	if !strings.Contains(stdout, "line-length") {
		t.Errorf("expected stdout to contain 'line-length', got: %s", stdout)
	}
}

func TestE2E_HelpRule_ByName(t *testing.T) {
	stdout, _, exitCode := runBinary(t, "", "help", "rule", "line-length")
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(stdout, "MDS001") {
		t.Errorf("expected stdout to contain MDS001, got: %s", stdout)
	}
}

func TestE2E_HelpRule_UnknownRule_ExitsTwo(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "", "help", "rule", "MDSXXX")
	if exitCode != 2 {
		t.Errorf("expected exit code 2, got %d", exitCode)
	}
	if !strings.Contains(stderr, "unknown rule") {
		t.Errorf("expected 'unknown rule' in stderr, got: %s", stderr)
	}
}

func TestE2E_HelpRule_ListAll(t *testing.T) {
	stdout, _, exitCode := runBinary(t, "", "help", "rule")
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(stdout, "MDS001") {
		t.Errorf("expected stdout to contain MDS001, got: %s", stdout)
	}
	if !strings.Contains(stdout, "line-length") {
		t.Errorf("expected stdout to contain 'line-length', got: %s", stdout)
	}
	// Should also include other rules
	if !strings.Contains(stdout, "MDS002") {
		t.Errorf("expected stdout to contain MDS002, got: %s", stdout)
	}
}

func TestE2E_Help_NoArgs_PrintsHelpUsage(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "", "help")
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(stderr, "rule") {
		t.Errorf("expected help usage to mention 'rule', got: %s", stderr)
	}
}

func TestE2E_Help_UnknownTopic_ExitsTwo(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "", "help", "bogus")
	if exitCode != 2 {
		t.Errorf("expected exit code 2, got %d", exitCode)
	}
	if !strings.Contains(stderr, "unknown topic") {
		t.Errorf("expected 'unknown topic' in stderr, got: %s", stderr)
	}
}

// --- Metrics command tests ---

func TestE2E_MetricsList_Text(t *testing.T) {
	stdout, _, exitCode := runBinary(t, "", "metrics", "list")
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(stdout, "MET001") {
		t.Errorf("expected MET001 in output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "bytes") {
		t.Errorf("expected bytes metric in output, got: %s", stdout)
	}
}

func TestE2E_MetricsList_JSON(t *testing.T) {
	stdout, _, exitCode := runBinary(t, "", "metrics", "list", "--format", "json")
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}

	var items []map[string]any
	if err := json.Unmarshal([]byte(stdout), &items); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nstdout: %s", err, stdout)
	}
	if len(items) == 0 {
		t.Fatal("expected non-empty metric list")
	}
}

func TestE2E_HelpMetrics_ListAndLookup(t *testing.T) {
	stdout, _, exitCode := runBinary(t, "", "help", "metrics")
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(stdout, "MET001") {
		t.Errorf("expected MET001 in output, got: %s", stdout)
	}

	stdout, _, exitCode = runBinary(t, "", "help", "metrics", "conciseness")
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(stdout, "MET006") {
		t.Errorf("expected MET006 content, got: %s", stdout)
	}
	if !strings.Contains(stdout, "conciseness") {
		t.Errorf("expected conciseness content, got: %s", stdout)
	}
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
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", exitCode, stderr)
	}

	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected header and one data row, got: %s", stdout)
	}
	if !strings.Contains(lines[1], "large.md") {
		t.Fatalf("expected top row to include large.md, got row: %s", lines[1])
	}
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
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", exitCode, stderr)
	}

	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected header and one data row, got: %s", stdout)
	}
	if !strings.Contains(lines[1], "verbose.md") {
		t.Fatalf("expected least concise file first, got row: %s", lines[1])
	}
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
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", exitCode, stderr)
	}

	header := strings.Split(strings.TrimSpace(stdout), "\n")[0]
	if !strings.Contains(header, "BYTES") ||
		!strings.Contains(header, "LINES") ||
		!strings.Contains(header, "WORDS") {
		t.Fatalf("unexpected header: %s", header)
	}
	if strings.Contains(header, "HEADINGS") {
		t.Fatalf("unexpected HEADINGS column in header: %s", header)
	}
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
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", exitCode, stderr)
	}

	var rows []map[string]any
	if err := json.Unmarshal([]byte(stdout), &rows); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nstdout: %s", err, stdout)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	firstPath, _ := rows[0]["path"].(string)
	secondPath, _ := rows[1]["path"].(string)
	if !strings.Contains(firstPath, "a.md") || !strings.Contains(secondPath, "b.md") {
		t.Fatalf("expected path tie-break order a.md, b.md; got %q then %q", firstPath, secondPath)
	}
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
	if exitCode != 2 {
		t.Fatalf("expected exit code 2, got %d", exitCode)
	}
	if !strings.Contains(stderr, "unknown metric") {
		t.Fatalf("expected unknown metric error, got: %s", stderr)
	}
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
	if exitCode != 1 {
		t.Errorf("expected exit code 1 for 130-char line with max=120, got %d; stderr: %s", exitCode, stderr)
	}
	if !strings.Contains(stderr, "MDS001") {
		t.Errorf("expected MDS001 in stderr, got: %s", stderr)
	}
}

// --- Verbose mode tests ---

func TestE2E_Check_Verbose_ShowsConfigAndFile(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "clean.md", "# Title\n\nSome content here.\n")
	configPath := writeFixture(t, dir, ".mdsmith.yml", "rules:\n  line-length: true\n")

	_, stderr, exitCode := runBinary(t, "", "check", "--verbose", "--no-color", "--config", configPath, path)
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d; stderr: %s", exitCode, stderr)
	}
	if !strings.Contains(stderr, "config: ") {
		t.Errorf("expected 'config: ' in verbose stderr, got: %s", stderr)
	}
	if !strings.Contains(stderr, "file: ") {
		t.Errorf("expected 'file: ' in verbose stderr, got: %s", stderr)
	}
	if !strings.Contains(stderr, "rule: ") {
		t.Errorf("expected 'rule: ' in verbose stderr, got: %s", stderr)
	}
}

func TestE2E_Check_Verbose_ShortFlag(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "clean.md", "# Title\n\nSome content here.\n")

	_, stderr, exitCode := runBinary(t, "", "check", "-v", "--no-color", path)
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d; stderr: %s", exitCode, stderr)
	}
	if !strings.Contains(stderr, "file: ") {
		t.Errorf("expected 'file: ' in verbose stderr with -v, got: %s", stderr)
	}
}

func TestE2E_Check_Verbose_SummaryLine(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "dirty.md", "# Title\n\nHello   \n")

	_, stderr, exitCode := runBinary(t, "", "check", "--verbose", "--no-color", path)
	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}
	if !strings.Contains(stderr, "checked 1 files") {
		t.Errorf("expected summary line in verbose output, got: %s", stderr)
	}
	if !strings.Contains(stderr, "issues found") {
		t.Errorf("expected 'issues found' in summary, got: %s", stderr)
	}
}

func TestE2E_Check_QuietSuppressesVerbose(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "dirty.md", "# Title\n\nHello   \n")

	_, stderr, exitCode := runBinary(t, "", "check", "--quiet", "--verbose", "--no-color", path)
	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}
	if strings.Contains(stderr, "config:") {
		t.Errorf("expected no verbose output with --quiet, got: %s", stderr)
	}
	if strings.Contains(stderr, "file:") {
		t.Errorf("expected no verbose output with --quiet, got: %s", stderr)
	}
	if strings.Contains(stderr, "rule:") {
		t.Errorf("expected no verbose output with --quiet, got: %s", stderr)
	}
	if strings.Contains(stderr, "checked") {
		t.Errorf("expected no verbose summary with --quiet, got: %s", stderr)
	}
}

func TestE2E_Check_Verbose_JSONStdoutClean(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "dirty.md", "# Title\n\nHello   \n")

	_, stderr, exitCode := runBinary(t, "", "check", "--verbose", "--no-color", "--format", "json", path)
	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}

	// Verbose output should be on stderr, not mixed into JSON.
	// Find the JSON array in stderr (it starts with [ and ends with ]).
	jsonStart := strings.Index(stderr, "[")
	jsonEnd := strings.LastIndex(stderr, "]")
	if jsonStart < 0 || jsonEnd < 0 {
		t.Fatalf("expected JSON array in stderr, got: %s", stderr)
	}
	jsonPart := stderr[jsonStart : jsonEnd+1]

	var diagnostics []map[string]interface{}
	if err := json.Unmarshal([]byte(jsonPart), &diagnostics); err != nil {
		t.Fatalf("JSON portion of stderr is not valid JSON: %v\njson: %s", err, jsonPart)
	}

	// Verbose lines should appear somewhere in stderr.
	if !strings.Contains(stderr, "file: ") {
		t.Errorf("expected verbose 'file: ' in stderr, got: %s", stderr)
	}
}

func TestE2E_Fix_Verbose_ShowsFixPasses(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "fixme.md", "# Title\n\nHello   \n")

	_, stderr, exitCode := runBinary(t, "", "fix", "--verbose", "--no-color", path)
	if exitCode != 0 {
		t.Errorf("expected exit code 0 after fix, got %d; stderr: %s", exitCode, stderr)
	}
	if !strings.Contains(stderr, "file: ") {
		t.Errorf("expected 'file: ' in verbose stderr, got: %s", stderr)
	}
	if !strings.Contains(stderr, "fix: pass") {
		t.Errorf("expected 'fix: pass' in verbose stderr, got: %s", stderr)
	}
	if !strings.Contains(stderr, "stable after") {
		t.Errorf("expected 'stable after' in verbose stderr, got: %s", stderr)
	}
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
	if exitCode != 1 {
		t.Errorf("expected exit code 1 (violations found via discovery), got %d; stderr: %s", exitCode, stderr)
	}
	if !strings.Contains(stderr, "MDS006") {
		t.Errorf("expected MDS006 in stderr, got: %s", stderr)
	}
}

func TestE2E_Check_NoArgs_CleanDirectory(t *testing.T) {
	dir := t.TempDir()

	// Create a clean file.
	writeFixture(t, dir, "clean.md", "# Title\n\nSome content here.\n")

	// Create config.
	writeFixture(t, dir, ".mdsmith.yml", "rules:\n  no-trailing-spaces: true\n")

	_, _, exitCode := runBinaryInDir(t, dir, "", "check", "--no-color")
	if exitCode != 0 {
		t.Errorf("expected exit code 0 for clean discovered files, got %d", exitCode)
	}
}

func TestE2E_Check_NoArgs_EmptyFilesConfig(t *testing.T) {
	dir := t.TempDir()

	// Create a dirty file that should not be discovered.
	writeFixture(t, dir, "dirty.md", "# Title\n\nHello   \n")

	// Create config with empty files list.
	writeFixture(t, dir, ".mdsmith.yml", "files: []\nrules:\n  no-trailing-spaces: true\n")

	_, _, exitCode := runBinaryInDir(t, dir, "", "check", "--no-color")
	if exitCode != 0 {
		t.Errorf("expected exit code 0 (empty files list means no discovery), got %d", exitCode)
	}
}

func TestE2E_Check_NoArgs_CustomFilesPattern(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Create files in different directories.
	writeFixture(t, dir, "docs/guide.md", "# Title\n\nHello   \n")
	writeFixture(t, dir, "README.md", "# Title\n\nHello   \n")

	// Config that only discovers files in docs/.
	writeFixture(t, dir, ".mdsmith.yml", "files:\n  - \"docs/**/*.md\"\nrules:\n  no-trailing-spaces: true\n")

	_, stderr, exitCode := runBinaryInDir(t, dir, "", "check", "--no-color")
	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d; stderr: %s", exitCode, stderr)
	}
	// Only docs/guide.md should be discovered.
	if !strings.Contains(stderr, "guide.md") {
		t.Errorf("expected guide.md in stderr, got: %s", stderr)
	}
	if strings.Contains(stderr, "README.md") {
		t.Errorf("README.md should not be in results (not in docs/), stderr: %s", stderr)
	}
}

func TestE2E_Check_StdinExplicitDash(t *testing.T) {
	// Passing - reads from stdin.
	_, stderr, exitCode := runBinary(t, "# Hello\n\nWorld   \n", "check", "--no-color", "-")
	if exitCode != 1 {
		t.Errorf("expected exit code 1 for stdin with -, got %d", exitCode)
	}
	if !strings.Contains(stderr, "<stdin>") {
		t.Errorf("expected <stdin> in diagnostics, got: %s", stderr)
	}
}

func TestE2E_Fix_NoArgs_DiscoversAndFixes(t *testing.T) {
	dir := t.TempDir()

	// Create a fixable file.
	writeFixture(t, dir, "fixme.md", "# Title\n\nHello   \n")

	// Create config.
	writeFixture(t, dir, ".mdsmith.yml", "rules:\n  no-trailing-spaces: true\n")

	// Run fix with no file args.
	_, _, exitCode := runBinaryInDir(t, dir, "", "fix", "--no-color")
	if exitCode != 0 {
		t.Errorf("expected exit code 0 after fix, got %d", exitCode)
	}

	// Verify file was fixed.
	content, err := os.ReadFile(filepath.Join(dir, "fixme.md"))
	if err != nil {
		t.Fatalf("reading fixed file: %v", err)
	}
	if strings.Contains(string(content), "Hello   ") {
		t.Error("file still contains trailing spaces after fix")
	}
}

func TestE2E_Fix_StdinDash_Rejected(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "# Hello\n\nWorld   \n", "fix", "-")
	if exitCode != 2 {
		t.Errorf("expected exit code 2 for fix with -, got %d", exitCode)
	}
	if !strings.Contains(stderr, "cannot fix stdin in place") {
		t.Errorf("expected error message about stdin fix, got: %s", stderr)
	}
}

func TestE2E_Check_NoArgs_GitignoreRespected(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "vendor"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Create a dirty file in an ignored directory.
	writeFixture(t, dir, "vendor/lib.md", "# Title\n\nHello   \n")

	// Create a clean file.
	writeFixture(t, dir, "clean.md", "# Title\n\nSome content here.\n")

	// Create .gitignore.
	writeFixture(t, dir, ".gitignore", "vendor/\n")

	// Create config.
	writeFixture(t, dir, ".mdsmith.yml", "rules:\n  no-trailing-spaces: true\n")

	_, stderr, exitCode := runBinaryInDir(t, dir, "", "check", "--no-color")
	if exitCode != 0 {
		t.Errorf("expected exit code 0 (vendor ignored via gitignore), got %d; stderr: %s", exitCode, stderr)
	}
}

func TestE2E_Check_NoArgs_NoGitignoreIncludesAll(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "vendor"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Create a dirty file in an ignored directory.
	writeFixture(t, dir, "vendor/lib.md", "# Title\n\nHello   \n")

	// Create a clean file.
	writeFixture(t, dir, "clean.md", "# Title\n\nSome content here.\n")

	// Create .gitignore.
	writeFixture(t, dir, ".gitignore", "vendor/\n")

	// Create config.
	writeFixture(t, dir, ".mdsmith.yml", "rules:\n  no-trailing-spaces: true\n")

	_, stderr, exitCode := runBinaryInDir(t, dir, "", "check", "--no-color", "--no-gitignore")
	if exitCode != 1 {
		t.Errorf("expected exit code 1 (vendor included with --no-gitignore), got %d; stderr: %s", exitCode, stderr)
	}
	if !strings.Contains(stderr, "MDS006") {
		t.Errorf("expected MDS006 in stderr, got: %s", stderr)
	}
}

func TestE2E_Check_NoArgs_NoConfig_ExitsZero(t *testing.T) {
	dir := t.TempDir()

	// Empty directory with no config - uses defaults, finds no md files.
	// Create .git boundary so config discovery stops.
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, _, exitCode := runBinaryInDir(t, dir, "", "check", "--no-color")
	if exitCode != 0 {
		t.Errorf("expected exit code 0 for empty dir with no files, got %d", exitCode)
	}
}
