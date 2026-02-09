# tidymark

A Markdown linter written in Go.

## Installation

```bash
go install github.com/jeduden/tidymark@latest
```

## Usage

```
tidymark [flags] [files...]
```

Files can be paths, directories (walked recursively for `*.md`), or glob patterns.
With no arguments and no piped input, tidymark exits 0.

### Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--config <file>` | `-c` | auto-discover | Override config file path |
| `--fix` | | `false` | Auto-fix issues in place |
| `--format <fmt>` | `-f` | `text` | Output format: `text`, `json` |
| `--no-color` | | `false` | Disable ANSI colors |
| `--quiet` | `-q` | `false` | Suppress non-error output |
| `--version` | `-v` | | Print version and exit |

### Examples

```bash
# Lint a single file
tidymark README.md

# Lint all Markdown in a directory
tidymark docs/

# Auto-fix issues
tidymark --fix README.md

# Pipe from stdin
cat README.md | tidymark

# JSON output
tidymark -f json docs/
```

### Output

Diagnostics are printed to stderr:

```
README.md:10:1 TM001 line too long (135 > 80)
```

Pattern: `file:line:col rule message`

### Exit codes

| Code | Meaning |
|------|---------|
| 0 | No lint issues found |
| 1 | Lint issues found |
| 2 | Runtime or configuration error |

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

## Rules

See [`rules/`](rules/) for the full list of rules with documentation.

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
