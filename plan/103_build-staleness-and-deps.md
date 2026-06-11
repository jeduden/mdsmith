---
id: 103
title: Build target staleness and dependency tracking
status: "🔳"
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
depends-on: [102, 2606101546]
---
# Build target staleness and dependency tracking

## Goal

The build pass inside `mdsmith fix` (plan
2606101546) runs only recipes whose inputs
or recipe spec changed, or whose outputs
are missing. One ActionID per target,
stored in JSON, decides.

## Context

Plan 102 adds `inputs:` and `outputs:`.
Plan 2606101546 wires the build pass into
`mdsmith fix`, rebuilding every target
unconditionally — wasted time, churned diffs.

### Pattern borrowed from `cmd/go/internal/cache`

Go's build cache hashes `(action description ‖
input contents)` into an ActionID. mdsmith
borrows the model; its cache is JSON keyed
by sorted `outputs` set with the ActionID
inside each entry (see "Cache file" below).
Content hashing beats mtime: git checkouts
rarely preserve it.

## Design

### Recipe-default inputs

Recipes may declare implicit inputs in
`build.recipes.NAME.default-inputs` —
literal relative paths or `{param}` tokens:

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

A param named in `default-inputs` is a
path param: its token expands to the
absolute root-joined path at exec time,
like `{inputs}`, so the recipe finds it
from the staging cwd (plan 2606101548).
The ActionID hashes the relative path the
param supplies (`demo.tape`), never the
absolute expansion.

### ActionID

The ActionID is sha256 over these fields,
each prefixed with its 8-byte big-endian
length. Hashed paths are always the
project-root-relative, slash-normalized
form, stable across clones. Symlink
resolution guards against root escape; it
never alters the hashed string. Inputs run
`EvalSymlinks` before hashing; outputs
resolve only the longest existing prefix
(plan 102). Fields, in order:

```text
recipe.command
canonical(directive.params)         (sorted; per pair: len(key)|key|len(value)|value)
canonical(sorted relative inputs)   (per entry: len(path)|path)
concat(sha256(content) per input, same order)
canonical(sorted relative outputs)  (per entry: len(path)|path)
cache.version
```

Every field has an outer 8-byte big-endian
length prefix. Each inner key, value, and
path is itself length-framed; no separator
bytes are used. Two-layer framing prevents
collisions when param keys contain `=` or
`\0` or filenames carry control bytes.

`cache.version` lets a future release rev
the schema and force a single rebuild.

### Staleness check

Per target, in order:

1. Resolve `inputs` (directive `inputs` ∪
   recipe `default-inputs`); if any non-glob
   entry is missing → build error.
2. If any entry in `outputs` does not exist
   on disk → stale.
3. Compute the ActionID.
4. Look up the entry by sorted-output-set
   key; absent or different ActionID → stale.
5. Hash each declared output; mismatch with
   the entry's stored hash → stale
   (tampered or regenerated externally).
6. Otherwise → fresh; skip the recipe.

Step 5 makes the cache *advisory*: it
catches poisoned or hand-edited entries.

A target is identified in the cache by its
sorted `outputs` list, length-framed and
joined. Any overlap across two directives'
`outputs:` paths — exact or directory-prefix
(`book/` vs `book/index.html`) — is a build
error reporting both source locations.
Plan 2606101547 reuses the rule for
parallel safety.

### Cache file

Stored at `.mdsmith/build-cache.json`: a
top-level `version`, then one entry per
target with:

- `outputs[]`: `{path, hash}` pairs sorted
  by path; `hash` is the artifact's sha256
  at build time (staleness step 5).
- `inputs[]`: sorted post-glob paths;
  informational (ActionID covers content).
- `action-id`: the ActionID serialized as
  `sha256-<64 lowercase hex>`; stored as
  entry metadata (*not* the JSON map key)
  and used as the log filename stem.
- `recipe`, `built-at`: informational; not
  in the ActionID, not consulted.

All paths are stored relative to the project
root.

Cache writes are atomic (temp + rename). A
mid-build crash leaves the previous cache
readable.

The recommended `.gitignore` snippet:

```text
.mdsmith/build-cache.json
.mdsmith/build-logs/
.mdsmith/build-staging/
```

Never ignore the whole `.mdsmith/` dir.
Its other folders (`kinds/`, `schemas/`,
`conventions/`) are checked-in config.

### Flags on `mdsmith fix`

Extends plan 2606101546's build-pass flag set:

| Flag                  | Behavior                                                                |
| --------------------- | ----------------------------------------------------------------------- |
| (none)                | Build only stale targets; refresh cache for rebuilt                     |
| `--build-force`       | Build every target; refresh all cache entries                           |
| `--build-check-stale` | Print every stale target, exit non-zero if any are stale, run no recipe |
| `--build-no-cache`    | Treat all targets as stale; do not read or write the cache (debugging)  |

`--build-check-stale` makes artifact
freshness a CI signal. The lint-fix pass
still runs unless combined with
`--build-only`. `--build-force` combined
with `--build-check-stale` or
`--build-no-cache` is a usage error.

### Interaction with plan 2606101546

- Staleness runs before `Builder.Build`;
  fresh targets are skipped silently.
- Per-target summary: `OK` (succeeded),
  `FAIL`, `SKIP` (was fresh).
- `--build-dry-run` gains a `STALE | FRESH`
  verdict per target.

### Out of scope

Reverse dependency tracking, watch mode,
cross-machine cache sharing, tool-version
hashing. Parallel builds: plan 2606101547.

## Tasks

1. Extend `RecipeCfg` in `internal/config/`
   with `DefaultInputs []string`: each entry
   `{param}` (declared, not reserved) or a
   literal relative path passing plan 102's
   path-shape rules. Cover in MDS040.
2. Implement `internal/build/cache.go`:
   load/save `.mdsmith/build-cache.json`,
   atomic write via temp+rename, version
   field, lookup by sorted output-set key.
3. Implement `internal/build/staleness.go`:
   resolve directive `inputs` ∪ recipe
   `default-inputs`, expand directive globs
   through plan 2606101546's resolver
   (`default-inputs` stay literal), compute
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
   build pass (plan 2606101546). Default skips
   fresh; refresh cache entries for rebuilt
   targets; atomic cache write at the end of
   the run. Per-target summary gains `SKIP`.
6. Add flags `--build-force`,
   `--build-check-stale`, `--build-no-cache`.
   Update `--build-dry-run` (plan 2606101546) to
   print `STALE | FRESH` per target.
7. Integration tests:

  - A `cp` recipe with `inputs: [src.txt]`
    skips on the second run; rebuilds when
    content changes.
  - Touching `src.txt` mtime without changing
    content does not trigger a rebuild.
  - Editing the recipe `command` triggers a
    rebuild for all targets using it.
  - A two-output directive rebuilds when
    either output is deleted from disk.
  - A glob `inputs:` entry that matches zero
    files is a build error.
  - Overlapping `outputs:` paths (exact or
    directory-prefix) is a build error.
  - `--build-force` rebuilds even when fresh.
  - `--build-check-stale` exits non-zero
    when stale, zero when fresh; no recipe
    runs.
  - `--build-no-cache` rebuilds everything
    and writes nothing to cache.

8. Document the staleness model and cache
   file in `docs/guides/directives/build.md`;
   add the build-state ignore snippet to the
   README and a future `mdsmith init`.

## Acceptance Criteria

- [x] A second `mdsmith fix` with no source
      changes runs zero recipes
- [x] Editing a declared input triggers a
      rebuild of just that target
- [x] Touching mtime without content change
      does not trigger a rebuild
- [x] Deleting any declared output triggers a
      rebuild of that target
- [x] Editing a recipe `command` invalidates
      every target using that recipe
- [x] An `inputs:` glob matching zero files
      is a build error
- [x] Overlapping `outputs:` paths (exact
      or directory-prefix) is a build error
      reporting both source locations
- [x] A recipe's `default-inputs` are folded
      into the input hash
- [x] `mdsmith fix --build-force` rebuilds
      every target
- [x] `mdsmith fix --build-check-stale`
      prints stale targets and exits non-zero
      without running any recipe
- [x] `mdsmith fix --build-no-cache` rebuilds
      everything; writes nothing to the cache
- [x] `mdsmith fix --build-dry-run` prints
      every target's `STALE | FRESH` verdict
- [x] Per-target summary distinguishes `OK`,
      `FAIL`, and `SKIP`
- [x] `.mdsmith/build-cache.json` has a
      `version` field and per-target entries
      with `outputs[]` (path + content hash),
      `action-id`, `built-at`, `inputs`,
      `recipe`
- [x] Hand-editing an artifact triggers a
      rebuild on the next `fix` (hash
      mismatch)
- [x] ActionID is length-framed: paths with
      NUL or sentinel bytes cannot collide
      with another input set
- [x] Cache writes are atomic (temp+rename);
      a mid-build crash leaves the previous
      cache readable
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no
      issues
