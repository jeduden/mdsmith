---
id: 102
title: Builder interface and mdsmith build subcommand
status: "🔲"
summary: >-
  Implement the `Builder` interface, built-in recipe
  drivers (`screenshot` via chromedp, `vhs` via exec,
  custom via `os/exec` argv), and the `mdsmith build`
  subcommand that walks files, dispatches to recipe
  drivers, and writes artifacts atomically.
model: opus
---
# Builder interface and mdsmith build subcommand

## Goal

Execute `<?build?>` directives: run the declared
recipe, write the artifact, and report per-file
success or failure. `mdsmith check` and `mdsmith fix`
remain unaffected — no external tool runs at lint
time.

## Context

Depends on plan 101 (`<?build?>` directive and
MDS039). The directive parser and recipe resolution
from plan 101 are reused here. The `build:` config
schema from plan 100 provides recipe declarations
and `base-url`.

## Design

### Builder interface

```go
// internal/build/builder.go
type Builder interface {
    Build(ctx context.Context, params map[string]string,
          output string) error
}
```

`params` contains the directive's key/value pairs.
`output` is the resolved absolute path for the
artifact. Each recipe driver implements `Builder`.

A registry maps recipe name → `Builder`. At startup,
the `mdsmith build` command registers built-ins and
constructs a custom `Builder` for each entry in
`build.recipes`.

### Built-in recipe drivers

**`screenshot`** — uses
[chromedp](https://github.com/chromedp/chromedp):

| Param      | Required | Default    |
|------------|----------|------------|
| `url`      | yes      | —          |
| `selector` | no       | full page  |
| `viewport` | no       | `1280x800` |
| `wait`     | no       | `0` ms     |
| `click`    | no       | —          |
| `hide`     | no       | `[]`       |

`build.base-url` is prepended to path-only `url`
values (starting with `/`).

**`vhs`** — runs the `vhs` binary via `os/exec`:

| Param   | Required |
|---------|----------|
| `input` | yes      |

Skipped when `vhs` is not in `PATH`.

### Custom recipe driver

User-declared recipes in `build.recipes` are
compiled into a `Builder` at config load time:

1. `command` is split on whitespace into an argv
   list (the same split used by MDS040 in plan 100).
2. `{param}` tokens are replaced with the
   corresponding directive param value at call time.
3. The resulting argv is passed directly to
   `os/exec.Cmd` — no `sh -c`, no shell
   metacharacter expansion.

A value like `foo; rm -rf /` is passed literally to
the binary as a single argument, not interpreted by
a shell.

### `mdsmith build` subcommand

```text
mdsmith build [paths...] [flags]
```

Flags:

| Flag                 | Description                             |
|----------------------|-----------------------------------------|
| `--recipe NAME`      | Only build directives using this recipe |
| `--base-url URL`     | Override `build.base-url` from config   |
| `--dry-run`          | List every target; run no tool          |
| `--timeout DURATION` | Per-recipe timeout (default `30s`)      |

Behavior:

1. Walk `paths` (default: current directory); collect
   all `<?build?>` blocks via the plan 101 directive
   parser.
2. Apply `--recipe` filter when set.
3. `--dry-run`: print each target (`recipe → output`)
   and exit 0.
4. Dispatch to the recipe driver. Write the artifact
   to `output` atomically (write to a temp file, then
   rename).
5. Print a per-file `OK | FAIL` summary. Exit non-zero
   if any recipe fails.

### Security

Custom recipe `command` values are split into an argv
list at config load; `{param}` tokens become
individual arguments passed to `exec.Cmd`. No shell is
involved at any stage. MDS040 (plan 100) enforces
this at lint time.

Path params (`output`, `input`) are validated by
MDS039 (plan 101) as relative paths with no `..`
components before the build command runs.

### CI without chromium

The `screenshot` builder is skipped and its tests
are marked `t.Skip` when chromium is not in `PATH`.
All other tests run normally. CI without chromium
still passes.

## Tasks

1. Define the `Builder` interface and recipe registry
   in `internal/build/`. Implement the custom recipe
   driver that tokenizes `command` at config load and
   dispatches via `os/exec`.
2. Implement the `screenshot` builder using chromedp.
   Support `selector`, `viewport`, `wait`, `click`,
   and `hide` params. Prepend `build.base-url` to
   path-only `url` values.
3. Implement the `vhs` builder via `os/exec`. Skip
   when `vhs` is not in `PATH`.
4. Add `mdsmith build` subcommand with all flags.
   Wire the directive parser (plan 101) and recipe
   registry. Write artifacts atomically. Print
   `OK | FAIL` summary; exit non-zero on failure.
5. Integration tests:

  - `screenshot` against `httptest.Server` writes
     a non-empty PNG. Skip when chromium absent.
  - A `cp`-based custom recipe declared in
     `build.recipes` writes the output file.

6. Update `docs/guides/directives/build.md` (from
   plan 101) with a section covering `mdsmith build`
   flags, the `--dry-run` output format, and the
   `OK | FAIL` summary.
7. Update `demo.tape` to use a static HTML file
   (no dev server) as the screenshot source.

## Acceptance Criteria

- [ ] `mdsmith build` against `httptest.Server`
      writes a non-empty PNG for a `screenshot` recipe
- [ ] A `cp`-based custom recipe declared in
      `build.recipes` writes the output file
- [ ] Custom recipe `command` is executed via
      `os/exec` with an explicit argv list — no shell
      interpreter is invoked
- [ ] `build.base-url` is prepended to path-only
      `url` values; a full URL is passed unchanged
- [ ] `mdsmith build --dry-run` lists every target
      (`recipe → output`) without running any tool
      and exits 0
- [ ] `mdsmith build` exits non-zero on failure with
      a per-file `OK | FAIL` summary
- [ ] `mdsmith build --recipe screenshot` only runs
      `screenshot` directives; other recipes are skipped
- [ ] `mdsmith build --timeout 5s` applies the
      timeout per recipe invocation
- [ ] Artifacts are written atomically (temp file +
      rename); a failed recipe leaves no partial file
- [ ] CI without chromium still passes; `screenshot`
      tests are skipped, not failed
- [ ] `mdsmith check` does **not** run any external
      tool (lint and build remain separate)
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
