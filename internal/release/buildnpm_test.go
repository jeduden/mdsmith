package release

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// fakeArtifacts populates the layout `actions/download-artifact`
// produces under `merge-multiple: true` — one binary per asset in
// a single flat directory.
func fakeArtifacts(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	for _, asset := range []string{
		"mdsmith-linux-amd64",
		"mdsmith-linux-arm64",
		"mdsmith-darwin-amd64",
		"mdsmith-darwin-arm64",
		"mdsmith-windows-amd64.exe",
	} {
		body := []byte("#!/bin/sh\necho fake-" + asset + "\n")
		if err := os.WriteFile(filepath.Join(dir, asset), body, 0o755); err != nil {
			t.Fatalf("write %s: %v", asset, err)
		}
	}
}

func assertPlatformPackage(t *testing.T, out, dir, bin, expectedOS, expectedCPU, expectedVer string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(out, dir, "bin", bin)); err != nil {
		t.Errorf("missing binary %s/bin/%s: %v", dir, bin, err)
		return
	}
	manifest := filepath.Join(out, dir, "package.json")
	body, err := os.ReadFile(manifest)
	if err != nil {
		t.Errorf("read %s: %v", manifest, err)
		return
	}
	var pkg struct {
		Name    string   `json:"name"`
		Version string   `json:"version"`
		OS      []string `json:"os"`
		CPU     []string `json:"cpu"`
		Files   []string `json:"files"`
	}
	if err := json.Unmarshal(body, &pkg); err != nil {
		t.Errorf("decode %s: %v\n%s", manifest, err, body)
		return
	}
	if want := "@mdsmith/" + dir; pkg.Name != want {
		t.Errorf("%s: name=%q, want %q", manifest, pkg.Name, want)
	}
	if pkg.Version != expectedVer {
		t.Errorf("%s: version=%q, want %s", manifest, pkg.Version, expectedVer)
	}
	if len(pkg.OS) != 1 || pkg.OS[0] != expectedOS {
		t.Errorf("%s: os=%v, want [%s]", manifest, pkg.OS, expectedOS)
	}
	if len(pkg.CPU) != 1 || pkg.CPU[0] != expectedCPU {
		t.Errorf("%s: cpu=%v, want [%s]", manifest, pkg.CPU, expectedCPU)
	}
}

func TestBuildNpmPlatformsLayout(t *testing.T) {
	const ver = "4.5.6"
	root := t.TempDir()
	fixtureManifests(t, root)
	if err := Stamp(root, ver); err != nil {
		t.Fatalf("Stamp: %v", err)
	}
	artifacts := filepath.Join(root, "artifacts")
	fakeArtifacts(t, artifacts)
	out := filepath.Join(root, "dist")

	if err := BuildNpmPlatforms(root, artifacts, out); err != nil {
		t.Fatalf("BuildNpmPlatforms: %v", err)
	}

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
	if err := Stamp(root, ver); err != nil {
		t.Fatalf("Stamp: %v", err)
	}
	// Stage every artifact except one. The build must fail with
	// an actionable message naming the missing file.
	artifacts := filepath.Join(root, "artifacts")
	fakeArtifacts(t, artifacts)
	if err := os.Remove(filepath.Join(artifacts, "mdsmith-darwin-arm64")); err != nil {
		t.Fatalf("remove artifact: %v", err)
	}

	err := BuildNpmPlatforms(root, artifacts, filepath.Join(root, "dist"))
	if err == nil {
		t.Fatal("expected missing-asset error")
	}
}
