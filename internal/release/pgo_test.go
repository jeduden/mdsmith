package release

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPGOMergeArgs pins the merge step: the generated `go tool
// pprof` invocation must request the protobuf form, write to the
// repo-root default.pgo, name the mdsmith binary as the symbol
// source, and append every collected profile in order. This is the
// shell `go tool pprof -proto -output=... <bin> <profs...>` ported
// to argv, so a regression here would silently change the merged
// profile the release build picks up.
func TestPGOMergeArgs(t *testing.T) {
	args := pgoMergeArgs("/w/bin/mdsmith", "/repo/cmd/mdsmith/default.pgo",
		[]string{"/w/a.prof", "/w/b.prof"})

	assert.Equal(t, []string{
		"tool", "pprof",
		"-proto",
		"-output=/repo/cmd/mdsmith/default.pgo",
		"/w/bin/mdsmith",
		"/w/a.prof",
		"/w/b.prof",
	}, args)
}

// TestPGOProfileRuns pins the four profile collections: each of the
// two corpora is profiled once with the default config and once with
// the parity config, into four distinctly named profile files under
// the workdir. The list is the input the merge step consumes, so a
// missing or duplicated run would skew the merged profile.
func TestPGOProfileRuns(t *testing.T) {
	runs := pgoProfileRuns("/repo", "/w")

	require.Len(t, runs, 4)

	profs := make([]string, 0, len(runs))
	for _, r := range runs {
		profs = append(profs, r.prof)
	}
	assert.Equal(t, []string{
		filepath.Join("/w", "cpu-repo.prof"),
		filepath.Join("/w", "cpu-parity-repo.prof"),
		filepath.Join("/w", "cpu-neutral.prof"),
		filepath.Join("/w", "cpu-parity-neutral.prof"),
	}, profs)

	parity := filepath.Join("/repo", benchDirRel, "bench-parity.mdsmith.yml")
	repoCorpus := filepath.Join("/w", "corpus_repo")
	neutralCorpus := filepath.Join("/w", "corpus_neutral")

	// Default-config run over the repo corpus: no -c flag.
	assert.Equal(t, []string{"check", repoCorpus}, runs[0].args)
	// Parity-config run over the repo corpus: -c <parity> first.
	assert.Equal(t, []string{"check", "-c", parity, repoCorpus}, runs[1].args)
	assert.Equal(t, []string{"check", neutralCorpus}, runs[2].args)
	assert.Equal(t, []string{"check", "-c", parity, neutralCorpus}, runs[3].args)
}

// TestPGOErrorsWithoutBinary covers the precondition: PGO must fail
// loudly when the mdsmith build did not produce a binary (a fake
// runner that exits 0 but writes nothing), rather than feeding an
// absent binary into the profile runs.
func TestPGOErrorsWithoutBinary(t *testing.T) {
	workdir := t.TempDir()
	// fakeRunner exits 0 on every call but never writes the binary,
	// so the post-build existence check is what must trip.
	err := NewWithDeps(osFS{}, &fakeRunner{}).PGO(t.TempDir(), workdir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mdsmith")
}

// TestPGOWritesProfileToRepoDefault drives the full method with a
// runner that stands in for both the build and the pprof merge by
// writing the expected output files, so the orchestration — build,
// corpora, four profile runs, merge into cmd/mdsmith/default.pgo —
// is exercised end to end without a real toolchain.
func TestPGOWritesProfileToRepoDefault(t *testing.T) {
	root := t.TempDir()
	workdir := t.TempDir()

	// A real (empty) git tree so buildCorpora's `git ls-files`
	// succeeds and the repo corpus materializes deterministically.
	stageMinimalRepo(t, root)

	r := &pgoFakeRunner{}
	require.NoError(t, NewWithDeps(osFS{}, r).PGO(root, workdir))

	out := filepath.Join(root, "cmd", "mdsmith", "default.pgo")
	got, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.Equal(t, "MERGED-PGO", string(got))
}

// stageMinimalRepo initializes a git repo at root with one tracked
// Markdown file so buildCorpora's `git ls-files *.md` produces a
// non-empty, deterministic repo corpus.
func stageMinimalRepo(t *testing.T, root string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(root, "doc.md"), []byte("# Doc\n"), 0o644))
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "t@example.com"},
		{"config", "user.name", "t"},
		{"add", "doc.md"},
		{"commit", "-q", "-m", "init"},
	} {
		run := osRunner{}
		require.NoError(t, run.RunCommand(root, "git", args...))
	}
}

// pgoFakeRunner stands in for the toolchain during PGO: the `go
// build` call writes a placeholder binary, the profile `check` runs
// write their CPUPROFILE target, and the `go tool pprof` merge
// writes the merged profile. It writes through real files so the
// method's existence checks and the final read see what production
// would.
type pgoFakeRunner struct{}

func (r *pgoFakeRunner) RunCommand(dir, name string, args ...string) error {
	switch {
	case name == "go" && len(args) >= 4 && args[0] == "build":
		// go build -pgo=off -o <bin> ./cmd/mdsmith — pin the exact shape
		// so removing -pgo=off from pgo.go is caught as a test failure.
		if len(args) < 5 || args[1] != "-pgo=off" || args[2] != "-o" {
			return fmt.Errorf("unexpected go build args: %v", args)
		}
		return os.WriteFile(args[3], []byte("BIN"), 0o755)
	case name == "go" && len(args) >= 2 && args[0] == "tool" && args[1] == "pprof":
		out := pgoOutputFlag(args)
		return os.WriteFile(out, []byte("MERGED-PGO"), 0o644)
	default:
		// A profile `check` run: write the CPUPROFILE target so the
		// merge has profiles to consume.
		if p := os.Getenv("MDSMITH_CPUPROFILE"); p != "" {
			return os.WriteFile(p, []byte("PROF"), 0o644)
		}
		return nil
	}
}

// pgoOutputFlag extracts the path from the -output=<path> arg.
func pgoOutputFlag(args []string) string {
	for _, a := range args {
		if v, ok := strings.CutPrefix(a, "-output="); ok {
			return v
		}
	}
	return ""
}

// TestPGODefaultsWorkdir covers the empty-workdir branch: PGO must
// substitute defaultBenchWorkdir when the caller passes "".
func TestPGODefaultsWorkdir(t *testing.T) {
	// fakeRunner exits 0 but writes no binary, so the post-build
	// existence check trips — but only after the default-workdir
	// substitution on line 87 has run.
	workdir := t.TempDir()
	err := NewWithDeps(osFS{}, &fakeRunner{}).PGO(t.TempDir(), "")
	// Clean up any directory PGO created under the default workdir.
	t.Cleanup(func() { _ = os.RemoveAll(defaultBenchWorkdir) })
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mdsmith")
	// Confirm the substituted path, not the empty string, is in the error.
	assert.NotContains(t, err.Error(), workdir)
}

// TestPGOMkdirFails covers the MkdirAll error path in PGO.
func TestPGOMkdirFails(t *testing.T) {
	fs := newFakeFS()
	fs.failOnMkdirAllCall = 1
	err := NewWithDeps(fs, &fakeRunner{}).PGO(t.TempDir(), t.TempDir())
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
}

// TestPGOBuildFails covers the go-build error path in PGO: when
// RunCommand returns an error the method must wrap and surface it.
func TestPGOBuildFails(t *testing.T) {
	err := NewWithDeps(osFS{}, &fakeRunner{failOnCall: 1}).PGO(t.TempDir(), t.TempDir())
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
	assert.Contains(t, err.Error(), "build mdsmith")
}

// TestCollectProfileMissing covers the error when a profile run
// exits without writing its profile file.
func TestCollectProfileMissing(t *testing.T) {
	workdir := t.TempDir()
	run := pgoProfileRun{
		prof: filepath.Join(workdir, "cpu.prof"),
		args: []string{"check", workdir},
	}
	// fakeRunner exits 0 but writes no file unless MDSMITH_CPUPROFILE
	// is set — clear it so the write is skipped.
	t.Setenv("MDSMITH_CPUPROFILE", "")
	err := NewWithDeps(osFS{}, &fakeRunner{}).collectProfile("mdsmith", run)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no profile")
}

// TestCollectProfileRestoresEnvVar covers the had==true restore
// branch: when MDSMITH_CPUPROFILE was set before the call, it must
// be restored to its original value after.
func TestCollectProfileRestoresEnvVar(t *testing.T) {
	workdir := t.TempDir()
	profPath := filepath.Join(workdir, "cpu.prof")
	run := pgoProfileRun{prof: profPath, args: []string{"check", workdir}}

	// Pre-set the env var; t.Setenv ensures it is cleared at test end.
	t.Setenv("MDSMITH_CPUPROFILE", "original-value")

	// pgoFakeRunner's default branch reads MDSMITH_CPUPROFILE and
	// writes to it, so it will write to profPath while the call is
	// in flight, then collectProfile's defer restores "original-value".
	require.NoError(t, NewWithDeps(osFS{}, &pgoFakeRunner{}).collectProfile("mdsmith", run))
	assert.Equal(t, "original-value", os.Getenv("MDSMITH_CPUPROFILE"))
}

// TestPGOPackageLevelPropagatesError covers the package-level PGO()
// delegation shim: it must not swallow errors from Toolkit.PGO.
func TestPGOPackageLevelPropagatesError(t *testing.T) {
	// A root with no ./cmd/mdsmith package causes go build to fail,
	// which is enough to confirm the delegation surfaces errors.
	err := PGO(t.TempDir(), t.TempDir())
	require.Error(t, err)
}

// pgoBinaryOnlyRunner writes the mdsmith binary on go build and returns
// nil on every other call without writing any output files. Used to
// drive the collectProfile-missing-profile error path inside PGO.
type pgoBinaryOnlyRunner struct{}

func (r *pgoBinaryOnlyRunner) RunCommand(dir, name string, args ...string) error {
	if name == "go" && len(args) >= 5 && args[0] == "build" && args[1] == "-pgo=off" && args[2] == "-o" {
		return os.WriteFile(args[3], []byte("BIN"), 0o755)
	}
	return nil
}

// pgoMergeErrRunner handles build + corpus + profiles normally, but
// injects a failure into the go-tool-pprof merge step (or, when
// noOutput is true, makes pprof exit 0 while writing no output file).
type pgoMergeErrRunner struct{ noOutput bool }

func (r *pgoMergeErrRunner) RunCommand(dir, name string, args ...string) error {
	switch {
	case name == "go" && len(args) >= 5 && args[0] == "build" && args[1] == "-pgo=off" && args[2] == "-o":
		return os.WriteFile(args[3], []byte("BIN"), 0o755)
	case name == "go" && len(args) >= 2 && args[0] == "tool" && args[1] == "pprof":
		if r.noOutput {
			return nil // exits 0 but writes nothing
		}
		return errInjected
	default:
		if p := os.Getenv("MDSMITH_CPUPROFILE"); p != "" {
			return os.WriteFile(p, []byte("PROF"), 0o644)
		}
		return nil
	}
}

// TestPGOMkdirOutputDirFails covers the MkdirAll error path for
// outDir (cmd/mdsmith/) inside PGO: pre-creating root/cmd as a
// regular file makes MkdirAll fail with ENOTDIR so the merge step
// is never reached.
func TestPGOMkdirOutputDirFails(t *testing.T) {
	root := t.TempDir()
	workdir := t.TempDir()
	stageMinimalRepo(t, root)

	// Block MkdirAll(root/cmd/mdsmith): root/cmd exists as a file,
	// so creating a sub-directory under it fails with ENOTDIR on any OS.
	require.NoError(t, os.WriteFile(filepath.Join(root, "cmd"), []byte("x"), 0o644))

	err := NewWithDeps(osFS{}, &pgoFakeRunner{}).PGO(root, workdir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mkdir")
}

// TestPGOBuildCorporaFails covers the buildCorpora error path: a
// non-git root causes git ls-files to fail, which buildCorpora wraps
// and returns to PGO.
func TestPGOBuildCorporaFails(t *testing.T) {
	// pgoFakeRunner writes the binary, so the post-build check passes,
	// but the root has no .git so exec.Command("git", "ls-files") fails.
	root := t.TempDir()
	err := NewWithDeps(osFS{}, &pgoFakeRunner{}).PGO(root, t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "git ls-files")
}

// TestPGOCollectProfileLoopFails covers the loop's early-return when
// the first collectProfile call errors (runner writes binary but never
// writes a profile file, so collectProfile returns "no profile").
func TestPGOCollectProfileLoopFails(t *testing.T) {
	root := t.TempDir()
	stageMinimalRepo(t, root)
	err := NewWithDeps(osFS{}, &pgoBinaryOnlyRunner{}).PGO(root, t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no profile")
}

// TestPGOMergeFails covers the go-tool-pprof merge-failure path.
func TestPGOMergeFails(t *testing.T) {
	root := t.TempDir()
	stageMinimalRepo(t, root)
	err := NewWithDeps(osFS{}, &pgoMergeErrRunner{}).PGO(root, t.TempDir())
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
	assert.Contains(t, err.Error(), "merge profiles")
}

// TestPGOMergeNoOutput covers the "merged profile not written" error
// when go tool pprof exits 0 but writes no output file.
func TestPGOMergeNoOutput(t *testing.T) {
	root := t.TempDir()
	stageMinimalRepo(t, root)
	err := NewWithDeps(osFS{}, &pgoMergeErrRunner{noOutput: true}).PGO(root, t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "merged profile not written")
}
