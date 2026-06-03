package release

import (
	"archive/zip"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// obsidianDistFiles is the exact set of files PackageObsidian stores
// in the zip, flat (base name only). Mirrors the list Obsidian loads
// and the workflows' upload-artifact contract.
var obsidianDistFiles = []string{
	"main.js",
	"manifest.json",
	"styles.css",
	"mdsmith.wasm",
	"wasm_exec.js",
}

// stageObsidianDist writes a fake plugin dist dir with the five files
// PackageObsidian zips, including a manifest.json carrying version.
func stageObsidianDist(t *testing.T, dir, version string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	for _, name := range obsidianDistFiles {
		body := []byte("// " + name + "\n")
		if name == "manifest.json" {
			body = []byte(`{
  "id": "mdsmith",
  "name": "mdsmith",
  "version": "` + version + `"
}
`)
		}
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), body, 0o644))
	}
}

// zipEntryNames returns the sorted entry names of the zip at path.
func zipEntryNames(t *testing.T, path string) []string {
	t.Helper()
	zr, err := zip.OpenReader(path)
	require.NoError(t, err)
	defer func() { _ = zr.Close() }()
	names := make([]string, 0, len(zr.File))
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	sort.Strings(names)
	return names
}

func TestPackageObsidian(t *testing.T) {
	dir := t.TempDir()
	dist := filepath.Join(dir, "dist")
	stageObsidianDist(t, dist, "1.2.3")
	out := filepath.Join(dir, "out")

	zipPath, err := PackageObsidian(dist, out)
	require.NoError(t, err)

	// The zip lands at <out>/mdsmith-obsidian-<version>.zip so the
	// workflows' upload-artifact `path:` glob matches.
	assert.Equal(t, filepath.Join(out, "mdsmith-obsidian-1.2.3.zip"), zipPath)
	_, statErr := os.Stat(zipPath)
	require.NoError(t, statErr)

	// Exactly the five files, stored flat (no dist/ prefix).
	want := append([]string(nil), obsidianDistFiles...)
	sort.Strings(want)
	assert.Equal(t, want, zipEntryNames(t, zipPath))
}

func TestPackageObsidianMissingManifest(t *testing.T) {
	dir := t.TempDir()
	dist := filepath.Join(dir, "dist")
	require.NoError(t, os.MkdirAll(dist, 0o755))
	// All files but manifest.json.
	for _, name := range obsidianDistFiles {
		if name == "manifest.json" {
			continue
		}
		require.NoError(t, os.WriteFile(filepath.Join(dist, name), []byte("x"), 0o644))
	}

	_, err := PackageObsidian(dist, filepath.Join(dir, "out"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "manifest.json")
}

func TestPackageObsidianMissingVersion(t *testing.T) {
	dir := t.TempDir()
	dist := filepath.Join(dir, "dist")
	stageObsidianDist(t, dist, "1.0.0")
	// Overwrite manifest.json with no version field.
	require.NoError(t, os.WriteFile(
		filepath.Join(dist, "manifest.json"),
		[]byte(`{"id":"mdsmith","name":"mdsmith"}`+"\n"), 0o644))

	_, err := PackageObsidian(dist, filepath.Join(dir, "out"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "version")
}

func TestPackageObsidianMissingRequiredFile(t *testing.T) {
	dir := t.TempDir()
	dist := filepath.Join(dir, "dist")
	stageObsidianDist(t, dist, "2.0.0")
	// Remove a required file the manifest does not name.
	require.NoError(t, os.Remove(filepath.Join(dist, "mdsmith.wasm")))

	_, err := PackageObsidian(dist, filepath.Join(dir, "out"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mdsmith.wasm")
}

func TestPackageObsidianRejectsUnsafeVersion(t *testing.T) {
	dir := t.TempDir()
	dist := filepath.Join(dir, "dist")
	stageObsidianDist(t, dist, "1.0.0")
	// A version with path separators must be rejected: it would otherwise
	// build a zip filename that escapes outDir.
	require.NoError(t, os.WriteFile(
		filepath.Join(dist, "manifest.json"),
		[]byte(`{
  "id": "mdsmith",
  "version": "1.0.0/../../evil"
}
`), 0o644))

	_, err := PackageObsidian(dist, filepath.Join(dir, "out"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid version")
}

func TestPackageObsidianDevSentinelVersion(t *testing.T) {
	dir := t.TempDir()
	dist := filepath.Join(dir, "dist")
	stageObsidianDist(t, dist, "0.0.0-dev")
	out := filepath.Join(dir, "out")

	zipPath, err := PackageObsidian(dist, out)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(out, "mdsmith-obsidian-0.0.0-dev.zip"), zipPath)
}
