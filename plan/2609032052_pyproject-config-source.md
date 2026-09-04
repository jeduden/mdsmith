---
id: 2609032052
title: "Resolve config from `pyproject.toml` under `[tool.mdsmith]`"
status: "🔲"
model: opus
summary: >-
  Discover and load mdsmith config from a `pyproject.toml`
  `[tool.mdsmith]` table, with the same shape as `.mdsmith.yml`, and
  turn config errors into positioned diagnostics that point at the
  offending line in either file. The TOML dependency stays out of the
  WASM/TinyGo artifact via a build tag.
depends-on: []
---
# Resolve config from `pyproject.toml`

## Goal

Let a Python project configure mdsmith in its existing
`pyproject.toml`. The settings live under a
`[tool.mdsmith]` table. They use the same keys as
`.mdsmith.yml`, so no second config file is needed. A bad
value in either file points at the offending line. This
is [issue 832](https://github.com/jeduden/mdsmith/issues/832).

## Background

Today mdsmith reads one config source. The filename is a
single package constant, `.mdsmith.yml`, in
[internal/config/load.go](../internal/config/load.go).
[`Discover`](../internal/config/load.go) walks up from
the working directory, looking only for that name until
the `.git` boundary.
[`Load`](../internal/config/load.go) then reads and
parses the file. Both funnel into `loadFromBytes`, the
shared pipeline that runs the safe YAML unmarshal,
deprecation detection, `.mdsmith/` sidecar discovery,
schema resolution, and semantic validation.

Issue 832 asks for the same config under a
`pyproject.toml` `[tool.mdsmith]` table. The pyproject
convention is the singular `tool`, not the plural the
issue text uses. This is handy for PyPI users, who
already keep one `pyproject.toml`.

Config errors carry no position today. The CLI prints a
plain wrapped `error` to stderr and exits 2. The LSP
sends a `window/logMessage`, never a squiggle on the
config file. Every validation site is a `fmt.Errorf`
with a string label like `overrides[%d]`. So neither
file gets a line or column on a bad value.

The seams for positioned diagnostics already exist.
[`lint.Diagnostic`](../internal/lint/diagnostic.go) and
its `RelatedLocation` carry `File`, `Line`, and `Column`,
and are documented to point at the config file. The
config-target rule path in
[`runConfigTargetRules`](../internal/engine/runner.go)
already anchors diagnostics on the config file (MDS040
uses it, but pins line and column to 1). And both parsers
expose positions: `yaml.v3` nodes carry `Line`/`Column`,
and `go-toml` exposes per-key positions on its tree.

Four facts shape the approach.

1. The `Config` struct in
   [internal/config/config.go](../internal/config/config.go)
   uses custom `yaml.Node` decoders. Examples are the
   `RuleCfg` bool-or-mapping union and `KindSchemaRef`.
   Re-doing these for TOML would copy fragile code. So
   the conversion runs the other way. Parse the TOML,
   lift the `[tool.mdsmith]` sub-tree, and re-marshal it
   to YAML. Then reuse `loadFromBytes` unchanged.

2. `github.com/pelletier/go-toml v1.9.5` is already a
   direct requirement in [go.mod](../go.mod). Only the
   release tooling in
   [internal/release](../internal/release/messaging_patcher.go)
   uses it today. Reusing it adds no module-graph entry.
   So the lean-`go.mod` policy that keeps `go install`
   light is unaffected.

3. `internal/config` is linked into the WebAssembly
   engine. The WASM host has no filesystem. It gets
   config as an inline string through `ParseBytes`, not
   from disk. So pyproject discovery is native-only.
   `go-toml` must stay out of both WASM artifacts and
   their size budgets in
   [cmd/mdsmith-wasm/size_test.go](../cmd/mdsmith-wasm/size_test.go).
   So it lives in a `//go:build !wasm` file, with a
   `//go:build wasm` stub that errors.

4. A structured key path is the bridge to positions.
   Validation attaches a format-neutral path, such as
   `["overrides", 0, "foreign-regions"]`. A resolver then
   maps that path to a line and column in the real source.
   The YAML resolver walks `yaml.v3` nodes; the TOML
   resolver walks the `go-toml` tree. So the config core
   stays format-neutral, and only the resolver differs.

## Approach

The pyproject source:

- New paired files in `internal/config`: a
  `//go:build !wasm` file holding the `go-toml` import,
  a `loadPyproject(path)` that converts `[tool.mdsmith]`
  to a `*Config` via the YAML re-marshal, and a
  `pyprojectHasMdsmithTable(path)` probe; plus a
  `//go:build wasm` stub of both that returns an error.
- `Load` dispatches by extension: `.toml` paths go
  through `loadPyproject`, everything else keeps the YAML
  path. The 1 MB `maxConfigBytes` read cap applies to
  the TOML read as well.
- `Discover` recognizes a `pyproject.toml` that contains
  `[tool.mdsmith]`. Precedence: within one directory a
  `.mdsmith.yml` wins; across directories the nearest
  file wins; a `pyproject.toml` without `[tool.mdsmith]`
  is ignored and the walk continues (still bounded by
  `.git` and the filesystem root).
- Centralize discover-plus-dispatch so every native
  caller resolves pyproject identically: the CLI, the LSP
  `resolveConfig` (which already injects
  `config.Discover`), and the `DefaultConfigPath` callers
  in
  [internal/directivefiles](../internal/directivefiles/directivefiles.go)
  and
  [internal/gitattributes](../internal/gitattributes/gitattributes.go).

The positioned diagnostics:

- Add a typed config issue carrying a message, a severity,
  and a structured key path. Validation sites attach the
  path they already describe in their string labels.
- A position resolver maps a key path to a line and
  column. The YAML resolver reads `yaml.v3` node
  positions and works everywhere, WASM included. The TOML
  resolver reads `go-toml` tree positions, prepends
  `tool.mdsmith`, and lives behind the `!wasm` tag.
- Syntax errors come straight from the parser. Both
  `yaml.v3` and `go-toml` report the failing line and
  column on a parse error.
- The load boundary turns each issue into a
  `lint.Diagnostic` anchored on the config file. The CLI
  prints these like any other diagnostic
  (`file:line:column`), and the LSP publishes them on the
  config file so an editor shows squiggles, keeping a
  `logMessage` summary for when the file is not open.

## Non-Goals

- Changing the default. `.mdsmith.yml` stays the primary
  source and wins ties; `pyproject.toml` is an
  alternative, not a replacement, and YAML is not
  deprecated.
- Adding TOML support to the WASM/TinyGo artifact. WASM
  hosts keep passing inline config text; `ParseBytes`
  stays TOML-free. YAML config there still resolves
  positions, since the YAML resolver needs no `go-toml`.
- Linting `pyproject.toml` content outside `[tool.mdsmith]`.
  Other tables are read past, never diagnosed.
- Recognizing the plural `[tools.mdsmith]`. Only the
  conventional `[tool.mdsmith]` is a config source; a
  present-but-plural table earns a one-line hint, not
  silent use.

## Tasks

Phase A — positioned config diagnostics (YAML first):

1. [ ] Red/green: add a typed config issue (message,
   severity, structured key path) and a resolver
   interface that maps a key path to a line and column.
   Add the `yaml.v3` node resolver over the parsed
   `.mdsmith.yml` tree.
2. [ ] Red/green: thread key paths through the validation
   sites in
   [validate.go](../internal/config/validate.go),
   [foreignregion.go](../internal/config/foreignregion.go),
   the `RuleCfg` decoder, wordlist validation, convention
   application, and build validation — one area per
   commit, each with a test asserting the resolved line.
3. [ ] Red/green: at the load boundary, turn config
   issues into `lint.Diagnostic`s anchored on the config
   file. Have the CLI render them as
   `file:line:column` diagnostics (exit 2), replacing the
   plain `mdsmith: %v` string; update the affected error
   tests.
4. [ ] Red/green: have the LSP publish config diagnostics
   on the config-file document so an editor shows
   squiggles, keeping the `logMessage` summary when the
   file is not open. A malformed `.mdsmith.yml` now points
   at the offending line.

Phase B — the pyproject source:

5. [ ] Red/green: add the paired `internal/config` files
   — `loadPyproject(path)` behind `//go:build !wasm`
   (parse TOML, lift `[tool.mdsmith]`, re-marshal to YAML,
   call `loadFromBytes` with the pyproject path as the
   sidecar anchor and `mergeKinds` true) and a
   `//go:build wasm` stub returning an error. Test that a
   `[tool.mdsmith]` config and the equivalent
   `.mdsmith.yml` produce an identical `*Config` —
   including a rule-off bool, a rule sub-table, an
   `[[overrides]]` array of tables, and a kind.
6. [ ] Red/green: make `Load` dispatch on the `.toml`
   extension to `loadPyproject`; the YAML path is
   unchanged. Apply the `maxConfigBytes` cap to the TOML
   read. Test `--config path/to/pyproject.toml` and an
   arbitrary `--config foo.toml` both read from
   `[tool.mdsmith]`.
7. [ ] Red/green: extend `Discover` to return a
   `pyproject.toml` that contains `[tool.mdsmith]`, with
   the precedence and skip rules above (probe behind the
   same build tag). Extend the discovery tests in
   [config_test.go](../internal/config/config_test.go).
8. [ ] Red/green: route the native callers through the
   shared discover-plus-dispatch — the CLI in
   [main.go](../cmd/mdsmith/main.go), the LSP in
   [server_session.go](../internal/lsp/server_session.go),
   and the `DefaultConfigPath` callers — so a
   pyproject-only project is linted identically by
   `check`, `fix`, the editor, and the
   build/gitattributes paths.
9. [ ] Red/green: add the TOML position resolver over the
   `go-toml` tree (prepending `tool.mdsmith`), so a bad
   value or a syntax error in `[tool.mdsmith]` produces a
   diagnostic at the right line and column in the
   `pyproject.toml`.
10. [ ] Red/green: a `pyproject.toml` with a plural
    `[tools.mdsmith]` table but no `[tool.mdsmith]` emits a
    one-line hint pointing at the correct key and is not
    used as a config source.

Phase C — guardrails and docs:

11. [ ] Confirm the WASM boundary: the standard-Go and
    TinyGo builds in
    [build.sh](../cmd/mdsmith-wasm/build.sh) still
    compile, and
    [size_test.go](../cmd/mdsmith-wasm/size_test.go)
    passes within both budgets with `go-toml` absent from
    the artifact.
12. [ ] Docs: add a reference page for config discovery
    order and the `[tool.mdsmith]` source under
    [docs/reference](../docs/reference/index.md), note that
    config errors are positioned diagnostics, link it from
    the `check` and `init` CLI pages and the
    [linter comparison](../docs/background/markdown-linters.md),
    then run `mdsmith fix` to regenerate catalogs.
13. [ ] Run `mdsmith fix PLAN.md`, `mdsmith check .`,
    `go test ./...`, and
    `go tool -modfile=tools/go.mod golangci-lint run`.

## Acceptance Criteria

- [ ] A project whose only config is a `pyproject.toml`
      with `[tool.mdsmith]` is linted with that config by
      `mdsmith check` and `mdsmith fix`.
- [ ] The same settings written under `[tool.mdsmith]`
      and in `.mdsmith.yml` produce identical effective
      config across rules, overrides, kinds, and
      conventions.
- [ ] A bad value in `.mdsmith.yml` produces a diagnostic
      at the offending line and column.
- [ ] A bad value in a `pyproject.toml` `[tool.mdsmith]`
      table produces a diagnostic at the offending line
      and column in the `pyproject.toml`.
- [ ] A syntax error in either file points at the failing
      line.
- [ ] The CLI prints config errors as
      `file:line:column` diagnostics; the LSP shows them as
      squiggles on the config file when it is open.
- [ ] A `.mdsmith.yml` takes precedence over a
      same-directory `pyproject.toml`; the nearest config
      file wins across directories; a `pyproject.toml` with
      no `[tool.mdsmith]` is ignored.
- [ ] `--config pyproject.toml` and `--config foo.toml`
      load from the `[tool.mdsmith]` table.
- [ ] The LSP, build-directive, and gitattributes paths
      honor a pyproject-only project.
- [ ] The standard-Go and TinyGo WASM builds compile and
      stay within the size budgets; `go-toml` is not
      linked into the WASM artifact.
- [ ] A plural `[tools.mdsmith]` table produces a hint
      and is not used as config.
- [ ] Reference docs describe the discovery order, the
      pyproject source, and positioned config diagnostics.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool -modfile=tools/go.mod golangci-lint run`
      reports no issues.
- [ ] `mdsmith check .` — 0 failures.
