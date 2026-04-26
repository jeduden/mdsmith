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

**🔗 Catch broken links before they merge.**
Refactors silently break Markdown links and anchors.
[`cross-file-reference-integrity`](internal/rules/MDS027-cross-file-reference-integrity/README.md)
flags every missing file and missing heading anchor in PR
review. Pair it with
[`required-structure`](internal/rules/MDS020-required-structure/README.md)
to enforce that each file has the sections it should
(reusable schemas live under `mdsmith archetypes`), and
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
`mdsmith query 'status: "✅"' plan/` lists every plan
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

## 📦 Installation

```bash
go install github.com/jeduden/mdsmith/cmd/mdsmith@latest
```

## 🚀 Usage

```text
mdsmith <command> [flags] [files...]
```

### Commands

| Command        | Description                                    |
|----------------|------------------------------------------------|
| `check`        | Lint files (default command)                   |
| `fix`          | Auto-fix issues in place                       |
| `query`        | Select files by CUE expression on front matter |
| `help`         | Show help for rules and topics                 |
| `metrics`      | List and rank Markdown metrics                 |
| `merge-driver` | Git merge driver for regenerable sections      |
| `archetypes`   | Discover, show, and locate archetype schemas   |
| `init`         | Generate `.mdsmith.yml`                        |
| `version`      | Print version, exit                            |

Files can be paths, directories (walked recursively for `*.md`
and `*.markdown`), or glob patterns. Directories respect
`.gitignore` by default;
use `--no-gitignore` to override. Explicitly named files are
never filtered by `.gitignore`.

### Flags

| Flag             | Description      |
|------------------|------------------|
| `-c`, `--config` | Config path      |
| `-f`, `--format` | `text` or `json` |
| `--no-color`     | Plain output     |
| `--no-gitignore` | Skip gitignore   |
| `-q`, `--quiet`  | Quiet mode       |

### Examples

```bash
mdsmith check docs/            # lint a directory
mdsmith fix README.md          # auto-fix in place
mdsmith check -f json docs/    # JSON output
mdsmith metrics rank --by bytes --top 10 .
```

### Output

Diagnostics are printed to stderr with source context when available:

```text
README.md:10:81 MDS001 line too long (135 > 80)
 8 | Context lines appear above and below the diagnostic with line numbers.
 9 | They help you see the surrounding code at a glance.
10 | This line is deliberately made long so it exceeds the eighty character limit and keeps going and going.
·····················································································^
11 | A dot path runs from column 1 to the caret, marking the line and column.
12 | Up to two lines of context are shown on each side.
```

### Exit codes

| Code | Meaning                        |
|------|--------------------------------|
| 0    | No lint issues found           |
| 1    | Lint issues found              |
| 2    | Runtime or configuration error |

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
  - files: ["CHANGELOG.md"]
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
