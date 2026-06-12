package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cfgTrust writes a .mdsmith.yml at cfg and an identical .trust marker so the
// trust gate is satisfied. Returns the config path.
func cfgTrust(t *testing.T, dir string, body []byte) string {
	t.Helper()
	cfg := filepath.Join(dir, ".mdsmith.yml")
	require.NoError(t, os.WriteFile(cfg, body, 0o644))
	require.NoError(t, os.WriteFile(cfg+".trust", body, 0o600))
	return cfg
}

func TestRunTrustIO_MissingConfig(t *testing.T) {
	dir := t.TempDir()
	var stdout, stderr strings.Builder
	code := runTrustIO(
		[]string{"-c", filepath.Join(dir, "nonexistent.yml")},
		strings.NewReader(""), &stdout, &stderr,
	)
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr.String(), "trust")
}

func TestRunTrustIO_AlreadyTrusted(t *testing.T) {
	body := []byte("build: {}\n")
	cfg := cfgTrust(t, t.TempDir(), body)
	var stdout, stderr strings.Builder
	code := runTrustIO(
		[]string{"-c", cfg},
		strings.NewReader(""), &stdout, &stderr,
	)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout.String(), "already trusted")
}

func TestRunTrustIO_ConfirmYes(t *testing.T) {
	dir := t.TempDir()
	body := []byte("build: {}\n")
	cfg := filepath.Join(dir, ".mdsmith.yml")
	require.NoError(t, os.WriteFile(cfg, body, 0o644))
	require.NoError(t, os.WriteFile(cfg+".trust", []byte("old: 1\n"), 0o600))

	var stdout, stderr strings.Builder
	code := runTrustIO(
		[]string{"-c", cfg},
		strings.NewReader("y\n"), &stdout, &stderr,
	)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout.String(), "Trusted")
}

func TestRunTrustIO_ConfirmNo(t *testing.T) {
	dir := t.TempDir()
	body := []byte("build: {}\n")
	cfg := filepath.Join(dir, ".mdsmith.yml")
	require.NoError(t, os.WriteFile(cfg, body, 0o644))
	require.NoError(t, os.WriteFile(cfg+".trust", []byte("old: 1\n"), 0o600))

	var stdout, stderr strings.Builder
	code := runTrustIO(
		[]string{"-c", cfg},
		strings.NewReader("n\n"), &stdout, &stderr,
	)
	assert.Equal(t, 1, code)
	assert.Contains(t, stdout.String(), "Aborted")
}

func TestRunTrustIO_YesFlag(t *testing.T) {
	dir := t.TempDir()
	body := []byte("build: {}\n")
	cfg := filepath.Join(dir, ".mdsmith.yml")
	require.NoError(t, os.WriteFile(cfg, body, 0o644))
	require.NoError(t, os.WriteFile(cfg+".trust", []byte("old: 1\n"), 0o600))

	var stdout, stderr strings.Builder
	code := runTrustIO(
		[]string{"-c", cfg, "--yes"},
		strings.NewReader(""), &stdout, &stderr,
	)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout.String(), "Trusted")
}

func TestRunTrustIO_FlagParseError(t *testing.T) {
	var stdout, stderr strings.Builder
	code := runTrustIO(
		[]string{"--no-such-flag"},
		strings.NewReader(""), &stdout, &stderr,
	)
	assert.Equal(t, 2, code)
}

func TestRunTrustIO_HelpFlag(t *testing.T) {
	var stdout, stderr strings.Builder
	code := runTrustIO(
		[]string{"--help"},
		strings.NewReader(""), &stdout, &stderr,
	)
	assert.Equal(t, 0, code)
	assert.Contains(t, stderr.String(), "trust")
}

func TestRunTrustIO_WriteTrustMarkerError(t *testing.T) {
	dir := t.TempDir()
	body := []byte("build: {}\n")
	cfg := filepath.Join(dir, ".mdsmith.yml")
	require.NoError(t, os.WriteFile(cfg, body, 0o644))
	// Marker exists with different content so TrustDiff returns changed=true.
	require.NoError(t, os.WriteFile(cfg+".trust", []byte("old: 1\n"), 0o600))

	// Place a non-empty directory where atomicWriteFile would put its temp file
	// (same dir as the marker). We can't directly block the rename, but we can
	// make the parent dir unwritable only on non-root systems.
	if os.Getuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	require.NoError(t, os.Chmod(dir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	var stdout, stderr strings.Builder
	code := runTrustIO(
		[]string{"-c", cfg, "--yes"},
		strings.NewReader(""), &stdout, &stderr,
	)
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr.String(), "trust")
}

func TestConfirmTrust_YesVariants(t *testing.T) {
	for _, ans := range []string{"y\n", "yes\n", "Y\n", "YES\n"} {
		var out strings.Builder
		got := confirmTrust(strings.NewReader(ans), &out)
		assert.True(t, got, "answer %q should confirm", ans)
	}
}

func TestConfirmTrust_NoVariants(t *testing.T) {
	for _, ans := range []string{"n\n", "\n", ""} {
		var out strings.Builder
		got := confirmTrust(strings.NewReader(ans), &out)
		assert.False(t, got, "answer %q should not confirm", ans)
	}
}
