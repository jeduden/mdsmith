# tidymark

A Markdown linter written in Go.

## Installation

```bash
go install github.com/jeduden/tidymark@latest
```

## Usage

```text
tidymark <command> [flags] [files...]
```

### Commands

| Command | Description                  |
|---------|------------------------------|
| `check`   | Lint files (default command) |
| `fix`     | Auto-fix issues in place     |
| `init`    | Generate `.tidymark.yml`       |
| `version` | Print version, exit          |

Files can be paths, directories (walked recursively for `*.md`),
or glob patterns.
With no arguments and no piped input, tidymark exits 0.

When walking directories, tidymark respects `.gitignore` files by default.
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
tidymark check README.md

# Lint all Markdown in a directory
tidymark check docs/

# Auto-fix issues
tidymark fix README.md

# Pipe from stdin
cat README.md | tidymark check

# JSON output
tidymark check -f json docs/

# Generate default config
tidymark init
```

### Output

Diagnostics are printed to stderr:

```text
README.md:10:1 TM001 line too long (135 > 80)
```

Pattern: `file:line:col rule message`

### Exit codes

| Code | Meaning                        |
|------|--------------------------------|
| 0    | No lint issues found           |
| 1    | Lint issues found              |
| 2    | Runtime or configuration error |

## Configuration

Create a `.tidymark.yml` in your project root.
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

### Bootstrapping with `tidymark init`

Run `tidymark init` to generate a `.tidymark.yml` with every rule and its
default settings. This pins the config to the current defaults so that future
tidymark upgrades (which may change defaults) do not silently alter your
lint results. Review the generated file and adjust settings to match your
project's conventions.

```bash
tidymark init
# creates .tidymark.yml with all rule defaults
```

Commit the generated file to version control.
This ensures every contributor uses the same rule settings.
Upgrades become an explicit, reviewable change.

## Rules

<!-- catalog
glob: "rules/TM*/README.md"
sort: id
header: |
  | Rule | Name | Description |
  |------|------|-------------|
row: "| [{{.id}}]({{.filename}}) | `{{.name}}` | {{.description}} |"
-->
| Rule  | Name                               | Description                                                                             |
|-------|------------------------------------|-----------------------------------------------------------------------------------------|
| [TM001](rules/TM001-line-length/README.md) | `line-length`                        | Line exceeds maximum length.                                                            |
| [TM002](rules/TM002-heading-style/README.md) | `heading-style`                      | Heading style must be consistent.                                                       |
| [TM003](rules/TM003-heading-increment/README.md) | `heading-increment`                  | Heading levels should increment by one. No jumping from `#` to `###`.                       |
| [TM004](rules/TM004-first-line-heading/README.md) | `first-line-heading`                 | First line of the file should be a heading.                                             |
| [TM005](rules/TM005-no-duplicate-headings/README.md) | `no-duplicate-headings`              | No two headings should have the same text.                                              |
| [TM006](rules/TM006-no-trailing-spaces/README.md) | `no-trailing-spaces`                 | No trailing whitespace at the end of lines.                                             |
| [TM007](rules/TM007-no-hard-tabs/README.md) | `no-hard-tabs`                       | No tab characters. Use spaces instead.                                                  |
| [TM008](rules/TM008-no-multiple-blanks/README.md) | `no-multiple-blanks`                 | No more than one consecutive blank line.                                                |
| [TM009](rules/TM009-single-trailing-newline/README.md) | `single-trailing-newline`            | File must end with exactly one newline character.                                       |
| [TM010](rules/TM010-fenced-code-style/README.md) | `fenced-code-style`                  | Fenced code blocks must use a consistent delimiter.                                     |
| [TM011](rules/TM011-fenced-code-language/README.md) | `fenced-code-language`               | Fenced code blocks must specify a language.                                             |
| [TM012](rules/TM012-no-bare-urls/README.md) | `no-bare-urls`                       | URLs must be wrapped in angle brackets or as a link, not left bare.                     |
| [TM013](rules/TM013-blank-line-around-headings/README.md) | `blank-line-around-headings`         | Headings must have a blank line before and after.                                       |
| [TM014](rules/TM014-blank-line-around-lists/README.md) | `blank-line-around-lists`            | Lists must have a blank line before and after.                                          |
| [TM015](rules/TM015-blank-line-around-fenced-code/README.md) | `blank-line-around-fenced-code`      | Fenced code blocks must have a blank line before and after.                             |
| [TM016](rules/TM016-list-indent/README.md) | `list-indent`                        | List items must use consistent indentation.                                             |
| [TM017](rules/TM017-no-trailing-punctuation-in-heading/README.md) | `no-trailing-punctuation-in-heading` | Headings should not end with punctuation.                                               |
| [TM018](rules/TM018-no-emphasis-as-heading/README.md) | `no-emphasis-as-heading`             | Don't use bold or emphasis on a standalone line as a heading substitute.                |
| [TM019](rules/TM019-catalog/README.md) | `catalog`                            | Catalog content must reflect selected front matter fields from files matching its glob. |
| [TM020](rules/TM020-required-structure/README.md) | `required-structure`                 | Document must match the heading structure defined by its template.                      |
| [TM021](rules/TM021-include/README.md) | `include`                            | Include section content must match the referenced file.                                 |
| [TM022](rules/TM022-max-file-length/README.md) | `max-file-length`                    | File must not exceed maximum number of lines.                                           |
| [TM023](rules/TM023-paragraph-readability/README.md) | `paragraph-readability`              | Paragraph readability grade must not exceed a threshold.                                |
| [TM024](rules/TM024-paragraph-structure/README.md) | `paragraph-structure`                | Paragraphs must not exceed sentence and word limits.                                    |
| [TM025](rules/TM025-table-format/README.md) | `table-format`                       | Tables must have consistent column widths and padding.                                  |
<!-- /catalog -->

## Development

### Prerequisites

- Go 1.25+
- [golangci-lint](https://golangci-lint.run/)

### Lint

```bash
golangci-lint run
```

### Test

```bash
go test ./...
```

## License

[MIT](LICENSE)
