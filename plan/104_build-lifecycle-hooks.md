---
id: 104
title: Build lifecycle hooks (before/after)
status: "🔲"
summary: >-
  Add `build.hooks.before` and `build.hooks.after`
  config blocks: argv-tokenized commands run once
  per `mdsmith fix` build pass, around the recipe
  pass. Lets users start a dev server before
  screenshots and stop it after, or warm a cache
  before generating diagrams. Same `os/exec` argv
  path and MDS040 lint as recipes — no shell.
model: sonnet
depends-on: [2606101546, 103]
---
# Build lifecycle hooks (before/after)

## Goal

The `mdsmith fix` build pass (plan 2606101546) runs
declared `before` commands once before any
recipe and declared `after` commands once
after the recipe pass. Failure semantics are
explicit and CI-friendly.

## Context

Plan 2606101546 ships the build pass inside `mdsmith
fix`; plan 103 adds staleness. Neither
provides setup/teardown lifecycle. The
motivating example is a user-declared
screenshot recipe that needs a dev server
running. Hooks fold "start server, run
recipes, stop server" into one command.

## Design

### Config schema

Extend the `build:` block from plan 100:

```yaml
build:
  hooks:
    before:
      - command: "make dev-server-start"
      - command: "scripts/wait-for-port {port}"
        params:
          port: "3000"
    after:
      - command: "make dev-server-stop"
```

Each hook entry shape:

| Field     | Required | Description                                    |
| --------- | -------- | ---------------------------------------------- |
| `command` | yes      | Argv template; same `{param}` rules as recipes |
| `params`  | no       | Map of param name → literal value              |
| `name`    | no       | Display name (defaults to first token)         |

Hooks have no directive surface. They are a
config-level construct, run once per `fix`
build pass, not per directive.

### Execution order

```text
1. Lint-fix pass                   (existing fix behavior)
2. before[0], before[1], …          (in order, plan 104)
3. recipe pass                       (plan 2606101546)
4. after[0], after[1], …             (in order, plan 104)
```

Hooks are part of the build pass.
`--no-build` (plan 2606101546) skips the build pass
and therefore both hook lists. `--build-only`
(plan 2606101546) skips step 1 (lint-fix) but still
runs steps 2–4 in order — hooks bracket the
recipe pass either way.

### Failure semantics

- **`before` fails** (non-zero exit): print
  stderr and exit code, run no recipes, run
  no `after` hooks. The lint-fix pass already
  ran; its results stand.
- **Recipe fails**: finish the recipe pass
  per plan 2606101546's `OK | FAIL` summary (plan
  103 adds `SKIP` once staleness lands),
  then run `after` hooks. Final exit non-zero.
- **`after` fails**: print stderr and exit
  code, continue running remaining `after`
  hooks. Final exit non-zero.
- **Multiple failures** — final exit code
  priority: lint-fix errors → `before-fail` →
  recipe-fail → `after-fail` → 0.

The asymmetry is intentional. A failed
`before` means setup is incomplete. Recipes
would produce garbage, so abort. A failed
`after` means teardown is broken. Artifacts
are written; report and exit non-zero.

### Argv expansion

Same as recipes (plan 100):

1. Split `command` on whitespace.
2. Expand `{param}` tokens using the hook's
   `params` map.
3. Pass the resulting argv to `os/exec.Cmd`.
   No shell.

A `{param}` token with no matching `params`
entry is a config error caught by MDS040. The
reserved-name list (`inputs`, `outputs`, plan
102) applies: a hook's `params` may not
declare them, and a hook's `command` may not
reference `{inputs}` or `{outputs}` (hooks
have no directive context).

### MDS040 extension

Extend MDS040 (plan 100) to lint hook
`command` strings with the same rules as
recipes:

- Non-empty.
- First token is not a shell interpreter.
- No shell operators in static parts.
- No fused `{param}` placeholders.
- No `..` in the executable token.
- Reserved names (`inputs`, `outputs`) absent.

A hook `params` entry must be referenced by
at least one `{param}` token in its
`command`; unused params are a warning.

### Hook param value validation

Hook `params` values are pure config strings.
MDS040's path-shape checks apply only to the
executable token, not to substituted values,
so `params: { target: "../../etc/shadow" }`
with `command: "cat {target}"` slips through
without further checks.

MDS040 enforces a baseline on every hook
param value: no NUL byte, no newline or
carriage return, no leading or trailing
whitespace, length ≤ 4 KB. Operators who
need stricter checks (port range, URL shape,
project-relative paths) wrap the binary in a
script that does its own validation.

The baseline is intentionally narrow.
Per-kind value schemas (`kind: path | port
| url`) are a future extension; for now
the bar is "no control characters in argv"
and operators keep flexibility.

### Execution gate

The build pass refuses to run if MDS040
emits any error against `build.hooks` or
`build.recipes`. A lint-clean config is a
precondition for executing any user-declared
binary. `--no-build` still works for
debugging without the gate.

### Flags on `mdsmith fix`

| Flag                            | Behavior                                                       |
| ------------------------------- | -------------------------------------------------------------- |
| `--no-build`                    | (plan 2606101546) Skip the entire build pass — including hooks |
| `--build-no-hooks`              | Run the build pass but skip both `before` and `after` hooks    |
| `--build-skip-hooks-when-fresh` | Skip both lists when no target is stale; run them otherwise    |

`--build-recipe NAME` (plan 2606101546) does not
filter hooks — they are global.
`--build-dry-run` (plan 2606101546) lists hooks
alongside recipes; nothing executes.
`--build-check-stale` (plan 103) also runs
no hooks.

### Interaction with staleness (plan 103)

If every target is up-to-date and would be
skipped, `before` and `after` still run by
default — they may have effects beyond the
recipes (publishing, notifications). To skip
them when nothing would build, use
`--build-skip-hooks-when-fresh`. The naming is
intentional: the default favors
predictability.

### Out of scope

Per-recipe hooks. Separate hook timeouts.
Conditional `if:` clauses. Per-kind param
schemas. Background hooks — `before`
returns synchronously; spawn-then-kill
via PID file.

## Tasks

1. Extend `BuildConfig` in `internal/config/`
   with `Hooks HooksCfg`. Define `HooksCfg`
   with `Before []HookCfg` and `After []HookCfg`.
   Validate each `HookCfg` like
   `RecipeCfg.command`, including the
   reserved-name list from plan 102.
2. Extend MDS040 to lint hook `command`
   strings using the existing rule set. Add
   fixtures covering shell interpreter,
   shell operator, fused-placeholder, and
   reserved-name cases.
3. Add `internal/build/hooks.go`: a
   `runHooks` helper that takes a list of
   `HookCfg`, tokenizes and expands each,
   dispatches via `os/exec`, returns the
   first failure.
4. Wire `runHooks` into the `mdsmith fix`
   build pass (plan 2606101546): run `before`
   immediately before the recipe pass; on
   failure exit immediately with the hook's
   exit code, run no recipes, run no `after`
   hooks. After the recipe pass, run `after`
   regardless of recipe results.
5. Add `--build-no-hooks` and
   `--build-skip-hooks-when-fresh` flags.
   Update `--build-dry-run` to list hooks.
6. Integration tests:

  - Test harness starts an `httptest.Server`
    in setup. A `before` hook touches a
    sentinel; a user-declared screenshot
    recipe (real headless-browser binary if
    available, else a `cp`-based stub)
    captures the server; an `after` hook
    touches a second sentinel. Assert both
    sentinels exist and the artifact was
    written.
  - `before` hook returning non-zero aborts
    the build pass with no recipes and no
    `after` hooks; lint-fix results stand.
  - `after` hook returning non-zero is
    reported but exits with the recipe-pass
    exit code priority.
  - `--build-no-hooks` skips both lists.
  - `--build-skip-hooks-when-fresh` with
    all-fresh skips both; with any stale
    runs both.
  - `mdsmith fix --no-build` skips hooks
    entirely.

7. Document the hook lifecycle, failure
   semantics, and flag matrix in
   `docs/guides/directives/build.md`. Cover
   the dev-server-around-screenshots example
   end-to-end with a user-declared recipe.

## Acceptance Criteria

- [ ] `before` hooks run in declaration
      order, once per `fix` build pass,
      before any recipe
- [ ] `after` hooks run in declaration
      order, once per build pass, after the
      recipe pass
- [ ] A failing `before` aborts with no
      recipes and no `after` hooks; lint-fix
      pass results are preserved
- [ ] A failing `after` is reported but does
      not prevent later `after` hooks from
      running
- [ ] Final exit code prioritises lint-fix
      errors over `before-fail` over
      `recipe-fail` over `after-fail`
- [ ] Hook `command` is split into argv and
      dispatched via `os/exec`; no shell
      interpreter is invoked
- [ ] MDS040 flags hook commands that start
      with `bash`/`sh`, contain shell
      operators, contain fused `{param}`
      placeholders, or reference reserved
      names (`inputs`, `outputs`)
- [ ] `{param}` tokens in a hook `command`
      expand from the hook's `params` map; an
      unmatched token is a config error
- [ ] `mdsmith fix --build-no-hooks` skips
      both lists and runs only the recipe
      pass
- [ ] `mdsmith fix --no-build` skips both
      hooks and recipes (no build pass)
- [ ] `mdsmith fix --build-dry-run` lists
      each hook alongside the recipes it
      bookends
- [ ] `mdsmith fix --build-skip-hooks-when-fresh`
      skips both lists when no target is
      stale and runs both when any target
      is stale
- [ ] A config without `build.hooks` parses
      cleanly and runs `mdsmith fix` without
      hook overhead
- [ ] MDS040 rejects hook `params` values
      with NUL, newline, leading/trailing
      whitespace, or > 4 KB
- [ ] Build pass refuses to run when MDS040
      reports any error against `build.hooks`
      or `build.recipes`
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no
      issues
