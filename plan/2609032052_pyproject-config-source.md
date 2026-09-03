---
id: 2609032052
title: "Resolve config from `pyproject.toml` under `[tool.mdsmith]`"
status: "🔲"
model: opus
summary: >-
  Discover and load mdsmith config from a `pyproject.toml`
  `[tool.mdsmith]` table, with the same shape as `.mdsmith.yml`.
  A dedicated `.mdsmith.yml` still wins ties; the TOML dependency
  stays out of the WASM/TinyGo artifact via a build tag.
depends-on: []
---
# Resolve config from `pyproject.toml`

## Goal

Let a Python project configure mdsmith in its existing
`pyproject.toml`. The settings live under a
`[tool.mdsmith]` table. They use the same keys as
`.mdsmith.yml`, so no second config file is needed. This
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

Three facts shape the approach.

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

## Approach

- New paired files in `internal/config`: a
  `//go:build !wasm` file holding the `go-toml` import,
  a `loadPyproject(path)` that converts `[tool.mdsmith]`
  to a `*Config` via the YAML re-marshal, and a
  `pyprojectHasMdsmithTable(path)` probe; plus a
  `//go:build wasm` stub of both that returns an error
  (never reached at runtime, present only so the package
  compiles for WASM without `go-toml`).
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
  caller resolves pyproject identically: the CLI
  (`discoverConfigPath` / `loadConfigRaw`), the LSP
  (`resolveConfig`, which already injects
  `config.Discover`), and the `DefaultConfigPath`-based
  callers in
  [internal/directivefiles](../internal/directivefiles/directivefiles.go)
  and
  [internal/gitattributes](../internal/gitattributes/gitattributes.go).

## Non-Goals

- Changing the default. `.mdsmith.yml` stays the primary
  source and wins ties; `pyproject.toml` is an
  alternative, not a replacement, and YAML is not
  deprecated.
- Adding TOML support to the WASM/TinyGo artifact. WASM
  hosts keep passing inline config text; `ParseBytes`
  stays TOML-free.
- Precise `pyproject.toml` line numbers in diagnostics.
  Because config errors route through the re-marshalled
  YAML, positions are not mapped back to TOML lines.
- Recognizing the plural `[tools.mdsmith]`. Only the
  conventional `[tool.mdsmith]` is a config source; a
  present-but-plural table earns a one-line hint, not
  silent use.

## Tasks

1. [ ] Red/green: add the paired `internal/config` files
   — `loadPyproject(path)` behind `//go:build !wasm`
   (parse TOML, lift `[tool.mdsmith]`, re-marshal to
   YAML, call `loadFromBytes` with the pyproject path as
   the sidecar anchor and `mergeKinds` true) and a
   `//go:build wasm` stub returning an error. Test that
   a `[tool.mdsmith]` config and the equivalent
   `.mdsmith.yml` produce an identical `*Config`,
   including a rule-off bool, a rule sub-table, an
   `[[overrides]]` array of tables, and a kind.
2. [ ] Red/green: make `Load` dispatch on the `.toml`
   extension to `loadPyproject`; the YAML path is
   unchanged. Apply `readLimitedConfig`'s cap to the
   TOML read. Test `--config path/to/pyproject.toml` and
   an arbitrary `--config foo.toml` both read from
   `[tool.mdsmith]`, and that malformed TOML yields a
   clear error naming the file.
3. [ ] Red/green: extend `Discover` to return a
   `pyproject.toml` that contains `[tool.mdsmith]`, with
   the precedence and skip rules above (probe behind the
   same build tag). Extend the discovery tests in
   [internal/config/config_test.go](../internal/config/config_test.go):
   `.mdsmith.yml` beats a same-dir `pyproject.toml`, a
   nearer `pyproject.toml` beats a farther `.mdsmith.yml`,
   a `pyproject.toml` without the table is skipped, and
   the `.git` boundary still stops the walk.
4. [ ] Red/green: route the native callers through the
   shared discover-plus-dispatch — the CLI
   `discoverConfigPath` / `loadConfigRaw` in
   [cmd/mdsmith/main.go](../cmd/mdsmith/main.go), the LSP
   `resolveConfig` in
   [internal/lsp/server_session.go](../internal/lsp/server_session.go),
   and the `DefaultConfigPath` callers in
   [internal/directivefiles](../internal/directivefiles/directivefiles.go)
   and
   [internal/gitattributes](../internal/gitattributes/gitattributes.go)
   — so a pyproject-only project is linted identically by
   `check`, `fix`, the editor, and the build/gitattributes
   paths.
5. [ ] Red/green: a `pyproject.toml` with a plural
   `[tools.mdsmith]` table but no `[tool.mdsmith]` emits
   a one-line hint pointing at the correct key and is not
   used as a config source.
6. [ ] Confirm the WASM boundary: the standard-Go and
   TinyGo builds in
   [cmd/mdsmith-wasm/build.sh](../cmd/mdsmith-wasm/build.sh)
   still compile, and
   [cmd/mdsmith-wasm/size_test.go](../cmd/mdsmith-wasm/size_test.go)
   passes within both budgets with `go-toml` absent from
   the artifact.
7. [ ] Docs: add a reference page describing config
   discovery order and the `pyproject.toml`
   `[tool.mdsmith]` source under
   [docs/reference](../docs/reference/index.md), link it
   from the `check` and `init` CLI pages, and note the
   pyproject source in the
   [linter comparison](../docs/background/markdown-linters.md).
   Run `mdsmith fix` to regenerate catalogs.
8. [ ] Run `mdsmith fix PLAN.md`, `mdsmith check .`,
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
- [ ] A `.mdsmith.yml` takes precedence over a
      same-directory `pyproject.toml`; the nearest config
      file wins across directories.
- [ ] A `pyproject.toml` with no `[tool.mdsmith]` table
      is ignored and discovery continues upward.
- [ ] `--config pyproject.toml` and `--config foo.toml`
      load from the `[tool.mdsmith]` table.
- [ ] The LSP, build-directive, and gitattributes paths
      honor a pyproject-only project.
- [ ] The standard-Go and TinyGo WASM builds compile and
      stay within the size budgets; `go-toml` is not
      linked into the WASM artifact.
- [ ] A plural `[tools.mdsmith]` table produces a hint
      and is not used as config.
- [ ] Reference docs describe the discovery order and the
      pyproject source.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool -modfile=tools/go.mod golangci-lint run`
      reports no issues.
- [ ] `mdsmith check .` — 0 failures.
