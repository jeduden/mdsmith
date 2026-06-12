package build

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cfgIn writes a .mdsmith.yml in dir and returns its path.
func cfgIn(t *testing.T, dir string, body []byte) string {
	t.Helper()
	p := filepath.Join(dir, ".mdsmith.yml")
	require.NoError(t, os.WriteFile(p, body, 0o644))
	return p
}

func TestCheckTrust_MissingMarkerBlocks(t *testing.T) {
	cfg := cfgIn(t, t.TempDir(), []byte("build: {}\n"))
	res := CheckTrust(cfg, func(string) bool { return false })
	assert.False(t, res.Trusted)
	assert.NotEmpty(t, res.Reason)
}

func TestCheckTrust_MatchingMarkerTrusts(t *testing.T) {
	body := []byte("build:\n  recipes: {}\n")
	cfg := cfgIn(t, t.TempDir(), body)
	require.NoError(t, os.WriteFile(cfg+".trust", body, 0o644))

	res := CheckTrust(cfg, func(string) bool { return false })
	assert.True(t, res.Trusted)
}

func TestCheckTrust_StaleMarkerBlocks(t *testing.T) {
	cfg := cfgIn(t, t.TempDir(), []byte("build:\n  a: 1\n"))
	require.NoError(t, os.WriteFile(cfg+".trust", []byte("build:\n  a: 2\n"), 0o644))

	res := CheckTrust(cfg, func(string) bool { return false })
	assert.False(t, res.Trusted)
	assert.Contains(t, res.Reason, "changed")
}

func TestCheckTrust_EnvOverrideTrusts(t *testing.T) {
	cfg := cfgIn(t, t.TempDir(), []byte("build: {}\n"))
	// No marker file; env override grants trust.
	res := CheckTrust(cfg, func(name string) bool { return name == envTrustBuild })
	assert.True(t, res.Trusted)
	assert.True(t, res.ViaEnv)
}

func TestCheckTrust_MissingConfigBlocks(t *testing.T) {
	// No config file at the given path.
	res := CheckTrust(filepath.Join(t.TempDir(), ".mdsmith.yml"), func(string) bool { return false })
	assert.False(t, res.Trusted)
}

func TestCheckTrust_CustomConfigPathPinsItsOwnMarker(t *testing.T) {
	dir := t.TempDir()
	body := []byte("build:\n  recipes: {}\n")
	custom := filepath.Join(dir, "custom.yml")
	require.NoError(t, os.WriteFile(custom, body, 0o644))
	// A default .mdsmith.yml with different content must NOT satisfy the
	// gate for the custom config: the marker is named after the loaded file.
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".mdsmith.yml"), []byte("other\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".mdsmith.yml.trust"), []byte("other\n"), 0o644))

	res := CheckTrust(custom, func(string) bool { return false })
	assert.False(t, res.Trusted, "custom.yml has no custom.yml.trust marker")

	require.NoError(t, os.WriteFile(custom+".trust", body, 0o644))
	res = CheckTrust(custom, func(string) bool { return false })
	assert.True(t, res.Trusted)
}

func TestTrustMarkerPath(t *testing.T) {
	assert.Equal(t, filepath.Join("/r", ".mdsmith.yml.trust"),
		TrustMarkerPath(filepath.Join("/r", ".mdsmith.yml")))
	assert.Equal(t, "/r/custom.yml.trust", TrustMarkerPath("/r/custom.yml"))
}

func TestWriteTrustMarker(t *testing.T) {
	body := []byte("build:\n  recipes: {}\n")
	cfg := cfgIn(t, t.TempDir(), body)

	require.NoError(t, WriteTrustMarker(cfg))

	got, err := os.ReadFile(cfg + ".trust")
	require.NoError(t, err)
	assert.Equal(t, body, got)
}

func TestWriteTrustMarker_Mode0600(t *testing.T) {
	cfg := cfgIn(t, t.TempDir(), []byte("a: 1\n"))
	require.NoError(t, WriteTrustMarker(cfg))
	info, err := os.Lstat(cfg + ".trust")
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestTrustDiff_ReportsChange(t *testing.T) {
	cfg := cfgIn(t, t.TempDir(), []byte("a: 1\n"))
	require.NoError(t, os.WriteFile(cfg+".trust", []byte("a: 2\n"), 0o644))

	diff, changed, err := TrustDiff(cfg)
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Contains(t, diff, "a: 1")
	assert.Contains(t, diff, "a: 2")
}

func TestTrustDiff_NoMarkerIsChange(t *testing.T) {
	cfg := cfgIn(t, t.TempDir(), []byte("a: 1\n"))

	diff, changed, err := TrustDiff(cfg)
	require.NoError(t, err)
	assert.True(t, changed)
	assert.NotEmpty(t, diff)
}

func TestTrustDiff_IdenticalNoChange(t *testing.T) {
	body := []byte("a: 1\n")
	cfg := cfgIn(t, t.TempDir(), body)
	require.NoError(t, os.WriteFile(cfg+".trust", body, 0o644))

	_, changed, err := TrustDiff(cfg)
	require.NoError(t, err)
	assert.False(t, changed)
}
