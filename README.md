# 🔨 mdsmith

[![Build][ci-badge]][ci-link]
[![Quality][grc-badge]][grc-link]
[![Coverage][cov-badge]][cov-link]

Live build, code-quality, and coverage signals —
[Quality you can verify](docs/features/quality.md) explains what
each one gates.

A fast, auto-fixing Markdown linter and formatter for docs, READMEs,
and AI-generated content. Checks style, readability, structure, and
cross-file integrity. Written in Go.

<!-- Rendered by .github/workflows/demo.yml on push to main; published to the assets branch -->
![mdsmith demo](https://raw.githubusercontent.com/jeduden/mdsmith/assets/assets/demo.gif)

<?include
file: docs/features/index.md
strip-frontmatter: "true"
heading-level: "absolute"
?>
## Why mdsmith

mdsmith is one rule engine behind every surface: the CLI, the LSP
server, the VS Code extension, and the Claude Code plugin all run
the same checks. This page is the shared overview. The README
includes it; the website renders it and links each card to a
fuller page.

**[Auto-fix Markdown formatting](docs/features/auto-fix.md).**
`mdsmith fix` rewrites whitespace, headings, code fences, bare
URLs, list indentation, and table alignment in place. It loops
until edits stabilize. `mdsmith check` is the read-only CI
sibling.

**[Live diagnostics wherever you write](docs/features/live-diagnostics.md).**
`mdsmith lsp` emits diagnostics, quick-fixes, and navigation. Any
LSP-aware editor can consume it. The VS Code extension and the
Claude Code plugin surface the same data.

**[Cross-file integrity](docs/features/cross-file-integrity.md).**
Built-in rules flag broken links and missing anchors, enforce
per-file section schemas, and keep Markdown in the right folders.
Schemas can be inline on a file kind or shared via `proto.md`
files.

**[Guardrails for AI-generated docs](docs/features/ai-guardrails.md).**
Cap file, section, and token-budget size. Enforce reading grade
and sentence count. Flag verbatim copy-paste across files.

**[Self-maintaining sections](docs/features/self-maintaining-sections.md).**
On `mdsmith fix`, `<?toc?>` rebuilds a heading TOC, `<?catalog?>`
generates an index from front matter, and `<?include?>` splices
in another file. A Git merge driver resolves conflicts in those
blocks.

**[Gate releases on doc status](docs/features/release-gating.md).**
`mdsmith list query` selects files by a CUE expression on front
matter. `mdsmith metrics rank` ranks files by any shared metric.
Both pipe straight into a release script.

**[Fast on every run](docs/features/performance.md).**
A single static Go binary with no runtime to boot. The workspace
walk runs in parallel and embeds are linted once, so CI and
editor feedback stay instant.

**[Quality you can verify](docs/features/quality.md).**
The build, Go Report Card, and coverage badges at the top of the
README report live project health. mdsmith lints its own docs
with the rules it ships, and a coverage gate blocks merges that
drop below the line.
<?/include?>

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
