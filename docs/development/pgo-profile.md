---
summary: >-
  Why no PGO profile is committed: a checked-in
  `cmd/mdsmith/default.pgo` burdens every merge with a binary
  artifact, and the merge tooling must stay free of
  repo-specific entries. How to generate a profile locally,
  and how the release workflow generates one inside the pipeline
  so published binaries are profile-guided without a tracked artifact.
---
# PGO and the uncommitted profile

Go's profile-guided optimization (PGO) reads a pprof CPU
profile at compile time. Since Go 1.21, `go build`
automatically uses a `default.pgo` file in the main package
directory. The compiler then favors the recorded hot paths:
it inlines hot functions past the normal budget,
devirtualizes interface calls that mostly hit one concrete
type, and lays out hot code together.

## Why the profile is not committed

Committing `cmd/mdsmith/default.pgo` would make every plain
`go build` profile-guided. But the file is a binary pprof
snapshot. Two branches that both refresh it always conflict,
and no byte-level merge is meaningful. A take-current merge
driver was tried and rejected. The managed `.gitattributes`
block and `mdsmith merge-driver install` are product
features. They run in users' repositories and must never
carry entries that only fit mdsmith's own repository.

The path is gitignored so a locally generated profile cannot
be committed by accident. On this workload PGO measured
within noise (~0-2%), so plain builds lose nothing
user-visible.

## How the release build gets a profile

The release workflow generates the profile inside the
pipeline. So the published binaries are profile-guided
without a tracked artifact. `release.yml` runs a `pgo` job
after `preflight` and before the build matrix. The job builds
`mdsmith-release` and runs `mdsmith-release pgo
/tmp/mdsmith-pgo`.

That subcommand:

1. Builds the `mdsmith` binary.
2. Builds the two benchmark corpora using the same plumbing
   as `mdsmith-release bench`: `corpus_repo` from the repo's
   tracked Markdown, `corpus_neutral` from the pinned Rust
   Book and Reference.
3. Records a CPU profile over each corpus in both the
   default and `parity`
   (`bench-parity.mdsmith.yml`) configurations via
   `MDSMITH_CPUPROFILE` — four runs total.
4. Merges the four runs with `go tool pprof -proto` into
   `cmd/mdsmith/default.pgo`.

The `pgo` job uploads that file as the `pgo-profile` artifact.
Each `build` matrix job downloads it to `cmd/mdsmith/` and
prints the profile size before running `go build`. Go finds
`cmd/mdsmith/default.pgo` in the main package directory
automatically — no flag needed. The file is never committed.

The benchmark harness deliberately does **not** build with
this profile (see
[the benchmark page](../research/benchmarks/README.md)). The
published numbers measure the plain, reproducible build, not
the PGO-optimized release output.

## Generating a profile locally

The release `pgo` job's logic is also a local command. From
the repo root, `mdsmith-release pgo [workdir]` runs the whole
recipe below — build, corpora, four profiled `check` runs,
merge — and writes `cmd/mdsmith/default.pgo` for you; the
`workdir` defaults to `/tmp/mdsmith-bench`.

For experiments, the equivalent shell records the real
workload — both benchmark corpora, both configurations — and
merges the runs. Staleness is safe: samples that match no
function are ignored, so an old profile cannot break a build;
it just helps less.

```bash
# Corpora as built by the benchmark harness (run.sh /
# mdsmith-release bench) under its workdir:
W=/tmp/mdsmith-bench
P=docs/research/benchmarks/bench-parity.mdsmith.yml
go build -o "$W/mdsmith" ./cmd/mdsmith
for c in corpus_repo corpus_neutral; do
  MDSMITH_CPUPROFILE="$W/full-$c.prof" \
    "$W/mdsmith" check "$W/$c" || true
  MDSMITH_CPUPROFILE="$W/parity-$c.prof" \
    "$W/mdsmith" check -c "$P" "$W/$c" || true
done
go tool pprof -proto -output=cmd/mdsmith/default.pgo \
  "$W/mdsmith" "$W"/*.prof
```

Rebuild after writing the file; `go build` picks it up from
`cmd/mdsmith/` automatically. PGO never changes behavior —
tests, output, and rule results are identical with or
without it.
