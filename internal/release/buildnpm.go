package release

import (
	"fmt"
	"io/fs"
	"path/filepath"
)

// platformBuild pins one entry of the npm distribution matrix.
// Stays in lock-step with .github/workflows/release.yml's build
// matrix (asset name) and the optionalDependencies block in
// npm/mdsmith/package.json (NodeTarget).
type platformBuild struct {
	Asset      string // release-asset basename (e.g. "mdsmith-linux-amd64")
	NodeTarget string // npm sub-package suffix (e.g. "linux-x64")
	Exe        string // installed binary name ("mdsmith" or "mdsmith.exe")
	NodeOS     string // npm package.json `os` value
	NodeArch   string // npm package.json `cpu` value
}

var npmPlatformBuilds = []platformBuild{
	{"mdsmith-linux-amd64", "linux-x64", "mdsmith", "linux", "x64"},
	{"mdsmith-linux-arm64", "linux-arm64", "mdsmith", "linux", "arm64"},
	{"mdsmith-darwin-amd64", "darwin-x64", "mdsmith", "darwin", "x64"},
	{"mdsmith-darwin-arm64", "darwin-arm64", "mdsmith", "darwin", "arm64"},
	{"mdsmith-windows-amd64.exe", "win32-x64", "mdsmith.exe", "win32", "x64"},
}

// BuildNpmPlatforms emits one ready-to-publish npm sub-package
// directory per supported platform under outDir, copying the
// matching release artifact from artifactsDir. Stamp must run
// first because the version is taken from
// rootDir/npm/mdsmith/package.json.
func (t *Toolkit) BuildNpmPlatforms(rootDir, artifactsDir, outDir string) error {
	version, err := t.readJSONVersion(filepath.Join(rootDir, "npm", "mdsmith", "package.json"))
	if err != nil {
		return err
	}
	if err := t.fs.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	for _, pb := range npmPlatformBuilds {
		if err := t.buildOneNpmPlatform(rootDir, artifactsDir, outDir, version, pb); err != nil {
			return err
		}
	}
	return nil
}

// BuildNpmPlatforms delegates to a default-OS Toolkit (see Stamp).
func BuildNpmPlatforms(rootDir, artifactsDir, outDir string) error {
	return New().BuildNpmPlatforms(rootDir, artifactsDir, outDir)
}

func (t *Toolkit) buildOneNpmPlatform(rootDir, artifactsDir, outDir, version string, pb platformBuild) error {
	src := filepath.Join(artifactsDir, pb.Asset)
	if _, err := t.fs.Stat(src); err != nil {
		return fmt.Errorf("missing release asset: %s", src)
	}

	pkgDir := filepath.Join(outDir, pb.NodeTarget)
	binDir := filepath.Join(pkgDir, "bin")
	if err := t.fs.MkdirAll(binDir, 0o755); err != nil {
		return err
	}
	if err := t.copyFile(src, filepath.Join(binDir, pb.Exe), 0o755); err != nil {
		return err
	}

	manifest := fmt.Sprintf(`{
  "name": "@mdsmith/%s",
  "version": "%s",
  "description": "Prebuilt mdsmith binary for %s %s.",
  "license": "MIT",
  "homepage": "https://mdsmith.dev",
  "repository": {
    "type": "git",
    "url": "git+https://github.com/jeduden/mdsmith.git"
  },
  "os": ["%s"],
  "cpu": ["%s"],
  "files": ["bin/"]
}
`, pb.NodeTarget, version, pb.NodeOS, pb.NodeArch, pb.NodeOS, pb.NodeArch)

	if err := t.fs.WriteFile(filepath.Join(pkgDir, "package.json"), []byte(manifest), 0o644); err != nil {
		return err
	}

	// LICENSE is optional; we copy it next to each platform manifest
	// when the repo root has one so the published tarball mirrors the
	// repo. A missing LICENSE just skips the copy.
	if license, err := t.fs.ReadFile(filepath.Join(rootDir, "LICENSE")); err == nil {
		if err := t.fs.WriteFile(filepath.Join(pkgDir, "LICENSE"), license, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// copyFile reads src in full and writes it at dst with the given
// mode. Reading the binary into memory is acceptable here — the
// release binaries are a few MB at most and the FS interface
// keeps the test surface minimal (no streaming Open/Create).
func (t *Toolkit) copyFile(src, dst string, mode fs.FileMode) error {
	data, err := t.fs.ReadFile(src)
	if err != nil {
		return err
	}
	return t.fs.WriteFile(dst, data, mode)
}

func (t *Toolkit) readJSONVersion(path string) (string, error) {
	body, err := t.fs.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	sub := jsonVersionRE.FindSubmatch(body)
	if sub == nil {
		return "", fmt.Errorf("%s: no version field found", path)
	}
	return string(sub[2]), nil
}
