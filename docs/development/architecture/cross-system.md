---
title: Cross-system contracts
summary: >-
  External-surface contracts: LSP, CLI,
  .mdsmith.yml, generated markers, plugin
  manifest, distribution shims. Public APIs
  with stricter compatibility rules than
  internal code.
---
# Cross-system contracts

mdsmith integrates with multiple external
systems. Each integration is a contract
that is more expensive to change than
internal code, because consumers ship and
release on their own schedule.

## The boundaries

| Boundary                              | Owner in repo                                        | Consumers                                  |
|---------------------------------------|------------------------------------------------------|--------------------------------------------|
| LSP wire protocol                     | `internal/lsp`                                       | VS Code extension, other editors           |
| CLI flags + exit codes                | `cmd/mdsmith`                                        | shell scripts, CI, git hooks               |
| `.mdsmith.yml` schema                 | `internal/config`                                    | every project using mdsmith                |
| Generated section markers             | `internal/archetype/gensection`                      | every project's Markdown files             |
| Claude plugin manifest (published)    | `editors/claude-code/.claude-plugin/plugin.json`     | end users via Claude Code marketplace      |
| Claude plugin manifest (contributors) | `editors/claude-code-dev/.claude-plugin/plugin.json` | mdsmith contributors via local marketplace |
| Claude marketplace listing            | `.claude-plugin/marketplace.json`                    | Claude Code marketplace                    |
| Claude skill definitions              | `.claude/skills/*/SKILL.md`                          | Claude Code sessions                       |
| npm package shim                      | `npm/mdsmith/`                                       | Node users                                 |
| PyPI wheel shim                       | `python/`                                            | Python users                               |
| asdf / mise plugin                    | external repos                                       | language-tool users                        |
| VS Code `contributes`                 | `editors/vscode/package.json`                        | the extension host                         |

A breaking change at any of these surfaces
is a SemVer-major event. Treat them as
public APIs.

## Dependency inversion at the boundary

The Go binary is the source of truth.
Every external surface adapts to it; it
does not adapt to them.

- **LSP**: the Go server exposes
  capabilities; the editor client
  subscribes. The server does not branch
  on which editor connected.
- **Shims (npm, PyPI)**: exec the binary
  with forwarded args. They never inspect
  Markdown or duplicate rule logic.
- **Plugin manifests**: the published
  manifest at
  `editors/claude-code/.claude-plugin/plugin.json`
  declares the mdsmith LSP for end users.
  The contributor manifest at
  `editors/claude-code-dev/.claude-plugin/plugin.json`
  declares `gopls` and
  `typescript-language-server` so any agent
  working in this repo gets code
  intelligence. Skills live separately under
  `.claude/skills/*/SKILL.md`; the
  marketplace listing lives at
  `.claude-plugin/marketplace.json`. None of
  these embed parsing or linting logic.
- **VS Code extension**: consumes the LSP
  server and the binary. It does not
  implement any rule.

If a shim needs to translate something —
platform-specific binary download, fallback
to a checked-in copy, JSON re-shape — that
translation lives in the shim and is
unit-tested there. It is not allowed to
reach into the binary's internals.

## Interface segregation across surfaces

A consumer of the npm package should not
be forced to install the LSP dependencies.
A consumer of the VS Code extension should
not be forced to install the Claude
plugin. Keep each surface narrow and
self-contained; package-level optional
dependencies should match that.

When the Go binary gains a feature, ask
which surface(s) need to expose it. Only
the ones that benefit. Resist exposing
new fields on every JSON envelope "in case
someone wants them".

## Versioning and compatibility

- The **CLI** follows SemVer. Adding a flag
  is minor; renaming or removing one is
  major. Behavior changes that turn a
  passing run into a failing run are major.
- **LSP capabilities** can be additive
  without bumping major. Removing a
  capability is major.
- The **`.mdsmith.yml` schema** is additive
  within a major. Removing a field
  requires a deprecation window or a major
  bump. Renaming a field is removing one
  field and adding another.
- **Generated section markers** are
  forever. Once a marker syntax is
  shipped, mdsmith must continue to parse
  it (possibly emitting a deprecation
  diagnostic) until the next major.
- The **plugin manifest** schema follows
  Claude Code's own versioning. Treat the
  set of declared skills, commands, and
  hooks as the visible interface.

## Liskov across distribution shims

Every shim — npm, PyPI, asdf, mise — is a
substitute for invoking the Go binary
directly. Running `mdsmith check .` via
any shim must behave identically to running
the binary directly:

- Same exit codes.
- Same stdout/stderr formats.
- Same flag parsing.

If a shim deviates (e.g. adds a `--quiet`
flag the binary does not have), that is a
Liskov violation. Either push the flag
down to the binary or remove it from the
shim.

## Anti-patterns

- A shim that parses Markdown to decide
  which args to pass.
- A VS Code command that reads
  `.mdsmith.yml` directly instead of
  asking the binary or the LSP server.
- A plugin command that wraps a shell call
  with bespoke logic — promote the logic
  into a binary subcommand and call that.
- A field added to `.mdsmith.yml` for a
  feature that should be a CLI flag (or
  vice versa). The rule of thumb:
  file-wide configuration lives in the
  YAML; per-run overrides live in the
  CLI.
- A rule whose diagnostic JSON shape
  varies by output mode. The shape is the
  contract; if a mode wants different
  data, define a different mode.
- A breaking change to a shipped CLI flag
  or YAML field with no deprecation
  window.
- A new LSP capability that the VS Code
  extension is expected to use but does
  not yet opt into.

## Reviewing a new surface

When adding a new integration — a new
editor, a new distribution channel, a new
plugin host — walk this list:

1. Identify which existing surface(s) it
   composes with. If none, ask whether the
   new surface really needs to exist or
   could be served by an existing one.
2. Write down the contract — what data
   flows in and out, in what format. Store
   it under `docs/reference/` and add a
   catalog entry so future developers can
   find it.
3. Add a test that locks the contract
   inside the binary (CLI flag test, LSP
   fixture, etc.). Without a contract
   test, the surface drifts under refactor
   pressure.
4. Decide the versioning policy. Note it
   in the new docs page.
5. Confirm the surface follows the
   dependency-inversion rule: it adapts to
   the binary; the binary does not adapt
   to it.

## Refactor moves that usually work

- A surface with duplicate logic across
  shims → push the logic into a binary
  subcommand consumed by all shims.
- A field that lives in two contracts
  (e.g. CLI + YAML) and means different
  things → split into two fields with
  distinct names.
- A capability that exists only in one
  editor → either generalize it to LSP or
  scope it to that editor's package, never
  both half-formed.
- A test that touches three contracts →
  split into three tests, one per
  contract.
