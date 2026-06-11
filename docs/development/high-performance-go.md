---
title: High-Performance Go
summary: >-
  Process and patterns for keeping mdsmith's Go core fast:
  the benchmark→profile→fix loop, the patterns to reach
  for, and the anti-patterns that have already cost the
  project real CPU and GC time.
---
# High-Performance Go

mdsmith's hot path is the rule set running over every file
in the workspace. The Large gate parses 600 files through
the full rule set; one extra alloc per `Check` means tens
of thousands per run, and one accidental O(n) rescan turns
a fast run into a slow one. This page is the playbook.

This page is the methodology behind the project's budgets.
The ≤ 10 alloc ceiling lives in
[Allocation Budget](index.md#allocation-budget); the corpus
gates live in the
[benchmark notes](../research/benchmarks/README.md); the
session narrative lives in
[perf-parity-session](../research/perf-parity-session.md).

## Process

**Apply best-practice patterns first. Then measure. Then
fix what is still hot.** The patterns in
[Patterns to apply](#patterns-to-apply) are known wins
across the Go core — pre-size a slice, hoist a regex,
return `nil` for an empty result. Use them by default;
they cost nothing.

Past that, profile before you rewrite — a flat profile in
your suspected hot path stops a useless fix. The loop:

1. **State the goal numerically.** "Function X under N
   allocs/call on representative input" — not "make it
   faster". Name the symbol (function, rule, package) and
   the input that hits its hot frame.
2. **Lock in a baseline** by running the package's
   existing benchmarks multiple times:

   ```bash
   go test -run=^$ -bench=. -count=10 -benchmem \
     ./path/to/package > old.txt
   ```

3. **Profile** the baseline. CPU profile if you don't know
   the bottleneck; alloc profile if `b.ReportAllocs` shows
   allocations; trace if latency is bad but CPU is idle.
4. **Change one thing.** Re-run the same benchmark, same
   count.
5. **Decide with `benchstat`,** not eyeballs. Re-run the
   benchmark into `new.txt`, then:

   ```bash
   benchstat old.txt new.txt
   ```

   `~` in the delta column means no significant change.
   p < 0.05 with a meaningful effect size is the bar.

**Write benchmarks that always run.** Put the bench next
to the code. Pin its budget inline with `b.Fatalf` on
overshoot, as `BenchmarkRule_MDS024` does. CI then catches
the next slip on its own.

Opt-in rules (those returning
`EnabledByDefault() == false`) skip `BenchmarkCheckCorpus*`,
so `perrule_bench_test.go` is their only time gate. It pins
each a `perRuleBenchBudget` row — `Time` near 5× the logged
baseline, `Allocs` near baseline plus `max(20%, 4)`.
`optInRules` finds new opt-in rules from `rule.All()`, so
the gate fails with "no pinned budget" until you add the
row. It times parse+Check together: parse dwarfs Check, but
constant parse cost lets the sum still catch a regression.
Allocs stay the tight gate (subtracted, deterministic).

To decide whether a rule needs the AST,
`testdata/rule_walk_audit.json` records each rule's class
(plan 2606022126). **Category A** rules scan `f.Lines` with no
AST. **Category B** rules drive `f.ProseRanges()` instead
of re-implementing fences. **AST-required** rules keep the
tree. No AST-walking rule is cleanly Category A today.

### Which profile answers which question

| Profile | Source                               | Question                          |
| ------- | ------------------------------------ | --------------------------------- |
| CPU     | `-cpuprofile cpu.out`                | Where is time going?              |
| Memory  | `-memprofile m.out`                  | What allocates and what is live   |
| Block   | `runtime.SetBlockProfileRate(1)`     | Where do goroutines wait?         |
| Mutex   | `runtime.SetMutexProfileFraction(1)` | Who holds contended locks?        |
| Trace   | `-trace trace.out`                   | Scheduler / GC / syscall timeline |

View memory profiles with `go tool pprof -alloc_objects`
(every allocation, including freed) or `-inuse_objects`
(what is resident now).

mdsmith ships a profile hook for the CLI
(`internal/profiling/profiling.go`):

```bash
MDSMITH_CPUPROFILE=cpu.out mdsmith check .
go tool pprof -http=:8080 cpu.out
```

No CLI flag on purpose — the command line stays
byte-identical to production. For diffs, use
`go tool pprof -base=old.prof new.prof`.

### Escape analysis

Before adding `sync.Pool`, read what the compiler already
does:

```bash
go build -gcflags="-m=2" ./pkg/markdown 2>&1 | grep escape
```

Common causes of escape: returning a pointer to a local;
storing a value in `interface{}` / `any`; capturing a
variable in a closure that outlives the frame; slice or
map growth past a compile-time-known size. The stack costs
nothing; the heap costs an alloc plus future GC scan.

### Profile-guided optimization

PGO has been GA since Go 1.21 and lands 2–14% wins on real
binaries. Run `mdsmith check` over a representative corpus
with `MDSMITH_CPUPROFILE=cmd/mdsmith/default.pgo`; `go
build` then picks the file up automatically. Refresh after
major rule changes. Worth it for release builds, not for
one-off debug builds.

## Patterns to apply

The list below extends the project's existing
[Allocation Budget](index.md#allocation-budget) rules.
Reach for these first.

### Allocations

Each removed alloc in a per-file `Check` saves one alloc
per workspace file.

- **Pre-size slices.** `make([]T, 0, n)` when `n` is
  known. `append` doubles capacity up to ~1024 then grows
  ~25%, copying each step.
- **Reuse loop-local buffers.** `buf = buf[:0]` clears
  length, keeps capacity. See `extractTextBufPool` in
  `internal/mdtext/mdtext.go`.
- **`sync.Pool` for transient state.** Always reset
  before `Put`; entries can be reaped by GC without
  notice. Examples: `internal/punkt/tokenizer.go`; the
  parse-arena pool behind `lint.NewFileFromSourcePooled`
  (~40% of all `check` allocation until `engine.lintFile`
  became its release boundary). Pool only where the
  release point provably outlives every reference.
- **Return `nil`, not `[]T{}`.** Project convention.
  `nil` and a non-nil empty slice are distinguishable in
  tests, JSON, and `reflect`; sticking to `nil` for "no
  result" keeps callers uniform.
- **Compile regexes at package scope.**
  `var foo = regexp.MustCompile(…)`. Compiling inside a
  hot function builds the NFA every call.

### Strings and bytes

- **Stay in `[]byte`.** Each `string(b)` allocates and
  copies. `bytes.IndexByte` and `bytes.Contains` are
  SIMD-accelerated on amd64; faster than `strings.*` once
  you already have bytes.
- **`strings.Builder` over `+`.** Concatenation in a loop
  allocates a new backing array each time. Call
  `Grow(n)` first if you know the final size.
- **`strconv` over `fmt.Sprintf`.** `strconv.Itoa(n)` is
  ~3× faster than `fmt.Sprintf("%d", n)` and skips
  reflection.
- **`strings.EqualFold` for case-insensitive compare.**
  One pass, no allocation; beats `ToLower` + `==`.
- **`unsafe.String` / `unsafe.Slice` (Go 1.20+)** for
  zero-copy `[]byte`↔`string`. The caller must guarantee
  the source isn't mutated and outlives the view. Use
  sparingly, with a comment naming the invariant.

### Fixed-string search beats regex

`bytes.IndexByte('#')` is a hardware-assisted single-byte
scan. `regexp.MustCompile("#").FindIndex` builds an NFA
and walks it. For anything expressible as a literal,
substring, or prefix/suffix check, skip `regexp`.

### Data structures

- **Fixed-size arrays beat slices** when the size is
  known — no header, no escape.
- **`map[K]struct{}` for sets** — zero-byte value type.
- **Sorted slice + binary search** beats a map for
  n < ~100, thanks to cache locality. Benchmark at your
  real n.
- **Swiss tables in Go 1.24+.** Free 30–60% map speedup
  and up to 70% map-memory reduction; no code change
  needed.

### Struct layout

- **Order fields large-to-small** to minimize padding.
  The `fieldalignment` analyzer in `golang.org/x/tools`
  flags layouts with wasted bytes and can rewrite them.
- **Hot/cold split.** Frequently-read fields in one
  struct, rarely-read in another behind a pointer.
  Better cache utilization in the hot path.
- **Prefer `[]Foo` over `[]*Foo`.** A value slice is one
  GC-scanned allocation with zero internal pointers; the
  pointer slice forces N pointer scans every cycle.

### Skip work you don't need

The cheapest call is the one you never make. Two real
mdsmith wins live here:

- **Memoize per-input computations.** When a helper
  recurs over one `*lint.File`, cache the result on the
  File — `LineOfOffset`'s newline index (~24% of `check`
  CPU before the fix) and `RunCache.GlobMatches` follow it.
- **Gate expensive analyzers behind a cheap pre-check.**
  MDS024 skips the sentence tokenizer when no paragraph
  can violate a limit; byte-needles gate regex paths.
- **Declare interest instead of filtering inside.**
  `rule.KindScopedChecker` lifts "is this node mine?"
  into a per-kind dispatch table. Related: never exec a
  subprocess per item (`git rev-parse` once did).

### Inlining

The inliner has a budget (~80 nodes per function). Keep
hot functions tiny so they inline; outline the slow path
into a separate function. The canonical model is
`sync.Mutex.Lock`: the uncontended CAS inlines; the
contended slow path is a separate function.

Inspect with `go build -gcflags="-m=2"` and look for
`can inline foo` / `inlining call to foo`.

### Concurrency

- **`sync/atomic`** for one-word flags and counters
  (`atomic.Bool`, `atomic.Int64`, `atomic.Pointer[T]`).
- **`sync.Once`** for lazy init. After the first call,
  it costs a single atomic load.
- **`sync.Mutex`** for >1-word critical sections.
  Default choice. Cheaper than `sync.RWMutex` under low
  contention.
- **Channels for handoff or backpressure**, not for
  protecting a single variable — under contention a
  channel is orders of magnitude slower than a mutex.
- **`errgroup.SetLimit(n)`** for bounded fan-out. Size
  by bottleneck: `runtime.NumCPU()` for CPU-bound, much
  higher for I/O-bound.
- **Every goroutine must exit on `ctx.Done()`.** Test
  with `go.uber.org/goleak`.

## Patterns to avoid

Every [Patterns to apply](#patterns-to-apply) rule has an
inverse anti-pattern. Reaching for `fmt.Sprintf`, `+` in a
loop, an un-`make`d `append`, `[]T{}`, `any`, `regexp` for
a literal, goroutine-per-item, or a channel for one
variable are the obvious ones. A few more are below.

| Avoid                                | Why                                | Use instead                        |
| ------------------------------------ | ---------------------------------- | ---------------------------------- |
| `defer` in a tight loop              | falls off the open-coded fast path | hoist or inline cleanup            |
| `reflect` in hot paths               | type-info walks, allocations       | code-gen or hand-roll              |
| `log.Printf` per item in a hot loop  | format + lock + I/O                | sample, or batch outside the loop  |
| `time.Now()` in a tight loop         | wall + monotonic read each call    | read once, use `time.Since`        |
| `os.ReadFile` on huge inputs         | one giant alloc, all resident      | `bufio.Reader` with a tuned buffer |
| `context.Background()` deep in calls | loses cancellation                 | propagate caller's `ctx`           |

## Tooling

See [Process](#process) for the `benchstat`, `pprof`, and
`goleak` workflow. `go tool -modfile=tools/go.mod
golangci-lint run` is the lint gate. Its `perfsprint`,
`prealloc`, and gocritic performance group are worth
enabling on top of `.golangci.yml`.

## References

- [Dave Cheney — High Performance Go Workshop][cheney]
- [Damian Gryski — go-perfbook][perfbook]
- [Go blog — Profile-Guided Optimization in Go 1.21][pgo]
- [Go blog — Faster Go maps with Swiss Tables][swiss]
- [Go blog — `testing.B.Loop`][bloop]
- [Filippo Valsorda — Efficient Go APIs with the inliner][filippo]
- [Eli Bendersky — Common pitfalls in Go benchmarking][eli]
- [PlanetScale — Generics can make your Go code slower][ps]

[cheney]: https://dave.cheney.net/high-performance-go-workshop/dotgo-paris.html
[perfbook]: https://github.com/dgryski/go-perfbook/blob/master/performance.md
[pgo]: https://go.dev/blog/pgo
[swiss]: https://go.dev/blog/swisstable
[bloop]: https://go.dev/blog/testing-b-loop
[filippo]: https://words.filippo.io/efficient-go-apis-with-the-inliner/
[eli]: https://eli.thegreenplace.net/2023/common-pitfalls-in-go-benchmarking/
[ps]: https://planetscale.com/blog/generics-can-make-your-go-code-slower
