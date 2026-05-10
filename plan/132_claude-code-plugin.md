---
id: 132
title: Package mdsmith LSP as a Claude Code plugin
status: "🔳"
model: sonnet
summary: >-
  Ship a `.claude-plugin/marketplace.json` plus a
  plugin manifest declaring `lspServers` so Claude
  Code can install mdsmith from this repository and
  auto-spawn `mdsmith lsp` on `.md` files, exposing
  diagnostics, definitions, references, symbols,
  implementations, and call hierarchy to the agent
  without manual editor setup.
---
# Package mdsmith LSP as a Claude Code plugin

## Goal

Let a Claude Code user install mdsmith from this
repository with two standard `/plugin` commands.
First, register the catalog one-time with `/plugin
marketplace add jeduden/mdsmith`. Then install the
plugin with `/plugin install mdsmith-lsp@mdsmith`.
After install, the agent sees Markdown diagnostics
inline after every edit. The agent can also use
go-to-definition, find references, symbol search,
and call hierarchy across the docs.

## Background

Claude Code's [code intelligence][cc-ci] bundles
LSP plugins for a fixed set of languages
(`gopls-lsp`, `pyright-lsp`, `typescript-lsp`, …).
Markdown is not on the list. The same docs note
that users can [create their own LSP plugin][cc-lsp]
for languages outside the bundle.

[cc-ci]: https://code.claude.com/docs/en/discover-plugins#code-intelligence
[cc-lsp]: https://code.claude.com/docs/en/plugins-reference#lsp-servers

Plan 121 shipped `mdsmith lsp` over stdio with
diagnostics and code actions. Plan 131 added the
six navigation methods Claude Code consumes
(`documentSymbol`, `definition`, `implementation`,
`references`, `workspace/symbol`, `callHierarchy`).
The server is wire-ready; only the Claude Code
discovery layer is missing. Today a user has to
hand-edit settings to spawn the binary.

## Non-Goals

- Committing per-platform `mdsmith` binaries inside
  the plugin repo. The plugin instead spawns
  `npx -y -p @mdsmith/cli mdsmith lsp`, leaning on
  the npm package shipped by plan 130 — first
  launch fetches the platform binary subpackage
  from npm and caches it locally.
- Authoring tools (slash commands, agents, hooks).
  Scope is the LSP wiring only.
- Submitting to the official Anthropic marketplace.
  This plan only ships a self-hosted marketplace
  inside `jeduden/mdsmith`; submission is a separate
  decision.

## Design

### Repository layout

```text
.claude-plugin/
  marketplace.json        # marketplace catalog at repo root
editors/
  claude-code/
    .claude-plugin/
      plugin.json         # plugin manifest with lspServers
    README.md             # install + binary prerequisite
  vscode/                 # existing VS Code extension
```

The marketplace at the repo root lists one plugin
sourced from `./editors/claude-code/`. This mirrors
the per-editor split already in place under
[editors/](../editors/) and keeps the VS Code
extension untouched.

### `marketplace.json`

```json
{
  "$schema": "https://json.schemastore.org/claude-code-plugin-marketplace.json",
  "name": "mdsmith",
  "owner": {
    "name": "Jan-Eric Duden",
    "url": "https://github.com/jeduden"
  },
  "description": "Markdown linter for Claude Code via LSP",
  "plugins": [
    {
      "name": "mdsmith-lsp",
      "source": "./editors/claude-code",
      "description": "Inline mdsmith diagnostics and Markdown navigation"
    }
  ]
}
```

### `plugin.json`

The manifest uses the inline `lspServers` form so
the plugin ships in a single file. `extensionToLanguage`
maps both common Markdown extensions to the
`markdown` language identifier.

```json
{
  "$schema": "https://json.schemastore.org/claude-code-plugin-manifest.json",
  "name": "mdsmith-lsp",
  "description": "Inline mdsmith diagnostics and Markdown navigation via LSP",
  "homepage": "https://github.com/jeduden/mdsmith",
  "repository": "https://github.com/jeduden/mdsmith",
  "license": "MIT",
  "keywords": ["markdown", "linter", "lsp"],
  "lspServers": {
    "mdsmith": {
      "command": "npx",
      "args": ["-y", "-p", "@mdsmith/cli", "mdsmith", "lsp"],
      "extensionToLanguage": {
        ".md": "markdown",
        ".markdown": "markdown"
      }
    }
  }
}
```

`npx` is bundled with npm, which standard Node.js
installers ship and Claude Code already requires.
First launch downloads `@mdsmith/cli` plus the
platform binary subpackage (e.g.
`@mdsmith/linux-x64`) from npm and caches them; the
explicit `-p @mdsmith/cli mdsmith` form makes the
bin selection unambiguous. `npx` prepends its
package bin dir to the spawned command's `$PATH`,
so the package's `mdsmith` always wins regardless of
any system install. Users who want a pinned version
edit `args` to use `@mdsmith/cli@<ver>` in place of
`@mdsmith/cli`.

### Discovery and install

Two install paths, both standard Claude Code flows:

- `/plugin marketplace add jeduden/mdsmith` then
  `/plugin install mdsmith-lsp@mdsmith` (browse and
  install from the catalog).
- Direct: `/plugin install mdsmith-lsp@mdsmith`
  after the marketplace is added.

After install, `/reload-plugins` activates the
server. Subsequent `.md` opens trigger
`textDocument/didOpen` and the diagnostics flow.

### Backwards compatibility

The plugin manifest is additive. The repo's
existing VS Code extension, npm scripts, and CI are
untouched. Users on other editors keep using the
documented stdio invocation.

## Tasks

1. Add `.claude-plugin/marketplace.json` at the
   repository root with one plugin entry pointing
   to `./editors/claude-code`.
2. Add `editors/claude-code/.claude-plugin/plugin.json`
   declaring the inline `lspServers` block above.
3. Add `editors/claude-code/README.md` with:
   install command, binary prerequisite, pointer
   to [the install guide](../docs/guides/install.md)
   for the binary, troubleshooting (the same
   `Executable not found in $PATH` failure mode the
   Claude Code docs warn about).
4. Add a "Claude Code" section to
   [`docs/guides/install.md`](../docs/guides/install.md)
   listing the two `/plugin` commands and linking
   to the new editor README.
5. No change to
   [.mdsmith.yml](../.mdsmith.yml)'s
   `directory-structure.allowed` list is needed:
   the rule only applies to Markdown files, and
   the new `.claude-plugin/` tree contains JSON
   only. `editors/**` is already allowed, so the
   Markdown README under `editors/claude-code/`
   does not need a separate entry either. Add a
   tight per-glob override under `overrides:` for
   `editors/claude-code/**/*.md` (line-length 72,
   max-file-length 80, paragraph-structure 3
   sentences / 25 words, paragraph-readability 12,
   token-budget 2000) so the new plugin README
   stays terse. This change is config-touching, so
   surface the diff before applying per
   [CLAUDE.md](../CLAUDE.md).
6. Validate the manifest locally with
   `claude plugin validate ./editors/claude-code` (or
   `/plugin validate` inside an active Claude Code
   session).
7. Smoke-test end-to-end: in a scratch repo, run
   `/plugin marketplace add ./` (pointing at a
   local clone), `/plugin install mdsmith-lsp@mdsmith`,
   open a `.md` file with a known violation, confirm
   the diagnostic appears in Claude Code's output,
   and confirm a navigation request (e.g.
   `definition` on an anchor link) round-trips.
8. Add the plugin install path to
   [`docs/reference/cli/lsp.md`](../docs/reference/cli/lsp.md)
   alongside the existing VS Code pointer.

## Acceptance Criteria

- [ ] `/plugin marketplace add jeduden/mdsmith`
      registers the marketplace without errors.
      *(Pending end-to-end smoke test inside an active
      Claude Code session.)*
- [ ] `/plugin install mdsmith-lsp@mdsmith` installs
      the plugin to user scope. *(Pending end-to-end
      smoke test.)*
- [ ] After install and `/reload-plugins`, opening
      a `.md` file with a known MDS rule violation
      surfaces the diagnostic to the agent (visible
      via Ctrl+O when the "diagnostics found"
      indicator appears). *(Pending end-to-end smoke
      test.)*
- [ ] After install, the agent can run
      `textDocument/definition` on an anchor link
      and receive the heading location. *(Pending
      end-to-end smoke test.)*
- [ ] `claude plugin validate` reports no errors on
      the new manifests. *(Pending — `claude` CLI not
      available in the build sandbox.)*
- [x] `mdsmith check .` passes against the new
      `.claude-plugin/marketplace.json` location
      without any change to
      `directory-structure.allowed` (the rule only
      fires on Markdown files, and the new tree
      contains JSON only).
- [ ] When Node.js is absent from `$PATH`, the
      `/plugin` Errors tab shows `Executable not
      found in $PATH` (no silent hang, no generic
      crash). *(Pending end-to-end smoke test;
      `mdsmith` itself is fetched via npx so its
      absence is no longer a failure mode.)*
- [x] [`docs/guides/install.md`](../docs/guides/install.md)
      documents the Claude Code install path and
      the binary prerequisite.
- [x] `mdsmith check .` passes with the new files.
- [x] All tests pass: `go test ./...`.

## Open Questions

- **Marketplace location.** Hosting under
  `jeduden/mdsmith` gates discovery on the
  upstream repo. A separate
  `jeduden/mdsmith-claude-plugin` repo would
  decouple plugin releases from binary releases
  but doubles the maintenance surface. Default to
  in-repo; revisit if release cadences diverge.
- **Plugin name collisions.** `mdsmith-lsp`
  matches the official plugin naming convention
  (`<tool>-lsp`). Confirm the name is unclaimed if
  the plugin is later submitted to the official
  Anthropic marketplace.
- **Binary version pinning.** The plugin will use
  whatever `mdsmith` version is on `$PATH`. If a
  future LSP capability requires a specific
  version, the plugin would need a version probe
  on `initialize`. Out of scope here.

## ...

<?allow-empty-section?>
