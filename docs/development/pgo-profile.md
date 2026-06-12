---
summary: >-
  The checked-in PGO profile at `cmd/mdsmith/default.pgo`: every
  `go build ./cmd/mdsmith` compiles against it automatically, how
  to regenerate it from the benchmark corpora, and why a merge
  conflict on it is always resolved by regenerating — never by
  merging bytes.
---
# The committed PGO profile

`cmd/mdsmith/default.pgo` is a merged pprof CPU profile.
Profile-guided optimization (PGO) reads it at compile time.
Since Go 1.21, `go build` automatically uses a `default.pgo`
file in the main package directory. The compiler then favors
the recorded hot paths. It inlines hot functions past the
normal budget. It devirtualizes interface calls that mostly
hit one concrete type, and it lays out hot code together.

## Why it is checked in

Committing the profile means every plain `go build
./cmd/mdsmith` is profile-guided with no extra flags or steps:

- the release pipeline's binaries,
- the benchmark harness build (`mdsmith-release bench` runs a
  plain `go build`, so published numbers reflect the optimized
  binary users install),
- and any contributor build.

The alternative — generating the profile in CI per build —
would make builds non-reproducible run to run and add a slow
profiling step to every pipeline. A committed snapshot is
deterministic and reviewed like any other change.

PGO never changes behavior. The profile only steers compiler
heuristics; tests, output, and rule results are identical with
or without it.

## Staleness is safe, refresh is cheap

The profile is a snapshot of the hot paths of the code it was
recorded against. After the code changes, samples that match
no function are ignored. A stale profile cannot break the
build or the binary; it just helps less.

Refresh it after large engine or rule changes. Also refresh
it when the benchmark page's numbers are re-measured. Record
the real workload — both corpora, both configurations — and
merge the runs:

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

Then rebuild once more. That way the committed profile and
the measured binary agree. Re-run the benchmark harness when
the refresh accompanies a published-number update.

## Resolving merge conflicts

The file is binary (a gzipped protobuf). Git cannot merge two
profiles. No byte-level resolution is meaningful. Here is the
procedure when two branches both refreshed it:

1. Take either side to clear the conflict marker — the content
   does not matter, it is about to be replaced:

   ```bash
   git checkout --theirs cmd/mdsmith/default.pgo
   git add cmd/mdsmith/default.pgo
   ```

2. Regenerate the profile against the merged code with the
   commands above, and commit the fresh file with the merge.

Skipping step 2 is acceptable when in a hurry. Either
parent's profile is merely slightly stale, and PGO tolerates
staleness by design. Never hand-edit or concatenate the
files.
