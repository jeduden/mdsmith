# CLAUDE.md

## Project

tidymark — a Markdown linter written in Go.

## Build & Test Commands

- `go build ./...` — build all packages
- `go test ./...` — run all tests
- `go test -run TestName ./pkg/...` — run a specific test
- `go tool golangci-lint run` — run linter
- `go vet ./...` — run go vet

## Project Layout

Follow the [standard Go project layout](https://go.dev/doc/modules/layout):

- `cmd/tidymark/` — main application entry point
- `internal/` — private packages not importable by other modules
- `rules/` — rule documentation (`rules/<id>-<name>/README.md`)
- `testdata/` — test fixtures (markdown files for testing rules)

## Development Workflow

- New features are test-driven: write a failing test (red),
  make it pass (green), commit
- Keep commits small and focused on one change

## Code Style

- Follow standard Go conventions (gofmt, goimports)
- Use golangci-lint for linting
- Keep functions small and focused
- Error messages should be lowercase, no trailing punctuation
- Prefer returning errors over panicking

## CLI Design

### Usage

```text
tidymark [flags] [files...]
```

Files are positional arguments. Accepts multiple file paths,
directories, and glob patterns.
No file args and no stdin exits 0
(graceful empty invocation for pre-commit hooks).

### Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--config <file>` | `-c` | auto-discover | Override config file path |
| `--fix` | | false | Auto-fix issues in place |
| `--format <fmt>` | `-f` | `text` | Output format: `text`, `json` |
| `--no-color` | | false | Disable ANSI colors |
| `--no-gitignore` | | false | Disable `.gitignore` filtering when walking directories |
| `--quiet` | `-q` | false | Suppress non-error output |
| `--version` | `-v` | | Print version and exit |
| `--help` | `-h` | | Show help |

Use `--` to separate flags from filenames starting with `-`.

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | No lint issues found |
| 1 | Lint issues found |
| 2 | Runtime or configuration error |

### Output

Lint output goes to **stderr**. Format:

**text** (default):

```text
README.md:10:5 TM001 line too long (120 > 80)
docs/guide.md:3:1 TM002 first line should be a heading
```

Pattern: `file:line:col rule message`

**json**:

```json
[
  {
    "file": "README.md",
    "line": 10,
    "column": 5,
    "rule": "TM001",
    "name": "line-length",
    "severity": "error",
    "message": "line too long (120 > 80)"
  }
]
```

### Pre-commit (lefthook)

```yaml
# lefthook.yml
pre-commit:
  commands:
    tidymark:
      glob: "*.{md,markdown}"
      run: tidymark {staged_files}
      # To auto-fix and re-stage:
      # run: tidymark --fix {staged_files}
      # stage_fixed: true
```

## Plans

Task plans live in [`plan/`](plan/). Use [`plan/proto.md`](plan/proto.md)
as a template when creating new plans.

Each plan has acceptance criteria with behavioral tests. Work test-driven: write
a failing test (red), make it pass (green), commit.

Plan files must pass `tidymark check plan/` with zero diagnostics.

## Config & Rules

See [README.md](README.md#configuration) for config file format and examples.
Each rule is documented in [`rules/<id>-<name>/README.md`](rules/).
