---
summary: CLI commands, flags, exit codes, and output format.
---
# CLI Design

## Usage

```text
mdsmith <command> [flags] [files...]
```

## Commands

| Command      | Description                               |
|--------------|-------------------------------------------|
| `check`        | Lint files                                |
| `fix`          | Auto-fix issues in place                  |
| `help`         | Show help for rules/topics                |
| `metrics`      | List and rank shared metrics              |
| `merge-driver` | Git merge driver for regenerable sections |
| `init`         | Generate `.mdsmith.yml`                     |
| `version`      | Print version, exit                       |

The `check` and `fix` commands accept file paths,
directories, and glob patterns as positional arguments.
Pass `-` to read from stdin.

When no file arguments are given, `check` and `fix`
discover files using the `files` glob patterns from config
(default: `["**/*.md", "**/*.markdown"]`). If no files
match, exits 0.

## Subcommand Flags (check, fix)

| Flag           | Description    |
|----------------|----------------|
| `-c`, `--config`   | Config path    |
| `-f`, `--format`   | `text` or `json`   |
| `--no-color`     | Plain output   |
| `--no-gitignore` | Skip gitignore |
| `-q`, `--quiet`    | Quiet mode     |
| `-v`, `--verbose`  | Verbose output |

## Global Flags

| Flag   | Short | Description |
|--------|-------|-------------|
| `--help` | `-h`    | Show help   |

Use `--` to separate flags from filenames starting with `-`.

## Exit Codes

| Code | Meaning                        |
|------|--------------------------------|
| 0    | No lint issues found           |
| 1    | Lint issues found              |
| 2    | Runtime or configuration error |

## Output

Lint output goes to **stderr**. Format:

**text** (default):

```text
README.md:10:5 MDS001 line too long (120 > 80)
docs/guide.md:3:1 MDS004 first line should be a level 1 heading
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

## Pre-commit (lefthook)

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
