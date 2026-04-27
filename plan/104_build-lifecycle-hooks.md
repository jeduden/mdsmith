---
id: 104
title: Build lifecycle hooks (before/after)
status: "🔲"
summary: >-
  Add `build.hooks.before` and `build.hooks.after`
  config blocks: argv-tokenized commands run once
  per `mdsmith build` invocation around the recipe
  pass. Lets users start a dev server before
  screenshots and stop it after, or warm a cache
  before generating diagrams. Uses the same
  os/exec argv path and MDS040 lint as recipes —
  no shell.
model: sonnet
---
# Build lifecycle hooks (before/after)

## Goal

`mdsmith build` runs declared `before` commands
once before any recipe, and declared `after`
commands once after the recipe pass completes.
Failure semantics are explicit and CI-friendly.

## Context

Plan 102 ships `mdsmith build` that walks
directives and runs recipes. Plan 103 adds
staleness detection. Both stop short of the
"setup/teardown" lifecycle every real build
system provides: `make` has prerequisites,
`bazel` has actions, `npm` has `pre*`/`post*`
scripts.

The motivating example is the `screenshot`
recipe: it needs a running dev server. Today
the user has to start the server in a separate
terminal, run `mdsmith build`, then stop the
server. Hooks fold that into a single command.

The article that motivated this PR
([bgslabs.org/blog/why-are-we-using-markdown][post])
explicitly argues for "custom hooks to be
executed before, during and after the
compilation". The "during" hook is the
`<?build?>` directive itself (plan 101).
This plan adds "before" and "after".

[post]: https://bgslabs.org/blog/why-are-we-using-markdown
[plan-100]: 100_build-config-and-mds040.md
[plan-102]: 102_build-subcommand.md

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

| Field     | Required | Description                                                         |
|-----------|----------|---------------------------------------------------------------------|
| `command` | yes      | Argv template; same `{param}` rules as recipes (plan 100)           |
| `params`  | no       | Map of param name → literal value, used to expand `{param}` tokens  |
| `name`    | no       | Display name for diagnostics (defaults to first token of `command`) |

Hooks have *no* directive surface. They are
purely a config-level construct, run once per
`mdsmith build` invocation, not per directive.

### Execution order

```text
1. before[0], before[1], …          (in order)
2. recipe pass                       (plan 102)
3. after[0], after[1], …             (in order)
```

### Failure semantics

- **`before` hook fails** (non-zero exit): print
  the hook's stderr and exit code, run **no**
  recipes, run **no** `after` hooks (the failing
  `before` is responsible for any partial
  cleanup it needs), exit non-zero with the
  hook's exit code.
- **Recipe fails** (any directive): finish the
  recipe pass per plan 102's per-file `OK |
  FAIL` summary, then run `after` hooks. Final
  exit code is non-zero.
- **`after` hook fails**: print the hook's
  stderr and exit code, continue running
  remaining `after` hooks. Final exit code is
  non-zero.
- **Multiple failures**: choose the final exit
  code by priority, not boolean combination. If
  a `before` hook failed, return that hook's
  exit code. Otherwise, if the recipe pass had
  any failure, return the recipe-pass failure
  status from plan 102. Otherwise, if any
  `after` hook failed, return the first failing
  `after` hook's exit code. Otherwise return 0.

The asymmetry is intentional. A failed `before`
means setup is incomplete. Recipes would produce
garbage, so abort. A failed `after` means
teardown is broken. The artifacts are already
written, so report and exit non-zero.

### Argv expansion

Same as recipes (plan 100):

1. Split `command` on whitespace into tokens.
2. For each token, expand `{param}` tokens
   using the hook's `params` map.
3. Pass the resulting argv to `os/exec.Cmd`.
   No shell.

`{param}` tokens with no matching entry in
`params` are a config error caught by MDS040.

### Lint surface (MDS040 extension)

MDS040 (plan 100) already lints recipe
`command` strings. Extend it to lint hook
`command` strings with the *same* rules:

- Non-empty.
- First token is not a shell interpreter.
- No shell operators in static parts.
- No fused `{param}` placeholders.
- No `..` in the executable token.

A hook `params` entry must be referenced by at
least one `{param}` token in its `command`;
unused params are a warning. (Same as recipes,
per plan 100 rule 6.)

### Flags

| Flag         | Behavior                                                     |
|--------------|--------------------------------------------------------------|
| `--no-hooks` | Skip both `before` and `after` hooks (debugging, CI bypass)  |
| `--dry-run`  | List hooks alongside recipes; run nothing (extends plan 102) |

`--recipe NAME` (from plan 102) does not filter
hooks — they are global. A user who wants
recipe-specific setup should declare a custom
recipe whose `command` does the setup inline,
or split into multiple `mdsmith build` runs
with different `.mdsmith.yml` overrides.

### Interaction with staleness (plan 103)

If every target is up-to-date and would be
skipped, `before` and `after` still run by
default — they may have effects beyond the
recipes (e.g. publishing, notifications). To
skip them when nothing would build, add
`--skip-hooks-when-fresh`. (Naming intentional:
the default favors predictability; the flag is
explicit opt-in.)

### Out of scope

- Per-recipe hooks (`build.recipes.NAME.hooks`).
  Adds combinatorial complexity for marginal
  benefit; users who need it can wrap the
  recipe in a script.
- Hook timeouts separate from recipe timeouts.
  `mdsmith build --timeout` (plan 102) applies
  to recipes; hooks share the same timeout
  budget. If hook timeouts become important,
  add `hook-timeout` later.
- Conditional hooks (`if:` clauses). Compose at
  the script level instead.
- Background hooks (long-running side
  processes). `before` returns synchronously;
  if you want a background server, your `before`
  spawns it and returns; your `after` kills it
  by PID file.

## Tasks

1. Extend `BuildConfig` in `internal/config/`
   with a `Hooks HooksCfg` field. Define
   `HooksCfg` with `Before []HookCfg` and
   `After []HookCfg` fields. Validate each
   `HookCfg` the same way as `RecipeCfg.command`.
2. Extend MDS040 (plan 100) to lint hook
   `command` strings using the existing rule
   set. Add fixtures covering shell interpreter,
   shell operator, and fused-placeholder cases.
3. Add `internal/build/hooks.go`: a `runHooks`
   helper that takes a list of `HookCfg`,
   tokenizes and expands each, dispatches via
   `os/exec`, and returns the first failure.
4. Wire `runHooks` into `mdsmith build`: run
   `before` hooks; on failure, exit immediately
   with the hook's exit code, run no recipes,
   run no `after` hooks. After the recipe pass,
   run `after` hooks regardless of recipe
   results.
5. Add `--no-hooks` and
   `--skip-hooks-when-fresh` flags. Update
   `--dry-run` to list hooks alongside recipes.
6. Integration tests:

  - The test harness starts an
     `httptest.Server` in setup. A `before`
     hook touches a sentinel file; a
     `screenshot` recipe captures the server;
     an `after` hook touches a second
     sentinel. The test asserts both
     sentinels exist and the screenshot was
     written, proving hook ordering and that
     hooks ran via `os/exec`.
  - `before` hook returning non-zero aborts
     the run with no recipes executed and no
     `after` hooks executed.
  - `after` hook returning non-zero is
     reported but exits with the recipe-pass
     exit code priority.
  - `--no-hooks` skips both lists.
  - `--skip-hooks-when-fresh` with all-fresh
     targets skips both lists; with any stale
     target runs both.

7. Document the hook lifecycle, failure
   semantics, and flag matrix in
   `docs/guides/directives/build.md`. Cover the
   dev-server-around-screenshots example
   end-to-end.

## Acceptance Criteria

- [ ] `before` hooks run in declaration order,
      once per `mdsmith build` invocation,
      before any recipe
- [ ] `after` hooks run in declaration order,
      once per invocation, after the recipe pass
- [ ] A failing `before` hook aborts with no
      recipes executed and no `after` hooks
      executed
- [ ] A failing `after` hook is reported but
      does not prevent later `after` hooks from
      running
- [ ] Final exit code prioritises `before-fail`
      over `recipe-fail` over `after-fail`
- [ ] `command` is split into argv and dispatched
      via `os/exec`; no shell interpreter is
      invoked
- [ ] MDS040 flags hook commands that start with
      `bash`/`sh`, contain shell operators, or
      contain fused `{param}` placeholders
- [ ] `{param}` tokens in a hook `command`
      expand from the hook's `params` map; an
      unmatched token is a config error
- [ ] `mdsmith build --no-hooks` skips both
      lists and runs only the recipe pass
- [ ] `mdsmith build --dry-run` lists each hook
      alongside the recipes it bookends
- [ ] `mdsmith build --skip-hooks-when-fresh`
      skips both lists when no target is stale
      and runs both when any target is stale
- [ ] A config without `build.hooks` parses
      cleanly and runs `mdsmith build` without
      hook overhead
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no
      issues
