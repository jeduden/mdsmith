<p align="center">
<picture>
<source media="(prefers-color-scheme: dark)" srcset="website/static/img/logo-lockup-inverse.svg">
<img src="website/static/img/logo-lockup.svg" alt="mdsmith" width="320">
</picture>
</p>

[![Build][ci-badge]][ci-link]
[![Quality][grc-badge]][grc-link]
[![Coverage][cov-badge]][cov-link]

Mark*down*, smithed.

<?include
file: docs/brand/messaging.md
extract: eyebrow.text
?>
Markdown as a single source of truth
<?/include?>

<?include
file: docs/brand/messaging.md
extract: tagline.text
?>
Write content; mdsmith keeps your Markdown neat and consistent — fast enough to stay out of your way. Auto-fix on save, instant navigation, cross-file integrity, and generated sections that keep a single source of truth in sync across files and pipelines.
<?/include?>

One static Go binary checks this whole repository in well under a
second. That is an order of magnitude faster than Node
markdownlint. It does more per file than the Rust linters. The
figures are re-measured on every merge and refreshed each release.
See the [latest cross-tool benchmark][bench-live] for the current
per-tool ratios on both corpora.

<!-- Rendered by .github/workflows/demo.yml on push to main; published to the assets branch -->
<p align="center">
<img alt="mdsmith auto-fixing and checking Markdown" width="880"
src="https://raw.githubusercontent.com/jeduden/mdsmith/assets/assets/demo.gif">
</p>

<?include
file: docs/features/index.md
strip-frontmatter: "true"
heading-level: "2"
?>
## Why mdsmith

mdsmith is a Markdown linter and formatter written in Go. It
checks style, readability, structure, and cross-file integrity,
and auto-fixes what fixes cleanly. Where markdownlint-compatible
linters stop at per-file style, mdsmith adds the cross-file graph,
generated sections, structure schemas, and readability budgets.
Together they keep a whole docs tree consistent as it grows, so
the same Markdown can drive your README, your docs site, and
downstream pipelines.

Already on markdownlint? `mdsmith init --from-markdownlint`
converts your config and notes whatever needs review.

One rule engine runs everywhere you work: in CI, in your editor
through `mdsmith lsp`, and in your coding agent through a Claude
Code plugin. The check that blocks a merge is the same one you see
as you type, so feedback never depends on which tool you opened.

### Clean, consistent Markdown

Catch style, formatting, and readability problems on every file.
`mdsmith fix` rewrites the ones with a single correct fix;
`mdsmith check` is the read-only gate for CI.

**[Auto-fix Markdown formatting](docs/features/auto-fix.md).**
`mdsmith fix` rewrites whitespace, headings, code fences, bare
URLs, list indentation, and table alignment in place, looping
until edits stabilize. `mdsmith check` runs the same rules
read-only for CI.

**[Conventions and flavors](docs/features/markdown-conventions.md).**
Pin one convention to apply a curated rule preset and a target
renderer flavor together. `MDS034` flags syntax the flavor will
not render; a placeholder vocabulary leaves template tokens like
`{name}` alone.

**[Size and readability limits](docs/features/size-and-readability.md).**
Cap file, section, and token-budget size, enforce a reading grade
and sentence count, and flag verbatim copy-paste between files.
Three rules ship on by default; two are opt-in.

### One engine, every surface

The same engine runs in CI, in your editor, and in your coding
agent, from one fast static binary you can install through any
channel.

**[Editors and agents](docs/features/editor-agent-integration.md).**
A bundled VS Code extension and a Claude Code plugin drive the
same `mdsmith lsp` server, so diagnostics, fix-on-save, and
navigation reach your editor and your agent with no separate
install. The `.vsix` is republished to Open VSX for Cursor,
VSCodium, and Theia.

**[Live diagnostics wherever you write](docs/features/live-diagnostics.md).**
`mdsmith lsp` serves diagnostics, quick-fixes, and navigation
(definition, references, symbol search, and a call hierarchy) to
any LSP-aware editor over stdio.

**[Fast on every run](docs/features/performance.md).**
One static Go binary, no runtime to start. The workspace walk runs
across all cores, and includes are linted once. A full check of
this repository's Markdown takes about 0.5 s, an order of
magnitude faster than Node markdownlint.

**[Installs everywhere](docs/features/install-everywhere.md).**
The same version-stamped binary ships through go install, npm,
pip, uvx, Homebrew, mise, asdf, and GitHub Releases. No
postinstall network call, so locked-down CI installs offline.

### A connected docs tree

mdsmith reads the links, includes, and headings that tie your
files together, so a rename or a move never strands a reference.

**[Cross-file integrity](docs/features/cross-file-integrity.md).**
`MDS027` flags broken links and missing anchors across the
workspace, `MDS020` validates each file against its section
schema, and `MDS033` keeps files in their allowed folders.

**[Rename without breaking links](docs/features/rename.md).**
Rename a heading and mdsmith rewrites every workspace anchor link
that resolved to its slug in one atomic edit. Link-reference
labels rename with their uses; a colliding slug fails loudly
instead of breaking links.

**[See the dependency graph](docs/features/dependency-graph.md).**
`mdsmith deps` lists what a file pulls in (includes, catalogs,
build inputs, and links), or every file that points at it with
`--incoming`. The editor walks the same graph as a call hierarchy.

**[File kinds and schemas](docs/features/file-kinds-schemas.md).**
Tag each file with a `kind`, then validate its headings, section
order, and front matter against a schema. Declare the schema
inline on the kind or share it from a `proto.md` template, so a
whole directory obeys one contract.

### Markdown as a single source of truth

Each file stays the single source of truth. mdsmith keeps the
generated parts in sync, and can project the file out as JSON,
YAML, or msgpack.

**[Self-maintaining sections](docs/features/self-maintaining-sections.md).**
On `mdsmith fix`, `<?toc?>` rebuilds a heading table of contents,
`<?catalog?>` generates an index from front matter, and
`<?include?>` splices in another file. A Git merge driver resolves
conflicts inside those blocks.

**[Agent-ready docs index](docs/features/progressive-disclosure.md).**
A `<?catalog?>` in `CLAUDE.md` keeps one `summary` line per
tracked doc, so an agent reads a few thousand tokens of metadata
up front and opens only the files a task touches. mdsmith's own
[`CLAUDE.md`](CLAUDE.md) is the live example.

**[Build artifacts in sync](docs/features/build-artifacts.md).**
The `<?build?>` directive declares an artifact and a recipe;
`mdsmith fix` rebuilds only the targets whose inputs, recipe, or
outputs changed, tracking freshness in
`.mdsmith/build-cache.json`. `MDS040` shell-safety-checks the
recipe without running it.

**[Markdown as a data source](docs/features/markdown-as-data.md).**
`mdsmith extract` projects a schema-conformant file to a JSON,
YAML, or msgpack tree, and `<?include extract:?>` reads one value
back into another file. `mdsmith export` writes a portable,
directive-free copy that renders on any Markdown tool.

### Built for your pipeline

Release gates, a Git merge driver, transparent config, and a
coverage-gated build make mdsmith safe to wire into a shared
repository.

**[Gate releases on doc status](docs/features/release-gating.md).**
`mdsmith list query 'status: "✅"' plan/` selects files by a CUE
expression on front matter, and `mdsmith metrics rank` orders
files by any shared metric. Both print plain lines ready to pipe
into a release script.

**[Git-native, conflict-free](docs/features/git-native.md).**
A Git merge driver re-runs the directive and keeps the regenerated
body when two branches both touch a generated block. A
pre-merge-commit hook re-runs `mdsmith fix` and re-stages the
result, so generated content never blocks a merge.

**[Config you can explain](docs/features/config-transparency.md).**
Config layers deep-merge rule by rule: defaults, convention,
kinds, then per-glob overrides. `mdsmith check --explain` and
`mdsmith kinds resolve` show which layer set each effective value.

**[Quality you can verify](docs/features/quality.md).**
The CI, Go Report Card, and Codecov badges report live project
health. mdsmith lints its own docs with the rules it ships, and a
coverage gate blocks any merge that drops below the line.
<?/include?>

## 🚀 Quickstart

```bash
go install github.com/jeduden/mdsmith/cmd/mdsmith@latest  # or npm / pip / brew (see Installation)
mdsmith check .   # lint every Markdown file; non-zero exit on failure (CI-ready)
mdsmith fix .     # auto-fix what fixes cleanly, in place
```

`check` prints each problem — location, rule, and a source snippet
with a caret under the offending column — then a summary, exiting non-zero:

```text
docs/guide.md:1:81 MDS001 line too long (89 > 80)
1 | # This heading runs deliberately long so that it spills well past the eighty-column limit
····················································································^
stats: checked=1 fixed=0 failures=1 unfixed=1
```

## 📦 Installation

CLI:

```bash
go install github.com/jeduden/mdsmith/cmd/mdsmith@latest
npm install -g @mdsmith/cli    # or: npx @mdsmith/cli
pip install mdsmith            # or: uvx mdsmith / pipx install mdsmith
brew install jeduden/mdsmith/mdsmith   # macOS / Linux (Homebrew)
```

Editor extension (LSP-backed; runs `mdsmith lsp`):

```bash
code --install-extension jeduden.mdsmith     # VS Code, Codespaces (Marketplace)
codium --install-extension jeduden.mdsmith   # Cursor, VSCodium, Theia, Gitpod (Open VSX)
```

Claude Code plugin (diagnostics + cross-file navigation in your agent):

```text
/plugin marketplace add jeduden/mdsmith
/plugin install mdsmith-lsp@mdsmith
/reload-plugins
```

More: the [install guide](docs/guides/install.md) covers asdf, mise,
and direct downloads; [VS Code setup](docs/guides/editors/vscode.md)
covers settings and troubleshooting.

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
| Command                                                      | Description                                                                                                                               |
| ------------------------------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------- |
| [`check`](docs/reference/cli/check.md)                       | Lint Markdown files for style issues.                                                                                                     |
| [`deps`](docs/reference/cli/deps.md)                         | List a file's dependency-graph edges (includes, links, catalogs, builds).                                                                 |
| [`export`](docs/reference/cli/export.md)                     | Write a portable, directive-free copy of a Markdown file.                                                                                 |
| [`extract`](docs/reference/cli/extract.md)                   | Emit a schema-conformant Markdown file as a JSON/YAML/msgpack data tree.                                                                  |
| [`fix`](docs/reference/cli/fix.md)                           | Auto-fix lint issues in Markdown files in place.                                                                                          |
| [`help`](docs/reference/cli/help.md)                         | Show built-in documentation for rules, metrics, and concept pages.                                                                        |
| [`init`](docs/reference/cli/init.md)                         | Generate a default `.mdsmith.yml` config in the current directory, or convert an existing markdownlint config with `--from-markdownlint`. |
| [`kinds`](docs/reference/cli/kinds.md)                       | Inspect declared file kinds and resolve effective rule config per file.                                                                   |
| [`list`](docs/reference/cli/list.md)                         | Selection-style commands that walk the workspace and emit matches.                                                                        |
| [`list backlinks`](docs/reference/cli/backlinks.md)          | List workspace links that point at a file.                                                                                                |
| [`list query`](docs/reference/cli/query.md)                  | Select Markdown files by a CUE expression on front matter.                                                                                |
| [`lsp`](docs/reference/cli/lsp.md)                           | Run a Language Server Protocol server on stdio for editor integrations.                                                                   |
| [`merge-driver`](docs/reference/cli/merge-driver.md)         | Git merge driver that resolves conflicts inside generated sections.                                                                       |
| [`metrics`](docs/reference/cli/metrics.md)                   | List and rank shared Markdown metrics (file length, token estimate, readability, …).                                                      |
| [`pre-merge-commit`](docs/reference/cli/pre-merge-commit.md) | Install / manage a pre-merge-commit hook that runs `mdsmith fix` after a merge.                                                           |
| [`rename`](docs/reference/cli/rename.md)                     | Rename a heading or link-reference label and rewrite every dependent edit.                                                                |
| [`version`](docs/reference/cli/version.md)                   | Print the mdsmith build version and exit.                                                                                                 |
<?/catalog?>

That command table and the feature list above are generated by mdsmith's own directives.

Files can be paths, directories (walked for `*.md`/`*.markdown`), or
globs; directories respect `.gitignore` (`--no-gitignore` overrides).
The [CLI reference](docs/reference/cli.md) covers shared flags, exit
codes, output, and merge semantics; each page has its own examples.

## ⚙️ Configuration

`mdsmith init` writes a `.mdsmith.yml` of all rules at their defaults;
without one, built-in defaults apply.

```yaml
rules:
  line-length:
    max: 120
  fenced-code-language: false

overrides:
  - glob: ["CHANGELOG.md"]
    rules:
      no-duplicate-headings: false
```

Each rule is `true` (default), `false` (off), or an object of settings.
`overrides` apply per glob; later entries win. Config is found by walking
up to the repo root. Commit it so contributors share one ruleset and
upgrades stay reviewable.

## 📚 More

- [How mdsmith compares](docs/background/markdown-linters.md) to other linters
- [Guides](docs/guides/index.md) — directives, structure, migration
- [Rule directory](internal/rules/index.md) — every rule with status
- [CLI reference](docs/reference/cli.md)
- [Contributor guide](docs/development/index.md) — Go 1.25+, build, test, style

## 📄 License

[MIT](LICENSE)

<!-- badges -->

[ci-badge]: https://github.com/jeduden/mdsmith/actions/workflows/ci.yml/badge.svg?branch=main
[ci-link]: https://github.com/jeduden/mdsmith/actions/workflows/ci.yml?query=branch%3Amain
[grc-badge]: https://goreportcard.com/badge/github.com/jeduden/mdsmith
[grc-link]: https://goreportcard.com/report/github.com/jeduden/mdsmith
[cov-badge]: https://codecov.io/gh/jeduden/mdsmith/branch/main/graph/badge.svg
[cov-link]: https://codecov.io/gh/jeduden/mdsmith/branch/main
[bench-live]: https://github.com/jeduden/mdsmith/blob/assets/assets/benchmarks/results.fragment.md
