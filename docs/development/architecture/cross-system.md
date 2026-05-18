---
title: Cross-system contracts
slug: cross
summary: >-
  External-surface contracts: LSP, CLI,
  .mdsmith.yml, generated markers, plugin
  manifest, distribution shims. Public APIs.
---
# Cross-system contracts

mdsmith's external surfaces are public
APIs. Each one has its own spec doc; this
page only covers the cross-cutting
architecture — how the surfaces relate,
which way the dependencies point, and
what versioning policy ties them
together.

The
[solid-architecture skill](../../../.claude/skills/solid-architecture/SKILL.md)
reads this page in design and audit
modes.

## The boundaries

Each row points at the package that owns
the contract. The surface's own spec
(flags, schema, marker syntax, install
steps) lives where the table's "Spec
doc" column says.

| Boundary                              | Owner in repo                                        | Spec doc                                                                        | Consumers                                                  |
|---------------------------------------|------------------------------------------------------|---------------------------------------------------------------------------------|------------------------------------------------------------|
| LSP wire protocol                     | `internal/lsp`                                       | [CLI reference: `lsp`](../../reference/cli/lsp.md)                              | VS Code extension, other editors                           |
| CLI flags + exit codes                | `cmd/mdsmith`                                        | [CLI reference](../../reference/cli.md)                                         | shell scripts, CI, git hooks                               |
| `.mdsmith.yml` schema                 | `internal/config`                                    | [Conventions](../../reference/conventions.md)                                   | every project using mdsmith                                |
| Generated section markers             | `internal/archetype/gensection`                      | [Generated sections](../../background/concepts/generated-section.md)            | every project's Markdown files                             |
| Claude plugin manifest (published)    | `editors/claude-code/.claude-plugin/plugin.json`     | [Install: Claude plugin](../../guides/install.md)                               | end users via Claude Code marketplace                      |
| Claude plugin manifest (contributors) | `editors/claude-code-dev/.claude-plugin/plugin.json` | [editors/claude-code-dev/README.md](../../../editors/claude-code-dev/README.md) | mdsmith contributors                                       |
| Claude marketplace listing            | `.claude-plugin/marketplace.json`                    | [Install: Claude plugin](../../guides/install.md)                               | Claude Code marketplace                                    |
| Claude skill definitions              | `.claude/skills/*/SKILL.md`                          | [proto](../../../.claude/skills/proto.md)                                       | Claude Code sessions                                       |
| npm package shim                      | `npm/mdsmith/`                                       | [Install: npm](../../guides/install.md)                                         | Node users                                                 |
| PyPI wheel shim                       | `python/`                                            | [Install: PyPI](../../guides/install.md)                                        | Python users                                               |
| asdf / mise plugin                    | external repos                                       | [Install: asdf / mise](../../guides/install.md)                                 | language-tool users                                        |
| VS Code `contributes`                 | `editors/vscode/package.json`                        | [VS Code integration](../../guides/editors/vscode.md)                           | the extension host                                         |
| Public Markdown library               | `pkg/markdown`                                       | [Public Markdown Library](../markdown-library.md)                               | `internal/lint`, `internal/release`, external Go importers |

Treat each surface as a public API.
mdsmith is at major 0 today, so strict
SemVer does not bind us yet — but breaks
must be deliberate and noted in the
changelog so downstream consumers can
keep up. Once we hit 1.0 a break is a
SemVer-major event.

## Dependency inversion at the boundary (DIP)

The Go binary is the source of truth.
Every external surface adapts to it; the
binary does not adapt to them.

Concretely:

- The LSP server exposes capabilities;
  clients subscribe.
- Distribution shims forward args to
  the binary.
- Plugin manifests declare an
  invocation.
- The VS Code extension consumes the
  LSP server and the binary.

None of them re-implement domain logic.

If a shim must translate something
(platform-specific binary download,
JSON re-shape, fallback to a bundled
copy), that translation lives in the
shim and is unit-tested there. It is
not allowed to reach into the binary's
internals.

## Interface segregation across surfaces (ISP)

A consumer of one surface should not be
forced to install another. The npm shim
consumer should not need the LSP
server; the VS Code extension consumer
should not need the Claude plugin. Keep
each surface narrow and self-contained.

When the binary gains a feature, ask
which surface(s) need to expose it.
Resist exposing new fields on every
output envelope "in case someone wants
them".

## Liskov across distribution shims (LSP)

Every shim is a substitute for invoking
the `mdsmith` binary directly: same
exit codes, same stdout / stderr
formats, same flag parsing.

A shim that adds a flag the binary does
not have is a Liskov violation. Push
the flag down to the binary, or drop it
from the shim.

## Schema composition across kinds

When a file resolves to several kinds that each
declare a `required-structure` schema, the
schemas compose. Three rules apply:

- Frontmatter conjoins. A key required by any
  kind is required. Shared keys intersect with
  CUE `&`.
- Sections merge. Scopes with the same heading
  text combine their child lists. Other scopes
  append in input order.
- The stricter `closed:` wins.

Multiple kinds can layer schemas. For example,
`directive-rule-readme` builds on top of
`rule-readme`. See the
[Schemas guide](../../guides/schemas.md) for a
worked example.

## Versioning policy (post-1.0)

Today mdsmith is at major 0; the rules
below describe the contract we are
converging on. Each surface's own spec
doc says how the rule applies to it.

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
  field requires a deprecation window
  or a major bump. Renaming a field is
  removing one and adding another.
- **Generated section markers** are
  forever. Once a marker syntax is
  shipped, `mdsmith` must continue to
  parse it (possibly with a deprecation
  diagnostic) until the next major.
- **Plugin manifests** follow the
  host's versioning. Treat the declared
  servers, commands, and hooks as the
  visible interface.
- The **`pkg/markdown` Go API** is
  additive within a major: adding a
  function, type, or field is minor;
  renaming, removing, or re-signing an
  exported symbol is major. A goldmark
  major upgrade that changes the AST
  shape `Parse` returns is a break for
  this surface. `Splice` output is
  byte-exact and pinned by the sync-docs
  golden corpus. Full policy in
  [Public Markdown Library](../markdown-library.md).

## Common violations to flag

The audit watches for these
cross-system anti-patterns:

- **A shim that parses Markdown** to
  decide which args to pass.
- **A consumer reading `.mdsmith.yml`
  directly** instead of asking the
  binary or the LSP server. The schema
  is owned by `internal/config`;
  everything else consumes it through
  `mdsmith`.
- **A plugin command wrapping a shell
  call with bespoke logic.** The logic
  belongs in a `mdsmith` subcommand.
- **A field added to one surface that
  belongs in another.** File-wide
  configuration lives in the YAML;
  per-run overrides live on the CLI.
- **A response shape that varies by
  output mode.** The shape is the
  contract; if a mode wants different
  data, define a different mode.
- **A breaking change to a shipped
  surface with no deprecation window.**
- **A new LSP capability the VS Code
  extension is expected to use but
  does not yet opt into.**

## When adding a new surface

1. Identify which existing surface(s)
   it composes with. If none, ask
   whether the new surface really needs
   to exist or could be served by an
   existing one.
2. Write down the contract — what data
   flows in and out, in what format.
   Store it under `docs/reference/` or
   `docs/guides/` and link it from the
   boundaries table above.
3. Add a test that locks the contract
   inside the binary (a CLI flag test,
   an LSP fixture). Without a contract
   test, the surface drifts under
   refactor pressure.
4. Decide the versioning policy and
   record it in the new doc.
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
