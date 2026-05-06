package release

import (
	"archive/zip"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func haveCmd(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func haveModule(t *testing.T, mod string) bool {
	t.Helper()
	py := pythonExe()
	if py == "" {
		return false
	}
	return exec.Command(py, "-c", "import "+mod).Run() == nil
}

func pythonExe() string {
	if haveCmd("python") {
		return "python"
	}
	if haveCmd("python3") {
		return "python3"
	}
	return ""
}

func readZipMember(t *testing.T, whlPath, member string) string {
	t.Helper()
	r, err := zip.OpenReader(whlPath)
	if err != nil {
		t.Fatalf("open %s: %v", whlPath, err)
	}
	defer func() { _ = r.Close() }()
	for _, f := range r.File {
		if strings.HasSuffix(f.Name, member) {
			rc, err := f.Open()
			if err != nil {
				t.Fatalf("open zip member %s: %v", f.Name, err)
			}
			body, err := io.ReadAll(rc)
			_ = rc.Close()
			if err != nil {
				t.Fatalf("read zip member %s: %v", f.Name, err)
			}
			return string(body)
		}
	}
	return ""
}

func zipHasFile(t *testing.T, whlPath, name string) bool {
	t.Helper()
	r, err := zip.OpenReader(whlPath)
	if err != nil {
		t.Fatalf("open %s: %v", whlPath, err)
	}
	defer func() { _ = r.Close() }()
	for _, f := range r.File {
		if f.Name == name {
			return true
		}
	}
	return false
}

// stagePython copies the real python/ tree from the repo into root
// so BuildWheels has something to assemble. The fixtureManifests
// helper already wrote a stub pyproject; we replace it with the
// real one (and the package source) so hatchling has the real
// build configuration.
func stagePython(t *testing.T, root string) {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	repo := filepath.Clean(filepath.Join(wd, "..", ".."))

	for _, p := range []string{
		"python/pyproject.toml",
		"python/README.md",
		"python/mdsmith/__init__.py",
		"python/mdsmith/__main__.py",
	} {
		body, err := os.ReadFile(filepath.Join(repo, p))
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		dst := filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(dst), err)
		}
		if err := os.WriteFile(dst, body, 0o644); err != nil {
			t.Fatalf("write %s: %v", dst, err)
		}
	}
}

type wheelCase struct {
	uniqueFilenameSubstr string
	tagInWheelMetadata   string
	binName              string
}

func wheelCases() []wheelCase {
	return []wheelCase{
		{"x86_64.manylinux", "manylinux_2_17_x86_64", "mdsmith"},
		{"aarch64.manylinux", "manylinux_2_17_aarch64", "mdsmith"},
		{"macosx_11_0_x86_64", "macosx_11_0_x86_64", "mdsmith"},
		{"macosx_11_0_arm64", "macosx_11_0_arm64", "mdsmith"},
		{"win_amd64", "win_amd64", "mdsmith.exe"},
	}
}

func assertWheel(t *testing.T, out string, entries []os.DirEntry, c wheelCase) {
	t.Helper()
	var match string
	for _, e := range entries {
		if strings.Contains(e.Name(), c.uniqueFilenameSubstr) {
			match = e.Name()
			break
		}
	}
	if match == "" {
		names := []string{}
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("no wheel matched filename containing %q in %v", c.uniqueFilenameSubstr, names)
		return
	}
	whl := filepath.Join(out, match)
	meta := readZipMember(t, whl, "/WHEEL")
	if !strings.Contains(meta, c.tagInWheelMetadata) {
		t.Errorf("%s: WHEEL metadata missing platform tag %q\n%s", whl, c.tagInWheelMetadata, meta)
	}
	if strings.Contains(meta, "py3-none-any") {
		t.Errorf("%s: WHEEL metadata still claims py3-none-any\n%s", whl, meta)
	}
	if !zipHasFile(t, whl, "mdsmith/_bin/"+c.binName) {
		t.Errorf("%s: bundled binary mdsmith/_bin/%s missing", whl, c.binName)
	}
}

// TestBuildWheelsFailsWhenPythonSourceMissing exercises the
// fast-fail path that runs before any python invocation, so the
// test does not need python on PATH.
func TestBuildWheelsFailsWhenPythonSourceMissing(t *testing.T) {
	root := t.TempDir()
	artifacts := filepath.Join(root, "artifacts")
	fakeArtifacts(t, artifacts)

	err := BuildWheels(root, artifacts, filepath.Join(root, "wheels"))
	if err == nil {
		t.Fatal("expected python-source-missing error")
	}
	if !strings.Contains(err.Error(), "python source missing") {
		t.Errorf("error did not flag missing python tree: %v", err)
	}
}

// TestBuildWheelsFailsWhenArtifactMissing covers the buildOneWheel
// path that fails on os.Stat(asset) before any python invocation.
// The fixture writes python/pyproject.toml so the
// python-source-missing fast-fail above does not fire.
func TestBuildWheelsFailsWhenArtifactMissing(t *testing.T) {
	root := t.TempDir()
	fixtureManifests(t, root)
	emptyArtifacts := t.TempDir()

	err := BuildWheels(root, emptyArtifacts, filepath.Join(root, "wheels"))
	if err == nil {
		t.Fatal("expected missing-artifact error")
	}
	if !strings.Contains(err.Error(), "missing release asset") {
		t.Errorf("error did not flag missing asset: %v", err)
	}
}

// Helper-level tests so the staging/listing/moving primitives
// have direct coverage instead of only the happy-path big test.

func TestListWheelsEmpty(t *testing.T) {
	wheels, err := listWheels(t.TempDir())
	if err != nil {
		t.Fatalf("listWheels: %v", err)
	}
	if len(wheels) != 0 {
		t.Errorf("expected zero wheels, got %v", wheels)
	}
}

func TestListWheelsFiltersNonWheels(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"foo.whl", "bar.tar.gz", "baz.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	wheels, err := listWheels(dir)
	if err != nil {
		t.Fatalf("listWheels: %v", err)
	}
	if len(wheels) != 1 || filepath.Base(wheels[0]) != "foo.whl" {
		t.Errorf("expected [foo.whl], got %v", wheels)
	}
}

func TestListWheelsErrorOnMissingDir(t *testing.T) {
	_, err := listWheels(filepath.Join(t.TempDir(), "missing"))
	if err == nil {
		t.Fatal("expected error for missing dir")
	}
}

func TestMoveWheelsEmpty(t *testing.T) {
	// moveWheels iterates listWheels output; an empty staging dir
	// must be a no-op, not an error.
	staging := t.TempDir()
	out := t.TempDir()
	if err := moveWheels(staging, out); err != nil {
		t.Errorf("moveWheels on empty staging: %v", err)
	}
}

func TestMoveWheelsRelocates(t *testing.T) {
	staging := t.TempDir()
	out := t.TempDir()
	for _, name := range []string{"a.whl", "b.whl"} {
		if err := os.WriteFile(filepath.Join(staging, name), []byte(name), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	if err := moveWheels(staging, out); err != nil {
		t.Fatalf("moveWheels: %v", err)
	}
	for _, name := range []string{"a.whl", "b.whl"} {
		if _, err := os.Stat(filepath.Join(out, name)); err != nil {
			t.Errorf("missing %s in out: %v", name, err)
		}
		if _, err := os.Stat(filepath.Join(staging, name)); !os.IsNotExist(err) {
			t.Errorf("%s still in staging: %v", name, err)
		}
	}
}

func TestCopyDirCopiesNestedTree(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "dst")
	if err := os.MkdirAll(filepath.Join(src, "sub", "deep"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	files := map[string]string{
		"a.txt":          "hello",
		"sub/b.txt":      "world",
		"sub/deep/c.txt": "deep",
	}
	for rel, body := range files {
		if err := os.WriteFile(filepath.Join(src, rel), []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	if err := copyDir(src, dst); err != nil {
		t.Fatalf("copyDir: %v", err)
	}
	for rel, want := range files {
		got, err := os.ReadFile(filepath.Join(dst, rel))
		if err != nil {
			t.Errorf("read %s: %v", rel, err)
			continue
		}
		if string(got) != want {
			t.Errorf("%s: got %q, want %q", rel, got, want)
		}
	}
}

func TestStagePythonTreeFailsWhenAssetMissing(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "pyproject.toml"),
		[]byte("[project]\nname=\"x\"\n"), 0o644); err != nil {
		t.Fatalf("write pyproject: %v", err)
	}
	stage, err := stagePythonTree(src, filepath.Join(t.TempDir(), "missing-asset"), "mdsmith")
	if err == nil {
		_ = os.RemoveAll(stage)
		t.Fatal("expected missing-asset error")
	}
}

// TestBuildWheelsLayout calls BuildWheels directly and asserts
// (a) one wheel per platform tag, (b) the dist-info/WHEEL metadata
// inside each wheel claims the matching platform tag instead of
// the py3-none-any default, and (c) the bundled binary lives at
// mdsmith/_bin/.
func TestBuildWheelsLayout(t *testing.T) {
	if pythonExe() == "" {
		t.Skip("python is required to exercise BuildWheels")
	}
	if !haveModule(t, "build") || !haveModule(t, "wheel") || !haveModule(t, "hatchling") {
		t.Skip("python -m build, python -m wheel, and hatchling are required")
	}

	const ver = "7.8.9"
	root := t.TempDir()
	fixtureManifests(t, root)
	stagePython(t, root)
	if err := Stamp(root, ver); err != nil {
		t.Fatalf("Stamp: %v", err)
	}
	artifacts := filepath.Join(root, "artifacts")
	fakeArtifacts(t, artifacts)
	out := filepath.Join(root, "wheels")

	if err := BuildWheels(root, artifacts, out); err != nil {
		t.Fatalf("BuildWheels: %v", err)
	}

	cases := wheelCases()
	entries, err := os.ReadDir(out)
	if err != nil {
		t.Fatalf("readdir %s: %v", out, err)
	}
	if len(entries) != len(cases) {
		names := []string{}
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("expected %d wheels, got %d: %v", len(cases), len(entries), names)
	}
	for _, c := range cases {
		assertWheel(t, out, entries, c)
	}
}
