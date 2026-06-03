---
title: mdsmith for VS Code
summary: >-
  Install the mdsmith VS Code extension and use its inline
  diagnostics, quick fixes, fix-on-save, and cross-file
  navigation — one bundled binary, no extra setup.
---
# mdsmith for VS Code

The mdsmith extension puts the linter, the formatter, and
cross-file navigation in your editor. It runs the same rule
engine as `mdsmith check` on the command line and in CI, so a
file that is clean as you edit is clean in the pipeline.
Diagnostics, quick fixes, and navigation all come from one
bundled binary, with nothing else to install.

If you use markdownlint today, mdsmith covers the same style
rules and adds cross-file checks. The
[migration guide](../migrate-from-markdownlint.md) maps every
rule and rewrites your config.

## What you get

Each feature links to a page with its rules and examples.

### As you write

**Inline diagnostics.** Every rule violation shows as a
squiggle — live as you type by default, or only on save,
set by `mdsmith.run`.
The [same checks run in CI](../../reference/cli/check.md),
so the editor never disagrees with the build.

**[One-click quick fixes](../../features/auto-fix.md).** Each
fixable rule contributes a lightbulb. One click rewrites every
occurrence of that rule in the file, not only the line you
clicked.

**Fix on save.** Add `source.fixAll.mdsmith` to VS Code's
`editor.codeActionsOnSave` — the same way ESLint uses
`source.fixAll.eslint`:

```jsonc
{
  "editor.codeActionsOnSave": {
    "source.fixAll.mdsmith": "explicit"
  }
}
```

Each save then fixes trailing whitespace, heading style, code
fences, bare URLs, list indentation, and table alignment.

**Preview before applying.** Enable `mdsmith.previewFix` and
quick fixes and fix-on-save route through VS Code's Refactor
Preview pane, so you see the diff and confirm it before it is
written. The preview rides the code-action path above (the
lightbulb and `editor.codeActionsOnSave`); that path carries
the confirmation into the save.

**Hover for help.** Hover a diagnostic for the rule's one-line
summary plus a link that opens its full documentation offline:
a read-only tab rendered from the binary's embedded README, no
browser or network. Hover inside a `<?…?>` directive for its
guide page.

### Across your project

**[Cross-file navigation](../../features/live-diagnostics.md).**
Go to Definition and Find All References resolve links,
anchors, reference labels, `kind:` values, and directive
arguments. Workspace symbol search spans every heading and
label in the project.

**[Dependency call hierarchy](../../features/dependency-graph.md).**
Walk `<?include?>`, `<?catalog?>`, `<?build?>`, and Markdown
links as incoming and outgoing calls, so you can trace what a
page embeds and what depends on it.

**[Rename without breaking links](../../features/rename.md).**
Rename a heading and every workspace anchor link to it is
rewritten in one edit. A colliding slug fails loudly instead
of breaking the link.

**[Generated sections stay in sync](../../features/self-maintaining-sections.md).**
A quick fix on a `<?toc?>`, `<?catalog?>`, or `<?include?>`
block regenerates its body from the source.

**[Cross-file integrity](../../features/cross-file-integrity.md).**
Broken links, missing anchors, and misfiled documents surface
as diagnostics, the same as any style error.

### Without extra setup

**[A bundled binary](../../features/install-everywhere.md).**
The `.vsix` ships a binary for every supported platform
(Linux, macOS, and Windows on x64 and arm64) and selects yours
at startup. No separate `mdsmith` install, no postinstall
network call.

**Works beyond VS Code.** The same `.vsix` publishes to Open
VSX, so Cursor, VSCodium, Theia, and Gitpod install it too.

**Command Palette actions.** Run Initialize Config, Fix All
Markdown, Install Git Merge Driver, Explain Rule on This File,
and Show Resolved Config without leaving the editor.

## Install

Each release publishes the extension to the Visual Studio
Marketplace, to Open VSX, and as a `.vsix` on the GitHub
release. The three carry the identical artifact; pick the
channel for your editor:

```bash
# VS Code, Codespaces, github.dev (Marketplace)
code --install-extension jeduden.mdsmith
# Cursor, VSCodium, Theia, Gitpod (Open VSX)
codium --install-extension jeduden.mdsmith
# Air-gapped or pinned — download from the release page
code --install-extension mdsmith-<version>.vsix
```

You need VS Code 1.85 or later. A config file is optional:
mdsmith lints with built-in defaults, so the extension works
as soon as you install it. To tune the rules, run the
**mdsmith: Initialize Config** command, which writes a starter
`.mdsmith.yml`. The server then finds that file by walking up
from the workspace root to the nearest `.mdsmith.yml` or
`.git`, the same as `mdsmith check`. See
[Installation: VS Code extension](../install.md#vs-code-extension)
for the channel-by-channel breakdown.

## Settings

Project overrides go in `.vscode/settings.json`; global
preferences go in your user settings. Changing any setting
takes effect on the next document event, with no window
reload.

| Setting                | Default   | Purpose                                                                            |
| ---------------------- | --------- | ---------------------------------------------------------------------------------- |
| `mdsmith.run`          | `onType`  | When to lint: `onType` (default), `onSave`, or `off` (off stops automatic linting) |
| `mdsmith.previewFix`   | `false`   | Show the diff (Refactor Preview) before applying a fix (quick fix or fix-on-save)  |
| `mdsmith.config`       | `""`      | Override the `.mdsmith.yml` path (absolute or workspace)                           |
| `mdsmith.path`         | `mdsmith` | Pin a binary; the default runs the bundled per-platform one                        |
| `mdsmith.trace.server` | `off`     | LSP trace verbosity: `off`, `messages`, or `verbose`                               |

`mdsmith.run` defaults to `onType`, so diagnostics update live
as you type; `onSave` defers them to save, and `off` stops
them entirely (quick fixes still work on demand).

Fix-on-save is configured through VS Code's native
`editor.codeActionsOnSave`, shown above under **Fix on save** —
not through an mdsmith setting. The former `mdsmith.fixOnSave`
toggle is now a deprecated no-op. Fix-on-save runs independently
of `mdsmith.run`, and `mdsmith.previewFix` decides whether each
save shows the diff before writing.

## Troubleshooting

**No diagnostics appear.** Confirm the binary resolves: open
the integrated terminal and run `mdsmith version`. If it is
not found, set `mdsmith.path` to an absolute path. Set
`mdsmith.trace.server` to `messages` and read the "mdsmith"
Output channel.

**`spawn mdsmith ENOENT`.** Reachable only when you set
`mdsmith.path` to a bare name and your platform was not
bundled. The extension host does not source `~/.bashrc`, so a
`go install` location such as `~/go/bin` is invisible to it.
Clear `mdsmith.path` to use the bundled binary, or set it to
an absolute path.

**Server crashed too many times.** The restart limiter
tripped because the binary crashes on every request. Open the
"mdsmith" Output channel for the stack trace, fix the cause,
then run `mdsmith: Restart Language Server`.

**Two mdsmith servers running; I want one.** A reload or
update can leave the old extension host alive next to the new
one, each running its own server. The newest server wins: it
claims the workspace and the older one exits, sending an
`mdsmith/superseded` notice first so the client does not
restart it. If an older build left an orphan, kill its
extension host once — not the `mdsmith` process, which the
host respawns.

## See also

- [`mdsmith lsp`](../../reference/cli/lsp.md) — the protocol
  reference: capabilities, diagnostic mapping, symbol
  navigation, and the latency budget
- [`mdsmith check`](../../reference/cli/check.md) and
  [`mdsmith fix`](../../reference/cli/fix.md) — the CLI
  surfaces the extension reuses
- [Neovim Integration](neovim.md) — the same server in a
  different editor
- [Migrate from markdownlint](../migrate-from-markdownlint.md)
  — the rule mapping and the config rewrite
- [Markdown linter comparison](../../background/markdown-linters.md)
  — how mdsmith editor support compares to peers
