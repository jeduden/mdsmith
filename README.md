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

**🔧 Stop hand-formatting Markdown.**
Whitespace, heading style, code fences, bare URLs, list
indentation, table alignment — `mdsmith fix` handles them
in place. Multi-pass fixing resolves cascading changes so
you don't run it twice. `mdsmith check` is the read-only
sibling for CI.

**✏️ See lint errors live in your editor.**
The [VS Code extension][vsc-mp] runs `mdsmith lsp` and
shows inline squiggles, per-rule quick fixes, and an
opt-in `source.fixAll.mdsmith` action for fix-on-save
(set `mdsmith.fixOnSave: true` to enable). Also on
[Open VSX][vsc-ovsx] for Cursor, VSCodium, Theia, and
Gitpod. Any LSP-aware editor — Neovim, Helix, JetBrains —
works the same way by pointing at `mdsmith lsp`.

**🔗 Catch broken links before they merge.**
Refactors silently break Markdown links and anchors.
[`cross-file-reference-integrity`](internal/rules/MDS027-cross-file-reference-integrity/README.md)
flags every missing file and missing heading anchor in PR
review. Pair it with
[`required-structure`](internal/rules/MDS020-required-structure/README.md)
to enforce that each file has the sections it should
(reusable schemas live in your repo and are referenced by
path or named via `kinds:`), and
[`directory-structure`](internal/rules/MDS033-directory-structure/README.md)
to keep Markdown in the folders it belongs.

**🤖 Stop AI from bloating your docs.**
LLMs produce walls of text. Cap file length with
[`max-file-length`](internal/rules/MDS022-max-file-length/README.md),
section length with
[`max-section-length`](internal/rules/MDS036-max-section-length/README.md),
and tokens with
[`token-budget`](internal/rules/MDS028-token-budget/README.md).
[`paragraph-readability`](internal/rules/MDS023-paragraph-readability/README.md)
and
[`paragraph-structure`](internal/rules/MDS024-paragraph-structure/README.md)
hold reading-grade and sentence count in line.
[`duplicated-content`](internal/rules/MDS037-duplicated-content/README.md)
flags verbatim repetition across files.

**📋 Make tables of contents and indexes maintain themselves.**
Embed `<?toc?>` for a heading list,
`<?catalog?>` for a table built from front matter,
or `<?include?>` to splice in another file. `mdsmith fix`
regenerates them in place. After a merge conflict in one
of these blocks, `merge-driver install` registers a Git
driver that resolves it automatically.

**📊 Gate releases on doc status.**
`mdsmith list query 'status: "✅"' plan/` lists every plan
that's done — pipe it to a release script, or fail the
release if anything is still open.
`mdsmith metrics rank --by token-estimate --top 10 docs/` is the
PR-time complement: spot the file an AI just doubled in
size before it lands.

**📖 Make rule docs readable by AI agents (and humans).**
`mdsmith help rule [name]` prints the full rule spec —
settings, examples, diagnostics — straight from the
binary. No network calls. Drop the output into
`.cursor/rules`, `AGENTS.md`, or `CLAUDE.md` and your
agent knows the rules without an extra fetch.

**🆚 How does it compare?** See:
<?catalog
glob:
  - "docs/background/markdown-linters.md"
row: "- [{summary}]({filename})"
?>
- [How mdsmith compares to other Markdown linters.](docs/background/markdown-linters.md)
<?/catalog?>

## 📦 Installation

```bash
go install github.com/jeduden/mdsmith/cmd/mdsmith@latest
npm install -g @mdsmith/cli    # or: npx @mdsmith/cli
pip install mdsmith            # or: uvx mdsmith / pipx install mdsmith
```

More options live in
[docs/guides/install.md](docs/guides/install.md). It covers direct
downloads, the VS Code extension on the Marketplace and Open VSX,
and asdf and mise once their registry entries land.

## ✏️ Editor integration

The mdsmith VS Code extension renders diagnostics inline.
Each fixable rule contributes a quick fix. A
`source.fixAll.mdsmith` action runs `mdsmith fix` on the
buffer.

```bash
# stock VS Code, GitHub Codespaces, GitHub.dev (Marketplace)
code --install-extension jeduden.mdsmith
# Cursor, VSCodium, Theia, Gitpod (Open VSX)
codium --install-extension jeduden.mdsmith
```

The extension is a thin LSP client. The lint pipeline runs
in the Go binary, so any LSP-aware editor (Neovim, Helix,
JetBrains via the LSP plugin) gets the same diagnostics by
pointing at `mdsmith lsp`.

See [VS Code Integration](docs/guides/editors/vscode.md)
for settings (`mdsmith.path`, `mdsmith.run`,
`mdsmith.fixOnSave`, `mdsmith.trace.server`), code actions,
and troubleshooting.

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
