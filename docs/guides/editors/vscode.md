---
title: mdsmith for VS Code
summary: >-
  Install the mdsmith VS Code extension and use its inline
  diagnostics, quick fixes, fix-on-save, and cross-file
  navigation — one bundled binary, no extra setup.
---
# mdsmith for VS Code

mdsmith is a Markdown linter and formatter that runs inside
VS Code. It flags style, readability, structure, and broken
cross-file links as you type, and fixes what fixes cleanly on
save. The editor runs the same rule engine as `mdsmith check`
on the command line and in CI, so a file that is clean as you
edit is clean in the pipeline.

If you use markdownlint today, mdsmith covers the same style
rules and adds checks markdownlint has no model for: links
and anchors across files, generated sections, heading and
front-matter schemas, and readability budgets. `mdsmith init
--from-markdownlint` converts your config in one command; the
[migration guide](../migrate-from-markdownlint.md) covers the
rest of the move.

Diagnostics, quick fixes, and navigation all come from one
bundled binary, with nothing else to install.

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
Enable `mdsmith.previewFix` to see the diff in VS Code's
Refactor Preview before each save writes.

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

You need VS Code 1.85 or later. See
[Installation: VS Code extension](../install.md#vs-code-extension)
for the channel-by-channel breakdown.

## Settings and troubleshooting

A config file is optional: mdsmith lints with built-in
defaults, so the extension works as soon as you install it. To
tune the rules, run the **mdsmith: Initialize Config** command,
which writes a starter `.mdsmith.yml`. The server then finds
that file by walking up from the workspace root to the nearest
`.mdsmith.yml` or `.git`, the same as `mdsmith check`.

For the full settings table — `mdsmith.run`, `mdsmith.path`,
`mdsmith.config`, `mdsmith.previewFix`, and
`mdsmith.trace.server` — and fixes for common failure modes,
see the
[VS Code extension reference](../../reference/vscode-extension.md).

## See also

- [VS Code extension reference](../../reference/vscode-extension.md)
  — the settings table and troubleshooting
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
