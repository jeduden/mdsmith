# 🔨 mdsmith

A fast, auto-fixing Markdown linter and formatter for docs, READMEs,
and AI-generated content. Checks style, readability, and structure.
Written in Go.

## ✨ Why mdsmith

**📋 Progressive disclosure with catalogs.**
The [`catalog`](rules/MDS019-catalog/README.md) rule generates summary
tables from file front matter and keeps them in sync.
Link each row to the full document —
readers see the overview first and drill down on demand.
Run `mdsmith fix` and the table updates itself.

**🤖 Keep AI verbosity in check.**
AI tools produce walls of text.
[`max-file-length`](rules/MDS022-max-file-length/README.md) caps document size,
[`paragraph-readability`](rules/MDS023-paragraph-readability/README.md)
enforces a reading-grade ceiling,
and [`paragraph-structure`](rules/MDS024-paragraph-structure/README.md)
limits sentence count and length.
[`token-budget`](rules/MDS028-token-budget/README.md) adds a token-aware
budget with heuristic and tokenizer modes.
Set the thresholds in `.mdsmith.yml` and let CI enforce them.

**📖 AI-ready rule specs — no remote calls.**
`mdsmith help rule` lists every rule with its ID and description.
`mdsmith help rule <name>` prints the full spec: settings, examples,
diagnostics. All docs are compiled into the binary — works offline,
works in CI, works as a source for `.cursor/rules` or `AGENTS.md`.

**🔧 Auto-fix.**
`mdsmith fix` corrects most rules in place.
Whitespace, heading style, code fences, bare URLs, list indentation,
table alignment, and generated sections — all handled.
Multi-pass fixing resolves cascading changes automatically.

## 📦 Installation

```bash
go install github.com/jeduden/mdsmith@latest
```

## 🚀 Usage

```text
mdsmith <command> [flags] [files...]
```

### Commands

| Command | Description                  |
|---------|------------------------------|
| `check`   | Lint files (default command) |
| `fix`     | Auto-fix issues in place     |
| `init`    | Generate `.mdsmith.yml`        |
| `version` | Print version, exit          |

Files can be paths, directories (walked recursively for `*.md`),
or glob patterns.
With no arguments and no piped input, mdsmith exits 0.

When walking directories, mdsmith respects `.gitignore` files by default.
Files matched by `.gitignore` patterns are skipped, including patterns from
nested `.gitignore` files in subdirectories and ancestor directories.
Explicitly named file paths are never filtered by gitignore.
Use `--no-gitignore` to disable this behavior and lint all files.

### Flags

| Flag           | Description    |
|----------------|----------------|
| `-c`, `--config`   | Config path    |
| `-f`, `--format`   | `text` or `json`   |
| `--no-color`     | Plain output   |
| `--no-gitignore` | Skip gitignore |
| `-q`, `--quiet`    | Quiet mode     |

### Examples

```bash
# Lint a single file
mdsmith check README.md

# Lint all Markdown in a directory
mdsmith check docs/

# Auto-fix issues
mdsmith fix README.md

# Pipe from stdin
cat README.md | mdsmith check

# JSON output
mdsmith check -f json docs/

# Generate default config
mdsmith init
```

### Output

Diagnostics are printed to stderr:

```text
README.md:10:1 MDS001 line too long (135 > 80)
```

Pattern: `file:line:col rule message`

### Exit codes

| Code | Meaning                        |
|------|--------------------------------|
| 0    | No lint issues found           |
| 1    | Lint issues found              |
| 2    | Runtime or configuration error |

## ⚙️ Configuration

Create a `.mdsmith.yml` in your project root.
Without one, all rules are enabled with defaults.

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

Rules can be `true` (enable with defaults), `false` (disable),
or an object with settings.
The `overrides` list applies different rules per file pattern.
Later overrides take precedence.

Config is discovered by walking up from the current directory to the repo root.
Use `--config` to override.

### Bootstrapping with `mdsmith init`

Run `mdsmith init` to generate a `.mdsmith.yml` with every rule and its
default settings. This pins the config to the current defaults so that future
mdsmith upgrades (which may change defaults) do not silently alter your
lint results. Review the generated file and adjust settings to match your
project's conventions.

```bash
mdsmith init
# creates .mdsmith.yml with all rule defaults
```

Commit the generated file to version control.
This ensures every contributor uses the same rule settings.
Upgrades become an explicit, reviewable change.

## 📚 Guides

<!-- catalog
glob: "guides/*.md"
sort: title
header: |
  | Guide | Description |
  |-------|-------------|
row: "| [{{.title}}]({{.filename}}) | {{.description}} |"
empty: |
  | Guide         | Description                                           |
  |---------------|-------------------------------------------------------|
  | No guides yet | Add guide files under `guides/` to populate this index. |
-->
| Guide                                                       | Description                                                                                                      |
|-------------------------------------------------------------|------------------------------------------------------------------------------------------------------------------|
| [Choosing Readability, Conciseness, and Token Budget Metrics](guides/metrics-tradeoffs.md) | Trade-offs, examples, and threshold guidance for readability, structure, length, conciseness, and token budgets. |
<!-- /catalog -->

## 📏 Rules

<!-- catalog
glob: "rules/MDS*/README.md"
sort: id
header: |
  | Rule | Name | Description |
  |------|------|-------------|
row: "| [{{.id}}]({{.filename}}) | `{{.name}}` | {{.description}} |"
-->
| Rule   | Name                               | Description                                                                             |
|--------|------------------------------------|-----------------------------------------------------------------------------------------|
| [MDS001](rules/MDS001-line-length/README.md) | `line-length`                        | Line exceeds maximum length.                                                            |
| [MDS002](rules/MDS002-heading-style/README.md) | `heading-style`                      | Heading style must be consistent.                                                       |
| [MDS003](rules/MDS003-heading-increment/README.md) | `heading-increment`                  | Heading levels should increment by one. No jumping from `#` to `###`.                       |
| [MDS004](rules/MDS004-first-line-heading/README.md) | `first-line-heading`                 | First line of the file should be a heading.                                             |
| [MDS005](rules/MDS005-no-duplicate-headings/README.md) | `no-duplicate-headings`              | No two headings should have the same text.                                              |
| [MDS006](rules/MDS006-no-trailing-spaces/README.md) | `no-trailing-spaces`                 | No trailing whitespace at the end of lines.                                             |
| [MDS007](rules/MDS007-no-hard-tabs/README.md) | `no-hard-tabs`                       | No tab characters. Use spaces instead.                                                  |
| [MDS008](rules/MDS008-no-multiple-blanks/README.md) | `no-multiple-blanks`                 | No more than one consecutive blank line.                                                |
| [MDS009](rules/MDS009-single-trailing-newline/README.md) | `single-trailing-newline`            | File must end with exactly one newline character.                                       |
| [MDS010](rules/MDS010-fenced-code-style/README.md) | `fenced-code-style`                  | Fenced code blocks must use a consistent delimiter.                                     |
| [MDS011](rules/MDS011-fenced-code-language/README.md) | `fenced-code-language`               | Fenced code blocks must specify a language.                                             |
| [MDS012](rules/MDS012-no-bare-urls/README.md) | `no-bare-urls`                       | URLs must be wrapped in angle brackets or as a link, not left bare.                     |
| [MDS013](rules/MDS013-blank-line-around-headings/README.md) | `blank-line-around-headings`         | Headings must have a blank line before and after.                                       |
| [MDS014](rules/MDS014-blank-line-around-lists/README.md) | `blank-line-around-lists`            | Lists must have a blank line before and after.                                          |
| [MDS015](rules/MDS015-blank-line-around-fenced-code/README.md) | `blank-line-around-fenced-code`      | Fenced code blocks must have a blank line before and after.                             |
| [MDS016](rules/MDS016-list-indent/README.md) | `list-indent`                        | List items must use consistent indentation.                                             |
| [MDS017](rules/MDS017-no-trailing-punctuation-in-heading/README.md) | `no-trailing-punctuation-in-heading` | Headings should not end with punctuation.                                               |
| [MDS018](rules/MDS018-no-emphasis-as-heading/README.md) | `no-emphasis-as-heading`             | Don't use bold or emphasis on a standalone line as a heading substitute.                |
| [MDS019](rules/MDS019-catalog/README.md) | `catalog`                            | Catalog content must reflect selected front matter fields from files matching its glob. |
| [MDS020](rules/MDS020-required-structure/README.md) | `required-structure`                 | Document must match the heading structure defined by its template.                      |
| [MDS021](rules/MDS021-include/README.md) | `include`                            | Include section content must match the referenced file.                                 |
| [MDS022](rules/MDS022-max-file-length/README.md) | `max-file-length`                    | File must not exceed maximum number of lines.                                           |
| [MDS023](rules/MDS023-paragraph-readability/README.md) | `paragraph-readability`              | Paragraph readability grade must not exceed a threshold.                                |
| [MDS024](rules/MDS024-paragraph-structure/README.md) | `paragraph-structure`                | Paragraphs must not exceed sentence and word limits.                                    |
| [MDS025](rules/MDS025-table-format/README.md) | `table-format`                       | Tables must have consistent column widths and padding.                                  |
| [MDS026](rules/MDS026-table-readability/README.md) | `table-readability`                  | Tables must stay within readability complexity limits.                                  |
| [MDS027](rules/MDS027-cross-file-reference-integrity/README.md) | `cross-file-reference-integrity`     | Links to local files and heading anchors must resolve.                                  |
| [MDS028](rules/MDS028-token-budget/README.md) | `token-budget`                       | File must not exceed a token budget.                                                    |
<!-- /catalog -->

## 🛠️ Development

### Prerequisites

- Go 1.24+
- [golangci-lint](https://golangci-lint.run/)

### Lint

```bash
golangci-lint run
```

### Test

```bash
go test ./...
```

### Pre-commit check

```bash
mdsmith check .
```

Run before committing to ensure all markdown files pass linting.

## 📄 License

[MIT](LICENSE)
