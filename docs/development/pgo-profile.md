---
summary: >-
  Why no PGO profile is committed: a checked-in
  `cmd/mdsmith/default.pgo` burdens every merge with a binary
  artifact, and the merge tooling must stay free of
  repo-specific entries. How to generate a profile locally,
  and the plan that moves generation into the release build.
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

[Plan 2606120633](../../plan/2606120633_release-built-pgo-profile.md)
moves profile generation into the release workflow instead,
so published binaries get PGO without a tracked artifact.

## Generating a profile locally

For experiments, record the real workload — both benchmark
corpora, both configurations — and merge the runs. Staleness
is safe: samples that match no function are ignored, so an
old profile cannot break a build; it just helps less.

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
