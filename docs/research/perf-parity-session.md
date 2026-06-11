---
summary: >-
  Findings from the session that drove the release benchmark to
  parity-at-mado and full-config ≤3x: which optimizations paid,
  which did not, the measurement traps, and the lifetime contracts
  the pooled-arena and zero-copy designs depend on.
---
# Performance session: parity at mado, full config under 3x

This page records what the benchmark-driven optimization session
(PR 587) learned, beyond the numbers the
[benchmark page](benchmarks/README.md) publishes. It exists so the
next performance pass starts from these conclusions instead of
re-deriving them. The durable patterns were folded into
[High-Performance Go](../development/high-performance-go.md); this
page keeps the narrative, the dead ends, and the measurement notes.

## Outcome

Committed snapshot movement, median ratio to mado per corpus:

| Ratio vs mado          | Before | After |
| ---------------------- | ------ | ----- |
| parity, repo corpus    | 1.8x   | 0.93x |
| parity, neutral corpus | 2.9x   | 1.26x |
| full, repo corpus      | 7.9x   | 2.80x |
| full, neutral corpus   | 5.4x   | 2.78x |

User CPU for a parity check of the neutral corpus fell from ~274 ms
to ~140 ms; the `BenchmarkCheckCorpusLarge` gate fell from ~0.8 s to
~0.10 s against its 12 s budget.

## What paid, in order of measured wall-time impact

1. **Subprocess elimination** (repo-config runs): MDS048 spawned
   `git rev-parse` once per distinct directory. Hundreds of execs
   serialized ~220 ms of wall time. A `.git` stat walk replaced it.
   Lesson: one exec per item dwarfs every micro-optimization on the
   same path; check for subprocesses first when a profile shows
   `Syscall6` plus `Waitid`.
2. **Unbuffered per-line output**: one `write(2)` and one `fmt`
   parse per formatted diagnostic line. A 64 KiB `bufio.Writer` per
   report plus an append-based formatter removed ~12% of CPU and
   most of the syscall column on diagnostic-heavy corpora.
3. **Arena pooling across parses**: per-parse AST slabs were ~40%
   of all allocation — alive exactly as long as one file's lint.
   Cursor-based slab reuse plus a `sync.Pool` keyed to the engine's
   per-file lifetime boundary cut allocation volume roughly in half
   and collapsed run-to-run variance (GC noise) visibly.
4. **Run-scoped caching of repeated derivations**: catalog glob
   walks (per host file → per run), code-span ranges (per rule →
   per file), include-edge scans gated by a `<?include` needle.
   The repeated work was invisible in any one rule's benchmark and
   obvious only in the whole-binary profile.
5. **Kind-scoped dispatch**: 25 NodeChecker rules × every AST node
   × enter and leave was ~11% pure dispatch overhead. A CSR table
   per file (pooled) plus a declared-kinds contract
   (`rule.KindScopedChecker`) reduced it to near zero.
6. **Byte-needle gates**: `)[` before the reversed-link regex,
   `http` before the URL regex, `#`/`>` before per-line map probes.
   Each gate is two lines and pays for itself on the first corpus.
7. **Parser line-level fast scan**: a 256-bit trigger set skips the
   per-byte inline classification for trigger-free prose lines.
   Bounded win (~5-8%) for surgical risk — only worth it with the
   equivalence harness guarding both allocation axes.

## What did not pay

- **GOGC=off** and **higher GOMAXPROCS**: both slower. After arena
  pooling, the measured GC sweet spot moved from 400 to 300 — the
  setting tracks allocation volume, so retune it after any large
  allocation change, not once.
- **PGO**: within noise on this workload (~0-2%); committed anyway
  because it is free at build time and `go build` picks it up.
- **Micro-optimizing rules below ~2% profile share**: three rounds
  of diminishing returns ended at a flat profile; the remaining
  neutral-corpus gap to mado (~1.26x) is parse plus per-byte rule
  cost with no single hot spot left.

## Measurement traps hit during the session

- **The harness runs from the repository root.** A full-config
  `mdsmith check <corpus>` discovers and applies the repo's own
  `.mdsmith.yml` — opt-in rules on. Half the session's full-config
  numbers were measured against the wrong config before this was
  noticed. Always reproduce the harness command line exactly,
  including the working directory.
- **Shared-runner noise**: absolute medians moved ±15% between
  quiet and contended windows; only within-run ratios were stable.
  Single-σ improvements were confirmed by re-running, never by one
  hyperfine summary.
- **Per-package coverage does not see cross-package gates**: the
  integration test that exercises every rule's `EnteringKinds`
  contributes nothing to the rule packages' own coverage, which the
  patch gate measures. Declarations need an in-package touch.
- **Profile merging needs explicit file lists**: `pprof` treats the
  first positional argument as the binary; globbing profiles into
  it silently produced empty reports.

## Lifetime contracts introduced (the risky part)

Two designs trade safety margins for speed and are only correct
under stated invariants:

- **Pooled parse arenas** (`lint.NewFileFromSourcePooled`): the
  File and everything aliasing its arena must die before `release`.
  `engine.lintFile` is the one blessed call site; the LSP's cached
  Files and the RunCache target loads stay on the unpooled
  constructor. The goldmark equivalence harness plus the
  `goldmark_upstream` CI axis guard parser behavior on both
  allocation paths.
- **Zero-copy source views** (`lint.BytesView`, `LineStrings`,
  diagnostic `SourceLines`): valid only because check never mutates
  a loaded source buffer and fix builds replacement content in
  fresh buffers. Any future in-place source mutation breaks these
  silently — the invariant is documented at each definition.

## Residual headroom

Neutral-corpus parity sits at ~1.26x mado. The remaining cost is
goldmark parse (~30% of user CPU) and per-byte rule scans on long
prose; the profile is flat. Plausible next steps, none cheap:
fused line-rule passes over a single scan, a leaner inline parser
for trigger-dense text, or SIMD-style line classification. The
2-physical-core CI runner caps parallel speedup near 2.5x, so
wall-time parity on that corpus requires user CPU within ~25% of
mado's, not just better scaling.
