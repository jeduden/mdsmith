---
title: Cross-system contracts
slug: cross
summary: >-
  External-surface contracts: LSP, CLI,
  .mdsmith.yml, generated markers, plugin
  manifest, distribution shims. Public APIs.
---
# Cross-system contracts

SOLID and clean-architecture rules for
mdsmith's external surfaces. This page
is the source of truth — it names the
boundaries, the versioning rules, and
the patterns audit watches for. The
[solid-architecture skill][skill-md]
reads it during design and audit modes.

[skill-md]: ../../../.claude/skills/solid-architecture/SKILL.md

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

Treat each surface as a public API. mdsmith
is at major 0 today, so strict SemVer does
not bind us yet — but breaks must be
deliberate and noted in the changelog so
downstream consumers can keep up. Once we
hit 1.0 a break is a SemVer-major event.

## Dependency inversion at the boundary (DIP)

The Go binary is the source of truth.
Every external surface adapts to it; the
binary does not adapt to them.

- **LSP**: `internal/lsp` exposes
  capabilities; VS Code and other clients
  subscribe. The server does not branch
  on which client connected.
- **Shims (npm, PyPI, asdf, mise)**: exec
  the `mdsmith` binary with forwarded
  args. None inspect Markdown or
  duplicate rule logic.
- **Plugin manifests**: the published
  manifest at
  `editors/claude-code/.claude-plugin/plugin.json`
  declares the mdsmith LSP for end
  users. The contributor manifest at
  `editors/claude-code-dev/.claude-plugin/plugin.json`
  declares `gopls` and
  `typescript-language-server` so any
  agent working in this repo gets code
  intelligence. Skills live separately
  under `.claude/skills/*/SKILL.md`; the
  marketplace listing lives at
  `.claude-plugin/marketplace.json`. None
  of these embed parsing or linting
  logic.
- **VS Code extension**: consumes the
  LSP server and the binary. It does not
  implement any rule.

If a shim must translate something — a
platform-specific binary download, a
fallback to a checked-in copy, a JSON
re-shape — that translation lives in the
shim and is unit-tested there. It is not
allowed to reach into the binary's
internals.

## Interface segregation across surfaces (ISP)

A consumer of one surface should not be
forced to install another. The npm shim
consumer should not need the LSP
binary; the editor extension should not
need the Claude plugin. Keep each
surface narrow and self-contained;
package-level optional dependencies
should reflect that.

When the binary gains a feature, ask
which surface(s) need to expose it.
Only the ones that benefit. Resist
exposing new fields on every JSON
envelope "in case someone wants them".

## Liskov across distribution shims (LSP)

Every shim — npm, PyPI, asdf, mise — is
a substitute for invoking the `mdsmith`
binary directly. Same exit codes, same
stdout / stderr formats, same flag
parsing.

If a shim deviates (e.g. adds a flag
the binary does not have), that is a
Liskov violation. Push the flag down to
the binary or drop it from the shim.

## Versioning rules (concrete)

The rules below describe the post-1.0
contract we are converging on. Today
(major 0) we still aim for them — every
break should be deliberate and noted in
the changelog.

- The **CLI** follows SemVer. Adding a
  flag is minor; renaming or removing
  one is major. Behavior changes that
  turn a passing run into a failing run
  are major.
- **LSP capabilities** can be additive
  without bumping major. Removing a
  capability is major.
- The **`.mdsmith.yml` schema** is
  additive within a major. Removing a
  field requires a deprecation window or
  a major bump. Renaming a field is
  removing one and adding another.
- **Generated section markers** are
  forever. Once a marker syntax is
  shipped, `mdsmith` must continue to
  parse it (possibly with a deprecation
  diagnostic) until the next major.
- The **plugin manifest** schema follows
  Claude Code's own versioning. Treat
  the declared servers, commands, and
  hooks as the visible interface.

## Common violations to flag

These are the cross-system shapes the
audit watches for:

- **A shim that parses Markdown** to
  decide which args to pass. The shim's
  only job is to forward args to the
  binary.
- **A VS Code command reading
  `.mdsmith.yml` directly** instead of
  asking the binary or the LSP server.
  The schema is owned by
  `internal/config`; everywhere else
  must consume it through `mdsmith`.
- **A plugin command wrapping a shell
  call with bespoke logic.** The logic
  belongs in a `mdsmith` subcommand the
  plugin calls.
- **A field added to `.mdsmith.yml` for
  a feature that should be a CLI flag**
  (or vice versa). Rule of thumb:
  file-wide configuration lives in the
  YAML; per-run overrides live on the
  CLI.
- **A diagnostic JSON shape that varies
  by output mode.** The shape is the
  contract; if a mode wants different
  data, define a different mode.
- **A breaking change to a shipped CLI
  flag or `.mdsmith.yml` field with no
  deprecation window.**
- **A new LSP capability the VS Code
  extension is expected to use but does
  not yet opt into.**

## When adding a new surface

Walk this list:

1. Identify which existing surface(s) it
   composes with. If none, ask whether
   the new surface really needs to exist
   or could be served by an existing one.
2. Write down the contract — what data
   flows in and out, in what format.
   Store it under `docs/reference/` and
   add a catalog entry.
3. Add a test that locks the contract
   inside the binary (a CLI flag test,
   an LSP fixture). Without a contract
   test, the surface drifts under
   refactor pressure.
4. Decide the versioning policy. Note it
   in the new docs page.
5. Confirm the surface follows the
   dependency-inversion rule: it adapts
   to the binary; the binary does not
   adapt to it.

## Refactor moves we have used

- Duplicate logic across shims → push
  the logic into a `mdsmith` subcommand
  consumed by all shims.
- A field that lives in two contracts
  (e.g. CLI + YAML) and means different
  things → split into two fields with
  distinct names.
- A capability that exists only in one
  editor → either generalize it to LSP
  or scope it to that editor's package,
  never both half-formed.
- A test that touches three contracts →
  split into three tests, one per
  contract.
