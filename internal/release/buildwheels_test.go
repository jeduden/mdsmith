package release

import (
	"archive/zip"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NoError(t, err, "open %s", whlPath)
	defer func() { _ = r.Close() }()
	for _, f := range r.File {
		if strings.HasSuffix(f.Name, member) {
			rc, err := f.Open()
			require.NoError(t, err, "open zip member %s", f.Name)
			body, err := io.ReadAll(rc)
			_ = rc.Close()
			require.NoError(t, err, "read zip member %s", f.Name)
			return string(body)
		}
	}
	return ""
}

func zipHasFile(t *testing.T, whlPath, name string) bool {
	t.Helper()
	r, err := zip.OpenReader(whlPath)
	require.NoError(t, err, "open %s", whlPath)
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
	require.NoError(t, err)
	repo := filepath.Clean(filepath.Join(wd, "..", ".."))

	for _, p := range []string{
		"python/pyproject.toml",
		"python/README.md",
		"python/mdsmith/__init__.py",
		"python/mdsmith/__main__.py",
	} {
		body, err := os.ReadFile(filepath.Join(repo, p))
		require.NoError(t, err, "read %s", p)
		dst := filepath.Join(root, p)
		require.NoError(t, os.MkdirAll(filepath.Dir(dst), 0o755))
		require.NoError(t, os.WriteFile(dst, body, 0o644))
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
		assert.Failf(t, "no wheel matched filename containing %q", "got %v", c.uniqueFilenameSubstr, names)
		return
	}
	whl := filepath.Join(out, match)
	meta := readZipMember(t, whl, "/WHEEL")
	assert.Contains(t, meta, c.tagInWheelMetadata, "%s WHEEL metadata", whl)
	assert.NotContains(t, meta, "py3-none-any", "%s still claims py3-none-any", whl)
	assert.Truef(t, zipHasFile(t, whl, "mdsmith/_bin/"+c.binName),
		"%s: bundled binary mdsmith/_bin/%s missing", whl, c.binName)
}

// TestBuildWheelsFailsWhenPythonSourceMissing exercises the
// fast-fail path that runs before any python invocation, so the
// test does not need python on PATH.
func TestBuildWheelsFailsWhenPythonSourceMissing(t *testing.T) {
	root := t.TempDir()
	artifacts := filepath.Join(root, "artifacts")
	fakeArtifacts(t, artifacts)

	err := BuildWheels(root, artifacts, filepath.Join(root, "wheels"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "python source missing")
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
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing release asset")
}

// Helper-level tests so the staging/listing/moving primitives
// have direct coverage. Use New() to drive the OS-backed Toolkit;
// fault-injection coverage of error returns lives in
// fault_test.go behind a fake FS.

func TestListWheelsEmpty(t *testing.T) {
	wheels, err := New().listWheels(t.TempDir())
	require.NoError(t, err)
	assert.Empty(t, wheels)
}

func TestListWheelsFiltersNonWheels(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"foo.whl", "bar.tar.gz", "baz.txt"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644))
	}
	// A directory whose name happens to end in `.whl` must be
	// ignored so listWheels never returns directory entries to
	// `python -m wheel tags` / os.Rename.
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "subdir.whl"), 0o755))

	wheels, err := New().listWheels(dir)
	require.NoError(t, err)
	require.Len(t, wheels, 1)
	assert.Equal(t, "foo.whl", filepath.Base(wheels[0]))
}

func TestMoveWheelsEmpty(t *testing.T) {
	// moveWheels iterates listWheels output; an empty staging dir
	// must be a no-op, not an error.
	assert.NoError(t, New().moveWheels(t.TempDir(), t.TempDir()))
}

func TestMoveWheelsRelocates(t *testing.T) {
	staging := t.TempDir()
	out := t.TempDir()
	for _, name := range []string{"a.whl", "b.whl"} {
		require.NoError(t, os.WriteFile(filepath.Join(staging, name), []byte(name), 0o644))
	}
	require.NoError(t, New().moveWheels(staging, out))
	for _, name := range []string{"a.whl", "b.whl"} {
		_, err := os.Stat(filepath.Join(out, name))
		assert.NoError(t, err, "%s missing in out", name)
		_, err = os.Stat(filepath.Join(staging, name))
		assert.True(t, os.IsNotExist(err), "%s still in staging", name)
	}
}

func TestCopyDirCopiesNestedTree(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "dst")
	require.NoError(t, os.MkdirAll(filepath.Join(src, "sub", "deep"), 0o755))
	files := map[string]string{
		"a.txt":          "hello",
		"sub/b.txt":      "world",
		"sub/deep/c.txt": "deep",
	}
	for rel, body := range files {
		require.NoError(t, os.WriteFile(filepath.Join(src, rel), []byte(body), 0o644))
	}
	require.NoError(t, New().copyDir(src, dst))
	for rel, want := range files {
		got, err := os.ReadFile(filepath.Join(dst, rel))
		require.NoError(t, err, "%s", rel)
		assert.Equal(t, want, string(got), "%s content", rel)
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
	require.NoError(t, Stamp(root, ver))

	artifacts := filepath.Join(root, "artifacts")
	fakeArtifacts(t, artifacts)
	out := filepath.Join(root, "wheels")
	require.NoError(t, BuildWheels(root, artifacts, out))

	cases := wheelCases()
	entries, err := os.ReadDir(out)
	require.NoError(t, err)
	require.Len(t, entries, len(cases))
	for _, c := range cases {
		assertWheel(t, out, entries, c)
	}
}
