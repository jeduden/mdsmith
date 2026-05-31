package release

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stageFlatpakArtifacts writes the x86_64 Linux release binary
// build-flatpak reads into dir.
func stageFlatpakArtifacts(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "mdsmith-linux-amd64"), []byte("#!/bin/sh\necho fake\n"), 0o755))
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

	// A single local-path source: the x86_64 linux release binary,
	// installed as mdsmith.
	assert.Contains(t, manifest, "- x86_64")
	assert.Contains(t, manifest, "path: mdsmith-linux-amd64")
	assert.Contains(t, manifest, "dest-filename: mdsmith")

	// flatpak-builder resolves `path:` next to the manifest, so the
	// binary is staged into out-dir.
	_, err = os.Stat(filepath.Join(out, "mdsmith-linux-amd64"))
	assert.NoError(t, err)

	// x86_64 only: no aarch64 source. And the bundle builds from a
	// local binary and ships as a single file, so the manifest carries
	// no release-download URL and no AppStream metainfo.
	assert.NotContains(t, manifest, "aarch64")
	assert.NotContains(t, manifest, "mdsmith-linux-arm64")
	assert.NotContains(t, manifest, "https://github.com/jeduden/mdsmith/releases")
	assert.NotContains(t, manifest, "metainfo")
	_, err = os.Stat(filepath.Join(out, "io.github.jeduden.mdsmith.metainfo.xml"))
	assert.True(t, os.IsNotExist(err), "no metainfo file")
}

func TestBuildFlatpakMissingBinary(t *testing.T) {
	// An empty artifacts dir surfaces the staging error naming the
	// missing binary.
	dir := t.TempDir()
	artifacts := filepath.Join(dir, "artifacts")
	require.NoError(t, os.MkdirAll(artifacts, 0o755))

	err := BuildFlatpak(artifacts, filepath.Join(dir, "out"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stage mdsmith-linux-amd64")
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
	require.NoError(t, os.MkdirAll(filepath.Join(out, "mdsmith-linux-amd64"), 0o755))

	err := BuildFlatpak(artifacts, out)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stage mdsmith-linux-amd64")
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
