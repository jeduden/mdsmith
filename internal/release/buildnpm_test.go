package release

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// fakeArtifacts populates the layout that `actions/download-artifact`
// produces under `merge-multiple: true` — a single flat directory
// holding one binary per asset name.
func fakeArtifacts(t *testing.T, dir string) {
	t.Helper()
	for _, asset := range []string{
		"mdsmith-linux-amd64",
		"mdsmith-linux-arm64",
		"mdsmith-darwin-amd64",
		"mdsmith-darwin-arm64",
		"mdsmith-windows-amd64.exe",
	} {
		path := filepath.Join(dir, asset)
		body := []byte("#!/bin/sh\necho fake-" + asset + "\n")
		if err := os.WriteFile(path, body, 0o755); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
}

// stageBuildNpm prepares a temp tree that mirrors the repo layout
// build-npm-platforms.sh expects (manifests stamped at the target
// version, fake release artifacts staged, the script copied so its
// `dirname/..` resolves to the temp root) and returns the artifacts
// dir, the staged-script path, and the output dir.
func stageBuildNpm(t *testing.T, version string) (string, string, string) {
	t.Helper()
	repo := projectRoot(t)
	root := t.TempDir()
	fixtureManifests(t, root)

	if _, stderr, err := runScript(t,
		filepath.Join(repo, "scripts", "set-version.sh"),
		version, "--root", root,
	); err != nil {
		t.Fatalf("set-version.sh: %v\nstderr: %s", err, stderr)
	}

	artifacts := filepath.Join(root, "artifacts")
	if err := os.MkdirAll(artifacts, 0o755); err != nil {
		t.Fatalf("mkdir artifacts: %v", err)
	}
	fakeArtifacts(t, artifacts)

	scriptsDir := filepath.Join(root, "scripts")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		t.Fatalf("mkdir scripts: %v", err)
	}
	scriptSrc := filepath.Join(repo, "scripts", "build-npm-platforms.sh")
	scriptDst := filepath.Join(scriptsDir, "build-npm-platforms.sh")
	body, err := os.ReadFile(scriptSrc)
	if err != nil {
		t.Fatalf("read script: %v", err)
	}
	if err := os.WriteFile(scriptDst, body, 0o755); err != nil {
		t.Fatalf("copy script: %v", err)
	}

	return artifacts, scriptDst, filepath.Join(root, "dist")
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
	if runtime.GOOS == "windows" {
		t.Skip("build-npm-platforms.sh requires a POSIX shell")
	}
	const ver = "4.5.6"
	artifacts, script, out := stageBuildNpm(t, ver)

	if _, stderr, err := runScript(t, script, artifacts, out); err != nil {
		t.Fatalf("build-npm-platforms.sh failed: %v\nstderr: %s", err, stderr)
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
