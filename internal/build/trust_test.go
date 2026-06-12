package build

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckTrust_MissingMarkerBlocks(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, ".mdsmith.yml"), []byte("build: {}\n"), 0o644))

	res := CheckTrust(root, func(string) bool { return false })
	assert.False(t, res.Trusted)
	assert.NotEmpty(t, res.Reason)
}

func TestCheckTrust_MatchingMarkerTrusts(t *testing.T) {
	root := t.TempDir()
	body := []byte("build:\n  recipes: {}\n")
	require.NoError(t, os.WriteFile(filepath.Join(root, ".mdsmith.yml"), body, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".mdsmith.yml.trust"), body, 0o644))

	res := CheckTrust(root, func(string) bool { return false })
	assert.True(t, res.Trusted)
}

func TestCheckTrust_StaleMarkerBlocks(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, ".mdsmith.yml"), []byte("build:\n  a: 1\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".mdsmith.yml.trust"), []byte("build:\n  a: 2\n"), 0o644))

	res := CheckTrust(root, func(string) bool { return false })
	assert.False(t, res.Trusted)
	assert.Contains(t, res.Reason, "changed")
}

func TestCheckTrust_EnvOverrideTrusts(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, ".mdsmith.yml"), []byte("build: {}\n"), 0o644))
	// No marker file; env override grants trust.
	res := CheckTrust(root, func(name string) bool { return name == envTrustBuild })
	assert.True(t, res.Trusted)
	assert.True(t, res.ViaEnv)
}

func TestCheckTrust_MissingConfigBlocks(t *testing.T) {
	root := t.TempDir()
	// No .mdsmith.yml at all.
	res := CheckTrust(root, func(string) bool { return false })
	assert.False(t, res.Trusted)
}

func TestTrustMarkerPath(t *testing.T) {
	assert.Equal(t, filepath.Join("/r", ".mdsmith.yml.trust"), TrustMarkerPath("/r"))
}

func TestWriteTrustMarker(t *testing.T) {
	root := t.TempDir()
	body := []byte("build:\n  recipes: {}\n")
	require.NoError(t, os.WriteFile(filepath.Join(root, ".mdsmith.yml"), body, 0o644))

	require.NoError(t, WriteTrustMarker(root))

	got, err := os.ReadFile(filepath.Join(root, ".mdsmith.yml.trust"))
	require.NoError(t, err)
	assert.Equal(t, body, got)
}

func TestTrustDiff_ReportsChange(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, ".mdsmith.yml"), []byte("a: 1\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".mdsmith.yml.trust"), []byte("a: 2\n"), 0o644))

	diff, changed, err := TrustDiff(root)
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Contains(t, diff, "a: 1")
	assert.Contains(t, diff, "a: 2")
}

func TestTrustDiff_NoMarkerIsChange(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, ".mdsmith.yml"), []byte("a: 1\n"), 0o644))

	diff, changed, err := TrustDiff(root)
	require.NoError(t, err)
	assert.True(t, changed)
	assert.NotEmpty(t, diff)
}

func TestTrustDiff_IdenticalNoChange(t *testing.T) {
	root := t.TempDir()
	body := []byte("a: 1\n")
	require.NoError(t, os.WriteFile(filepath.Join(root, ".mdsmith.yml"), body, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".mdsmith.yml.trust"), body, 0o644))

	_, changed, err := TrustDiff(root)
	require.NoError(t, err)
	assert.False(t, changed)
}
