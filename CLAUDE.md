# CLAUDE.md

## Project

mdsmith — a Markdown linter written in Go.

## Build & Test Commands

Requires Go 1.24+.

- `go build ./...` — build all packages
- `go test ./...` — run all tests
- `go test -run TestName ./pkg/...` — run a specific test
- `go tool golangci-lint run` — run linter
- `go vet ./...` — run go vet

## Project Layout

Follow the [standard Go project layout](https://go.dev/doc/modules/layout):

- `cmd/mdsmith/` — main application entry point
- `internal/` — private packages not importable by other modules
- `rules/` — rule documentation (`rules/<id>-<name>/README.md`)
- `testdata/` — test fixtures (markdown files for testing rules)

## Development Workflow

- New features are test-driven: write a failing test (red),
  make it pass (green), commit
- Keep commits small and focused on one change
- Run `mdsmith check .` before committing to ensure all
  markdown files pass linting

## Code Style

- Follow standard Go conventions (gofmt, goimports)
- Use golangci-lint for linting
- Keep functions small and focused
- Error messages should be lowercase, no trailing punctuation
- Prefer returning errors over panicking

## CLI Design

### Usage

```text
mdsmith <command> [flags] [files...]
```

### Commands

| Command | Description                  |
|---------|------------------------------|
| `check`   | Lint files (default command) |
| `fix`     | Auto-fix issues in place     |
| `help`    | Show help for rules/topics   |
| `init`    | Generate `.mdsmith.yml`        |
| `version` | Print version, exit          |

Files are positional arguments. Accepts multiple file paths,
directories, and glob patterns.
No file args and no stdin exits 0
(graceful empty invocation for pre-commit hooks).

### Subcommand Flags (check, fix)

| Flag           | Description    |
|----------------|----------------|
| `-c`, `--config`   | Config path    |
| `-f`, `--format`   | `text` or `json`   |
| `--no-color`     | Plain output   |
| `--no-gitignore` | Skip gitignore |
| `-q`, `--quiet`    | Quiet mode     |
| `-v`, `--verbose`  | Verbose output |

### Global Flags

| Flag   | Short | Description |
|--------|-------|-------------|
| `--help` | `-h`    | Show help   |

Use `--` to separate flags from filenames starting with `-`.

### Exit Codes

| Code | Meaning                        |
|------|--------------------------------|
| 0    | No lint issues found           |
| 1    | Lint issues found              |
| 2    | Runtime or configuration error |

### Output

Lint output goes to **stderr**. Format:

**text** (default):

```text
README.md:10:5 MDS001 line too long (120 > 80)
docs/guide.md:3:1 MDS002 first line should be a heading
```

Pattern: `file:line:col rule message`

**json**:

```json
[
  {
    "file": "README.md",
    "line": 10,
    "column": 5,
    "rule": "MDS001",
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
    mdsmith:
      glob: "*.{md,markdown}"
      run: mdsmith check {staged_files}
      # To auto-fix and re-stage:
      # run: mdsmith fix {staged_files}
      # stage_fixed: true
```

## Plans

Task plans live in [`plan/`](plan/). See [`PLAN.md`](PLAN.md)
for the current status of all plans. Use [`plan/proto.md`](plan/proto.md)
as a template when creating new plans.

Each plan has acceptance criteria with behavioral tests. Work test-driven: write
a failing test (red), make it pass (green), commit.

Plan files must pass `mdsmith check plan/` with zero diagnostics.

## Test Fixtures

Rule test fixtures live in `rules/<id>-<name>/`. Each rule
has `good/` and `bad/` examples (or `good.md` / `bad.md`).

Good fixtures must pass **all** rules, not just their own.
When a good fixture uses non-default settings (e.g. setext
headings, tilde fences), add a matching override in
`.mdsmith.yml` so that `mdsmith check .` also passes.

Bad fixtures are excluded via the `ignore:` section in
`.mdsmith.yml`.

## Config & Rules

See [README.md](README.md#configuration) for config file format and examples.
Each rule is documented in [`rules/<id>-<name>/README.md`](rules/).
Use [`rules/proto.md`](rules/proto.md) as template and content
guide when writing rule READMEs (instructions are in HTML comments).

When writing descriptions, state the concrete constraint: what
specific data must satisfy what condition. Name the inputs
(front matter fields, glob pattern, heading level) not just the
mechanism. Avoid vague verbs (match, sync, reflect) without
saying what is checked against what.
