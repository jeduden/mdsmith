package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestRunStampThenCheck exercises the CLI dispatcher end-to-end:
// stamp a temp tree with a real version, then run check against
// the same tree (which should now succeed nowhere because the
// dev sentinel is gone). Confirms the subcommand wiring and the
// cwd-as-root contract.
func TestRunStampThenCheck(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root)

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	if code := run([]string{"stamp", "1.2.3"}); code != 0 {
		t.Fatalf("stamp exited %d", code)
	}
	// After stamping, check should fail because the manifests no
	// longer carry the dev sentinel.
	if code := run([]string{"check"}); code != 1 {
		t.Fatalf("check after stamp: exit code %d, want 1", code)
	}
}

func TestRunRejectsUnknownCommand(t *testing.T) {
	if code := run([]string{"frobnicate"}); code != 2 {
		t.Errorf("unknown command: exit code %d, want 2", code)
	}
}

func TestRunHelpExitsZero(t *testing.T) {
	for _, arg := range []string{"-h", "--help", "help"} {
		if code := run([]string{arg}); code != 0 {
			t.Errorf("%s: exit code %d, want 0", arg, code)
		}
	}
}

func TestRunNoArgsPrintsUsage(t *testing.T) {
	if code := run(nil); code != 2 {
		t.Errorf("no args: exit code %d, want 2", code)
	}
}

func TestRunRejectsBadArity(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"stamp without version", []string{"stamp"}},
		{"stamp with extra args", []string{"stamp", "1.2.3", "extra"}},
		{"check with extra arg", []string{"check", "extra"}},
		{"build-npm without args", []string{"build-npm"}},
		{"build-npm with one arg", []string{"build-npm", "art"}},
		{"build-wheels without args", []string{"build-wheels"}},
		{"build-wheels with one arg", []string{"build-wheels", "art"}},
	}
	for _, c := range cases {
		if code := run(c.args); code != 2 {
			t.Errorf("%s: exit code %d, want 2", c.name, code)
		}
	}
}

func TestRunStampReturnsErrorOnInvalidVersion(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root)
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	if code := run([]string{"stamp", "v1.2.3"}); code != 1 {
		t.Errorf("invalid version: exit code %d, want 1", code)
	}
}

// TestReportErrorMapsExitCodes pins the wrapper that translates a
// (possibly nil) error into the integer exit code main returns.
// This is the smallest unit but it gates whether the CLI surfaces
// the underlying error to CI.
func TestReportErrorMapsExitCodes(t *testing.T) {
	if got := reportError(nil); got != 0 {
		t.Errorf("nil error: exit code %d, want 0", got)
	}
	if got := reportError(errors.New("sentinel error")); got != 1 {
		t.Errorf("non-nil error: exit code %d, want 1", got)
	}
}

// TestSubcommandHelpExitsZero exercises the pflag --help branch
// of reportFlagParseErr per subcommand. pflag prints the Usage
// itself, so the dispatcher just needs to surface exit code 0.
func TestSubcommandHelpExitsZero(t *testing.T) {
	cases := []string{"stamp", "check", "build-npm", "build-wheels"}
	for _, sub := range cases {
		if code := run([]string{sub, "--help"}); code != 0 {
			t.Errorf("%s --help: exit code %d, want 0", sub, code)
		}
	}
}

// TestSubcommandRejectsUnknownFlag exercises the non-help, non-nil
// branch of reportFlagParseErr.
func TestSubcommandRejectsUnknownFlag(t *testing.T) {
	cases := []string{"stamp", "check", "build-npm", "build-wheels"}
	for _, sub := range cases {
		if code := run([]string{sub, "--bogus"}); code != 2 {
			t.Errorf("%s --bogus: exit code %d, want 2", sub, code)
		}
	}
}

// writeFixture mirrors internal/release/version_test.go's
// fixtureManifests but without taking a dependency back on the
// internal package's test helpers.
func writeFixture(t *testing.T, root string) {
	t.Helper()
	files := map[string]string{
		"editors/vscode/package.json": `{
  "name": "mdsmith",
  "version": "0.0.0-dev"
}
`,
		"npm/mdsmith/package.json": `{
  "name": "@mdsmith/cli",
  "version": "0.0.0-dev",
  "optionalDependencies": {
    "@mdsmith/linux-x64": "0.0.0-dev",
    "@mdsmith/linux-arm64": "0.0.0-dev",
    "@mdsmith/darwin-x64": "0.0.0-dev",
    "@mdsmith/darwin-arm64": "0.0.0-dev",
    "@mdsmith/win32-x64": "0.0.0-dev"
  }
}
`,
		"python/pyproject.toml": `[project]
name = "mdsmith"
version = "0.0.0-dev"
`,
	}
	for rel, body := range files {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}
}
