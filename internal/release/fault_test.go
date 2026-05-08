package release

import (
	"errors"
	"fmt"
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
	// MkdirAll call order in BuildWheels:
	//   1. outDir
	//   2. copyDir's dst-tree mkdir (inside stagePythonTree)
	//   3. stagePythonTree's mdsmith/_bin
	//   4. buildOneWheel's outDir/.staging-<plat>
	// We want to trip the staging mkdir specifically.
	root := t.TempDir()
	fixtureManifests(t, root)
	artifacts := filepath.Join(root, "artifacts")
	fakeArtifacts(t, artifacts)
	ff := newFakeFS()
	ff.failOnMkdirAllCall = 4

	err := NewWithFS(ff).BuildWheels(root, artifacts, filepath.Join(root, "wheels"))
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
	assert.Contains(t, err.Error(), "mkdir staging")
}

func TestBuildWheelsFailsOnStagingWipe(t *testing.T) {
	// RemoveAll call order in BuildWheels (one buildOneWheel
	// iteration):
	//   1. buildOneWheel's wipe of outDir/.staging-<plat>
	// (subsequent RemoveAll calls only fire on defer at function
	// end; the first iteration's wipe is call #1.)
	root := t.TempDir()
	fixtureManifests(t, root)
	artifacts := filepath.Join(root, "artifacts")
	fakeArtifacts(t, artifacts)
	ff := newFakeFS()
	ff.failOnRemoveAllCall = 1

	err := NewWithFS(ff).BuildWheels(root, artifacts, filepath.Join(root, "wheels"))
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
	assert.Contains(t, err.Error(), "wipe staging")
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

func TestCopyDirRecursiveFailureBubblesUp(t *testing.T) {
	// Stage src with a top-level file AND a subdirectory file;
	// fail on the SECOND ReadFile so the recursive copyDir(sp,
	// dp) call returns an error from inside the sub. Covers the
	// outer "if err := t.copyDir(sp, dp); err != nil { return err }"
	// branch.
	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "a.txt"), []byte("x"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(src, "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(src, "sub", "b.txt"), []byte("y"), 0o644))
	ff := newFakeFS()
	// ReadFile #1 = a.txt (succeeds), #2 = sub/b.txt (fails).
	ff.failOnReadFileCall = 2

	err := NewWithFS(ff).copyDir(src, filepath.Join(t.TempDir(), "dst"))
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
}

func TestMoveWheelsListWheelsErrorPropagates(t *testing.T) {
	// moveWheels delegates to listWheels first; a ReadDir fault
	// must surface as a moveWheels error, not as a no-op.
	ff := newFakeFS()
	ff.failOnReadDirCall = 1
	err := NewWithFS(ff).moveWheels(t.TempDir(), t.TempDir())
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

func TestCopyDirFailsOnDstMkdir(t *testing.T) {
	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "a.txt"), []byte("x"), 0o644))
	ff := newFakeFS()
	ff.failOnMkdirAllCall = 1
	err := NewWithFS(ff).copyDir(src, filepath.Join(t.TempDir(), "dst"))
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
}

func TestCheckManifestWithoutVersionField(t *testing.T) {
	// A manifest that exists but lacks a top-level "version"
	// field should fall through to the "no version field"
	// branch in checkManifest; without this test that branch
	// is unreachable from the OS-only path.
	root := t.TempDir()
	fixtureManifests(t, root)
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "editors/vscode/package.json"),
		[]byte(`{"name":"x"}`), 0o644))

	err := Check(root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no version field found")
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

// TestBuildOneWheelPropagatesStageFailure covers buildOneWheel's
// "if err != nil { return err }" arm right after stagePythonTree.
// Failing MkdirTemp inside stagePythonTree is the cleanest path
// since the asset Stat call has already succeeded.
func TestBuildOneWheelPropagatesStageFailure(t *testing.T) {
	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "pyproject.toml"),
		[]byte("[project]\nname=\"x\"\n"), 0o644))
	asset := filepath.Join(t.TempDir(), "asset")
	require.NoError(t, os.WriteFile(asset, []byte("bin"), 0o755))
	ff := newFakeFS()
	ff.failOnMkdirTempCall = 1

	wb := wheelBuilds[0]
	err := NewWithFS(ff).buildOneWheel(src, filepath.Dir(asset), t.TempDir(), wheelBuild{
		Asset: filepath.Base(asset), PlatTag: wb.PlatTag, Exe: wb.Exe,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
}

// errInfoEntry is a DirEntry whose Info() returns errInjected.
// Used to cover copyDir's `info, err := e.Info(); if err != nil`
// branch — a race-condition error that real filesystems do not
// reliably expose (the entry would have to vanish between
// ReadDir and Info).
type errInfoEntry struct{ name string }

func (e errInfoEntry) Name() string               { return e.name }
func (e errInfoEntry) IsDir() bool                { return false }
func (e errInfoEntry) Type() fs.FileMode          { return 0 }
func (e errInfoEntry) Info() (fs.FileInfo, error) { return nil, errInjected }

// entryInfoErrFS overrides only ReadDir to return a single
// errInfoEntry; every other operation delegates to the embedded
// FS so the rest of copyDir can run normally up to e.Info().
type entryInfoErrFS struct{ FS }

func (entryInfoErrFS) ReadDir(_ string) ([]os.DirEntry, error) {
	return []os.DirEntry{errInfoEntry{name: "ghost.txt"}}, nil
}

func TestCopyDirFailsOnEntryInfoError(t *testing.T) {
	err := NewWithFS(entryInfoErrFS{osFS{}}).
		copyDir(t.TempDir(), filepath.Join(t.TempDir(), "dst"))
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
}

// recordingRunner captures the arguments of every RunCommand
// call so a test can assert that downstream callers (e.g.
// `python -m build --outdir <…>`) are invoked with absolute
// paths. python interprets `--outdir` relative to its own
// cwd, which we set to a staged temp tree; if outDir is
// relative, the wheel lands somewhere we never look.
type recordingRunner struct {
	calls []recordedCall
}

type recordedCall struct {
	dir  string
	name string
	args []string
}

func (r *recordingRunner) RunCommand(dir, name string, args ...string) error {
	r.calls = append(r.calls, recordedCall{dir: dir, name: name, args: append([]string{}, args...)})
	return nil
}

// TestBuildWheelsPassesAbsoluteOutdirToPython is a regression
// for the "python/dist is empty after build" silent-failure.
// `python -m build --outdir <…>` with cmd.Dir pointing at a
// staged temp tree must receive an absolute --outdir;
// otherwise python writes the wheel under <stage>/<relative>/
// while the Go side reads from <repo-cwd>/<relative>/, finds
// nothing, and the workflow skips happily to publish.
func TestBuildWheelsPassesAbsoluteOutdirToPython(t *testing.T) {
	root := t.TempDir()
	fixtureManifests(t, root)
	artifacts := filepath.Join(root, "artifacts")
	fakeArtifacts(t, artifacts)
	rec := &recordingRunner{}

	// Run from a working directory where the relative outDir
	// resolves predictably; chdir into a temp parent so the
	// resulting absolute path is observable.
	wd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(wd) })
	require.NoError(t, os.Chdir(root))

	// Pass a relative outDir; the Toolkit must resolve it to
	// absolute before invoking python.
	err = NewWithDeps(osFS{}, rec).BuildWheels(root, artifacts, "rel-out")
	// Build always errors since the recordingRunner doesn't
	// actually create wheels — the empty-wheel guard fires on
	// the first iteration. The recorded calls before that are
	// what we verify.
	require.Error(t, err)

	require.NotEmpty(t, rec.calls, "no python invocations recorded")
	// First recorded call should be `python -m build --wheel
	// --outdir <abs>`. Find the --outdir arg and confirm it's
	// absolute.
	first := rec.calls[0]
	idx := -1
	for i, a := range first.args {
		if a == "--outdir" {
			idx = i
			break
		}
	}
	require.NotEqual(t, -1, idx, "no --outdir flag in first invocation: %v", first.args)
	require.Greater(t, len(first.args), idx+1, "--outdir has no value: %v", first.args)
	outdir := first.args[idx+1]
	assert.True(t, filepath.IsAbs(outdir),
		"python -m build received relative --outdir %q; "+
			"wheel would land under cmd.Dir not requested outDir",
		outdir)
}

// TestBuildOneWheelFailsWhenPythonProducesNoWheel pins the
// post-runPythonBuild guard: a Runner that exits 0 without
// writing any .whl into staging must still fail buildOneWheel,
// not silently move on. Earlier behaviour let an empty staging
// dir flow all the way to PyPI publish-time, where the
// pypi-publish action fails with "no distribution packages".
func TestBuildOneWheelFailsWhenPythonProducesNoWheel(t *testing.T) {
	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "pyproject.toml"),
		[]byte("[project]\nname=\"x\"\n"), 0o644))
	asset := filepath.Join(t.TempDir(), "asset")
	require.NoError(t, os.WriteFile(asset, []byte("bin"), 0o755))
	out := t.TempDir()
	wb := wheelBuilds[0]

	// Default fakeRunner exits 0 on every call without writing
	// anything; perfect for the "build appeared to succeed but
	// produced nothing" scenario.
	tk := NewWithDeps(osFS{}, &fakeRunner{})
	err := tk.buildOneWheel(src, filepath.Dir(asset), out, wheelBuild{
		Asset: filepath.Base(asset), PlatTag: wb.PlatTag, Exe: wb.Exe,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "produced no wheel")
}

// TestBuildOneWheelWipesStaleStaging is a regression for stale
// `.staging-<plat>/` left over by a killed previous run. Without
// the pre-build wipe, listWheels would see the stale wheel,
// the empty-wheel guard wouldn't fire, and retagWheels +
// moveWheels would relabel and ship the stale artifact.
func TestBuildOneWheelWipesStaleStaging(t *testing.T) {
	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "pyproject.toml"),
		[]byte("[project]\nname=\"x\"\n"), 0o644))
	asset := filepath.Join(t.TempDir(), "asset")
	require.NoError(t, os.WriteFile(asset, []byte("bin"), 0o755))
	out := t.TempDir()
	wb := wheelBuilds[0]

	// Plant a stale wheel where buildOneWheel will create its
	// staging dir. If the wipe is missing, listWheels finds it
	// and the empty-wheel guard would NOT fire — buildOneWheel
	// would happily move on, retag the stale wheel, and ship it.
	staleStaging := filepath.Join(out, ".staging-"+wb.PlatTag)
	require.NoError(t, os.MkdirAll(staleStaging, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(staleStaging, "stale.whl"),
		[]byte("stale"), 0o644))

	// Default fakeRunner exits 0 without writing anything new.
	tk := NewWithDeps(osFS{}, &fakeRunner{})
	err := tk.buildOneWheel(src, filepath.Dir(asset), out, wheelBuild{
		Asset: filepath.Base(asset), PlatTag: wb.PlatTag, Exe: wb.Exe,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "produced no wheel",
		"stale wheel must not satisfy the empty-wheel guard")
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

// wheelStagingRunner is a fakeRunner that, on the first
// `python -m build --outdir <dir>` invocation, drops a fake
// `*.whl` into <dir> — mimicking what the real interpreter
// produces. Lets tests reach retagWheels and moveWheels without
// pre-staging a wheel (which would now be wiped by the
// build-time RemoveAll).
type wheelStagingRunner struct {
	fakeRunner
}

func (r *wheelStagingRunner) RunCommand(dir, name string, args ...string) error {
	r.calls++
	// First call (python -m build): drop a fake.whl under
	// --outdir so listWheels finds it on the next pass.
	if r.calls == 1 {
		for i, a := range args {
			if a == "--outdir" && i+1 < len(args) {
				if err := os.WriteFile(filepath.Join(args[i+1], "fake.whl"),
					[]byte("x"), 0o644); err != nil {
					return fmt.Errorf("wheelStagingRunner: stage fake wheel: %w", err)
				}
				break
			}
		}
	}
	if r.failOnCall != 0 && r.calls == r.failOnCall {
		return errInjected
	}
	return nil
}

// stageBuildOneWheelInputs sets up the (src, artifactsDir,
// outDir, wheelBuild) inputs buildOneWheel expects without
// pre-staging the staging dir — that's the runner's job now.
func stageBuildOneWheelInputs(t *testing.T) (string, string, string, wheelBuild) {
	t.Helper()
	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "pyproject.toml"),
		[]byte("[project]\nname=\"x\"\n"), 0o644))
	asset := filepath.Join(t.TempDir(), "asset")
	require.NoError(t, os.WriteFile(asset, []byte("bin"), 0o755))
	out := t.TempDir()
	wb := wheelBuilds[0]
	return src, filepath.Dir(asset), out, wheelBuild{
		Asset: filepath.Base(asset), PlatTag: wb.PlatTag, Exe: wb.Exe,
	}
}

func TestBuildOneWheelPropagatesRetagFailure(t *testing.T) {
	// runPythonBuild succeeds AND drops a wheel (call 1);
	// retagWheels finds it and invokes the runner for `wheel
	// tags`, which we fail (call 2).
	src, artifacts, out, wb := stageBuildOneWheelInputs(t)
	tk := NewWithDeps(osFS{}, &wheelStagingRunner{
		fakeRunner: fakeRunner{failOnCall: 2},
	})
	err := tk.buildOneWheel(src, artifacts, out, wb)
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
}

func TestBuildOneWheelPropagatesMoveFailure(t *testing.T) {
	// runner succeeds for both python invocations and drops a
	// wheel during the first call; FS.Rename fails when
	// moveWheels tries to move the now-real wheel, covering the
	// buildOneWheel branch that returns moveWheels' error.
	src, artifacts, out, wb := stageBuildOneWheelInputs(t)
	ff := newFakeFS()
	ff.failOnRenameCall = 1
	tk := NewWithDeps(ff, &wheelStagingRunner{})
	err := tk.buildOneWheel(src, artifacts, out, wb)
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
}

// TestBuildOneWheelPropagatesListWheelsFailure covers the
// listWheels error branch that sits between runPythonBuild and
// the empty-wheel guard. A ReadDir flake on the staging dir
// must surface as an error, not be misread as "no wheels
// produced".
func TestBuildOneWheelPropagatesListWheelsFailure(t *testing.T) {
	src, artifacts, out, wb := stageBuildOneWheelInputs(t)
	ff := newFakeFS()
	// ReadDir #1: stagePythonTree's copyDir(src) (src holds
	// only pyproject.toml so copyDir doesn't recurse).
	// ReadDir #2: listWheels(staging) after runPythonBuild —
	// the branch under test.
	ff.failOnReadDirCall = 2
	tk := NewWithDeps(ff, &fakeRunner{})
	err := tk.buildOneWheel(src, artifacts, out, wb)
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
}

// TestBuildWheelsFailsOnOutDirAbs covers the absPath(outDir)
// error branch in BuildWheels. filepath.Abs only fails when
// os.Getwd does, which is unreachable from a test process —
// the package-level absPath seam lets us drive the branch
// without depending on a deleted-cwd hack.
func TestBuildWheelsFailsOnOutDirAbs(t *testing.T) {
	orig := absPath
	t.Cleanup(func() { absPath = orig })
	absPath = func(string) (string, error) { return "", errInjected }

	err := New().BuildWheels(t.TempDir(), t.TempDir(), "dist")
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
	assert.Contains(t, err.Error(), "resolve outDir")
}

// TestBuildWheelsFailsOnArtifactsDirAbs covers the
// absPath(artifactsDir) error branch. We let the first
// absPath call (outDir) succeed and fail only the second.
func TestBuildWheelsFailsOnArtifactsDirAbs(t *testing.T) {
	orig := absPath
	t.Cleanup(func() { absPath = orig })
	calls := 0
	absPath = func(p string) (string, error) {
		calls++
		if calls == 2 {
			return "", errInjected
		}
		return orig(p)
	}

	err := New().BuildWheels(t.TempDir(), t.TempDir(), t.TempDir())
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
	assert.Contains(t, err.Error(), "resolve artifactsDir")
}
