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
		// go build ... -o <bin> ... — scan for the -o value
		for i, a := range args {
			if a == "-o" && i+1 < len(args) {
				return os.WriteFile(args[i+1], []byte("BIN"), 0o755)
			}
		}
		return fmt.Errorf("no -o flag in go build args: %v", args)
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
