package release

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stageFlatpakArtifacts writes the two Linux release binaries
// build-flatpak reads into dir.
func stageFlatpakArtifacts(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	for _, asset := range []string{"mdsmith-linux-arm64", "mdsmith-linux-amd64"} {
		require.NoError(t, os.WriteFile(
			filepath.Join(dir, asset), []byte("#!/bin/sh\necho fake\n"), 0o755))
	}
}

func TestBuildFlatpak(t *testing.T) {
	dir := t.TempDir()
	artifacts := filepath.Join(dir, "artifacts")
	stageFlatpakArtifacts(t, artifacts)
	out := filepath.Join(dir, "flatpak")

	require.NoError(t, BuildFlatpak(artifacts, out))

	data, err := os.ReadFile(filepath.Join(out, "io.github.jeduden.mdsmith.yml"))
	require.NoError(t, err)
	manifest := string(data)

	// App id, runtime, command, and the host grant a linter needs to
	// read files outside the sandbox.
	assert.Contains(t, manifest, "app-id: io.github.jeduden.mdsmith")
	assert.Contains(t, manifest, "runtime-version: '24.08'")
	assert.Contains(t, manifest, "command: mdsmith")
	assert.Contains(t, manifest, "--filesystem=host")

	// One local-path source per arch, mapping the flatpak arch to the
	// matching linux release binary and installing it as mdsmith.
	assert.Contains(t, manifest, "- aarch64")
	assert.Contains(t, manifest, "path: mdsmith-linux-arm64")
	assert.Contains(t, manifest, "- x86_64")
	assert.Contains(t, manifest, "path: mdsmith-linux-amd64")
	assert.Contains(t, manifest, "dest-filename: mdsmith")

	// flatpak-builder resolves `path:` next to the manifest, so each
	// referenced binary is staged into out-dir.
	for _, asset := range []string{"mdsmith-linux-arm64", "mdsmith-linux-amd64"} {
		_, err := os.Stat(filepath.Join(out, asset))
		assert.NoError(t, err, asset)
	}

	// The bundle builds from local binaries and ships as a single
	// file, so the manifest carries no release-download URL and no
	// AppStream metainfo.
	assert.NotContains(t, manifest, "https://github.com/jeduden/mdsmith/releases")
	assert.NotContains(t, manifest, "metainfo")
	_, err = os.Stat(filepath.Join(out, "io.github.jeduden.mdsmith.metainfo.xml"))
	assert.True(t, os.IsNotExist(err), "no metainfo file")
}

func TestBuildFlatpakMissingBinary(t *testing.T) {
	dir := t.TempDir()
	artifacts := filepath.Join(dir, "artifacts")
	require.NoError(t, os.MkdirAll(artifacts, 0o755))
	// Only the arm64 binary staged — amd64 is missing.
	require.NoError(t, os.WriteFile(
		filepath.Join(artifacts, "mdsmith-linux-arm64"), []byte("x"), 0o755))

	err := BuildFlatpak(artifacts, filepath.Join(dir, "out"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mdsmith-linux-amd64")
}

func TestBuildFlatpakReadError(t *testing.T) {
	// A missing artifacts dir surfaces the read error for the first
	// asset and leaves no half-written out-dir behind.
	dir := t.TempDir()
	out := filepath.Join(dir, "out")
	err := BuildFlatpak(filepath.Join(dir, "nope"), out)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read mdsmith-linux-arm64")
	_, statErr := os.Stat(out)
	assert.True(t, os.IsNotExist(statErr), "out-dir not created on read error")
}

func TestBuildFlatpakMkdirError(t *testing.T) {
	// outDir under a regular file: MkdirAll fails.
	dir := t.TempDir()
	artifacts := filepath.Join(dir, "artifacts")
	stageFlatpakArtifacts(t, artifacts)
	parent := filepath.Join(dir, "afile")
	require.NoError(t, os.WriteFile(parent, []byte("x"), 0o644))

	err := BuildFlatpak(artifacts, filepath.Join(parent, "out"))
	require.Error(t, err)
}

func TestBuildFlatpakStageError(t *testing.T) {
	// A staged-binary target path that is already a directory makes
	// the copy's WriteFile fail.
	dir := t.TempDir()
	artifacts := filepath.Join(dir, "artifacts")
	stageFlatpakArtifacts(t, artifacts)
	out := filepath.Join(dir, "out")
	require.NoError(t, os.MkdirAll(filepath.Join(out, "mdsmith-linux-arm64"), 0o755))

	err := BuildFlatpak(artifacts, out)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stage mdsmith-linux-arm64")
}

func TestBuildFlatpakWriteManifestError(t *testing.T) {
	// The manifest target path is a directory, so WriteFile fails.
	dir := t.TempDir()
	artifacts := filepath.Join(dir, "artifacts")
	stageFlatpakArtifacts(t, artifacts)
	out := filepath.Join(dir, "out")
	require.NoError(t, os.MkdirAll(
		filepath.Join(out, "io.github.jeduden.mdsmith.yml"), 0o755))

	err := BuildFlatpak(artifacts, out)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "write manifest")
}
