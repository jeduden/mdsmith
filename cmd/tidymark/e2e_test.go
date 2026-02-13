package main_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var binaryPath string

func TestMain(m *testing.M) {
	// Build the binary once for all e2e tests.
	// go test runs from the package directory (cmd/tidymark/),
	// so "go build ." builds the main package in this directory.
	tmp, err := os.MkdirTemp("", "tidymark-e2e-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create temp dir: %v\n", err)
		os.Exit(1)
	}

	binaryPath = filepath.Join(tmp, "tidymark")
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

// runBinary runs the tidymark binary with the given args and optional stdin.
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

// runBinaryInDir runs the tidymark binary with the given args in the given directory.
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
	if !strings.HasPrefix(stdout, "tidymark ") {
		t.Errorf("expected version output to start with 'tidymark ', got: %s", stdout)
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

func TestE2E_Check_Violations_ExitsOne(t *testing.T) {
	dir := t.TempDir()
	// Trailing spaces on lines should trigger TM006.
	path := writeFixture(t, dir, "dirty.md", "# Title\n\nHello   \n")

	_, stderr, exitCode := runBinary(t, "", "check", "--no-color", path)
	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}
	if !strings.Contains(stderr, "TM006") {
		t.Errorf("expected stderr to contain TM006, got: %s", stderr)
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
	_, _, exitCode := runBinary(t, "# Hello\n\nWorld.\n", "check")
	if exitCode != 0 {
		t.Errorf("expected exit code 0 for clean stdin, got %d", exitCode)
	}
}

func TestE2E_Check_Stdin_Violations(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "# Hello\n\nWorld   \n", "check", "--no-color")
	if exitCode != 1 {
		t.Errorf("expected exit code 1 for stdin with violations, got %d", exitCode)
	}
	if !strings.Contains(stderr, "<stdin>") {
		t.Errorf("expected diagnostics to use <stdin> as file name, got: %s", stderr)
	}
	if !strings.Contains(stderr, "TM006") {
		t.Errorf("expected TM006 in stderr, got: %s", stderr)
	}
}

func TestE2E_Check_Stdin_JSONFormat(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "# Hello\n\nWorld   \n", "check", "--no-color", "--format", "json")
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

	// Create a file that violates no-trailing-spaces (TM006).
	path := writeFixture(t, dir, "test.md", "# Title\n\nHello   \n")

	// Create a config that disables no-trailing-spaces.
	configContent := "rules:\n  no-trailing-spaces: false\n"
	configPath := writeFixture(t, dir, ".tidymark.yml", configContent)

	// Run with the custom config; the violation should be suppressed.
	_, stderr, exitCode := runBinary(t, "", "check", "--no-color", "--config", configPath, path)
	if strings.Contains(stderr, "TM006") {
		t.Errorf("expected TM006 to be suppressed by config, but found in stderr: %s", stderr)
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

	// Run tidymark on the directory -- the ignored file should be skipped.
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
	if !strings.Contains(stderr, "TM006") {
		t.Errorf("expected TM006 in stderr, got: %s", stderr)
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
	_, stderr, exitCode := runBinary(t, "# Hello\n\nWorld   \n", "fix")
	if exitCode != 2 {
		t.Errorf("expected exit code 2 for fix with stdin, got %d", exitCode)
	}
	if !strings.Contains(stderr, "cannot fix stdin in place") {
		t.Errorf("expected error message about stdin fix, got: %s", stderr)
	}
}

// --- Init subcommand tests ---

func TestE2E_Init_CreatesConfig(t *testing.T) {
	dir := t.TempDir()

	_, stderr, exitCode := runBinaryInDir(t, dir, "", "init")
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d; stderr: %s", exitCode, stderr)
	}
	if !strings.Contains(stderr, "created .tidymark.yml") {
		t.Errorf("expected confirmation message, got: %s", stderr)
	}

	// Check that the file was created.
	configPath := filepath.Join(dir, ".tidymark.yml")
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
	writeFixture(t, dir, ".tidymark.yml", "rules: {}\n")

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
	_, stderr, exitCode := runBinary(t, input, "check", "--no-color")
	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d; stderr: %s", exitCode, stderr)
	}
	if !strings.Contains(stderr, "TM006") {
		t.Errorf("expected TM006 in stderr, got: %s", stderr)
	}
	// Verify the line number is 6 (original file), not 3 (stripped content).
	if !strings.Contains(stderr, "<stdin>:6:") {
		t.Errorf("expected line 6 in diagnostic, got: %s", stderr)
	}
}

func TestE2E_Check_Stdin_ConfigurableSettingsApplied(t *testing.T) {
	// Pipe a file with 101-char lines through stdin. With a config that
	// sets line-length max to 120, no TM001 diagnostic should fire.
	dir := t.TempDir()
	line101 := strings.Repeat("a", 101)
	input := "# Title\n\n" + line101 + "\n"

	// Create a config file that increases max line length.
	configContent := "rules:\n  line-length:\n    max: 120\n"
	configPath := writeFixture(t, dir, ".tidymark.yml", configContent)

	_, stderr, exitCode := runBinary(t, input, "check", "--no-color", "--config", configPath)
	if strings.Contains(stderr, "TM001") {
		t.Errorf("expected TM001 to be suppressed by max=120 setting, but found in stderr: %s", stderr)
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0 with max=120 for 101-char line, got %d; stderr: %s", exitCode, stderr)
	}
}

// --- Help rule subcommand tests ---

func TestE2E_HelpRule_ByID(t *testing.T) {
	stdout, _, exitCode := runBinary(t, "", "help", "rule", "TM001")
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(stdout, "TM001") {
		t.Errorf("expected stdout to contain TM001, got: %s", stdout)
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
	if !strings.Contains(stdout, "TM001") {
		t.Errorf("expected stdout to contain TM001, got: %s", stdout)
	}
}

func TestE2E_HelpRule_UnknownRule_ExitsTwo(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "", "help", "rule", "TMXXX")
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
	if !strings.Contains(stdout, "TM001") {
		t.Errorf("expected stdout to contain TM001, got: %s", stdout)
	}
	if !strings.Contains(stdout, "line-length") {
		t.Errorf("expected stdout to contain 'line-length', got: %s", stdout)
	}
	// Should also include other rules
	if !strings.Contains(stdout, "TM002") {
		t.Errorf("expected stdout to contain TM002, got: %s", stdout)
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

func TestE2E_Check_Stdin_ConfigurableSettingsViolation(t *testing.T) {
	// Pipe a file with 130-char lines through stdin. Even with max=120,
	// the 130-char line should still fire TM001.
	dir := t.TempDir()
	line130 := strings.Repeat("a", 130)
	input := "# Title\n\n" + line130 + "\n"

	configContent := "rules:\n  line-length:\n    max: 120\n"
	configPath := writeFixture(t, dir, ".tidymark.yml", configContent)

	_, stderr, exitCode := runBinary(t, input, "check", "--no-color", "--config", configPath)
	if exitCode != 1 {
		t.Errorf("expected exit code 1 for 130-char line with max=120, got %d; stderr: %s", exitCode, stderr)
	}
	if !strings.Contains(stderr, "TM001") {
		t.Errorf("expected TM001 in stderr, got: %s", stderr)
	}
}
