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

Each subcommand earns its place. The pillars below name the
subcommand (and rules) that deliver it.

**🔧 Lint and auto-fix — `check`, `fix`.**
`mdsmith check` reports lint diagnostics with source
context; `mdsmith fix` corrects most rules in place:
whitespace, heading style, code fences, bare URLs, list
indentation, table alignment. Multi-pass fixing resolves
cascading changes automatically.

**📋 Generated sections — `fix`, `merge-driver`.**
Embed live content via `<?catalog?>`, `<?toc?>`, and
`<?include?>` directives — summary tables from front
matter, tables of contents from headings, file
inclusions. `fix` regenerates them in place;
`merge-driver install` registers a Git driver that
auto-resolves merge conflicts inside those sections.

**🔗 Cross-file integrity — `check`, `archetypes`.**
Broken links rot in silence.
[`cross-file-reference-integrity`](internal/rules/MDS027-cross-file-reference-integrity/README.md)
flags missing files and missing heading anchors before merge.
[`required-structure`](internal/rules/MDS020-required-structure/README.md)
checks each file against a schema.
`mdsmith archetypes` manages those schemas as reusable
templates discovered under configured roots.
[`directory-structure`](internal/rules/MDS033-directory-structure/README.md)
keeps Markdown in the right folders.

**🤖 Keep AI verbosity in check — `check`.**
AI tools produce walls of text. Cap file length with
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

**🔍 Triage your corpus — `query`, `metrics`.**
`mdsmith query 'expr' paths…` filters files by a CUE
expression on front matter (e.g. status, owner).
`mdsmith metrics rank --by <metric>` ranks files by
size, tokens, sections, and other shared metrics —
useful for finding the biggest offenders.

**📖 AI-ready specs — `help`, no remote calls.**
`mdsmith help rule [name]` prints rule docs (settings,
examples, diagnostics) compiled into the binary. Works
offline, in CI, or as a source for `.cursor/rules` or
`AGENTS.md`. `mdsmith help metrics [name]` does the same
for shared file metrics.

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

Files can be paths, directories (walked recursively for `*.md`),
or glob patterns. Directories respect `.gitignore` by default;
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
