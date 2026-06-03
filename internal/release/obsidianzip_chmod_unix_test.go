//go:build !windows

package release

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// These drive PackageObsidian's MkdirAll and WriteFile error branches by
// chmod-ing paths read-only — the same technique as
// parity_chmod_unix_test.go. Unix-tagged because os.Geteuid and the
// chmod permission semantics are not portable to Windows builds.

// TestPackageObsidian_MkdirError makes the parent of outDir read-only so
// os.MkdirAll(outDir) fails after the dist files have been read.
func TestPackageObsidian_MkdirError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("chmod-based readonly test is unreliable as root")
	}
	dir := t.TempDir()
	dist := filepath.Join(dir, "dist")
	stageObsidianDist(t, dist, "1.0.0")

	base := filepath.Join(dir, "out")
	require.NoError(t, os.MkdirAll(base, 0o755))
	require.NoError(t, os.Chmod(base, 0o555))
	t.Cleanup(func() { _ = os.Chmod(base, 0o755) })

	_, err := PackageObsidian(dist, filepath.Join(base, "sub"))
	require.Error(t, err)
}

// TestPackageObsidian_WriteError pre-creates outDir read-only so
// MkdirAll is a no-op (the dir exists) and the subsequent WriteFile of
// the zip cannot create the file.
func TestPackageObsidian_WriteError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("chmod-based readonly test is unreliable as root")
	}
	dir := t.TempDir()
	dist := filepath.Join(dir, "dist")
	stageObsidianDist(t, dist, "1.0.0")

	out := filepath.Join(dir, "out")
	require.NoError(t, os.MkdirAll(out, 0o755))
	require.NoError(t, os.Chmod(out, 0o555))
	t.Cleanup(func() { _ = os.Chmod(out, 0o755) })

	_, err := PackageObsidian(dist, out)
	require.Error(t, err)
}
