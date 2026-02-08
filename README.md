# tidymark

A Markdown linter written in Go.

## Installation

```bash
go install github.com/jeduden/tidymark@latest
```

## Usage

```bash
tidymark <file.md>
```

## Configuration

Create a `.tidymark.yml` in your project root. Without one, all rules are enabled with defaults.

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

Rules can be `true` (enable with defaults), `false` (disable), or an object with settings.
The `overrides` list applies different rules per file pattern. Later overrides take precedence.

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
