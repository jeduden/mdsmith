package release

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// pythonExecutable picks `python` if it's on PATH, otherwise
// falls back to `python3`. Some distros only ship `python3`
// (Debian since 2020+), so hard-coding `python` would break on
// otherwise-correct hosts. The release workflow's PyPI job
// installs a `python3` symlink, but the Go test environment
// (developer machines, the test job) may only have python3.
//
// execLookPath is a package-level seam over [exec.LookPath] so
// tests can drive both branches portably — bare `python` files
// fail LookPath on Windows because of PATHEXT, and t.Setenv on
// PATH races with parallel tests in this package.
var execLookPath = exec.LookPath

func pythonExecutable() string {
	if _, err := execLookPath("python"); err == nil {
		return "python"
	}
	return "python3"
}

// absPath is a package-level indirection over filepath.Abs so
// tests can drive its error branch — filepath.Abs only fails
// when os.Getwd does, which is essentially unreachable from a
// running test process. BuildWheels wraps absPath errors with
// path context, and codecov flags those wraps as uncovered
// without this seam.
var absPath = filepath.Abs

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
func (t *Toolkit) BuildWheels(rootDir, artifactsDir, outDir string) error {
	// Resolve outDir and artifactsDir to absolute paths up
	// front. buildOneWheel runs `python -m build --outdir <…>`
	// with cmd.Dir set to a staged temp tree, so a relative
	// outDir would be interpreted by python relative to that
	// temp dir — the wheel would land somewhere we never look,
	// listWheels would return an empty slice, and (without the
	// post-build guard) the workflow would silently move on
	// with an empty python/dist before failing at publish time.
	absOut, err := absPath(outDir)
	if err != nil {
		return fmt.Errorf("resolve outDir %q: %w", outDir, err)
	}
	absArtifacts, err := absPath(artifactsDir)
	if err != nil {
		return fmt.Errorf("resolve artifactsDir %q: %w", artifactsDir, err)
	}
	if err := t.fs.MkdirAll(absOut, 0o755); err != nil {
		return err
	}
	src := filepath.Join(rootDir, "python")
	if _, err := t.fs.Stat(src); err != nil {
		return fmt.Errorf("python source missing: %w", err)
	}
	for _, wb := range wheelBuilds {
		if err := t.buildOneWheel(src, absArtifacts, absOut, wb); err != nil {
			return err
		}
	}
	return nil
}

// BuildWheels delegates to a default-OS Toolkit (see Stamp).
func BuildWheels(rootDir, artifactsDir, outDir string) error {
	return New().BuildWheels(rootDir, artifactsDir, outDir)
}

func (t *Toolkit) buildOneWheel(src, artifactsDir, outDir string, wb wheelBuild) error {
	asset := filepath.Join(artifactsDir, wb.Asset)
	if _, err := t.fs.Stat(asset); err != nil {
		return fmt.Errorf("missing release asset: %s", asset)
	}
	stage, err := t.stagePythonTree(src, asset, wb.Exe)
	if err != nil {
		return err
	}
	// Always remove the stage dir on the way out, even when a
	// downstream step (python -m build, wheel tags) fails — bash's
	// `trap RETURN` only fired on a clean return and leaked dirs on
	// failure.
	defer func() { _ = t.fs.RemoveAll(stage) }()

	staging := filepath.Join(outDir, ".staging-"+wb.PlatTag)
	// Wipe before mkdir so a stale `.staging-<plat>/` left over
	// from a killed previous run cannot fool the post-build
	// empty-wheel guard. RemoveAll on a missing path is a no-op.
	if err := t.fs.RemoveAll(staging); err != nil {
		return fmt.Errorf("wipe staging %s: %w", staging, err)
	}
	if err := t.fs.MkdirAll(staging, 0o755); err != nil {
		return fmt.Errorf("mkdir staging %s: %w", staging, err)
	}
	defer func() { _ = t.fs.RemoveAll(staging) }()

	if err := t.runPythonBuild(stage, staging, wb.PlatTag); err != nil {
		return err
	}
	// `python -m build --wheel` exits 0 even when, for whatever
	// reason, no wheel actually lands in the staging directory.
	// We can't catch that via Run() alone, and the empty-loop
	// silence in retagWheels / moveWheels would let the workflow
	// continue with an empty outDir and only fail later at
	// publish-time. Verify here instead.
	staged, err := t.listWheels(staging)
	if err != nil {
		return err
	}
	if len(staged) == 0 {
		return fmt.Errorf("python -m build (%s) produced no wheel in %s",
			wb.PlatTag, staging)
	}
	if err := t.retagWheels(staging, wb.PlatTag); err != nil {
		return err
	}
	return t.moveWheels(staging, outDir)
}

func (t *Toolkit) stagePythonTree(src, asset, exe string) (string, error) {
	stage, err := t.fs.MkdirTemp("", "mdsmith-wheel-*")
	if err != nil {
		return "", err
	}
	if err := t.copyDir(src, stage); err != nil {
		_ = t.fs.RemoveAll(stage)
		return "", fmt.Errorf("copy python tree: %w", err)
	}
	// Stage the root LICENSE alongside pyproject.toml so the wheel
	// carries the MIT notice (and the embedded third-party notice
	// for internal/punkt/). pyproject.toml's `license-files`
	// setting expects the file to be present; hatchling would fail
	// the build otherwise. We fail loudly here too — a wheel that
	// shipped without the notice would be a license-compliance bug,
	// and the symmetric upfront check produces a clearer error than
	// hatchling's downstream complaint.
	licensePath := filepath.Join(filepath.Dir(src), "LICENSE")
	license, err := t.fs.ReadFile(licensePath)
	if err != nil {
		_ = t.fs.RemoveAll(stage)
		return "", fmt.Errorf("read LICENSE %s: %w", licensePath, err)
	}
	stagedLicense := filepath.Join(stage, "LICENSE")
	if err := t.fs.WriteFile(stagedLicense, license, 0o644); err != nil {
		_ = t.fs.RemoveAll(stage)
		return "", fmt.Errorf("stage LICENSE at %s: %w", stagedLicense, err)
	}
	binDir := filepath.Join(stage, "mdsmith", "_bin")
	if err := t.fs.MkdirAll(binDir, 0o755); err != nil {
		_ = t.fs.RemoveAll(stage)
		return "", err
	}
	if err := t.copyFile(asset, filepath.Join(binDir, exe), 0o755); err != nil {
		_ = t.fs.RemoveAll(stage)
		return "", err
	}
	return stage, nil
}

// runPythonBuild shells out to `python -m build`. The FS-side
// effects (writing the wheel file inside outDir) happen inside
// the python interpreter and are not measured through Toolkit.fs;
// the Runner abstraction lets tests cover the failure branch
// without putting python on PATH.
func (t *Toolkit) runPythonBuild(stage, outDir, platTag string) error {
	if err := t.runner.RunCommand(stage, pythonExecutable(), "-m", "build", "--wheel", "--outdir", outDir); err != nil {
		return fmt.Errorf("python -m build (%s): %w", platTag, err)
	}
	return nil
}

func (t *Toolkit) retagWheels(staging, platTag string) error {
	wheels, err := t.listWheels(staging)
	if err != nil {
		return err
	}
	for _, whl := range wheels {
		if err := t.runner.RunCommand("", pythonExecutable(), "-m", "wheel", "tags",
			"--remove", "--platform-tag", platTag, whl); err != nil {
			return fmt.Errorf("python -m wheel tags (%s): %w", platTag, err)
		}
	}
	return nil
}

func (t *Toolkit) moveWheels(staging, outDir string) error {
	wheels, err := t.listWheels(staging)
	if err != nil {
		return err
	}
	for _, whl := range wheels {
		if err := t.fs.Rename(whl, filepath.Join(outDir, filepath.Base(whl))); err != nil {
			return fmt.Errorf("move %s: %w", whl, err)
		}
	}
	return nil
}

func (t *Toolkit) listWheels(dir string) ([]string, error) {
	entries, err := t.fs.ReadDir(dir)
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

// copyDir walks src via the Toolkit FS (no filepath.WalkDir) so a
// fault-injecting FS can drive ReadDir / MkdirAll / ReadFile /
// WriteFile failures at any level of the recursion.
func (t *Toolkit) copyDir(src, dst string) error {
	entries, err := t.fs.ReadDir(src)
	if err != nil {
		return err
	}
	if err := t.fs.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, e := range entries {
		sp := filepath.Join(src, e.Name())
		dp := filepath.Join(dst, e.Name())
		if e.IsDir() {
			if err := t.copyDir(sp, dp); err != nil {
				return err
			}
			continue
		}
		info, err := e.Info()
		if err != nil {
			return err
		}
		if err := t.copyFile(sp, dp, info.Mode()); err != nil {
			return err
		}
	}
	return nil
}
