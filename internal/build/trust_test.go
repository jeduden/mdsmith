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

func TestConfigPathForRoot(t *testing.T) {
	got := ConfigPathForRoot("/some/root")
	assert.Equal(t, filepath.Join("/some/root", ".mdsmith.yml"), got)
}

func TestTrustMarkerPath_EmptyConfigPath(t *testing.T) {
	// An empty configPath falls back to the default config name so the
	// marker is still named after the file the gate actually pins.
	got := TrustMarkerPath("")
	assert.Equal(t, ".mdsmith.yml"+trustMarkerSuffix, got)
}

func TestCheckTrust_EmptyConfigPath(t *testing.T) {
	// With an empty configPath the gate falls back to ".mdsmith.yml" in the
	// process working directory; since that file doesn't exist in our temp
	// dir the result is not-trusted.
	dir := t.TempDir()
	orig, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(orig) })

	res := CheckTrust("", func(string) bool { return false })
	assert.False(t, res.Trusted)
}

func TestCheckTrust_MarkerReadError(t *testing.T) {
	dir := t.TempDir()
	body := []byte("build: {}\n")
	cfg := cfgIn(t, dir, body)
	// A directory at the marker path makes os.ReadFile fail with EISDIR — a
	// non-ErrNotExist error that hits the "cannot read marker" branch
	// regardless of whether the process runs as root.
	require.NoError(t, os.Mkdir(cfg+".trust", 0o755))

	res := CheckTrust(cfg, func(string) bool { return false })
	assert.False(t, res.Trusted)
	assert.NotEmpty(t, res.Reason)
}

func TestWriteTrustMarker_EmptyConfigPath(t *testing.T) {
	dir := t.TempDir()
	orig, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(orig) })

	body := []byte("build: {}\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".mdsmith.yml"), body, 0o644))

	require.NoError(t, WriteTrustMarker(""))
	got, err := os.ReadFile(filepath.Join(dir, ".mdsmith.yml.trust"))
	require.NoError(t, err)
	assert.Equal(t, body, got)
}

func TestWriteTrustMarker_MissingConfig(t *testing.T) {
	err := WriteTrustMarker(filepath.Join(t.TempDir(), "nonexistent.yml"))
	require.Error(t, err)
}

func TestTrustDiff_EmptyConfigPath(t *testing.T) {
	// Empty configPath falls back to .mdsmith.yml in cwd; if absent,
	// TrustDiff returns an error rather than a diff.
	dir := t.TempDir()
	orig, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(orig) })

	_, _, err = TrustDiff("")
	require.Error(t, err)
}

func TestTrustDiff_MissingConfig(t *testing.T) {
	_, _, err := TrustDiff(filepath.Join(t.TempDir(), "nonexistent.yml"))
	require.Error(t, err)
}

func TestTrustDiff_MarkerReadError(t *testing.T) {
	dir := t.TempDir()
	body := []byte("a: 1\n")
	cfg := cfgIn(t, dir, body)
	// A directory at the marker path makes os.ReadFile fail with a
	// non-ErrNotExist error, hitting the default error branch — and it works
	// as root, unlike a chmod 000 file.
	require.NoError(t, os.Mkdir(cfg+".trust", 0o755))

	_, _, err := TrustDiff(cfg)
	require.Error(t, err)
}

func TestWriteTrustMarker_AtomicWriteError(t *testing.T) {
	// A directory at the marker path makes the atomic rename fail (it cannot
	// replace a non-empty directory), surfacing the "installing trust marker"
	// error — and it works as root.
	dir := t.TempDir()
	body := []byte("build: {}\n")
	cfg := cfgIn(t, dir, body)
	markerDir := cfg + ".trust"
	require.NoError(t, os.Mkdir(markerDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(markerDir, "blocker"), []byte("x"), 0o644))

	err := WriteTrustMarker(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "installing trust marker")
}
