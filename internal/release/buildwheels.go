package release

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// wheelBuild pins one entry of the PyPI distribution matrix.
// Stays in lock-step with the build matrix in
// .github/workflows/release.yml.
type wheelBuild struct {
	Asset   string // release-asset basename
	PlatTag string // wheel platform tag (filename + dist-info/WHEEL)
	Exe     string // bundled binary name under mdsmith/_bin/
}

var wheelBuilds = []wheelBuild{
	{"mdsmith-linux-amd64", "manylinux_2_17_x86_64.manylinux2014_x86_64", "mdsmith"},
	{"mdsmith-linux-arm64", "manylinux_2_17_aarch64.manylinux2014_aarch64", "mdsmith"},
	{"mdsmith-darwin-amd64", "macosx_11_0_x86_64", "mdsmith"},
	{"mdsmith-darwin-arm64", "macosx_11_0_arm64", "mdsmith"},
	{"mdsmith-windows-amd64.exe", "win_amd64", "mdsmith.exe"},
}

// BuildWheels builds one platform-tagged wheel per supported host
// from prebuilt binaries in artifactsDir, writing the wheels to
// outDir. The python source tree at rootDir/python is staged per
// build with the matching binary embedded under mdsmith/_bin/,
// then `python -m build` produces a py3-none-any wheel which
// `python -m wheel tags` retags to the correct platform tag (in
// both the filename and the dist-info/WHEEL metadata).
//
// Requires `python -m build`, `python -m wheel`, and the
// hatchling build backend on PATH. Stamp must run first so
// pyproject.toml carries the published version.
func BuildWheels(rootDir, artifactsDir, outDir string) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	src := filepath.Join(rootDir, "python")
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("python source missing: %w", err)
	}
	for _, wb := range wheelBuilds {
		if err := buildOneWheel(src, artifactsDir, outDir, wb); err != nil {
			return err
		}
	}
	return nil
}

func buildOneWheel(src, artifactsDir, outDir string, wb wheelBuild) error {
	asset := filepath.Join(artifactsDir, wb.Asset)
	if _, err := os.Stat(asset); err != nil {
		return fmt.Errorf("missing release asset: %s", asset)
	}
	stage, err := stagePythonTree(src, asset, wb.Exe)
	if err != nil {
		return err
	}
	// Always remove the stage dir on the way out, even when a
	// downstream step (python -m build, wheel tags) fails — bash's
	// `trap RETURN` only fired on a clean return and leaked dirs on
	// failure.
	defer func() { _ = os.RemoveAll(stage) }()

	staging := filepath.Join(outDir, ".staging-"+wb.PlatTag)
	if err := os.MkdirAll(staging, 0o755); err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(staging) }()

	if err := runPythonBuild(stage, staging, wb.PlatTag); err != nil {
		return err
	}
	if err := retagWheels(staging, wb.PlatTag); err != nil {
		return err
	}
	return moveWheels(staging, outDir)
}

func stagePythonTree(src, asset, exe string) (string, error) {
	stage, err := os.MkdirTemp("", "mdsmith-wheel-*")
	if err != nil {
		return "", err
	}
	if err := copyDir(src, stage); err != nil {
		_ = os.RemoveAll(stage)
		return "", fmt.Errorf("copy python tree: %w", err)
	}
	binDir := filepath.Join(stage, "mdsmith", "_bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		_ = os.RemoveAll(stage)
		return "", err
	}
	if err := copyFile(asset, filepath.Join(binDir, exe), 0o755); err != nil {
		_ = os.RemoveAll(stage)
		return "", err
	}
	return stage, nil
}

func runPythonBuild(stage, outDir, platTag string) error {
	cmd := exec.Command("python", "-m", "build", "--wheel", "--outdir", outDir)
	cmd.Dir = stage
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("python -m build (%s): %w", platTag, err)
	}
	return nil
}

func retagWheels(staging, platTag string) error {
	wheels, err := listWheels(staging)
	if err != nil {
		return err
	}
	for _, whl := range wheels {
		retag := exec.Command("python", "-m", "wheel", "tags",
			"--remove", "--platform-tag", platTag, whl)
		retag.Stdout = os.Stdout
		retag.Stderr = os.Stderr
		if err := retag.Run(); err != nil {
			return fmt.Errorf("python -m wheel tags (%s): %w", platTag, err)
		}
	}
	return nil
}

func moveWheels(staging, outDir string) error {
	wheels, err := listWheels(staging)
	if err != nil {
		return err
	}
	for _, whl := range wheels {
		if err := os.Rename(whl, filepath.Join(outDir, filepath.Base(whl))); err != nil {
			return fmt.Errorf("move %s: %w", whl, err)
		}
	}
	return nil
}

func listWheels(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("readdir %s: %w", dir, err)
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(e.Name(), ".whl") {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	return out, nil
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return copyFile(path, target, info.Mode())
	})
}
