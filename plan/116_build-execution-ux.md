---
id: 116
title: Build execution UX (stdout/stderr, debug, parallel)
status: "🔲"
summary: >-
  Make the build pass debuggable. Capture
  per-recipe stdout/stderr to per-target log
  files (ninja-style buffer-and-dump default;
  `--build-stream` for live tailing). Print
  rich failure diagnostics. Add
  `--build-explain TARGET` to print the
  ActionID inputs. Add `--build-verify`
  (run-twice diff) for non-determinism
  detection. Add `--build-jobs N` for
  parallel execution.
model: opus
---
# Build execution UX (stdout/stderr, debug, parallel)

## Goal

Make the build pass debuggable. Capture
every recipe's stdout and stderr. Persist
the streams under `.mdsmith/build-logs/`.
Print actionable failure messages. Add
helpers for staleness explanation, non-
determinism detection, and parallel
execution.

## Context

Plan 115 dispatches recipes and prints
`OK | FAIL`. Plan 117 hardens execution.
Neither helps debug a hung recipe or
explain a freshness verdict. Five gotchas
drive this plan:

- Buffered stdout hides hangs.
- Failure messages without argv, cwd,
  exit code, and log path are useless.
- Stale-cache surprises drive `make
  clean` unless mdsmith can explain
  freshness.
- Non-deterministic recipes silently
  defeat caching.
- Parallel builds collide on undeclared
  shared state.

[plan-100]: 100_build-config-and-mds040.md
[plan-103]: 103_build-staleness-and-deps.md
[plan-115]: 115_builder-execution-in-fix.md
[plan-117]: 117_build-execution-hardening.md

## Design

### Stdout/stderr capture

Per-recipe streams are captured in two
places at once:

- A buffered in-memory tail (last 50 lines
  of each stream).
- A file under
  `.mdsmith/build-logs/<action-id>.log`
  with both streams interleaved and each
  line prefixed `[stdout]` or `[stderr]`.

Default mode: **buffer**. The recipe's
streams stay quiet during execution; on
success, mdsmith prints `OK <target>`. On
failure, mdsmith prints the failure block
(see below) including the in-memory tail.

`--build-stream` switches to live mode.
Lines are forwarded to the user's terminal
as they arrive, prefixed with the target
name (e.g. `[book.html] reading
chapter 1...`). The log file is still
written. Useful for a single-target debug
run.

Log files are kept until the cache entry
that names them is invalidated, so a user
can read the log of a recipe that ran an
hour ago. Cache eviction (plan 103
schema-version bump or `--build-no-cache`)
deletes the corresponding log files.

### Failure diagnostic format

When a recipe exits non-zero or fails a
post-condition check (plan 117), mdsmith
prints:

```text
FAIL book.html (recipe: pandoc)
  source:   chapters/intro.md:12 <?build?>
  argv:     pandoc chapters/intro.md -o /…/book.html
  cwd:      /…/.mdsmith/build-staging/book-x7y2/
  exit:     1
  duration: 2.3s
  log:      .mdsmith/build-logs/sha256-abc.log
  --- last 20 lines of stderr ---
  pandoc: cannot open chapters/intro.md
  …
```

Six fields, then up to 20 lines from the
in-memory tail. The full log is one
filesystem path away.

### Hung-recipe diagnostic

When `--build-timeout` expires (plan 115),
mdsmith prints before sending SIGTERM:

```text
TIMEOUT book.html after 30s (pid 12345)
  --- last 20 lines of stdout ---
  …
  --- last 20 lines of stderr ---
  …
  sending SIGTERM to process group; SIGKILL in 5s
```

This gives a chance to see what the
recipe was doing before it was killed.

### `--build-explain TARGET`

Prints the ActionID inputs for one
target in hash-order. Fields shown:
`recipe.command`, sorted params, sorted
inputs (path + content sha), sorted
outputs, `cache.version`, the resulting
ActionID, and the cache verdict.

`TARGET` matches by the first declared
output path. The flag answers "why is
this fresh?" without diving into JSON.

### `--build-verify`

Run each recipe twice, in two separate
staging dirs, and `diff` the resulting
output bytes. Mismatch is a *warning*,
not a failure (some recipes are
intentionally non-deterministic — random
seeds, timestamps). The warning records
an "unstable" flag in the cache entry
so the next regular run skips the
re-verify but surfaces the flag in
`--build-explain`.

Cost: roughly 2× wall-clock. Used by
maintainers when adding a new recipe,
not by the default `fix` flow.

### `--build-jobs N`

Run up to N recipes concurrently. Default
is 1 (serial). The safety contract that
makes `N>1` work:

- Plan 117's per-recipe staging dir
  guarantees writes go to disjoint
  locations during execution.
- Plan 117's output post-conditions
  catch any recipe that violates its
  declared `outputs:` boundary.
- The cache write happens in a single
  pass after all recipes finish.

mdsmith refuses `N>1` if any directive
declares an `outputs:` set that
overlaps another's `outputs:`. Plan 103
already errors on duplicate output sets;
this rule extends to *any* path overlap
(e.g. one target's `outputs: [book/]`
and another's `outputs: [book/index.html]`
overlap and force serial).

Output ordering: per-target lines (`OK`,
`FAIL`, `SKIP`) print in the order
recipes *complete*, not the order they
were declared. The final summary lists
all targets in declared order.

### Flags on `mdsmith fix`

Extends the build-pass flag set:

| Flag                     | Behavior                                                    |
|--------------------------|-------------------------------------------------------------|
| `--build-stream`         | Live-stream recipe stdout/stderr (prefixed); log still kept |
| `--build-explain TARGET` | Print ActionID inputs for `TARGET`; run no recipe           |
| `--build-verify`         | Run each recipe twice; warn on output mismatch              |
| `--build-jobs N`         | Run up to N recipes concurrently (default 1)                |

### Out of scope

Persistent worker processes. Remote cache
sharing. IDE / LSP integration of build
output. Structured JSON output (future,
behind `--build-format json`).

## Tasks

1. Implement stdout/stderr capture in
   `internal/build/streams.go`: tee to
   in-memory ring buffer (50 lines per
   stream) and to
   `.mdsmith/build-logs/<action-id>.log`
   with `[stdout]` / `[stderr]` line
   prefixes.
2. Implement the failure diagnostic
   format. Add the source `.md` file:line
   to the `Target` struct (plan 115) so
   diagnostics can point to the
   directive.
3. Implement the timeout diagnostic
   (prints before SIGTERM, per plan 117's
   process-group kill).
4. Implement `--build-stream`: forward
   recipe streams to the user's terminal
   line-by-line with target-name prefix;
   log file still written.
5. Implement `--build-explain TARGET`:
   resolve target by first output path
   or recipe+first-param; print ActionID
   input fields and cache lookup result.
6. Implement `--build-verify`: run each
   recipe twice in independent staging
   dirs (plan 117), `diff` outputs,
   warn and set the `unstable` cache
   flag on mismatch.
7. Implement `--build-jobs N`:
   concurrent recipe execution behind a
   work-pool. Refuse `N>1` when any
   pair of targets has overlapping
   `outputs:` paths.
8. Wire log retention into cache
   invalidation (plan 103): on cache
   eviction or `--build-no-cache`,
   delete the corresponding log files.
9. Integration tests:

  - Default mode: failing recipe prints
    the six-field failure block and the
    last 20 lines of stderr.
  - `--build-stream`: a recipe printing
    100 lines streams them to stdout
    line-by-line.
  - `--build-explain TARGET` prints the
    ActionID inputs and the cache hit
    or miss verdict; runs no recipe.
  - `--build-verify` warns when a
    recipe writes a different output on
    its second run; the cache marks the
    target `unstable`.
  - `--build-jobs 4` runs four recipes
    concurrently against four
    independent targets; output is
    interleaved-but-line-coherent.
  - `--build-jobs 4` refuses with a
    clear message when two directives
    have overlapping `outputs:`.
  - Hung recipe printout includes the
    last 20 lines of each stream
    before the SIGTERM.
  - Log files are deleted when the
    corresponding cache entry is
    evicted.

10. Document the streams, log retention,
    diagnostic format, and the four new
    flags in
    `docs/guides/directives/build.md`.

## Acceptance Criteria

- [ ] Recipe stdout/stderr is captured
      to `.mdsmith/build-logs/<action-id>.log`
      with `[stdout]` / `[stderr]` line
      prefixes
- [ ] Default mode prints `OK | FAIL |
      SKIP` per target; on failure it
      prints the six-field block plus
      the last 20 lines of stderr
- [ ] `--build-stream` forwards recipe
      streams live with target-name
      prefix; log file is still written
- [ ] Timeout fires the diagnostic block
      before SIGTERM (plan 117)
- [ ] `--build-explain TARGET` prints
      every ActionID input field and the
      cache verdict; runs no recipe
- [ ] `--build-verify` runs every recipe
      twice and warns on output mismatch;
      the cache records an `unstable`
      flag on the target
- [ ] `--build-jobs N` (default 1) runs
      up to N recipes concurrently
- [ ] `--build-jobs N` with `N>1` is
      refused when any pair of targets
      has overlapping `outputs:` paths
- [ ] Log files are deleted on cache
      eviction or `--build-no-cache`
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run`
      reports no issues
