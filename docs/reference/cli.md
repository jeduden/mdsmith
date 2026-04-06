---
summary: CLI commands, flags, exit codes, and output format.
---
# CLI Design

## Usage

```text
mdsmith <command> [flags] [files...]
```

## Commands

| Command        | Description                                    |
|----------------|------------------------------------------------|
| `check`        | Lint files                                     |
| `fix`          | Auto-fix issues in place                       |
| `query`        | Select files by CUE expression on front matter |
| `help`         | Show help for rules/topics                     |
| `metrics`      | List and rank shared metrics                   |
| `merge-driver` | Git merge driver for regenerable sections      |
| `init`         | Generate `.mdsmith.yml`                        |
| `version`      | Print version, exit                            |

The `check` and `fix` commands accept file paths,
directories, and glob patterns as positional arguments.
Pass `-` to read from stdin.

When no file arguments are given, `check` and `fix`
discover files using the `files` glob patterns from config
(default: `["**/*.md", "**/*.markdown"]`). If no files
match, exits 0.

## Subcommand Flags (check, fix)

| Flag               | Description                          |
|--------------------|--------------------------------------|
| `-c`, `--config`   | Config path                          |
| `-f`, `--format`   | `text` or `json`                     |
| `--max-input-size` | Max file size (e.g. `2MB`, `0`=none) |
| `--no-color`       | Plain output                         |
| `--no-gitignore`   | Skip gitignore                       |
| `-q`, `--quiet`    | Quiet mode                           |
| `-v`, `--verbose`  | Verbose output                       |

`--max-input-size` sets the byte-size cap for any file
read during linting. Files exceeding the limit are skipped
with an error (exit code 2). Accepts `KB`, `MB`, `GB`
suffixes (binary: 1 MB = 1,048,576 bytes), bare integers
(bytes), or `0` to disable the limit. Default: `2MB`.
The CLI flag overrides the `max-input-size` key in
`.mdsmith.yml`.

## Global Flags

| Flag     | Short | Description |
|----------|-------|-------------|
| `--help` | `-h`  | Show help   |

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
README.md:10:81 MDS001 line too long (120 > 80)
 8 | Previous line of context.
 9 | Another context line.
10 | This very long line exceeds the configured 80 character limit and keeps going...
·····················································································^
11 | Next line of context.
12 | Another context line.
```

Each diagnostic prints a header line (`file:line:col rule message`).
When source context is available, up to 5 surrounding lines appear
with a dot path (`····^`) pointing to the exact column.

**json**:

```json
[
  {
    "file": "README.md",
    "line": 10,
    "column": 81,
    "rule": "MDS001",
    "name": "line-length",
    "severity": "error",
    "message": "line too long (120 > 80)",
    "source_lines": ["Previous line.", "Another context.", "The long line...", "Next.", "Another."],
    "source_start_line": 8
  }
]
```

The `source_lines` and `source_start_line` fields are omitted when
source context is unavailable (e.g., empty diagnostics).

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
