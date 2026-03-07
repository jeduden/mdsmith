# 🔨 mdsmith

A fast, auto-fixing Markdown linter and formatter for docs, READMEs,
and AI-generated content. Checks style, readability, and structure.
Written in Go.

## ✨ Why mdsmith

**📋 Progressive disclosure with catalogs.**
The [`catalog`](internal/rules/MDS019-catalog/README.md) rule generates summary
tables from file front matter and keeps them in sync.
Link each row to the full document —
readers see the overview first and drill down on demand.
Run `mdsmith fix` and the table updates itself.

**🤖 Keep AI verbosity in check.**
AI tools produce walls of text.
[`max-file-length`](internal/rules/MDS022-max-file-length/README.md)
caps document size,
[`paragraph-readability`](internal/rules/MDS023-paragraph-readability/README.md)
enforces a reading-grade ceiling,
and [`paragraph-structure`](internal/rules/MDS024-paragraph-structure/README.md)
limits sentence count and length.
[`token-budget`](internal/rules/MDS028-token-budget/README.md)
adds a token-aware
budget with heuristic and tokenizer modes.
Set the thresholds in `.mdsmith.yml` and let CI enforce them.

**📖 AI-ready rule specs — no remote calls.**
`mdsmith help rule` lists every rule with its ID and description.
`mdsmith help rule <name>` prints the full spec: settings, examples,
diagnostics. All docs are compiled into the binary — works offline,
works in CI, works as a source for `.cursor/rules` or `AGENTS.md`.
`mdsmith help metrics` and `mdsmith help metrics <name>` do the same
for shared file metrics.

**🔧 Auto-fix.**
`mdsmith fix` corrects most rules in place.
Whitespace, heading style, code fences, bare URLs, list indentation,
table alignment, and generated sections — all handled.
Multi-pass fixing resolves cascading changes automatically.

## 📦 Installation

```bash
go install github.com/jeduden/mdsmith/cmd/mdsmith@latest
```

## 🚀 Usage

```text
mdsmith <command> [flags] [files...]
```

### Commands

| Command      | Description                               |
|--------------|-------------------------------------------|
| `check`        | Lint files (default command)              |
| `fix`          | Auto-fix issues in place                  |
| `help`         | Show help for docs and topics             |
| `metrics`      | List and rank Markdown metrics            |
| `merge-driver` | Git merge driver for regenerable sections |
| `init`         | Generate `.mdsmith.yml`                     |
| `version`      | Print version, exit                       |

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
mdsmith check docs/            # lint a directory
mdsmith fix README.md          # auto-fix in place
mdsmith check -f json docs/    # JSON output
mdsmith metrics rank --by bytes --top 10 .
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

Create a `.mdsmith.yml` in your project root, or run
`mdsmith init` to generate one with every rule and its
default settings.
Without a config file, rules run with their built-in
defaults.

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
default enablement and settings. This pins the config to the current defaults so
that future
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

<?catalog
glob: "docs/guides/*.md"
sort: title
header: |
  | Guide | Description |
  |-------|-------------|
row: "| [{{.title}}]({{.filename}}) | {{.summary}} |"
empty: |
  | Guide         | Description                                                |
  |---------------|------------------------------------------------------------|
  | No guides yet | Add guide files under `docs/guides/` to populate this index. |
?>
| Guide                                                       | Description                                                                              |
|-------------------------------------------------------------|------------------------------------------------------------------------------------------|
| [Choosing Readability, Conciseness, and Token Budget Metrics](docs/guides/metrics-tradeoffs.md) | Trade-offs and threshold guidance for readability, structure, length, and token budgets. |
<?/catalog?>

## 📏 Rules

<?catalog
glob: "internal/rules/MDS*/README.md"
sort: id
header: |
  | Rule | Status | Description |
  |------|--------|-------------|
row: "| [{{.id}} {{.name}}]({{.filename}}) | {{.status}} | {{.description}} |"
?>
| Rule                                      | Status    | Description                                                                             |
|-------------------------------------------|-----------|-----------------------------------------------------------------------------------------|
| [MDS001 line-length](internal/rules/MDS001-line-length/README.md)                        | ready     | Line exceeds maximum length.                                                            |
| [MDS002 heading-style](internal/rules/MDS002-heading-style/README.md)                      | ready     | Heading style must be consistent.                                                       |
| [MDS003 heading-increment](internal/rules/MDS003-heading-increment/README.md)                  | ready     | Heading levels should increment by one. No jumping from `#` to `###`.                       |
| [MDS004 first-line-heading](internal/rules/MDS004-first-line-heading/README.md)                 | ready     | First line of the file should be a heading.                                             |
| [MDS005 no-duplicate-headings](internal/rules/MDS005-no-duplicate-headings/README.md)              | ready     | No two headings should have the same text.                                              |
| [MDS006 no-trailing-spaces](internal/rules/MDS006-no-trailing-spaces/README.md)                 | ready     | No trailing whitespace at the end of lines.                                             |
| [MDS007 no-hard-tabs](internal/rules/MDS007-no-hard-tabs/README.md)                       | ready     | No tab characters. Use spaces instead.                                                  |
| [MDS008 no-multiple-blanks](internal/rules/MDS008-no-multiple-blanks/README.md)                 | ready     | No more than one consecutive blank line.                                                |
| [MDS009 single-trailing-newline](internal/rules/MDS009-single-trailing-newline/README.md)            | ready     | File must end with exactly one newline character.                                       |
| [MDS010 fenced-code-style](internal/rules/MDS010-fenced-code-style/README.md)                  | ready     | Fenced code blocks must use a consistent delimiter.                                     |
| [MDS011 fenced-code-language](internal/rules/MDS011-fenced-code-language/README.md)               | ready     | Fenced code blocks must specify a language.                                             |
| [MDS012 no-bare-urls](internal/rules/MDS012-no-bare-urls/README.md)                       | ready     | URLs must be wrapped in angle brackets or as a link, not left bare.                     |
| [MDS013 blank-line-around-headings](internal/rules/MDS013-blank-line-around-headings/README.md)         | ready     | Headings must have a blank line before and after.                                       |
| [MDS014 blank-line-around-lists](internal/rules/MDS014-blank-line-around-lists/README.md)            | ready     | Lists must have a blank line before and after.                                          |
| [MDS015 blank-line-around-fenced-code](internal/rules/MDS015-blank-line-around-fenced-code/README.md)      | ready     | Fenced code blocks must have a blank line before and after.                             |
| [MDS016 list-indent](internal/rules/MDS016-list-indent/README.md)                        | ready     | List items must use consistent indentation.                                             |
| [MDS017 no-trailing-punctuation-in-heading](internal/rules/MDS017-no-trailing-punctuation-in-heading/README.md) | ready     | Headings should not end with punctuation.                                               |
| [MDS018 no-emphasis-as-heading](internal/rules/MDS018-no-emphasis-as-heading/README.md)             | ready     | Don't use bold or emphasis on a standalone line as a heading substitute.                |
| [MDS019 catalog](internal/rules/MDS019-catalog/README.md)                            | ready     | Catalog content must reflect selected front matter fields from files matching its glob. |
| [MDS020 required-structure](internal/rules/MDS020-required-structure/README.md)                 | ready     | Document must match the heading structure defined by its template.                      |
| [MDS021 include](internal/rules/MDS021-include/README.md)                            | ready     | Include section content must match the referenced file.                                 |
| [MDS022 max-file-length](internal/rules/MDS022-max-file-length/README.md)                    | ready     | File must not exceed maximum number of lines.                                           |
| [MDS023 paragraph-readability](internal/rules/MDS023-paragraph-readability/README.md)              | ready     | Paragraph readability grade must not exceed a threshold.                                |
| [MDS024 paragraph-structure](internal/rules/MDS024-paragraph-structure/README.md)                | ready     | Paragraphs must not exceed sentence and word limits.                                    |
| [MDS025 table-format](internal/rules/MDS025-table-format/README.md)                       | ready     | Tables must have consistent column widths and padding.                                  |
| [MDS026 table-readability](internal/rules/MDS026-table-readability/README.md)                  | ready     | Tables must stay within readability complexity limits.                                  |
| [MDS027 cross-file-reference-integrity](internal/rules/MDS027-cross-file-reference-integrity/README.md)     | ready     | Links to local files and heading anchors must resolve.                                  |
| [MDS028 token-budget](internal/rules/MDS028-token-budget/README.md)                       | ready     | File must not exceed a token budget.                                                    |
| [MDS029 conciseness-scoring](internal/rules/MDS029-conciseness-scoring/README.md)                | not-ready | Paragraph conciseness score must not fall below a threshold.                            |
| [MDS030 empty-section-body](internal/rules/MDS030-empty-section-body/README.md)                 | ready     | Section headings must include meaningful body content.                                  |
<?/catalog?>

## 🛠️ Development

Requires Go 1.24+. See
[`DEVELOPMENT.md`](DEVELOPMENT.md) for the full
contributor guide (build commands, project layout,
workflow, code style, and PR conventions).

## 📂 Documentation

- [CLI design](docs/design/cli.md)
- [Design archetypes](docs/design/archetypes/)
- [Guides](docs/guides/)
- [Background](docs/background/)
- [Plans](plan/)

## 📄 License

[MIT](LICENSE)
