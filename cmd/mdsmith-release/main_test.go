package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunStampThenCheck exercises the CLI dispatcher end-to-end:
// stamp a temp tree with a real version, then run check against
// the same tree (which should now fail because the dev sentinel
// is gone). Confirms the subcommand wiring and the cwd-as-root
// contract.
func TestRunStampThenCheck(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root)

	wd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(wd) })
	require.NoError(t, os.Chdir(root))

	assert.Equal(t, 0, run([]string{"stamp", "1.2.3"}))
	// After stamping, check should fail because the manifests no
	// longer carry the dev sentinel.
	assert.Equal(t, 1, run([]string{"check"}))
}

func TestRunRejectsUnknownCommand(t *testing.T) {
	assert.Equal(t, 2, run([]string{"frobnicate"}))
}

func TestRunHelpExitsZero(t *testing.T) {
	for _, arg := range []string{"-h", "--help", "help"} {
		assert.Equal(t, 0, run([]string{arg}), "%s", arg)
	}
}

func TestRunNoArgsPrintsUsage(t *testing.T) {
	assert.Equal(t, 2, run(nil))
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
		assert.Equal(t, 2, run(c.args), c.name)
	}
}

func TestRunStampReturnsErrorOnInvalidVersion(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root)
	wd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(wd) })
	require.NoError(t, os.Chdir(root))

	assert.Equal(t, 1, run([]string{"stamp", "v1.2.3"}))
}

// TestReportErrorMapsExitCodes pins the wrapper that translates a
// (possibly nil) error into the integer exit code main returns.
func TestReportErrorMapsExitCodes(t *testing.T) {
	assert.Equal(t, 0, reportError(nil))
	assert.Equal(t, 1, reportError(errors.New("sentinel error")))
}

// TestSubcommandHelpExitsZero exercises the pflag --help branch
// of reportFlagParseErr per subcommand. pflag prints the Usage
// itself, so the dispatcher just needs to surface exit code 0.
func TestSubcommandHelpExitsZero(t *testing.T) {
	for _, sub := range []string{"stamp", "check", "build-npm", "build-wheels"} {
		assert.Equal(t, 0, run([]string{sub, "--help"}), "%s --help", sub)
	}
}

// TestSubcommandRejectsUnknownFlag exercises the non-help, non-nil
// branch of reportFlagParseErr.
func TestSubcommandRejectsUnknownFlag(t *testing.T) {
	for _, sub := range []string{"stamp", "check", "build-npm", "build-wheels"} {
		assert.Equal(t, 2, run([]string{sub, "--bogus"}), "%s --bogus", sub)
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
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(body), 0o644))
	}
}
