package release

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
)

// obsidianZipFiles is the exact set of files the Obsidian release zip
// ships, stored flat (base name only, no dist/ prefix). It mirrors the
// five files Obsidian loads from a plugin directory and the
// upload-artifact contract in ci.yml / release.yml. manifest.json
// doubles as the version source.
var obsidianZipFiles = []string{
	"main.js",
	"manifest.json",
	"styles.css",
	"mdsmith.wasm",
	"wasm_exec.js",
}

// PackageObsidian builds the Obsidian plugin release zip from a built
// dist directory. It reads the plugin version from
// <distDir>/manifest.json (JSON `version` field) and writes
// <outDir>/mdsmith-obsidian-<version>.zip containing exactly the five
// files in obsidianZipFiles, stored flat. It returns the created zip's
// path.
//
// The zip is built with archive/zip rather than shelling out to the
// `zip` binary so the step is pure Go, cross-platform, and unit
// testable — the rule in docs/development/release-tooling.md. Every
// dist file is read into memory before outDir is touched, so a missing
// manifest.json, a manifest with no version, or any missing required
// file fails before the function creates anything.
func PackageObsidian(distDir, outDir string) (string, error) {
	version, err := readObsidianVersion(filepath.Join(distDir, "manifest.json"))
	if err != nil {
		return "", err
	}

	contents := make([][]byte, len(obsidianZipFiles))
	for i, name := range obsidianZipFiles {
		data, err := os.ReadFile(filepath.Join(distDir, name))
		if err != nil {
			return "", fmt.Errorf("read %s: %w", name, err)
		}
		contents[i] = data
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", err
	}
	zipPath := filepath.Join(outDir, "mdsmith-obsidian-"+version+".zip")
	archive := buildObsidianZip(contents)
	if err := os.WriteFile(zipPath, archive, 0o644); err != nil {
		return "", err
	}
	// Parity with the old `ls -l`: a one-line confirmation of the
	// artifact path and size.
	fmt.Printf("packaged %s (%d bytes)\n", zipPath, len(archive))
	return zipPath, nil
}

// readObsidianVersion reads the plugin version from a manifest.json at
// path. It reuses jsonVersionRE (the same matcher build-npm uses) so a
// missing file, a manifest without a "version" field, or an empty
// version all surface as a clear error.
func readObsidianVersion(path string) (string, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	sub := jsonVersionRE.FindSubmatch(body)
	if sub == nil {
		return "", fmt.Errorf("%s: no version field found", path)
	}
	version := string(sub[2])
	// The version becomes part of the output zip's filename, so a value
	// with a path separator (e.g. "1.0.0/../../x") could escape outDir.
	// package-obsidian takes an arbitrary dist dir, so accept only the
	// dev sentinel or a valid semver — the two forms the stamp step and a
	// checked-in manifest ever produce.
	if version != DevSentinel {
		if err := ValidateSemver(version); err != nil {
			return "", fmt.Errorf("%s: invalid version %q: %w", path, version, err)
		}
	}
	return version, nil
}

// buildObsidianZip encodes the dist file contents (indexed to match
// obsidianZipFiles) into zip bytes, each stored flat under its base
// name. The destination is an in-memory bytes.Buffer, which never fails
// a write, so the archive/zip calls cannot return a real error here and
// their results are intentionally discarded — the same deliberate-ignore
// idiom bench.go uses for `_ = out.Close()`. The only failure mode, a
// missing source file, is handled by PackageObsidian when it reads the
// files (before this is called), so this returns no error.
func buildObsidianZip(contents [][]byte) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for i, name := range obsidianZipFiles {
		w, _ := zw.Create(name)
		_, _ = w.Write(contents[i])
	}
	_ = zw.Close()
	return buf.Bytes()
}
