package release

import (
	"fmt"
	"io"
	"os"
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
func BuildNpmPlatforms(rootDir, artifactsDir, outDir string) error {
	version, err := readJSONVersion(filepath.Join(rootDir, "npm", "mdsmith", "package.json"))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	for _, pb := range npmPlatformBuilds {
		if err := buildOneNpmPlatform(rootDir, artifactsDir, outDir, version, pb); err != nil {
			return err
		}
	}
	return nil
}

func buildOneNpmPlatform(rootDir, artifactsDir, outDir, version string, pb platformBuild) error {
	src := filepath.Join(artifactsDir, pb.Asset)
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("missing release asset: %s", src)
	}

	pkgDir := filepath.Join(outDir, pb.NodeTarget)
	binDir := filepath.Join(pkgDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return err
	}
	if err := copyFile(src, filepath.Join(binDir, pb.Exe), 0o755); err != nil {
		return err
	}

	manifest := fmt.Sprintf(`{
  "name": "@mdsmith/%s",
  "version": "%s",
  "description": "Prebuilt mdsmith binary for %s %s.",
  "license": "MIT",
  "homepage": "https://github.com/jeduden/mdsmith",
  "repository": {
    "type": "git",
    "url": "https://github.com/jeduden/mdsmith"
  },
  "os": ["%s"],
  "cpu": ["%s"],
  "files": ["bin/"]
}
`, pb.NodeTarget, version, pb.NodeOS, pb.NodeArch, pb.NodeOS, pb.NodeArch)

	if err := os.WriteFile(filepath.Join(pkgDir, "package.json"), []byte(manifest), 0o644); err != nil {
		return err
	}

	// LICENSE is optional; we copy it next to each platform manifest
	// when the repo root has one so the published tarball mirrors the
	// repo. A missing LICENSE just skips the copy.
	if license, err := os.ReadFile(filepath.Join(rootDir, "LICENSE")); err == nil {
		if err := os.WriteFile(filepath.Join(pkgDir, "LICENSE"), license, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func readJSONVersion(path string) (string, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	sub := jsonVersionRE.FindSubmatch(body)
	if sub == nil {
		return "", fmt.Errorf("%s: no version field found", path)
	}
	return string(sub[2]), nil
}
