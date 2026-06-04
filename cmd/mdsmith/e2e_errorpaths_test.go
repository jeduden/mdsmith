package main_test

import (
	"bytes"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestKindsResolveBadMaxInputSizeFrontMatterDisabled exercises the
// front-matter-disabled arm of resolveFileFromCLI: with `front-matter:
// false` the kinds path skips front-matter parsing but still resolves the
// byte cap before its readability check, so a malformed `max-input-size`
// is surfaced as an error there (the path the default, front-matter-on
// arm reaches earlier). The command must exit 2 and name the bad setting.
func TestKindsResolveBadMaxInputSizeFrontMatterDisabled(t *testing.T) {
	cfg := "front-matter: false\nmax-input-size: \"not-a-size\"\n"
	dir := kindsTestDir(t, cfg, map[string]string{
		"doc.md": "# Doc\n\nBody paragraph.\n",
	})

	_, stderr, code := runBinaryInDir(t, dir, "", "kinds", "resolve", "doc.md")
	require.Equal(t, 2, code,
		"a malformed max-input-size must fail kinds resolve with exit 2")
	assert.Contains(t, stderr, "invalid max-input-size",
		"stderr should name the malformed max-input-size setting")
}

// TestCheckStdinReadError covers readStdinLimited's read-error branch: the
// limited read (default 2 MB cap) calls io.ReadAll on stdin, and a stdin
// whose Read fails must propagate as exit 2 rather than being treated as
// empty input. A directory file descriptor reads with EISDIR on Linux,
// which is a deterministic, dependency-free way to make stdin error.
func TestCheckStdinReadError(t *testing.T) {
	// A directory opened as a plain *os.File: read(2) on it returns an
	// error, so io.ReadAll(LimitReader(stdin, …)) fails.
	dirAsStdin, err := os.Open(t.TempDir())
	require.NoError(t, err)
	defer func() { _ = dirAsStdin.Close() }()

	// Run `check -` (read from stdin) in the isolated CWD with the
	// directory fd as stdin. runBinary only accepts a string stdin, so
	// invoke the binary directly to attach the file descriptor.
	cmd := exec.Command(binaryPath, "check", "-")
	cmd.Dir = isolatedCWD
	cmd.Env = envWithCoverDir(coverDir)
	cmd.Stdin = dirAsStdin
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	runErr := cmd.Run()
	exitErr, ok := runErr.(*exec.ExitError)
	require.True(t, ok, "expected a non-zero exit, got %v (stderr: %s)", runErr, errBuf.String())
	require.Equal(t, 2, exitErr.ExitCode(),
		"a stdin read failure must exit 2 (stderr: %s)", errBuf.String())
	assert.Contains(t, errBuf.String(), "mdsmith:",
		"stderr should carry the mdsmith error prefix for the stdin read failure")
}
