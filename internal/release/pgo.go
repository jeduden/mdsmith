// Package release: pgo.go ports docs/development/pgo-profile.md's
// manual profile-generation recipe into the mdsmith-release CLI so
// the release workflow can generate a fresh PGO profile inside the
// pipeline rather than committing a binary `default.pgo` artifact.
//
// The profile is recorded over the same two benchmark corpora the
// `bench` subcommand builds (corpus_repo, corpus_neutral), in both
// the default and the parity configurations, through the
// MDSMITH_CPUPROFILE hook. The four runs are merged with
// `go tool pprof -proto` into cmd/mdsmith/default.pgo at the repo
// root — the standard location `go build` picks up automatically.
// The path is gitignored, so the workflow uploads it as an artifact
// for the build matrix and the repository stays artifact-free.
package release

import (
	"fmt"
	"os"
	"path/filepath"
)

// pgoDefaultProfileRel is the repo-relative path `go build` reads as
// the default PGO profile for the mdsmith main package. It is
// gitignored; the release workflow uploads it as an artifact.
const pgoDefaultProfileRel = "cmd/mdsmith/default.pgo"

// pgoProfileRun is one profiling pass: run the mdsmith binary with
// args, directing its CPU profile (via MDSMITH_CPUPROFILE) to prof.
type pgoProfileRun struct {
	prof string
	args []string
}

// pgoProfileRuns enumerates the four profiling passes the merged
// profile is built from: each of the two corpora once with the
// default config and once with the parity config. The parity config
// is the same bench-parity.mdsmith.yml the benchmark harness uses,
// so the recorded hot paths span both configurations the released
// binary is measured against.
func pgoProfileRuns(root, workdir string) []pgoProfileRun {
	parity := filepath.Join(root, benchDirRel, "bench-parity.mdsmith.yml")
	repoCorpus := filepath.Join(workdir, "corpus_repo")
	neutralCorpus := filepath.Join(workdir, "corpus_neutral")
	return []pgoProfileRun{
		{
			prof: filepath.Join(workdir, "cpu-repo.prof"),
			args: []string{"check", repoCorpus},
		},
		{
			prof: filepath.Join(workdir, "cpu-parity-repo.prof"),
			args: []string{"check", "-c", parity, repoCorpus},
		},
		{
			prof: filepath.Join(workdir, "cpu-neutral.prof"),
			args: []string{"check", neutralCorpus},
		},
		{
			prof: filepath.Join(workdir, "cpu-parity-neutral.prof"),
			args: []string{"check", "-c", parity, neutralCorpus},
		},
	}
}

// pgoMergeArgs builds the `go tool pprof` argv that merges the
// collected profiles into the protobuf-form output the Go toolchain
// reads. The binary is named first as the symbol source, then every
// profile in order, mirroring the shell
// `go tool pprof -proto -output=<out> <bin> <profs...>`.
func pgoMergeArgs(mdsmithBin, outputPath string, profs []string) []string {
	args := []string{"tool", "pprof", "-proto", "-output=" + outputPath, mdsmithBin}
	return append(args, profs...)
}

// PGO generates a PGO profile from the repo root and writes it to
// cmd/mdsmith/default.pgo: build mdsmith, materialize the two
// benchmark corpora (reusing the bench plumbing), record a CPU
// profile over each corpus in both configurations through the
// MDSMITH_CPUPROFILE hook, then merge the four profiles with
// `go tool pprof -proto`. workdir caches the built binary and the
// corpora; CI passes a fresh /tmp path so every run is cold.
//
// The profile runs are expected to exit non-zero (the corpora carry
// lint findings); a run's exit status is ignored as long as it wrote
// its profile file, matching the manual recipe's `|| true`.
func (t *Toolkit) PGO(root, workdir string) error {
	if workdir == "" {
		workdir = defaultBenchWorkdir
	}
	binDir := filepath.Join(workdir, "bin")
	if err := t.fs.MkdirAll(binDir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", binDir, err)
	}

	mdsmithBin := filepath.Join(binDir, "mdsmith")
	if !t.exists(mdsmithBin) {
		fmt.Println("pgo: building mdsmith")
		if err := t.runner.RunCommand(root, "go", "build", "-pgo=off", "-o", mdsmithBin, "./cmd/mdsmith"); err != nil {
			return fmt.Errorf("build mdsmith: %w", err)
		}
	}
	if !t.exists(mdsmithBin) {
		return fmt.Errorf("mdsmith binary not produced at %s", mdsmithBin)
	}

	if err := t.buildCorpora(root, workdir); err != nil {
		return err
	}

	runs := pgoProfileRuns(root, workdir)
	profs := make([]string, 0, len(runs))
	for _, r := range runs {
		if err := t.collectProfile(mdsmithBin, r); err != nil {
			return err
		}
		profs = append(profs, r.prof)
	}

	out := filepath.Join(root, pgoDefaultProfileRel)
	outDir := filepath.Dir(out)
	if err := t.fs.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", outDir, err)
	}
	fmt.Printf("pgo: merging %d profiles into %s\n", len(profs), out)
	if err := t.runner.RunCommand(root, "go", pgoMergeArgs(mdsmithBin, out, profs)...); err != nil {
		return fmt.Errorf("merge profiles: %w", err)
	}
	if !t.exists(out) {
		return fmt.Errorf("merged profile not written to %s", out)
	}
	return nil
}

// collectProfile runs one profiling pass: it points MDSMITH_CPUPROFILE
// at r.prof so the just-built mdsmith writes its CPU profile there,
// then runs the check. The check exits non-zero on lint findings — the
// corpora are full of them — so a non-zero exit is ignored; only a
// missing profile file is fatal, since the merge needs every run.
func (t *Toolkit) collectProfile(mdsmithBin string, r pgoProfileRun) error {
	fmt.Printf("pgo: profiling %v -> %s\n", r.args, r.prof)
	prev, had := os.LookupEnv("MDSMITH_CPUPROFILE")
	_ = os.Setenv("MDSMITH_CPUPROFILE", r.prof)
	defer func() {
		if had {
			_ = os.Setenv("MDSMITH_CPUPROFILE", prev)
			return
		}
		_ = os.Unsetenv("MDSMITH_CPUPROFILE")
	}()
	// The lint exit code is expected to be non-zero; the profile file
	// is the real output, so ignore the run error and check the file.
	_ = t.runner.RunCommand("", mdsmithBin, r.args...)
	if !t.exists(r.prof) {
		return fmt.Errorf("profile run %v wrote no profile to %s", r.args, r.prof)
	}
	return nil
}

// PGO delegates to a default-OS Toolkit (see Bench).
func PGO(root, workdir string) error {
	return New().PGO(root, workdir)
}
