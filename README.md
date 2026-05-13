# 🔨 mdsmith

[![Build][ci-badge]][ci-link]
[![Quality][grc-badge]][grc-link]
[![Coverage][cov-badge]][cov-link]

A fast, auto-fixing Markdown linter and formatter for docs, READMEs,
and AI-generated content. Checks style, readability, structure, and
cross-file integrity. Written in Go.

<!-- Rendered by .github/workflows/demo.yml on push to main; published to the assets branch -->
![mdsmith demo](https://raw.githubusercontent.com/jeduden/mdsmith/assets/assets/demo.gif)

## ✨ Why mdsmith

**🔧 Auto-fix Markdown formatting.**
`mdsmith fix` rewrites whitespace, headings, code fences,
bare URLs, list indentation, and table alignment in place,
re-running up to 10 passes and stopping early once edits
stabilize. `mdsmith check` is the read-only CI sibling.

**✏️ Live diagnostics wherever you write.**
`mdsmith lsp` emits diagnostics, quick-fixes, and
navigation (definition, references, symbol search, and a
call-hierarchy over `<?include?>`/`<?catalog?>` and
cross-file links).
The [VS Code extension][vsc-mp] surfaces all of it with
opt-in fix-on-save (also on [Open VSX][vsc-ovsx] for
Cursor, VSCodium, Theia, and Gitpod). Generic LSP clients
(Neovim, Helix, JetBrains) expose what they support. The
[Claude Code plugin](docs/guides/install.md#claude-code-plugin)
feeds diagnostics and navigation to the agent so it sees
mdsmith inline while editing Markdown.

**🔗 Cross-file integrity.**
Built-in rules flag broken links and missing anchors
([`MDS027`](internal/rules/MDS027-cross-file-reference-integrity/README.md)),
enforce per-file section schemas
([`MDS020`](internal/rules/MDS020-required-structure/README.md)),
and keep Markdown in the right folders
([`MDS033`](internal/rules/MDS033-directory-structure/README.md)).
Schemas can be inline on a
[file kind](docs/guides/file-kinds.md) or shared via
`proto.md` files.

**🤖 Guardrails for AI-generated docs.**
Cap [file](internal/rules/MDS022-max-file-length/README.md),
[section](internal/rules/MDS036-max-section-length/README.md),
and
[token-budget](internal/rules/MDS028-token-budget/README.md)
size; enforce
[reading grade](internal/rules/MDS023-paragraph-readability/README.md)
and
[sentence count](internal/rules/MDS024-paragraph-structure/README.md);
flag
[verbatim copy-paste](internal/rules/MDS037-duplicated-content/README.md)
across files.

**📋 Self-maintaining sections.**
On `mdsmith fix`, `<?toc?>` rebuilds a heading
table-of-contents, `<?catalog?>` generates an
index — list, table, or any row template — from the front
matter of files matching a glob, and `<?include?>` splices
in another file. `mdsmith merge-driver install` registers
a Git driver that auto-resolves merge conflicts inside
those blocks.

**📊 Gate releases on doc status.**
`mdsmith list query 'status: "✅"' plan/` selects files by
a [CUE expression](docs/reference/cli/query.md) on front
matter;
`mdsmith metrics rank --by token-estimate --top 10 docs/`
ranks files by any shared metric — both ready to pipe into
a release script.

**🆚 How does it compare?** See:
<?catalog
glob:
  - "docs/background/markdown-linters.md"
row: "- [{summary}]({filename})"
?>
- [How mdsmith compares to other Markdown linters.](docs/background/markdown-linters.md)
<?/catalog?>

## 📦 Installation

CLI:

```bash
go install github.com/jeduden/mdsmith/cmd/mdsmith@latest
npm install -g @mdsmith/cli    # or: npx @mdsmith/cli
pip install mdsmith            # or: uvx mdsmith / pipx install mdsmith
```

Editor extension (LSP-backed; runs `mdsmith lsp`):

```bash
code --install-extension jeduden.mdsmith     # VS Code, Codespaces (Marketplace)
codium --install-extension jeduden.mdsmith   # Cursor, VSCodium, Theia, Gitpod (Open VSX)
```

Any LSP-aware editor (Neovim, Helix, JetBrains via the LSP
plugin) works by pointing at `mdsmith lsp`.

Claude Code plugin (inline diagnostics plus definition,
references, symbol search, and call-hierarchy queries
across your docs):

```text
/plugin marketplace add jeduden/mdsmith
/plugin install mdsmith-lsp@mdsmith
/reload-plugins
```

More: the [install guide](docs/guides/install.md) covers
direct downloads and mise (asdf pending).
[VS Code integration](docs/guides/editors/vscode.md) covers
settings, code actions, and troubleshooting.

## 🚀 Usage

```text
mdsmith <command> [flags] [files...]
```

### Commands

<?catalog
glob:
  - "docs/reference/cli/*.md"
sort: command
header: |
  | Command | Description |
  |---------|-------------|
row: "| [`{command}`]({filename}) | {summary} |"
?>
| Command                                                      | Description                                                                          |
|--------------------------------------------------------------|--------------------------------------------------------------------------------------|
| [`check`](docs/reference/cli/check.md)                       | Lint Markdown files for style issues.                                                |
| [`fix`](docs/reference/cli/fix.md)                           | Auto-fix lint issues in Markdown files in place.                                     |
| [`help`](docs/reference/cli/help.md)                         | Show built-in documentation for rules, metrics, and concept pages.                   |
| [`init`](docs/reference/cli/init.md)                         | Generate a default `.mdsmith.yml` config in the current directory.                   |
| [`kinds`](docs/reference/cli/kinds.md)                       | Inspect declared file kinds and resolve effective rule config per file.              |
| [`list`](docs/reference/cli/list.md)                         | Selection-style commands that walk the workspace and emit matches.                   |
| [`list backlinks`](docs/reference/cli/backlinks.md)          | List workspace links that point at a file.                                           |
| [`list query`](docs/reference/cli/query.md)                  | Select Markdown files by a CUE expression on front matter.                           |
| [`lsp`](docs/reference/cli/lsp.md)                           | Run a Language Server Protocol server on stdio for editor integrations.              |
| [`merge-driver`](docs/reference/cli/merge-driver.md)         | Git merge driver that resolves conflicts inside generated sections.                  |
| [`metrics`](docs/reference/cli/metrics.md)                   | List and rank shared Markdown metrics (file length, token estimate, readability, …). |
| [`pre-merge-commit`](docs/reference/cli/pre-merge-commit.md) | Install / manage a pre-merge-commit hook that runs `mdsmith fix` after a merge.      |
| [`version`](docs/reference/cli/version.md)                   | Print the mdsmith build version and exit.                                            |
<?/catalog?>

Files can be paths, directories (walked recursively for `*.md`
and `*.markdown`), or glob patterns. Directories respect
`.gitignore` by default;
use `--no-gitignore` to override. Explicitly named files are
never filtered by `.gitignore`.

### Examples

```bash
mdsmith check docs/            # lint a directory
mdsmith fix README.md          # auto-fix in place
mdsmith check -f json docs/    # JSON output
mdsmith metrics rank --by bytes --top 10 .
```

See the [CLI reference](docs/reference/cli.md) for shared
flags, exit codes, output format, and configuration merge
semantics. Individual subcommand pages above cover their
own flags and examples.

## ⚙️ Configuration

Run `mdsmith init` to generate a `.mdsmith.yml` with every rule and its
defaults. Without a config, rules run with built-in defaults.

```yaml
rules:
  line-length:
    max: 120
  fenced-code-language: false

ignore:
  - "vendor/**"

overrides:
  - glob: ["CHANGELOG.md"]
    rules:
      no-duplicate-headings: false
```

Rules are `true` (defaults), `false` (off), or an object with settings.
`overrides` apply per file pattern; later entries take precedence.
Config is discovered by walking up to the repo root; `--config` overrides.

Commit `.mdsmith.yml` so contributors share the same rule settings and
mdsmith upgrades become an explicit, reviewable change. Run
`mdsmith version` to see the build you have installed.

## 📚 More

- [Guides](docs/guides/index.md) — directives, structure, migration
- [Rule directory](internal/rules/index.md) — every rule with status
- [CLI reference](docs/reference/cli.md)
- [Contributor guide](docs/development/index.md) — Go 1.24+, build, test, style

## 📄 License

[MIT](LICENSE)

<!-- badges -->

[ci-badge]: https://github.com/jeduden/mdsmith/actions/workflows/ci.yml/badge.svg?branch=main
[ci-link]: https://github.com/jeduden/mdsmith/actions/workflows/ci.yml?query=branch%3Amain
[grc-badge]: https://goreportcard.com/badge/github.com/jeduden/mdsmith
[grc-link]: https://goreportcard.com/report/github.com/jeduden/mdsmith
[cov-badge]: https://codecov.io/gh/jeduden/mdsmith/branch/main/graph/badge.svg
[cov-link]: https://codecov.io/gh/jeduden/mdsmith/branch/main
[vsc-mp]: https://marketplace.visualstudio.com/items?itemName=jeduden.mdsmith
[vsc-ovsx]: https://open-vsx.org/extension/jeduden/mdsmith
