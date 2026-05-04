---
id: 103
title: Build target staleness and dependency tracking
status: "🔲"
summary: >-
  Make the `mdsmith fix` build pass
  Make/Bazel-style. Hash `(recipe spec ‖ sorted
  input contents ‖ output set)` into one
  ActionID per target and store it in
  `.mdsmith/build-cache.json`. Skip targets
  whose ActionID matches the cache and whose
  outputs all exist. Adds `--build-force` and
  `--build-check-stale` to `mdsmith fix`.
model: opus
---
# Build target staleness and dependency tracking

## Goal

The build pass inside `mdsmith fix` (plan 115)
runs only recipes whose inputs or recipe spec
changed, or whose declared outputs are
missing. mdsmith hashes one ActionID per
target and stores it in JSON; the next `fix`
skips fresh targets.

## Context

Plan 102 adds `inputs:` and `outputs:` to
`<?build?>`. Plan 115 wires the build pass
into `mdsmith fix` and rebuilds every target
unconditionally — wasting time and flooding
git diffs with regenerated artifacts whose
inputs did not change.

### Pattern borrowed from `cmd/go/internal/cache`

Go's build cache hashes `(action description ‖
input contents)` into an ActionID and keys
results by it. mdsmith borrows the
content-addressed input model but stores its
cache as JSON (hundreds of targets, not
millions) keyed by sorted `outputs` set, with
the ActionID stored *inside* each entry (see
"Cache file" below). Content hashing beats
mtime: git checkouts and CI restores rarely
preserve mtimes.

## Design

### Recipe-default inputs

Recipes may declare implicit inputs in
`build.recipes.NAME.default-inputs`. Each
entry is a literal relative path or a
`{param}` token. Example:

```yaml
build:
  recipes:
    vhs:
      command: "vhs {tape}"
      params:
        required: [tape]
      default-inputs: ["{tape}"]
```

A directive supplying `tape: demo.tape` has
its effective input set computed as
`{ demo.tape } ∪ directive.inputs`. Authors
do not restate the recipe's own source file.

### ActionID

The ActionID is sha256 over these fields,
each prefixed with its byte length (8 bytes
big-endian). Paths are project-root-
relative. Symlinks are resolved first.
Separators are normalized via
`filepath.ToSlash`. Plan 115's `Target`
carries the absolute `Root` separately. The
ActionID is therefore stable across clones
and platforms. Fields, in order:

```text
recipe.command
canonical(directive.params)         (sorted; per pair: len(key)|key|len(value)|value)
canonical(sorted relative inputs)   (per entry: len(path)|path)
concat(sha256(content) per input, same order)
canonical(sorted relative outputs)  (per entry: len(path)|path)
cache.version
```

Every field has an outer 8-byte big-endian
length prefix; each key, value, and path
inside the canonicalisations is itself
length-framed. No separator byte is used.
Two-layer framing prevents collisions even
when param keys contain `=` or `\0` and
when filenames contain control bytes from
hostile globs.

`cache.version` lets a future mdsmith
release rev the schema and force a single
rebuild without crashing on stale entries.

### Staleness check

Per target, in order:

1. Resolve `inputs` (directive `inputs` ∪
   recipe `default-inputs`); if any non-glob
   entry is missing → build error.
2. If any entry in `outputs` does not exist
   on disk → stale.
3. Compute the ActionID.
4. Look up the target's cache entry by
   sorted-output-set key. If absent or
   stored ActionID differs → stale.
5. Hash each declared output; mismatch with
   the cache entry's stored hash → stale
   (the artifact was tampered with or
   regenerated externally).
6. Otherwise → fresh; skip the recipe.

Step 5 makes the cache *advisory* rather
than authoritative — it catches poisoned
cache entries and hand-edited artifacts.

A target is identified in the cache by its
sorted `outputs` list, length-framed and
joined. Any overlap across two directives'
`outputs:` paths is a build error reporting
both source locations. Overlap covers exact
collisions and directory-prefix collisions
(`book/` vs `book/index.html`). Without it
cache ownership is ambiguous and serial
builds become "last writer wins"; plan 116
reuses the rule for parallel safety.

### Cache file

Stored at `.mdsmith/build-cache.json`. Each
entry has:

- `outputs[]`: `{path, hash}` pairs sorted
  by path; `hash` is sha256 of the artifact
  at build time, used by staleness step 5.
- `inputs[]`: sorted post-glob paths;
  informational (ActionID covers content).
- `action-id`: length-framed sha256
  serialized as `sha256-<64 lowercase hex>`.
  Stored as entry metadata (*not* the JSON
  map key); used as log filename
  (`<action-id>.log`) and on the wire.
- `recipe`, `built-at`: informational only;
  neither is in the ActionID or consulted
  by staleness.

All paths are stored relative to the project
root.

Cache writes are atomic: write to
`.mdsmith/build-cache.json.tmp` and rename. A
mid-build crash leaves the previous cache
readable.

`.mdsmith/` goes into a recommended
`.gitignore` snippet — per-clone state, like
`node_modules/`.

### Flags on `mdsmith fix`

Extends plan 115's build-pass flag set:

| Flag                  | Behavior                                                                |
|-----------------------|-------------------------------------------------------------------------|
| (none)                | Build only stale targets; refresh cache for rebuilt                     |
| `--build-force`       | Build every target; refresh all cache entries                           |
| `--build-check-stale` | Print every stale target, exit non-zero if any are stale, run no recipe |
| `--build-no-cache`    | Treat all targets as stale; do not read or write the cache (debugging)  |

`--build-check-stale` makes "every artifact is
up to date with its source" a CI signal a
reviewer can trust. The lint-fix pass still
runs unless combined with `--build-only`.

### Interaction with plan 115

- The build pass calls the staleness check
  before `Builder.Build`; fresh targets are
  skipped silently.
- Per-target summary: `OK` (ran, succeeded),
  `FAIL` (ran, failed), `SKIP` (was fresh).
- `--build-dry-run` (plan 115) gains a per-
  target verdict (`STALE | FRESH`).

### Out of scope

Reverse dependency tracking, watch mode,
cross-machine cache sharing, tool-version
hashing. Parallel builds: plan 116.

## Tasks

1. Extend `RecipeCfg` in `internal/config/`
   with `DefaultInputs []string`. Validate
   each entry is `{param}` (param declared,
   not reserved) or a literal relative path
   with no `..`. Add coverage in MDS040.
2. Implement `internal/build/cache.go`:
   load/save `.mdsmith/build-cache.json`,
   atomic write via temp+rename, version
   field, lookup by sorted output-set key.
3. Implement `internal/build/staleness.go`:
   resolve directive `inputs` ∪ recipe
   `default-inputs`, expand globs, compute
   the length-framed ActionID, check output
   presence and content hash, return
   `STALE | FRESH | ERROR` per target.
   On rebuild, hash each output and store
   in the cache entry.
4. Detect any overlap across declared
   `outputs:` paths — exact path collisions
   and directory-prefix collisions
   (`book/` vs `book/index.html`). Report a
   clear error naming both source locations;
   do not run either recipe.
5. Wire staleness into the `mdsmith fix`
   build pass (plan 115). Default skips
   fresh; refresh cache entries for rebuilt
   targets; atomic cache write at the end of
   the run. Per-target summary gains `SKIP`.
6. Add flags `--build-force`,
   `--build-check-stale`, `--build-no-cache`.
   Update `--build-dry-run` (plan 115) to
   print `STALE | FRESH` per target.
7. Integration tests:

  - `cp`-based recipe with `inputs:
    [src.txt]` skips on second `fix` run;
    rebuilds when `src.txt` content changes.
  - Touching `src.txt` mtime without changing
    content does not trigger a rebuild.
  - Editing the recipe `command` in
    `.mdsmith.yml` triggers a rebuild for
    all targets using it.
  - A two-output directive rebuilds when
    either output is deleted from disk.
  - A glob `inputs:` entry that matches zero
    files is a build error.
  - Overlapping `outputs:` paths (exact
    duplicates or directory-prefix
    collisions) is a build error.
  - `--build-force` rebuilds even when fresh.
  - `--build-check-stale` exits non-zero
    with stale output and zero with fresh
    output; no recipe runs.
  - `--build-no-cache` rebuilds everything
    and writes nothing to cache.

8. Document the staleness model and cache
   file in `docs/guides/directives/build.md`.
   Add the `.mdsmith/` ignore snippet to the
   README and to a future `mdsmith init`.

## Acceptance Criteria

- [ ] A second `mdsmith fix` with no source
      changes runs zero recipes
- [ ] Editing a declared input triggers a
      rebuild of just that target
- [ ] Touching mtime without content change
      does not trigger a rebuild
- [ ] Deleting any declared output triggers a
      rebuild of that target
- [ ] Editing a recipe `command` invalidates
      every target using that recipe
- [ ] An `inputs:` glob matching zero files
      is a build error
- [ ] Overlapping `outputs:` paths (exact
      or directory-prefix) is a build error
      reporting both source locations
- [ ] A recipe's `default-inputs` are folded
      into the input hash
- [ ] `mdsmith fix --build-force` rebuilds
      every target
- [ ] `mdsmith fix --build-check-stale`
      prints stale targets and exits non-zero
      without running any recipe
- [ ] `mdsmith fix --build-no-cache` rebuilds
      everything and writes nothing to
      `.mdsmith/build-cache.json`
- [ ] `mdsmith fix --build-dry-run` prints
      every target's `STALE | FRESH` verdict
- [ ] Per-target summary distinguishes `OK`,
      `FAIL`, and `SKIP`
- [ ] `.mdsmith/build-cache.json` has a
      `version` field and per-target entries
      with `outputs[]` (path + content hash),
      `action-id`, `built-at`, `inputs`,
      `recipe`
- [ ] Hand-editing an artifact triggers a
      rebuild on the next `fix` run (output
      content hash mismatch)
- [ ] ActionID computation is length-framed:
      a path containing NUL or a sentinel
      byte does not collide with another
      input set
- [ ] Cache writes are atomic (temp+rename);
      a mid-build crash leaves the previous
      cache readable
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no
      issues
