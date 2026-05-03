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
runs only the recipes whose declared inputs or
recipe spec have changed, or whose declared
outputs are missing. mdsmith hashes one
ActionID per target and stores it in JSON. The
next `fix` run skips fresh targets.

## Context

Plan 102 adds `inputs:` and `outputs:` to the
`<?build?>` directive. Plan 115 wires the
build pass into `mdsmith fix` and always
rebuilds every target. For doc trees with many
recipes this wastes time and floods git diffs
with regenerated artifacts whose inputs did
not change.

[plan-100]: 100_build-config-and-mds040.md
[plan-102]: 102_build-subcommand.md
[plan-115]: 115_builder-execution-in-fix.md

### Pattern borrowed from `cmd/go/internal/cache`

Go's build cache hashes
`(action description ‖ input contents)` into
an ActionID and keys results by it. The
package is `internal/`, but the model fits
mdsmith: inputs are content-addressed (not
mtime), recipe edits invalidate every target
using that recipe, and the cache is a flat
`ActionID → outputs` map. mdsmith mirrors
this with a JSON file instead of a content-
addressed store — simpler, and we have
hundreds of targets, not millions. Content
hashing also wins over mtime because git
checkouts and CI cache restores rarely
preserve mtimes.

## Design

### Recipe-default inputs

Recipes may declare implicit inputs in
`build.recipes.NAME.default-inputs`. Each
entry is either a literal relative path or a
`{param}` token that expands to the
directive's value for that param.

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
`{ demo.tape } ∪ directive.inputs`.

`default-inputs` lets recipes encode "the
recipe's source file is implicitly an input"
so authors do not have to restate it.

### ActionID

For each target, the ActionID is:

```text
sha256(
  recipe.command ‖
  sorted(directive.params) ‖
  sorted(resolved input paths) ‖
  concat(sha256(content) for each resolved input) ‖
  sorted(resolved output paths) ‖
  cache.version
)
```

`recipe.command` is the unparsed command-
template string (so renaming a flag
invalidates the cache). `directive.params` is
sorted by key. Resolved input paths are
post-glob-expansion and sorted. Output paths
are sorted for determinism even though
declared as a list.

`cache.version` lets a future mdsmith release
rev the schema and force a single rebuild
without crashing on stale entries.

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
5. Otherwise → fresh; skip the recipe.

A target is identified in the cache by its
sorted `outputs` list joined with `\0`. Two
directives declaring the same output set is a
build error reported with both source
locations; neither recipe runs.

### Cache file

Stored at `.mdsmith/build-cache.json`:

```json
{
  "version": 1,
  "entries": [
    {
      "outputs": ["book.epub", "book.html"],
      "action-id": "sha256:...",
      "built-at": "2026-04-27T12:00:00Z",
      "inputs": [
        "chapters/01-prologue.md",
        "chapters/intro.md"
      ],
      "recipe": "pandoc"
    }
  ]
}
```

`outputs` and `inputs` are stored relative to
the project root, sorted, and post-glob-
expansion. `recipe` is informational only.

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
  before invoking `Builder.Build`. Fresh
  targets are skipped silently.
- Per-target summary: `OK` (recipe ran and
  succeeded), `FAIL` (recipe ran and failed),
  `SKIP` (target was fresh).
- `--build-dry-run` (plan 115) gains a
  staleness verdict per target
  (`STALE | FRESH`).

### Out of scope

Reverse dependency tracking, watch mode, and
cross-machine cache sharing. Tool-version
hashing is also out — users who care should
bake a version tag into their `command`.
Parallel builds are tracked separately.

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
   ActionID, check output presence, return
   `STALE | FRESH | ERROR` per target.
4. Detect duplicate-output-set targets:
   report a clear error naming both source
   locations; do not run either recipe.
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
  - Two directives with the same `outputs:`
    set is a build error naming both
    locations.
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
- [ ] Two directives with the same `outputs:`
      set is a build error reported with both
      source locations and no recipe runs
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
      with `outputs`, `action-id`,
      `built-at`, `inputs`, `recipe`
- [ ] Cache writes are atomic (temp+rename);
      a mid-build crash leaves the previous
      cache readable
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no
      issues
