---
title: mdsmith for Obsidian
summary: >-
  Install the mdsmith Obsidian plugin and use its
  inline diagnostics, hover fixes, fix-on-save, and
  diagnostics panel — one WebAssembly runtime on
  desktop and mobile.
---
# mdsmith for Obsidian

The mdsmith plugin runs the linter inside an
Obsidian vault. It uses the same rule engine as
`mdsmith check` on the command line and in CI, so a
note that is clean as you edit is clean in the
pipeline. The engine is compiled to WebAssembly, so
one runtime works on every platform Obsidian
supports — desktop, iPadOS, iOS, and Android.

The plugin is not in the Obsidian community catalog.
Install it from the GitHub release zip, as the
[Install](#install) section describes.

## What you get

### As you write

**Inline diagnostics.** Every rule violation shows as
a wavy underline in the editor. Hover an underline
for the issue. The
[same checks run in CI](../../reference/cli/check.md),
so the editor never disagrees with the build.

**Hover for the fix.** The hover tooltip leads with
the message, then the schema constraint a value
violated (a link you can click to jump to the
constraint, when it has a source location), then the
rule code and a documentation link. A **Fix** link
applies the matching quick-fix.

**Per-line fix commands.** Each rule on the cursor
line registers a transient `mdsmith: Fix — {code}`
command in the palette. The set clears when the
cursor moves, so the palette only offers the rules
you can act on right now.

**Fix file.** Run `mdsmith: Fix file` to rewrite the
active note — trailing whitespace, heading style,
code fences, bare URLs, list indentation, and table
alignment — in one edit.

**Fix on save.** Turn on **Fix on save** (off by
default) and the plugin runs `Fix file` 200 ms after
each save. Setting **Run mode** to `off` suppresses
it.

### Across your vault

**Diagnostics panel.** Open the **mdsmith
Diagnostics** panel for a table of the diagnostics
from the notes you have checked or opened this
session. It does not scan the whole vault up front.
Click a row to jump to its source.

**Cross-file checks.** Because the plugin holds one
session over the whole vault, cross-file rules see
every note. Broken links and a catalog that drifted
out of date surface as diagnostics, the same as any
style error.

### Without extra setup

**One bundled runtime.** The release zip ships the
engine as a single `.wasm` file. There is no
subprocess to spawn, no native binary to load, and no
PATH lookup — so the plugin runs in Obsidian's
sandboxed mobile WebView, where all three are blocked.
`manifest.json` does not set `isDesktopOnly`.

## Install

Each release attaches `mdsmith-obsidian-<version>.zip`
to the [GitHub release][gh]. To install it:

1. Download `mdsmith-obsidian-<version>.zip` from the
   release page.
2. Create the folder
   `<vault>/.obsidian/plugins/mdsmith/`.
3. Unzip the five files into that folder: `main.js`,
   `manifest.json`, `styles.css`, `mdsmith.wasm`, and
   `wasm_exec.js`.
4. In Obsidian, open **Settings → Community plugins**,
   then enable **mdsmith**.

This vault-relative path is the same on every OS. The
`.obsidian` folder is hidden, though. Rather than browse to
it, open the plugins folder from **Settings → Community
plugins** — the folder icon by **Installed plugins** — and
drop the unzipped files there.

You need Obsidian 1.5 or later. A config file is
optional: mdsmith lints with built-in defaults, so the
plugin works as soon as you enable it. To tune the
rules, add a `.mdsmith.yml` to the vault root and the
plugin discovers it automatically — no setting to
change. To load a config from another location, set the
**Config path** setting to its vault-relative path.

[gh]: https://github.com/jeduden/mdsmith/releases

## Settings

Open **Settings → Community plugins → mdsmith**.

| Setting      | Default  | Purpose                                                              |
| ------------ | -------- | -------------------------------------------------------------------- |
| `configPath` | `""`     | Path to a `.mdsmith.yml`; empty auto-discovers one at the vault root |
| `runMode`    | `onSave` | When to re-lint: `onType`, `onSave`, or `off`                        |
| `fixOnSave`  | `false`  | Run `Fix file` 200 ms after each save                                |

`runMode` controls when diagnostics refresh. `onType`
re-lints on each edit; `onSave` re-lints only when you
save; `off` stops automatic linting and suppresses
fix-on-save. `fixOnSave` is subordinate to `runMode`:
when `runMode` is `off`, a save never rewrites the
note.

Changing `configPath` rebuilds the lint session over
the new config. To rebuild it by hand — after editing
the config file in place, for example — run the
`mdsmith: Restart session` command.

## Troubleshooting

**The engine fails to load.** A notice reports the
error on startup. Confirm all five files sit directly
in `<vault>/.obsidian/plugins/mdsmith/` — `main.js`,
`manifest.json`, `styles.css`, `mdsmith.wasm`, and
`wasm_exec.js`. A missing `mdsmith.wasm` or
`wasm_exec.js` is the usual cause. Re-unzip the
release and reload the plugin. If the notice instead
reads `parsing config file: …`, your `.mdsmith.yml`
has a syntax error — the engine refuses an invalid
config rather than linting on defaults — so fix the
YAML and run `mdsmith: Restart session`.

**No diagnostics appear.** Check that **Run mode** is
not `off`. With `onSave`, diagnostics refresh only
when you save the note. Open and re-save the file to
force a re-lint.

**The config is not applied.** With **Config path**
empty, the plugin loads a `.mdsmith.yml` only from the
vault root — confirm the file sits there, not in a
subfolder. An unreadable **Config path** reports a
notice and falls back to the engine defaults. Confirm
the path is vault-relative and the file exists, then run
`mdsmith: Restart session`. After editing a discovered
config in place, run `mdsmith: Restart session` too —
the session compiles config once at startup.

## See also

- [`mdsmith check`](../../reference/cli/check.md) and
  [`mdsmith fix`](../../reference/cli/fix.md) — the
  CLI surfaces the plugin reuses
- [Obsidian convention](../../reference/conventions.md) —
  pin `convention: obsidian` for wikilink and callout
  checks
- [mdsmith for VS Code](vscode.md) — the same engine in
  a different editor
- [Markdown linter comparison](../../background/markdown-linters.md)
  — how mdsmith compares to Obsidian's own plugins
