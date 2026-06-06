package release

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeArtifacts populates the layout `actions/download-artifact`
// produces under `merge-multiple: true` — one binary per asset in
// a single flat directory.
func fakeArtifacts(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	for _, asset := range []string{
		"mdsmith-linux-amd64",
		"mdsmith-linux-arm64",
		"mdsmith-darwin-amd64",
		"mdsmith-darwin-arm64",
		"mdsmith-windows-amd64.exe",
	} {
		body := []byte("#!/bin/sh\necho fake-" + asset + "\n")
		require.NoError(t, os.WriteFile(filepath.Join(dir, asset), body, 0o755))
	}
}

func assertPlatformPackage(t *testing.T, out, dir, bin, expectedOS, expectedCPU, expectedVer string) {
	t.Helper()
	_, err := os.Stat(filepath.Join(out, dir, "bin", bin))
	require.NoError(t, err, "binary %s/bin/%s missing", dir, bin)

	manifest := filepath.Join(out, dir, "package.json")
	body, err := os.ReadFile(manifest)
	require.NoError(t, err, "read %s", manifest)

	var pkg struct {
		Name       string   `json:"name"`
		Version    string   `json:"version"`
		Homepage   string   `json:"homepage"`
		OS         []string `json:"os"`
		CPU        []string `json:"cpu"`
		Files      []string `json:"files"`
		Repository struct {
			Type string `json:"type"`
			URL  string `json:"url"`
		} `json:"repository"`
	}
	require.NoError(t, json.Unmarshal(body, &pkg), "decode %s", manifest)
	assert.Equal(t, "@mdsmith/"+dir, pkg.Name, "%s name", manifest)
	assert.Equal(t, expectedVer, pkg.Version, "%s version", manifest)
	// Published metadata points users at the project site; the
	// repository field (asserted below) stays GitHub for npm
	// provenance and bug reports.
	assert.Equal(t, "https://mdsmith.dev", pkg.Homepage, "%s homepage", manifest)
	assert.Equal(t, []string{expectedOS}, pkg.OS, "%s os", manifest)
	assert.Equal(t, []string{expectedCPU}, pkg.CPU, "%s cpu", manifest)
	// repository.url uses the `git+https://…/repo.git` shape so
	// `npm publish` doesn't normalise it (and warn) at upload
	// time. A bare `https://github.com/...` URL trips the
	// "auto-corrected" warning and a future npm version may
	// outright reject it.
	assert.Equal(t, "git", pkg.Repository.Type, "%s repository.type", manifest)
	assert.Equal(t, "git+https://github.com/jeduden/mdsmith.git", pkg.Repository.URL,
		"%s repository.url", manifest)
}

func TestBuildNpmPlatformsLayout(t *testing.T) {
	const ver = "4.5.6"
	root := t.TempDir()
	fixtureManifests(t, root)
	require.NoError(t, Stamp(root, ver))

	artifacts := filepath.Join(root, "artifacts")
	fakeArtifacts(t, artifacts)
	out := filepath.Join(root, "dist")
	require.NoError(t, BuildNpmPlatforms(root, artifacts, out))

	cases := []struct {
		dir, bin, os, cpu string
	}{
		{"linux-x64", "mdsmith", "linux", "x64"},
		{"linux-arm64", "mdsmith", "linux", "arm64"},
		{"darwin-x64", "mdsmith", "darwin", "x64"},
		{"darwin-arm64", "mdsmith", "darwin", "arm64"},
		{"win32-x64", "mdsmith.exe", "win32", "x64"},
	}
	for _, c := range cases {
		assertPlatformPackage(t, out, c.dir, c.bin, c.os, c.cpu, ver)
	}
}

func TestBuildNpmPlatformsMissingArtifact(t *testing.T) {
	const ver = "4.5.6"
	root := t.TempDir()
	fixtureManifests(t, root)
	require.NoError(t, Stamp(root, ver))

	// Stage every artifact except one. The build must fail with
	// an actionable message naming the missing file.
	artifacts := filepath.Join(root, "artifacts")
	fakeArtifacts(t, artifacts)
	require.NoError(t, os.Remove(filepath.Join(artifacts, "mdsmith-darwin-arm64")))

	err := BuildNpmPlatforms(root, artifacts, filepath.Join(root, "dist"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing release asset")
}

func TestBuildNpmPlatformsFailsWhenRootManifestMissing(t *testing.T) {
	// BuildNpmPlatforms reads the version off npm/mdsmith/package.json.
	// A missing root manifest must produce an actionable error rather
	// than silently emitting empty-version sub-packages.
	root := t.TempDir()
	artifacts := filepath.Join(root, "artifacts")
	fakeArtifacts(t, artifacts)

	err := BuildNpmPlatforms(root, artifacts, filepath.Join(root, "dist"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "npm/mdsmith/package.json")
}

// TestNpmChannelDocMatchesPlatformBuilds is the consistency gate
// for the npm package enumeration. `docs/development/release-
// channels/npm.md` is documented as the canonical list (release.md
// and docs/guides/install.md link to it instead of duplicating);
// this test parses the first contiguous bullet list in that file
// and asserts an exact ordered match against `npmPlatformBuilds`
// (preceded by the `@mdsmith/cli` root). The exact match catches
// missing entries, extra/obsolete entries, reordering, and
// occurrences that happen to appear in unrelated prose or in code
// blocks — any of which would silently let publishing drift from
// the docs.
func TestNpmChannelDocMatchesPlatformBuilds(t *testing.T) {
	// Walk up from this package to the repo root. The doc path is
	// relative to that, not relative to internal/release.
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	docPath := filepath.Join(repoRoot, "docs", "development", "release-channels", "npm.md")
	body, err := os.ReadFile(docPath)
	require.NoError(t, err)

	// Extract the first contiguous run of bullet lines. The doc
	// has exactly one such block (the canonical list); any future
	// edit that adds a second list above it would fail this test
	// loudly, which is the intended UX. A blank line ends the run.
	var actual []string
	inList := false
	for _, line := range strings.Split(string(body), "\n") {
		// Strip trailing CR for CRLF-checked-out files.
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, "- ") {
			actual = append(actual, line)
			inList = true
			continue
		}
		if inList {
			break
		}
	}
	require.NotEmpty(t, actual, "npm.md: no bullet list found")

	expected := make([]string, 0, 1+len(npmPlatformBuilds))
	expected = append(expected, "- `@mdsmith/cli` — root, contains the shim")
	for _, pb := range npmPlatformBuilds {
		expected = append(expected, "- `@mdsmith/"+pb.NodeTarget+"`")
	}

	assert.Equal(t, expected, actual,
		"npm.md's first bullet list does not exactly match "+
			"npmPlatformBuilds (preceded by @mdsmith/cli). "+
			"Update both together — the doc is the single source "+
			"of truth that the release and install docs link to.")
}

func TestBuildNpmPlatformsFailsWhenOutDirIsFile(t *testing.T) {
	// outDir resolves to a regular file. os.MkdirAll on a path
	// whose parent is a file fails; the per-platform mkdir inside
	// buildOneNpmPlatform exercises that branch.
	const ver = "4.5.6"
	root := t.TempDir()
	fixtureManifests(t, root)
	require.NoError(t, Stamp(root, ver))
	artifacts := filepath.Join(root, "artifacts")
	fakeArtifacts(t, artifacts)
	outFile := filepath.Join(root, "out-as-file")
	require.NoError(t, os.WriteFile(outFile, []byte("x"), 0o644))

	err := BuildNpmPlatforms(root, artifacts, outFile)
	require.Error(t, err)
}

func TestBuildNpmPlatformsFailsWhenPkgDirCollides(t *testing.T) {
	// outDir is a real directory but contains a regular file at
	// the per-platform path. MkdirAll on a path whose final
	// component is a non-directory returns "not a directory".
	// Covers buildOneNpmPlatform's mkdir-fails branch.
	const ver = "4.5.6"
	root := t.TempDir()
	fixtureManifests(t, root)
	require.NoError(t, Stamp(root, ver))
	artifacts := filepath.Join(root, "artifacts")
	fakeArtifacts(t, artifacts)
	out := filepath.Join(root, "dist")
	require.NoError(t, os.MkdirAll(out, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(out, "linux-x64"), []byte("x"), 0o644))

	err := BuildNpmPlatforms(root, artifacts, out)
	require.Error(t, err)
}

func TestBuildNpmPlatformsCopiesLicense(t *testing.T) {
	// When the repo carries a top-level LICENSE, each platform
	// sub-package should ship the same file. A missing LICENSE
	// is fine — the copy is best-effort.
	const ver = "4.5.6"
	root := t.TempDir()
	fixtureManifests(t, root)
	require.NoError(t, Stamp(root, ver))

	licenseBody := []byte("MIT License — sentinel\n")
	require.NoError(t, os.WriteFile(filepath.Join(root, "LICENSE"), licenseBody, 0o644))

	artifacts := filepath.Join(root, "artifacts")
	fakeArtifacts(t, artifacts)
	out := filepath.Join(root, "dist")
	require.NoError(t, BuildNpmPlatforms(root, artifacts, out))

	for _, plat := range []string{"linux-x64", "darwin-arm64", "win32-x64"} {
		got, err := os.ReadFile(filepath.Join(out, plat, "LICENSE"))
		require.NoError(t, err, "%s LICENSE", plat)
		assert.Equal(t, string(licenseBody), string(got), "%s LICENSE content", plat)
	}
}
