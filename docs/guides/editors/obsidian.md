---
title: Obsidian Integration
summary: >-
  Install the mdsmith Obsidian plugin, configure how
  it spawns `mdsmith lsp`, and read diagnostics inline
  as you edit notes in your vault.
---
# Obsidian Integration

The mdsmith Obsidian plugin runs the bundled mdsmith
binary as a Language Server Protocol server, parses
its diagnostics, and renders them as wavy underlines in
the CodeMirror 6 editor that backs both Source and
Live Preview modes. A hover tooltip on a flagged span
shows the rule code, the message, and a Fix link. A
`mdsmith: Fix file` command applies the same edits
`mdsmith fix` produces on the same input.

The plugin is desktop-only. Mobile Obsidian runs no
Node and cannot spawn binaries; the iOS and Android
build of the catalog UI treats this plugin as absent
rather than crashing. Mobile support is planned
separately (see [plan 215][p215]).

[p215]: https://github.com/jeduden/mdsmith/blob/main/plan/215_obsidian-wasm-mobile.md

## Prerequisites

- Desktop Obsidian 1.5 or later.
- No separate `mdsmith` install. The release zip
  bundles a binary for every supported platform
  (Linux, macOS, Windows on x64 and arm64) and selects
  yours at startup by re-using the
  `@mdsmith/cli` npm package's own resolver — the same
  resolver the VS Code extension uses. Override only
  to pin a specific build: set the `Binary path`
  setting to an absolute path (from
  `go install github.com/jeduden/mdsmith/cmd/mdsmith@latest`,
  `npm install -g @mdsmith/cli`, or the
  [GitHub releases page](https://github.com/jeduden/mdsmith/releases)).
- A `.mdsmith.yml` reachable from the vault root by
  walking up to the nearest `.git` directory. The
  server matches the CLI's discovery (the same
  `config.Discover` walk), anchored at the vault
  passed to `initialize`.

## Install

Each release attaches a
`mdsmith-obsidian-<version>.zip` artifact to the
GitHub release page. The plugin is not published to
the Obsidian Community Plugins catalog — install by
hand:

1. Download `mdsmith-obsidian-<version>.zip` from the
   [releases page](https://github.com/jeduden/mdsmith/releases).
2. Extract it under
   `<vault>/.obsidian/plugins/mdsmith/`. The
   resulting directory holds `main.js`,
   `manifest.json`, `styles.css`, and `cli/` (the
   bundled per-platform binaries).
3. In Obsidian, open
   `Settings > Community plugins`, enable
   `Community plugins` if you haven't already, and
   toggle `mdsmith` on under `Installed plugins`.

## Settings

The plugin contributes the following settings under
`Settings > Community plugins > mdsmith`. Each value
is stored in the vault's plugin data file
(`<vault>/.obsidian/plugins/mdsmith/data.json`).

| Setting       | Default    | Purpose                                              |
| ------------- | ---------- | ---------------------------------------------------- |
| `binaryPath`  | `""`       | Override the bundled binary with an absolute path    |
| `configPath`  | `""`       | Pass `-c <path>` to the server                       |
| `runMode`     | `"onSave"` | When to lint: `onSave`, `onType`, or `off`           |
| `fixOnSave`   | `false`    | Run `Fix file` 200 ms after each save                |
| `traceServer` | `"off"`    | LSP trace verbosity: `off`, `messages`, or `verbose` |

Editing `binaryPath` or `configPath` restarts the
server. Editing `runMode`, `fixOnSave`, or
`traceServer` reconfigures the listeners without a
restart.

## Diagnostics

Open a Markdown file in the vault. Within a few
hundred milliseconds the LSP server reports its
diagnostics for the buffer. Each diagnostic decorates
its range with a wavy underline coloured by severity:

- Error (red) — the rule's findings block CI.
- Warning (orange) — the default for most rules.
- Info (blue) — advisory.
- Hint (grey) — suggestions.

Hover over an underlined span to see the rule code
and message in a tooltip. The tooltip's `Fix` link
asks the server for the matching quick fix and applies
the returned edit on click.

## Commands

The plugin contributes the following commands under
the Obsidian command palette
(`Ctrl/Cmd+P`):

- `mdsmith: Fix file` — runs `source.fixAll.mdsmith`
  on the active buffer and applies every returned
  edit. Produces the same buffer as
  `mdsmith fix <file>` from the CLI.
- `mdsmith: Restart server` — stops the running LSP
  server and starts a fresh one. Useful after
  upgrading `binaryPath` to a freshly-rebuilt
  binary, or after editing `.mdsmith.yml` if the
  config watcher misses the change.
- `mdsmith: Fix — {code}` (transient) — one entry per
  active rule on the cursor line. The set rebuilds on
  cursor move so the palette only lists rules you can
  act on right now.

## Fix on save

Toggle `fixOnSave` on under
`Settings > Community plugins > mdsmith` to have the
plugin debounce `vault.on('modify')` and trigger
`Fix file` 200 ms after the last save event. The
command path is shared with the palette entry, so
edits are identical to running `Fix file` by hand.

## Troubleshooting

- **Diagnostics never appear.** Open the developer
  console (`Ctrl/Cmd+Shift+I`) and look for
  `mdsmith:` lines. Set `traceServer` to `verbose`
  to log the full LSP exchange. The plugin surfaces
  spawn failures as `Notice` toasts on plugin load —
  if you missed it, run `mdsmith: Restart server`.
- **Wrong binary.** Run `which mdsmith` (or check the
  resolved command in the developer console) to see
  which binary the plugin spawned. Setting
  `binaryPath` to an absolute path pins the choice.
- **No quick-fix for a diagnostic.** A handful of
  rules report findings without a fix; the tooltip
  Fix link is omitted in that case. The full set of
  rules and their fix support lives in the
  [rules reference][rr].

[rr]: ../../reference/cli.md

## Pair with the `obsidian` convention

The Obsidian-flavoured ruleset lives under
[`convention: obsidian`][conv]. Set it in your
`.mdsmith.yml` to align mdsmith's syntax assumptions
with what Obsidian's renderer accepts:

```yaml
convention: obsidian
```

The plugin renders whatever the server emits, so the
convention choice is what determines which rules fire
in an Obsidian vault.

[conv]: ../../reference/conventions.md

## Non-goals

- LSP hover, completion, rename, and symbol
  navigation. Obsidian exposes those through
  different surfaces (graph view, Wikilinks,
  workspace search) — wiring the LSP versions warrants
  its own plan.
- Live-preview rendering changes. The plugin observes
  rather than rewrites the rendered HTML.
- Mobile support. Mobile is the scope of plan 215.
