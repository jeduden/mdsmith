---
title: Neovim Integration
summary: >-
  Wire `mdsmith lsp` into Neovim's built-in LSP client so
  diagnostics, code actions, and navigation work inline
  with no extra plugin.
---
# Neovim Integration

mdsmith ships an LSP server. Neovim has a built-in LSP
client. Point one at the other and the squiggles,
quick-fixes, and definition jumps work the same way they
do in VS Code — without an extra plugin.

## Prerequisites

- Neovim 0.10 or later (built-in `vim.lsp.start` API).
- A `mdsmith` binary on `$PATH`. Install via the
  [Quick start](../install.md#quick-start-cli-only) above,
  or any of the channels in the
  [install guide](../install.md).

## Minimal config

Add this to `init.lua`:

```lua
vim.api.nvim_create_autocmd("FileType", {
  pattern = "markdown",
  callback = function()
    vim.lsp.start({
      name = "mdsmith",
      cmd = { "mdsmith", "lsp" },
      root_dir = vim.fs.root(0, { ".mdsmith.yml", ".git" }),
    })
  end,
})
```

That's the whole integration. Opening a Markdown buffer
spawns one `mdsmith lsp` subprocess per workspace, scoped
to the nearest `.mdsmith.yml` or `.git` directory.

## With nvim-lspconfig

If you use `nvim-lspconfig`, declare a custom server:

```lua
local lspconfig = require("lspconfig")
local configs = require("lspconfig.configs")

if not configs.mdsmith then
  configs.mdsmith = {
    default_config = {
      cmd = { "mdsmith", "lsp" },
      filetypes = { "markdown" },
      root_dir = lspconfig.util.root_pattern(".mdsmith.yml", ".git"),
      settings = {},
    },
  }
end

lspconfig.mdsmith.setup({})
```

## Fix on save

The LSP server exposes a `source.fixAll.mdsmith` code
action. Bind it to `BufWritePre`:

```lua
vim.api.nvim_create_autocmd("BufWritePre", {
  pattern = "*.md",
  callback = function()
    vim.lsp.buf.code_action({
      context = { only = { "source.fixAll.mdsmith" } },
      apply = true,
    })
  end,
})
```

## Troubleshooting

**No diagnostics appear.** Confirm the binary resolves:
`:!mdsmith version` from inside Neovim. If the command is
not found, `:LspInfo` will show the spawn error and
`mdsmith` is missing from the editor's `$PATH`.

**Stale diagnostics after editing `.mdsmith.yml`.** The
server watches the file via
`workspace/didChangeWatchedFiles` and republishes
diagnostics on a change. If yours does not, save any open
Markdown buffer to force a re-lint, or run `:LspRestart`.

## See also

- [`mdsmith lsp`](../../reference/cli/lsp.md) — the LSP
  server reference (capabilities, settings, symbol
  navigation matrix).
- [VS Code Integration](vscode.md) — the same server,
  different host.
