---
id: 115
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

Builds on plan 102 (`outputs:` / `inputs:`
directive params) and plan 100 (`build:` config,
MDS040). Plan 103 layers staleness on top.
Plan 104 adds before / after hooks.

The original 102 plan proposed a separate
`mdsmith build` subcommand. It also shipped
two built-in recipe drivers: `screenshot`
(chromedp) and `vhs`. Both are removed:

- **No built-in recipes.** Every recipe lives
  in `build.recipes`. Plan 101 enforces this
  at lint time; plan 115 enforces it at
  execution time.
- **No `mdsmith build` subcommand.** Builds
  run inside `mdsmith fix`. `fix --build-only`
  runs only the build pass; `fix --no-build`
  skips it.

`build.base-url` is removed from the config
schema. It existed only for the deleted
built-in `screenshot`. Users who need URL
composition do it inside their recipe
`command`.

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
    Inputs  []string  // resolved absolute paths
    Outputs []string  // resolved absolute paths
}
```

The custom-recipe builder is the sole impl. It
is constructed once per recipe in
`build.recipes`. At `Build` time it tokenizes
`command` on whitespace, substitutes `{param}`
tokens (and `{inputs}` / `{outputs}` to N argv
entries), and dispatches via `os/exec.Cmd`. No
shell.

A value like `foo; rm -rf /` in any param is
passed literally as one argument.

### Atomic multi-output write

The contract:

1. mdsmith creates a temp dir per target:
   `<output-dir>/.mdsmith-build-<rand>/`.
2. Each declared output path maps to a file
   inside the temp dir. The recipe is invoked
   with the temp paths substituted for
   `{outputs}` and any output-path params.
3. On recipe success, mdsmith renames every
   temp file to its final location. All
   succeed or none do — partial-failure
   restores from backups.
4. On recipe failure, the temp dir is
   removed. No declared output is touched.

### Wiring into `mdsmith fix`

`mdsmith fix [paths] [flags]` gains a build
pass after the existing lint-fix pass:

1. Lint-fix pass runs (rules apply fixes,
   generated sections — including each
   `<?build?>` body — get re-rendered).
2. Build pass runs:

  - Collect all `<?build?>` directives in file
    order.
  - For each, dispatch to the recipe's
    `Builder.Build`.
  - Write outputs atomically.
  - Append a `OK | FAIL` line per target.

3. Final exit code is non-zero on any rule
   error or any build failure.

The build pass runs after lint-fix so that a
freshly-edited `outputs:` list is built with
its new value, not the old.

### `mdsmith fix` flags

| Flag                  | Behavior                                  |
|-----------------------|-------------------------------------------|
| (none)                | Lint-fix pass + build pass                |
| `--no-build`          | Lint-fix pass only                        |
| `--build-only`        | Build pass only                           |
| `--build-recipe NAME` | Only build directives with `recipe: NAME` |
| `--build-dry-run`     | Enumerate targets; run no recipe          |
| `--build-timeout DUR` | Per-recipe timeout (default `30s`)        |

`--no-build` and `--build-only` are mutually
exclusive. The `--build-*` prefix groups
build flags and avoids collision with future
lint-fix flags.

### Security

Custom recipe `command` values become argv at
config load. `{param}`, `{inputs}`,
`{outputs}` substitutions become individual
`os/exec` args. No shell. MDS040 enforces
this at lint time. Path params are validated
by MDS039 as relative paths with no `..`
before the build runs.

### Removed surface

- `mdsmith build` subcommand (whichever stub
  exists from earlier work).
- Built-in `screenshot` driver and chromedp
  dependency.
- Built-in `vhs` driver.
- `build.base-url` config field.

## Tasks

1. Define `Builder` and `Target` in a new
   package `internal/build/`. Implement the
   custom-recipe builder. Support `{param}`,
   `{inputs}`, `{outputs}` substitutions.
2. Implement multi-output atomic write: temp
   dir, post-recipe rename, full rollback on
   partial failure or non-zero exit.
3. Remove `build.base-url` from
   `internal/config/build.go` and its tests.
   Drop docs that reference it.
4. Wire the build pass into `mdsmith fix` in
   `cmd/mdsmith/`. Add the five flags above.
   Print per-target summary; non-zero exit
   on failure.
5. Integration tests:

  - `cp`-based single-output recipe runs via
    `fix`.
  - `tee`-based multi-output recipe writes
    both files atomically.
  - Failing recipe leaves no partial outputs.
  - `--no-build` skips build pass.
  - `--build-only` skips lint-fix pass.
  - `--build-dry-run` lists targets, runs
    none.
  - `--build-recipe NAME` filters correctly.
  - `--build-timeout 5s` enforces the
    timeout.
  - `mdsmith check` runs no recipe.

6. Update `docs/guides/directives/build.md`:
   document the build pass, the new `fix`
   flags, the per-target summary, and the
   `{outputs}` / `{inputs}` argv expansion.
   Remove all references to `mdsmith build`,
   built-in recipes, and `base-url`.
7. Update `demo.tape` to use a `cp`-based
   custom recipe so the demo shows `fix`
   running a build.

## Acceptance Criteria

- [ ] `mdsmith fix` runs the build pass after
      lint-fix; `mdsmith check` runs no
      recipe
- [ ] `mdsmith fix --no-build` skips the
      build pass
- [ ] `mdsmith fix --build-only` skips the
      lint-fix pass
- [ ] `mdsmith fix --no-build --build-only`
      is an error
- [ ] `mdsmith fix --build-recipe NAME` only
      builds matching directives
- [ ] `mdsmith fix --build-dry-run` lists
      targets without running any recipe
- [ ] `mdsmith fix --build-timeout 5s`
      applies per recipe invocation
- [ ] A `cp`-based recipe writes its single
      declared output via `fix`
- [ ] A multi-output recipe writes every
      declared output via one invocation
- [ ] A failing recipe leaves no partial
      output; pre-existing outputs survive
- [ ] Custom recipe `command` is dispatched
      via `os/exec` with explicit argv; no
      shell is invoked
- [ ] `{outputs}` expands to one argv per
      directive output entry
- [ ] `{inputs}` expands to one argv per
      resolved input entry
- [ ] `mdsmith fix` exits non-zero on any
      recipe failure with per-target
      `OK | FAIL` summary
- [ ] `build.base-url` is removed; a config
      that still sets it errors with
      "unknown field"
- [ ] No built-in recipes ship; an unknown
      `recipe:` is a lint error (MDS039,
      plan 101)
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no
      issues
