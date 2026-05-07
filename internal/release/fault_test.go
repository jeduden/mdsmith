package release

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeFS wraps osFS and lets a test arm faults on each FS method.
// Each fault is a (callIndex, error) plan — the wrapper counts
// successful + failed calls for each method, and on the Nth call
// (1-indexed via failOn{Method}Call) returns the configured
// error. Other calls passthrough to osFS so tests can stage real
// files and let the toolkit operate on them up to the fault.
//
// We deliberately keep this tiny: per-method "fail on the Nth
// call" is enough to exercise every error-return branch (first
// MkdirAll vs second, ReadFile of manifest vs ReadFile of asset)
// without growing into a general-purpose mock-fs.
type fakeFS struct {
	inner FS

	// failOn{Method}Call: 0 = never fail, 1 = first call, 2 = second, ...
	failOnStatCall      int
	failOnReadFileCall  int
	failOnWriteFileCall int
	failOnReadDirCall   int
	failOnMkdirAllCall  int
	failOnMkdirTempCall int
	failOnRenameCall    int
	failOnRemoveAllCall int

	err error // injected error returned by every armed fault

	statCalls, readFileCalls, writeFileCalls, readDirCalls     int
	mkdirAllCalls, mkdirTempCalls, renameCalls, removeAllCalls int
}

func newFakeFS() *fakeFS { return &fakeFS{inner: osFS{}, err: errInjected} }

func (f *fakeFS) trip(callIdx int, target int) bool { return target != 0 && callIdx == target }

func (f *fakeFS) Stat(name string) (os.FileInfo, error) {
	f.statCalls++
	if f.trip(f.statCalls, f.failOnStatCall) {
		return nil, f.err
	}
	return f.inner.Stat(name)
}
func (f *fakeFS) ReadFile(name string) ([]byte, error) {
	f.readFileCalls++
	if f.trip(f.readFileCalls, f.failOnReadFileCall) {
		return nil, f.err
	}
	return f.inner.ReadFile(name)
}
func (f *fakeFS) WriteFile(name string, data []byte, perm fs.FileMode) error {
	f.writeFileCalls++
	if f.trip(f.writeFileCalls, f.failOnWriteFileCall) {
		return f.err
	}
	return f.inner.WriteFile(name, data, perm)
}
func (f *fakeFS) ReadDir(name string) ([]os.DirEntry, error) {
	f.readDirCalls++
	if f.trip(f.readDirCalls, f.failOnReadDirCall) {
		return nil, f.err
	}
	return f.inner.ReadDir(name)
}
func (f *fakeFS) MkdirAll(path string, perm fs.FileMode) error {
	f.mkdirAllCalls++
	if f.trip(f.mkdirAllCalls, f.failOnMkdirAllCall) {
		return f.err
	}
	return f.inner.MkdirAll(path, perm)
}
func (f *fakeFS) MkdirTemp(dir, pattern string) (string, error) {
	f.mkdirTempCalls++
	if f.trip(f.mkdirTempCalls, f.failOnMkdirTempCall) {
		return "", f.err
	}
	return f.inner.MkdirTemp(dir, pattern)
}
func (f *fakeFS) Rename(oldpath, newpath string) error {
	f.renameCalls++
	if f.trip(f.renameCalls, f.failOnRenameCall) {
		return f.err
	}
	return f.inner.Rename(oldpath, newpath)
}
func (f *fakeFS) RemoveAll(path string) error {
	f.removeAllCalls++
	if f.trip(f.removeAllCalls, f.failOnRemoveAllCall) {
		return f.err
	}
	return f.inner.RemoveAll(path)
}

var errInjected = errors.New("injected fault")

// Stamp / Check error-path coverage.

func TestStampPropagatesReadFileError(t *testing.T) {
	root := t.TempDir()
	fixtureManifests(t, root)
	ff := newFakeFS()
	ff.failOnReadFileCall = 1

	err := NewWithFS(ff).Stamp(root, "1.2.3")
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
}

func TestStampPropagatesWriteFileError(t *testing.T) {
	root := t.TempDir()
	fixtureManifests(t, root)
	ff := newFakeFS()
	ff.failOnWriteFileCall = 1

	err := NewWithFS(ff).Stamp(root, "1.2.3")
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
}

func TestCheckReportsReadError(t *testing.T) {
	root := t.TempDir()
	fixtureManifests(t, root)
	ff := newFakeFS()
	ff.failOnReadFileCall = 1

	err := NewWithFS(ff).Check(root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read ")
	assert.Contains(t, err.Error(), "injected fault")
}

// BuildNpmPlatforms error-path coverage.

func TestBuildNpmPlatformsFailsOnOuterMkdir(t *testing.T) {
	root := t.TempDir()
	fixtureManifests(t, root)
	require.NoError(t, Stamp(root, "1.2.3"))
	artifacts := filepath.Join(root, "artifacts")
	fakeArtifacts(t, artifacts)
	// readJSONVersion does the ReadFile, then BuildNpmPlatforms's
	// own MkdirAll fires (call #1).
	ff := newFakeFS()
	ff.failOnMkdirAllCall = 1

	err := NewWithFS(ff).BuildNpmPlatforms(root, artifacts, filepath.Join(root, "dist"))
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
}

func TestBuildNpmPlatformsFailsOnPlatformMkdir(t *testing.T) {
	// The first MkdirAll in BuildNpmPlatforms is outDir; the
	// second is the per-platform binDir inside buildOneNpmPlatform.
	root := t.TempDir()
	fixtureManifests(t, root)
	require.NoError(t, Stamp(root, "1.2.3"))
	artifacts := filepath.Join(root, "artifacts")
	fakeArtifacts(t, artifacts)
	ff := newFakeFS()
	ff.failOnMkdirAllCall = 2

	err := NewWithFS(ff).BuildNpmPlatforms(root, artifacts, filepath.Join(root, "dist"))
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
}

func TestBuildNpmPlatformsFailsOnCopyRead(t *testing.T) {
	// readJSONVersion's ReadFile is call #1; buildOneNpmPlatform
	// then calls copyFile which ReadFile's the asset (call #2).
	root := t.TempDir()
	fixtureManifests(t, root)
	require.NoError(t, Stamp(root, "1.2.3"))
	artifacts := filepath.Join(root, "artifacts")
	fakeArtifacts(t, artifacts)
	ff := newFakeFS()
	ff.failOnReadFileCall = 2

	err := NewWithFS(ff).BuildNpmPlatforms(root, artifacts, filepath.Join(root, "dist"))
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
}

func TestBuildNpmPlatformsFailsOnCopyWrite(t *testing.T) {
	// copyFile = ReadFile + WriteFile. The asset Stat call lets
	// buildOneNpmPlatform get past the missing-asset check; the
	// first FS write is copyFile's WriteFile, which fails.
	root := t.TempDir()
	fixtureManifests(t, root)
	require.NoError(t, Stamp(root, "1.2.3"))
	artifacts := filepath.Join(root, "artifacts")
	fakeArtifacts(t, artifacts)
	ff := newFakeFS()
	ff.failOnWriteFileCall = 1

	err := NewWithFS(ff).BuildNpmPlatforms(root, artifacts, filepath.Join(root, "dist"))
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
}

func TestBuildNpmPlatformsFailsOnManifestWrite(t *testing.T) {
	// Per-platform WriteFile order: copyFile's binary copy (1),
	// the package.json manifest itself (2). Without LICENSE
	// staged, no third write fires.
	root := t.TempDir()
	fixtureManifests(t, root)
	require.NoError(t, Stamp(root, "1.2.3"))
	artifacts := filepath.Join(root, "artifacts")
	fakeArtifacts(t, artifacts)
	ff := newFakeFS()
	ff.failOnWriteFileCall = 2

	err := NewWithFS(ff).BuildNpmPlatforms(root, artifacts, filepath.Join(root, "dist"))
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
}

func TestBuildNpmPlatformsFailsOnLicenseWrite(t *testing.T) {
	// Stage a LICENSE so buildOneNpmPlatform takes the license-
	// copy branch. Calls in order per platform (5 platforms
	// total): copyFile-ReadFile asset, copyFile-WriteFile bin,
	// WriteFile package.json, ReadFile LICENSE, WriteFile
	// LICENSE. The 3rd WriteFile is the LICENSE write.
	root := t.TempDir()
	fixtureManifests(t, root)
	require.NoError(t, os.WriteFile(filepath.Join(root, "LICENSE"), []byte("MIT"), 0o644))
	require.NoError(t, Stamp(root, "1.2.3"))
	artifacts := filepath.Join(root, "artifacts")
	fakeArtifacts(t, artifacts)
	ff := newFakeFS()
	ff.failOnWriteFileCall = 3

	err := NewWithFS(ff).BuildNpmPlatforms(root, artifacts, filepath.Join(root, "dist"))
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
}

// BuildWheels orchestration error-path coverage.

func TestBuildWheelsFailsOnOutDirMkdir(t *testing.T) {
	root := t.TempDir()
	fixtureManifests(t, root)
	artifacts := filepath.Join(root, "artifacts")
	fakeArtifacts(t, artifacts)
	ff := newFakeFS()
	ff.failOnMkdirAllCall = 1

	err := NewWithFS(ff).BuildWheels(root, artifacts, filepath.Join(root, "wheels"))
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
}

func TestBuildWheelsFailsOnStagingMkdir(t *testing.T) {
	// outDir's MkdirAll is call #1; stagePythonTree's binDir
	// MkdirAll fires inside the loop. The .staging-<plat>
	// MkdirAll is call #3.
	root := t.TempDir()
	fixtureManifests(t, root)
	artifacts := filepath.Join(root, "artifacts")
	fakeArtifacts(t, artifacts)
	ff := newFakeFS()
	ff.failOnMkdirAllCall = 3

	err := NewWithFS(ff).BuildWheels(root, artifacts, filepath.Join(root, "wheels"))
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
}

func TestStagePythonTreeFailsOnMkdirTemp(t *testing.T) {
	ff := newFakeFS()
	ff.failOnMkdirTempCall = 1
	_, err := NewWithFS(ff).stagePythonTree(t.TempDir(),
		filepath.Join(t.TempDir(), "asset"), "mdsmith")
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
}

func TestStagePythonTreeFailsOnCopyDir(t *testing.T) {
	// Source dir is missing; stagePythonTree's copyDir fails
	// after MkdirTemp succeeds. Tests the cleanup-via-RemoveAll
	// branch.
	ff := newFakeFS()
	_, err := NewWithFS(ff).stagePythonTree(filepath.Join(t.TempDir(), "missing-src"),
		filepath.Join(t.TempDir(), "asset"), "mdsmith")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "copy python tree")
}

func TestStagePythonTreeFailsOnBinDirMkdir(t *testing.T) {
	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "pyproject.toml"),
		[]byte("[project]\nname=\"x\"\n"), 0o644))
	ff := newFakeFS()
	// MkdirAll calls: copyDir's mkdir for the staged tree (1),
	// stagePythonTree's mkdir for mdsmith/_bin (2).
	ff.failOnMkdirAllCall = 2
	_, err := NewWithFS(ff).stagePythonTree(src,
		filepath.Join(t.TempDir(), "asset"), "mdsmith")
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
}

func TestMoveWheelsFailsOnRename(t *testing.T) {
	staging := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(staging, "a.whl"), []byte("x"), 0o644))
	ff := newFakeFS()
	ff.failOnRenameCall = 1

	err := NewWithFS(ff).moveWheels(staging, t.TempDir())
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
}

func TestCopyDirFailsOnInnerWrite(t *testing.T) {
	// Stage a real source tree with one file; the WriteFile
	// inside copyFile is the only chance to fail (everything
	// else succeeds via osFS), so an injected WriteFile error
	// reliably surfaces from copyDir.
	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "a.txt"), []byte("x"), 0o644))
	ff := newFakeFS()
	ff.failOnWriteFileCall = 1

	err := NewWithFS(ff).copyDir(src, filepath.Join(t.TempDir(), "dst"))
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
}

func TestListWheelsErrorOnReadDirFault(t *testing.T) {
	ff := newFakeFS()
	ff.failOnReadDirCall = 1
	_, err := NewWithFS(ff).listWheels(t.TempDir())
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
}

func TestRetagWheelsErrorOnListFail(t *testing.T) {
	// retagWheels delegates to listWheels first, so a ReadDir
	// fault surfaces before any python invocation.
	ff := newFakeFS()
	ff.failOnReadDirCall = 1
	err := NewWithFS(ff).retagWheels(t.TempDir(), "win_amd64")
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
}

func TestReadJSONVersionPropagatesReadError(t *testing.T) {
	ff := newFakeFS()
	ff.failOnReadFileCall = 1
	_, err := NewWithFS(ff).readJSONVersion("any-path")
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
}

func TestReadJSONVersionRejectsManifestWithoutVersion(t *testing.T) {
	// A manifest that exists but lacks a top-level "version"
	// field should surface a clear error rather than returning
	// the empty string.
	dir := t.TempDir()
	manifest := filepath.Join(dir, "package.json")
	require.NoError(t, os.WriteFile(manifest, []byte(`{"name":"x"}`), 0o644))

	_, err := New().readJSONVersion(manifest)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no version field")
}

func TestStagePythonTreeFailsOnCopyAsset(t *testing.T) {
	// The asset's copyFile uses a ReadFile (the asset) followed
	// by a WriteFile (under mdsmith/_bin/). copyDir for the
	// staged tree consumes ReadFile #1 (and writes once, since
	// the src has only pyproject.toml). Asset ReadFile is the
	// 2nd ReadFile call.
	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "pyproject.toml"),
		[]byte("[project]\nname=\"x\"\n"), 0o644))
	asset := filepath.Join(t.TempDir(), "asset")
	require.NoError(t, os.WriteFile(asset, []byte("bin"), 0o755))
	ff := newFakeFS()
	ff.failOnReadFileCall = 2

	stage, err := NewWithFS(ff).stagePythonTree(src, asset, "mdsmith")
	if err == nil {
		_ = os.RemoveAll(stage)
	}
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
}

// readErrFS overrides only ReadFile to return a non-NotExist
// error. checkManifest treats fs.ErrNotExist specially; this
// covers the generic-error branch that note()'s with "read
// <path>: <err>" and returns.
type readErrFS struct{ FS }

func (r readErrFS) ReadFile(_ string) ([]byte, error) { return nil, errInjected }

func TestCheckManifestNonNotExistReadError(t *testing.T) {
	root := t.TempDir()
	fixtureManifests(t, root)

	err := NewWithFS(readErrFS{osFS{}}).Check(root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read ")
	assert.Contains(t, err.Error(), "injected fault")
}

func TestCopyFileSourceMissing(t *testing.T) {
	// readFileErr without a real source: the inner osFS bubbles
	// the os.ErrNotExist; we just need to assert copyFile
	// surfaces it instead of swallowing.
	err := New().copyFile(filepath.Join(t.TempDir(), "missing"),
		filepath.Join(t.TempDir(), "dst"), 0o644)
	require.Error(t, err)
}

// fakeRunner is a Runner that returns errInjected on the Nth
// call (1-indexed). Lets tests cover runPythonBuild and
// retagWheels failure branches without requiring python on PATH.
type fakeRunner struct {
	failOnCall int
	calls      int
}

func (r *fakeRunner) RunCommand(_, _ string, _ ...string) error {
	r.calls++
	if r.failOnCall != 0 && r.calls == r.failOnCall {
		return errInjected
	}
	return nil
}

func TestRunPythonBuildPropagatesFailure(t *testing.T) {
	tk := NewWithDeps(osFS{}, &fakeRunner{failOnCall: 1})
	err := tk.runPythonBuild(t.TempDir(), t.TempDir(), "win_amd64")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "python -m build")
	assert.ErrorIs(t, err, errInjected)
}

func TestRetagWheelsPropagatesFailure(t *testing.T) {
	staging := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(staging, "a.whl"), []byte("x"), 0o644))
	tk := NewWithDeps(osFS{}, &fakeRunner{failOnCall: 1})
	err := tk.retagWheels(staging, "win_amd64")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "python -m wheel tags")
	assert.ErrorIs(t, err, errInjected)
}

func TestBuildOneWheelPropagatesPythonFailure(t *testing.T) {
	// Stage a real source tree so stagePythonTree succeeds, then
	// fail on the first runner call (python -m build).
	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "pyproject.toml"),
		[]byte("[project]\nname=\"x\"\n"), 0o644))
	asset := filepath.Join(t.TempDir(), "asset")
	require.NoError(t, os.WriteFile(asset, []byte("bin"), 0o755))
	out := t.TempDir()
	wb := wheelBuilds[0]
	tk := NewWithDeps(osFS{}, &fakeRunner{failOnCall: 1})
	err := tk.buildOneWheel(src, filepath.Dir(asset), out, wheelBuild{
		Asset: filepath.Base(asset), PlatTag: wb.PlatTag, Exe: wb.Exe,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
}

// preStageWheel sets up a real source tree, asset, and an
// outDir/.staging-<plat>/fake.whl so buildOneWheel can reach
// retagWheels and moveWheels without invoking real python.
// Returns (src, artifactsDir, outDir, wheelBuild).
func preStageWheel(t *testing.T) (string, string, string, wheelBuild) {
	t.Helper()
	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "pyproject.toml"),
		[]byte("[project]\nname=\"x\"\n"), 0o644))
	asset := filepath.Join(t.TempDir(), "asset")
	require.NoError(t, os.WriteFile(asset, []byte("bin"), 0o755))
	out := t.TempDir()
	wb := wheelBuilds[0]
	staging := filepath.Join(out, ".staging-"+wb.PlatTag)
	require.NoError(t, os.MkdirAll(staging, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(staging, "fake.whl"),
		[]byte("x"), 0o644))
	return src, filepath.Dir(asset), out, wheelBuild{
		Asset: filepath.Base(asset), PlatTag: wb.PlatTag, Exe: wb.Exe,
	}
}

func TestBuildOneWheelPropagatesRetagFailure(t *testing.T) {
	// runPythonBuild succeeds (call 1); retagWheels finds the
	// pre-staged wheel and invokes the runner for `wheel tags`,
	// which we fail (call 2).
	src, artifacts, out, wb := preStageWheel(t)
	tk := NewWithDeps(osFS{}, &fakeRunner{failOnCall: 2})
	err := tk.buildOneWheel(src, artifacts, out, wb)
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
}

func TestBuildOneWheelPropagatesMoveFailure(t *testing.T) {
	// runner succeeds for both python invocations; FS.Rename
	// fails when moveWheels tries to move the pre-staged wheel,
	// covering the buildOneWheel branch that returns moveWheels'
	// error.
	src, artifacts, out, wb := preStageWheel(t)
	ff := newFakeFS()
	ff.failOnRenameCall = 1
	tk := NewWithDeps(ff, &fakeRunner{})
	err := tk.buildOneWheel(src, artifacts, out, wb)
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
}
