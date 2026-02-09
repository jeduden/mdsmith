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

	os.RemoveAll(tmp)
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

// writeFixture creates a file with the given content in the given directory.
func writeFixture(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writing fixture %s: %v", path, err)
	}
	return path
}

func TestE2E_NoArgs_ExitsZero(t *testing.T) {
	_, _, exitCode := runBinary(t, "")
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
}

func TestE2E_CleanFile_ExitsZero(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "clean.md", "# Title\n\nSome content here.\n")

	_, _, exitCode := runBinary(t, "", "--no-color", path)
	if exitCode != 0 {
		t.Errorf("expected exit code 0 for clean file, got %d", exitCode)
	}
}

func TestE2E_Violations_ExitsOne(t *testing.T) {
	dir := t.TempDir()
	// Trailing spaces on lines should trigger TM006.
	path := writeFixture(t, dir, "dirty.md", "# Title\n\nHello   \n")

	_, stderr, exitCode := runBinary(t, "", "--no-color", path)
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

func TestE2E_JSONFormat(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "dirty.md", "# Title\n\nHello   \n")

	_, stderr, exitCode := runBinary(t, "", "--no-color", "--format", "json", path)
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

func TestE2E_FixMode(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "fixme.md", "# Title\n\nHello   \n")

	// Run with --fix.
	_, _, exitCode := runBinary(t, "", "--no-color", "--fix", path)
	if exitCode != 0 {
		t.Errorf("expected exit code 0 after fix, got %d", exitCode)
	}

	// Read the file back and check that trailing spaces are removed.
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading fixed file: %v", err)
	}
	if strings.Contains(string(content), "Hello   ") {
		t.Error("file still contains trailing spaces after --fix")
	}
	if !strings.Contains(string(content), "Hello") {
		t.Error("file does not contain expected content after --fix")
	}

	// Re-run without --fix; should exit 0 now.
	_, _, exitCode = runBinary(t, "", "--no-color", path)
	if exitCode != 0 {
		t.Errorf("expected exit code 0 on re-lint after fix, got %d", exitCode)
	}
}

func TestE2E_CustomConfig(t *testing.T) {
	dir := t.TempDir()

	// Create a file that violates no-trailing-spaces (TM006).
	path := writeFixture(t, dir, "test.md", "# Title\n\nHello   \n")

	// Create a config that disables no-trailing-spaces.
	configContent := "rules:\n  no-trailing-spaces: false\n"
	configPath := writeFixture(t, dir, ".tidymark.yml", configContent)

	// Run with the custom config; the violation should be suppressed.
	_, stderr, exitCode := runBinary(t, "", "--no-color", "--config", configPath, path)
	if strings.Contains(stderr, "TM006") {
		t.Errorf("expected TM006 to be suppressed by config, but found in stderr: %s", stderr)
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0 with rule disabled, got %d", exitCode)
	}
}

func TestE2E_Version(t *testing.T) {
	stdout, _, exitCode := runBinary(t, "", "--version")
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
	if !strings.HasPrefix(stdout, "tidymark ") {
		t.Errorf("expected version output to start with 'tidymark ', got: %s", stdout)
	}
}

func TestE2E_VersionShorthand(t *testing.T) {
	stdout, _, exitCode := runBinary(t, "", "-v")
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
	if !strings.HasPrefix(stdout, "tidymark ") {
		t.Errorf("expected version output to start with 'tidymark ', got: %s", stdout)
	}
}

func TestE2E_Stdin_Clean(t *testing.T) {
	_, _, exitCode := runBinary(t, "# Hello\n\nWorld.\n")
	if exitCode != 0 {
		t.Errorf("expected exit code 0 for clean stdin, got %d", exitCode)
	}
}

func TestE2E_Stdin_Violations(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "# Hello\n\nWorld   \n", "--no-color")
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

func TestE2E_Stdin_JSONFormat(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "# Hello\n\nWorld   \n", "--no-color", "--format", "json")
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

func TestE2E_Stdin_Fix_ExitsTwo(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "# Hello\n\nWorld   \n", "--fix")
	if exitCode != 2 {
		t.Errorf("expected exit code 2 for --fix with stdin, got %d", exitCode)
	}
	if !strings.Contains(stderr, "cannot fix stdin in place") {
		t.Errorf("expected error message about stdin fix, got: %s", stderr)
	}
}
