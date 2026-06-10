---
id: 2606101547
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
depends-on: [2606101546, 103, 2606101548]
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

Plan 2606101546 dispatches recipes and prints
`OK | FAIL`. Plan 2606101548 hardens execution.
Neither helps debug a hung recipe or
explain a freshness verdict. Five gotchas
drive this plan:

- Buffered stdout hides hangs.
- Failure messages without argv, cwd, exit
  code, and log path are useless.
- Stale-cache surprises drive `make clean`
  without a freshness explanation.
- Non-deterministic recipes defeat caching.
- Parallel builds collide on undeclared
  shared state.

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

`--build-stream` switches to live mode:
lines are forwarded as they arrive,
prefixed with the target name (e.g.
`[book.html] reading chapter 1...`). The
log file is still written.

Each cache entry (plan 103) carries an
`action-id` whose log lives at
`.mdsmith/build-logs/<action-id>.log`.
Logs survive until their entry is
invalidated; a schema-version bump removes
both. `--build-no-cache` writes logs but
no entry; the next `fix` deletes logs
whose `<id>` matches no entry's
`action-id`.

### Failure diagnostic format

When a recipe exits non-zero or fails a
post-condition check (plan 2606101548), mdsmith
prints:

```text
FAIL book.html (recipe: pandoc)
  source:   chapters/intro.md:12 <?build?>
  argv:     pandoc /proj/chapters/intro.md -o /…/book-x7y2/book.html
  cwd:      /…/.mdsmith/build-staging/book-x7y2/
  exit:     1
  duration: 2.3s
  log:      .mdsmith/build-logs/sha256-abc.log
  --- last 20 lines of stderr ---
  pandoc: cannot open /proj/chapters/intro.md
  …
```

`Cmd.Dir` is the staging dir (plan 2606101548), so
`{inputs}` and `{outputs}` expand to
*absolute* paths (project-rooted source,
staging output). The `source:` line keeps
the relative form for readability.

Six fields, then up to 20 lines from the
in-memory tail. The full log is one path
away.

### Hung-recipe diagnostic

When `--build-timeout` expires (plan 2606101546),
mdsmith prints before sending SIGTERM:

```text
TIMEOUT book.html after 30s (pid 12345)
  --- last 20 lines of stdout ---
  …
  --- last 20 lines of stderr ---
  …
  sending SIGTERM to process group; SIGKILL in 5s
```

### `--build-explain TARGET`

Prints the ActionID inputs for one target
in hash-order: `recipe.command`, sorted
params, sorted inputs (path + content sha),
sorted outputs, `cache.version`, the
resulting ActionID, and the cache verdict.
`TARGET` matches by first declared output
path. Answers "why is this fresh?" without
diving into JSON.

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

Cost: roughly 2× wall-clock; for
maintainers adding a recipe, not the
default `fix` flow.

### `--build-jobs N`

Run up to N recipes concurrently. Default
is 1 (serial). `N>1` is safe because plan
2606101548's per-recipe staging dir keeps writes
disjoint, its output post-conditions catch
any `outputs:` boundary violation, and the
cache write happens in one pass after all
recipes finish.

Plan 103 rejects any overlap in declared
`outputs:` paths at target-graph load,
after every `<?build?>` directive has been
collected. A clean load guarantees disjoint
output paths, so the parallel-safety
contract holds for free.

Per-target lines (`OK`, `FAIL`, `SKIP`)
print in completion order; the final
summary lists all targets in declared
order.

### Flags on `mdsmith fix`

Extends the build-pass flag set:

| Flag                     | Behavior                                                    |
| ------------------------ | ----------------------------------------------------------- |
| `--build-stream`         | Live-stream recipe stdout/stderr (prefixed); log still kept |
| `--build-explain TARGET` | Print ActionID inputs for `TARGET`; run no recipe           |
| `--build-verify`         | Run each recipe twice; warn on output mismatch              |
| `--build-jobs N`         | Run up to N recipes concurrently (default 1)                |

`--build-explain` and `--build-verify` are
usage errors combined with each other or
with `--build-dry-run` or
`--build-check-stale`.

### Out of scope

Persistent workers, remote cache sharing,
IDE/LSP integration, structured JSON output
(future, behind `--build-format json`).
Also out: the `mdsmith targets --json` side
door from the [build
spikes](../docs/research/build-orchestrator/go-only.md).

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
   to the `Target` struct (plan 2606101546) so
   diagnostics can point to the
   directive.
3. Implement the timeout diagnostic
   (prints before SIGTERM, per plan 2606101548's
   process-group kill).
4. Implement `--build-stream`: forward
   recipe streams line-by-line to the
   terminal with target-name prefix; log
   file still written.
5. Implement `--build-explain TARGET`:
   match `TARGET` against each directive's
   first declared output (string equality
   after path normalization). No match
   exits non-zero with "no target named X".
   Plan 103's overlap rule eliminates
   ambiguity at target-graph load.
6. Implement `--build-verify`: run each
   recipe twice in independent staging
   dirs (plan 2606101548), `diff` outputs,
   warn and set the `unstable` cache
   flag on mismatch. The flag extends plan
   103's cache entry schema (a bool).
7. Implement `--build-jobs N`: concurrent
   recipe execution behind a work-pool.
   Plan 103 already rejects overlapping
   `outputs:` at target-graph load, so the
   work-pool may dispatch any pair of
   targets in parallel.
8. Wire log retention. Cache eviction
   deletes the matching log file. At the
   start of each `mdsmith fix`, delete any
   `.mdsmith/build-logs/<id>.log` whose
   `<id>` has no cache entry; this clears
   orphans
   from a prior `--build-no-cache` run.
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
  - Two directives with overlapping
    `outputs:` are rejected at
    target-graph load (plan 103)
    regardless of `--build-jobs`.
  - Hung recipe printout includes the
    last 20 lines of each stream
    before the SIGTERM.
  - Cache eviction deletes the matching
    log file; orphan logs from a previous
    `--build-no-cache` run are removed at
    the start of the next `mdsmith fix`.

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
      before SIGTERM (plan 2606101548)
- [ ] `--build-explain TARGET` prints
      every ActionID input field and the
      cache verdict; runs no recipe
- [ ] `--build-verify` runs every recipe
      twice and warns on output mismatch;
      the cache records an `unstable`
      flag on the target
- [ ] `--build-jobs N` (default 1) runs
      up to N recipes concurrently;
      overlapping `outputs:` paths are
      already rejected at target-graph
      load (plan 103)
- [ ] Cache eviction deletes the matching
      log file; orphan logs from a prior
      `--build-no-cache` run are deleted at
      the start of the next `mdsmith fix`
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run`
      reports no issues
