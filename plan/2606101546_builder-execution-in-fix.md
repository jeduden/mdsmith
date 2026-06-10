---
id: 2606101546
title: Builder execution wired into `mdsmith fix`
status: "🔲"
summary: >-
  Run `<?build?>` directives during `mdsmith fix`.
  A `Builder` interface dispatches the
  user-declared recipe via `os/exec` and writes
  every declared output atomically. No built-in
  recipes; no `mdsmith build` subcommand. New
  flags: `--no-build`, `--build-only`,
  `--build-recipe`, `--build-dry-run`,
  `--build-timeout`. Removes `build.base-url`.
model: opus
depends-on: [102]
---
# Builder execution wired into `mdsmith fix`

## Goal

`mdsmith fix` runs the build pass after the
existing lint-fix pass. Each `<?build?>`
directive dispatches its user-declared recipe
via `os/exec` and writes the declared
artifacts atomically. A per-target
`OK | FAIL` summary is printed. `mdsmith
check` stays lint-only.

## Context

Builds on plan 102 (directive params) and
plan 100 (config). Plan 103 layers
staleness on top. Plan 104 adds hooks. Plan
2606101547 covers stdout/stderr UX and parallel
execution.

The original 102 plan included a separate
`mdsmith build` subcommand and two built-in
recipe drivers (`screenshot` via chromedp,
`vhs`). All are removed: builds run inside
`mdsmith fix`; every recipe lives in
`build.recipes`. `build.base-url` is also
removed (its only consumer was the deleted
screenshot driver).

## Design

### Builder interface

```go
// internal/build/builder.go
type Builder interface {
    Build(ctx context.Context, target Target) error
}

type Target struct {
    Recipe  string
    Params  map[string]string
    Root    string    // absolute project root
    Inputs  []string  // project-root-relative, slash-normalized
    Outputs []string  // project-root-relative, slash-normalized
}
```

The custom-recipe builder is the sole impl.
One instance per recipe in `build.recipes`.
Paths are stored relative so plan 103's
ActionID is stable across clone locations;
absolute paths are recomposed via
`filepath.Join(target.Root, p)` at exec time.

### Argv tokenization

Tokenization uses `strings.Fields` on
`command` at config load. Substitution of
`{param}`, `{inputs}`, `{outputs}` happens
*after* tokenization, so a param value
containing whitespace stays a single argv
entry. A value like `foo; rm -rf /` is
passed literally as one argument.

### Atomic multi-output write

The basic contract (plan 2606101548 hardens it):

1. mdsmith creates a per-target temp dir.
2. The recipe is invoked with the staging
   paths substituted for `{outputs}`; named
   params pass through verbatim (plan 102).
3. On success, mdsmith renames every temp
   file to its final location.
4. On failure, the temp dir is removed; no
   declared output is touched.

Plan 2606101548 adds the security hardening.
That covers the trust gate, hermetic env,
hardened staging dir, output post-
conditions, and process-group kill on
timeout.

### Wiring into `mdsmith fix`

`mdsmith fix [paths] [flags]` gains a build
pass after the existing lint-fix pass:

1. Lint-fix pass runs (rules apply fixes,
   generated sections — including each
   `<?build?>` body — get re-rendered).
2. Build pass runs:

  - Collect all `<?build?>` directives in
    file order, reusing the lint pass's
    directive walk — no second workspace
    walk.
  - For each, dispatch to the recipe's
    `Builder.Build`.
  - Write outputs atomically.
  - Append a `OK | FAIL` line per target.

3. Final exit code is non-zero on any rule
   error or any build failure.

The build pass runs after lint-fix so that a
freshly-edited `outputs:` list is built with
its new value, not the old.

### CLI-only surface

The build pass lives in `cmd/mdsmith` and
`internal/build/`. It is not part of the
public `pkg/mdsmith` Session API and is
excluded from the WASM bindings (plan 215):
recipes exec processes, which the WASM and
LSP in-memory fix paths must never do. The
in-process fix API (`fix.Source`) has no
build stage, which exempts the LSP and the
merge driver by construction. The
pre-merge-commit hook script still runs the
CLI, so task 4 wires `--no-build` into its
generator (`internal/githooks`). A merge
never executes recipes.

### `mdsmith fix` flags

| Flag                  | Behavior                                                       |
| --------------------- | -------------------------------------------------------------- |
| (none)                | Lint-fix pass + build pass                                     |
| `--no-build`          | Lint-fix pass only                                             |
| `--build-only`        | Build pass only                                                |
| `--build-recipe NAME` | Only build directives with `recipe: NAME`                      |
| `--build-dry-run`     | Enumerate targets (and hooks once plan 104 lands); run nothing |
| `--build-timeout DUR` | Per-recipe timeout (default `30s`)                             |

`--no-build` and `--build-only` are mutually
exclusive. The `--build-*` prefix groups
build flags and avoids collision with future
lint-fix flags.

The same pattern governs later build
flags. Inspect modes run no recipe:
`--build-dry-run`, later
`--build-check-stale` and
`--build-explain`. They exclude each other
and the execution-altering `--build-force`,
`--build-no-cache`, and `--build-verify`;
combining them is a usage error. Tuning
flags like
`--build-timeout` or `--build-jobs`
combine freely with a run.

## Tasks

1. Define `Builder` and `Target` in a new
   package `internal/build/`. Implement the
   custom-recipe builder. Tokenize via
   `strings.Fields`; substitute `{param}`,
   `{inputs}`, `{outputs}` after. Resolve
   `inputs:` globs with the doublestar
   matcher, re-check every resolved path
   against the project root through plan
   102's shared validator, and enforce the
   10 000-match glob cap there.
2. Implement basic multi-output atomic
   write: per-target temp dir, post-recipe
   rename, full rollback on failure. Plan
   2606101548 adds the hardening.
3. Remove `build.base-url` from
   `internal/config/build.go` and its
   tests. Add an explicit top-level-key
   scan that errors if `build.base-url`
   is still present (non-strict YAML would
   otherwise ignore it silently). Drop
   docs that reference it.
4. Wire the build pass into `mdsmith fix`
   in `cmd/mdsmith/`. Add the five flags
   above. Print per-target summary;
   non-zero exit on failure. Keep the pass
   out of `pkg/mdsmith` and `fix.Source`.
   Pass `--no-build` in the
   `internal/githooks` script (regenerate
   its golden file).
5. Integration tests:

  - `cp`-based single-output recipe runs
    via `fix`.
  - `tee`-based multi-output recipe writes
    both files atomically.
  - Failing recipe leaves no partial
    outputs.
  - All five `--build-*` flags work as
    documented.
  - `mdsmith check` runs no recipe.

6. Update `docs/guides/directives/build.md`:
   document the build pass, the new `fix`
   flags, the per-target summary, and
   `{outputs}` / `{inputs}` argv expansion.
   Remove references to `mdsmith build`,
   built-in recipes, and `base-url`. Add a
   markdown-as-data example: a recipe runs
   `mdsmith extract` on an input file and
   pipes the JSON into a chart tool. Note
   in `docs/reference/telemetry.md` that
   recipes are user code; mdsmith itself
   still makes no network calls.
7. Update `demo.tape` to use a `cp`-based
   custom recipe so the demo shows `fix`
   running a build.

## Acceptance Criteria

- [ ] `mdsmith fix` runs the build pass
      after lint-fix; `mdsmith check` runs
      no recipe
- [ ] All five `--build-*` flags work as
      documented; `--no-build` and
      `--build-only` are mutually exclusive
- [ ] Single-output and multi-output `cp`/
      `tee` recipes write every declared
      output via one invocation
- [ ] A failing recipe leaves no partial
      output; pre-existing outputs survive
- [ ] Custom `command` is dispatched via
      `os/exec` with explicit argv; no
      shell is invoked
- [ ] `{outputs}` and `{inputs}` each
      expand to one argv per resolved entry
- [ ] A resolved input escaping the project
      root, or one glob matching more than
      10 000 files, is a build error
- [ ] Tokenization uses `strings.Fields`;
      param values containing whitespace
      pass as one argv entry
- [ ] `mdsmith fix` exits non-zero on any
      recipe failure with per-target
      `OK | FAIL` summary
- [ ] `build.base-url` is removed. Config
      loading uses non-strict YAML
      (`yamlutil.UnmarshalSafe`), so the
      struct field's absence alone is
      silent. An explicit top-level-key
      scan detects a lingering
      `build.base-url` and errors with
      "build.base-url was removed in plan
      2606101546; delete it"
- [ ] No built-in recipes ship; an unknown
      `recipe:` is a lint error (MDS039)
- [ ] `pkg/mdsmith`, the WASM bindings, LSP
      fix, the merge driver, and the
      pre-merge-commit hook never run the
      build pass
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no
      issues
