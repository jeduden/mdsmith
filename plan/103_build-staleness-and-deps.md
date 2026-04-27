---
id: 103
title: Build target staleness and dependency tracking
status: "🔲"
summary: >-
  Make `<?build?>` Make/Bazel-style: declare inputs
  per directive, hash them with the recipe spec,
  cache the hash per output, and rebuild only stale
  targets. Adds `--force` and `--check-stale` flags
  to `mdsmith build`. Without this, "build system"
  is aspirational; every run does a full rebuild.
model: opus
---
# Build target staleness and dependency tracking

## Goal

`mdsmith build` rebuilds only targets whose inputs
or recipe spec have changed since the last
successful build. Authors declare inputs per
directive; mdsmith computes a content hash and
stores it per output. The next run skips
up-to-date targets.

## Context

Plan 102 adds `mdsmith build` that always
rebuilds every target. For doc trees with many
recipes (every screenshot, every diagram, every
generated table), this wastes time and produces
noisy git diffs from regenerated artifacts whose
inputs did not change.

The article behind this PR
([bgslabs.org/blog/why-are-we-using-markdown][post])
calls out the missing piece directly. It frames
the build-system semantic as "sources update →
result must update". Plans 100–102 give the
shape. They lack the behavior.

[post]: https://bgslabs.org/blog/why-are-we-using-markdown
[plan-100]: 100_build-config-and-mds040.md
[plan-101]: 101_build-directive-mds039.md
[plan-102]: 102_build-subcommand.md

## Design

### Why content hash, not mtime

Git checkouts touch file mtimes. CI runners often
do not preserve mtime across cache restore. Two
checkouts of the same commit on different
machines disagree on mtime but agree on content.
Hash gives reproducible "did this actually
change" answers; mtime does not. Cost is one
read per declared input per `mdsmith build`
invocation; for typical doc trees (hundreds of
small files), negligible.

### `inputs` directive parameter

New optional parameter on `<?build?>`:

```text
<?build
recipe: pandoc
inputs:
  - chapters/intro.md
  - chapters/01-*.md
output: book.html
?>
[book.html](book.html)
<?/build?>
```

`inputs` is a list of paths or globs, each
resolved relative to the Markdown file
containing the `<?build?>` directive (matching
how plan 101 resolves `output`). Globs are
evaluated at build time, expanded to the
matching set, sorted, and folded into the
hash. An `inputs` entry that resolves to zero
files is a build error (likely a typo).

### Recipe-default inputs

Recipes may declare implicit inputs in
`build.recipes.NAME.default-inputs`. The token
`{param}` expands to the directive's value for
that param. Resolution is the same as for
directive `inputs`: relative to the Markdown
file containing the `<?build?>` directive.

Built-in defaults:

| Recipe       | `default-inputs`               |
|--------------|--------------------------------|
| `vhs`        | `[{input}]` (the `.tape` file) |
| `screenshot` | (none)                         |

For `screenshot`, the source is a live URL —
mdsmith cannot tell whether the page changed
without fetching it. The author opts in by
adding `inputs:` (e.g. when the page is built
from local files served by a static dev server).
With no inputs, `screenshot` targets are
considered always-stale and always rebuild.

### Staleness check

For each target, in order:

1. If `output` does not exist → stale.
2. Resolve `inputs` (directive `inputs` ∪
   recipe `default-inputs`); if any non-glob
   entry is missing → build error.
3. Compute `hash = sha256(spec ‖ each input
   content)` where `spec` is the canonical
   serialization of `(recipe, command-template,
   sorted directive params, sorted resolved
   input paths)`.
4. Look up `output` in the cache. If absent or
   stored hash differs → stale.
5. Otherwise → up-to-date; skip the recipe.

### Cache file

Stored at `.mdsmith/build-cache.json`. JSON,
human-inspectable:

```json
{
  "version": 1,
  "entries": {
    "book.html": {
      "hash": "sha256:...",
      "built-at": "2026-04-27T12:00:00Z",
      "inputs": [
        "chapters/intro.md",
        "chapters/01-prologue.md"
      ]
    }
  }
}
```

`output` paths are stored relative to the project
root. `inputs` are stored post-glob-expansion and
sorted, so a reviewer can see exactly what the
build considered.

The directory `.mdsmith/` is added to a
recommended `.gitignore` snippet in the user
guide; the file is per-clone state, like
`node_modules/`.

### Flags

| Flag            | Behavior                                                                               |
|-----------------|----------------------------------------------------------------------------------------|
| (none)          | Default: rebuild only stale targets; refresh cache entries for rebuilt targets         |
| `--force`       | Rebuild every target regardless of cache; refresh all cache entries                    |
| `--check-stale` | Print every stale target, exit non-zero if any are stale, do not rebuild (CI gate use) |
| `--no-cache`    | Treat all targets as stale, do not read or write the cache (debugging)                 |

`--check-stale` makes "every artifact is up to
date with its source" a CI signal a reviewer can
trust, parallel to "every generated section is
fresh" today.

### Cache invalidation

The cache key includes the recipe `command`
template, so changing a recipe in `.mdsmith.yml`
invalidates every target using it. The
`build-cache.json` `version` field exists so a
future mdsmith release can rev it (e.g. switching
hash algorithm) and force a single rebuild
without crashing on stale schemas.

### Interaction with plans 100–102

- [Plan 100][plan-100]: extends `RecipeCfg` with
  `default-inputs`. MDS040 validates each entry
  is a `{param}` token referring to a declared
  param, or a literal path with no `..`.
- [Plan 101][plan-101]: extends MDS039 to accept
  the optional `inputs` directive parameter.
  Validates each entry is a relative path (no
  absolute paths, no `..`).
- [Plan 102][plan-102]: `mdsmith build` reads
  and writes the cache file, runs the staleness
  check before invoking each Builder.

### Out of scope

- Reverse dependency tracking ("rebuild B when A
  rebuilds because B includes A"). One target →
  one build; declared `inputs` are the only edge.
- Parallel builds. Sequential is fine for
  documentation-scale work; revisit only if real
  use proves it slow.
- Watch mode (`mdsmith build --watch`). Use the
  shell or an editor task runner for now.
- Cross-machine cache sharing (Bazel-style
  remote cache). The `build-cache.json` is
  intentionally local.

## Tasks

1. Extend `RecipeCfg` in `internal/config/` with
   `default-inputs`. Validate each entry is
   either `{param}` (param must be declared in
   `params.required` or `params.optional`) or a
   relative path with no `..`. Add coverage in
   MDS040.
2. Extend MDS039 to accept the optional `inputs`
   directive parameter (list of strings).
   Validate each entry is a relative path with
   no `..`. Update fixtures.
3. Implement `internal/build/cache.go`:
   load/save `.mdsmith/build-cache.json`,
   atomic write via temp+rename.
4. Implement `internal/build/staleness.go`:
   resolve directive `inputs` ∪ recipe
   `default-inputs`, expand globs, compute
   `sha256(spec ‖ inputs)`, compare against
   cached hash, return stale/fresh.
5. Wire staleness into `mdsmith build`:
   default skips fresh, refreshes cache for
   rebuilt targets, atomic cache write at the
   end of the run.
6. Add flags `--force`, `--check-stale`,
   `--no-cache` with the behavior above.
7. Built-in `vhs` declares `default-inputs:
   [{input}]`. Built-in `screenshot` declares
   none (always-stale by default).
8. Integration tests:

  - A `cp`-based recipe with `inputs: [src.txt]`
     skips on second run; rebuilds when `src.txt`
     content changes.
  - Touching `src.txt` mtime without changing
     content does not trigger a rebuild.
  - Changing the recipe `command` in
     `.mdsmith.yml` triggers a rebuild for all
     targets using that recipe.
  - `--force` rebuilds even when fresh.
  - `--check-stale` exits non-zero with stale
     output and zero with fresh output.

9. Document the staleness model and cache file
   in `docs/guides/directives/build.md`. Add the
   `.mdsmith/` ignore snippet to the README and
   to a future `mdsmith init`.

## Acceptance Criteria

- [ ] A second `mdsmith build` invocation with
      no source changes runs zero recipes
- [ ] Changing the content of a declared input
      triggers a rebuild of just that target
- [ ] Touching mtime without changing content
      does not trigger a rebuild
- [ ] Changing a recipe `command` in
      `.mdsmith.yml` invalidates every target
      using that recipe
- [ ] An `inputs:` glob that matches zero files
      is a build error
- [ ] `screenshot` targets without `inputs:` are
      always rebuilt
- [ ] `vhs` targets default to `inputs:
      [{input}]` and skip when the `.tape` file
      is unchanged
- [ ] `mdsmith build --force` rebuilds every
      target regardless of cache
- [ ] `mdsmith build --check-stale` prints stale
      targets and exits non-zero without
      rebuilding
- [ ] `mdsmith build --no-cache` rebuilds
      everything and writes nothing to
      `.mdsmith/build-cache.json`
- [ ] `.mdsmith/build-cache.json` is JSON with a
      `version` field and per-output entries
      including `hash`, `built-at`, and resolved
      `inputs`
- [ ] Cache writes are atomic (temp+rename); a
      mid-build crash leaves the previous cache
      readable
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no
      issues
