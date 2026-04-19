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

## Other Subcommand Flags

`query` accepts `-c`/`--config`, `-v`/`--verbose`,
`-0`/`--null`, and `--max-input-size`.

`metrics rank` accepts `-c`/`--config`, `-f`/`--format`,
`--no-gitignore`, `--no-follow-symlinks`,
`--max-input-size`, plus `--metrics`, `--by`, `--order`,
`--top`.

## `--max-input-size`

Sets the byte-size cap for any input file read by
commands that support the flag (`check`, `fix`, `query`,
`metrics rank`). Behavior on oversize input:

- `check` / `fix`: file is skipped and reported as a
  runtime error on stderr. Exit code follows the usual
  precedence — `1` when lint diagnostics are found (even
  if some files were skipped), `2` when only runtime
  errors occur.
- `query`: file is skipped; the per-file error is only
  printed on stderr when `--verbose` is set. Exit code
  `1` if no files matched, `0` on a match.
- `metrics rank`: the whole run fails with exit code
  `2` on the first oversize file.

An invalid `--max-input-size` value always exits `2`.
Accepts `KB`, `MB`, `GB` suffixes (binary:
1 MB = 1,048,576 bytes), bare integers (bytes), or `0`
to disable the limit. Default: `2MB`. The CLI flag
overrides the `max-input-size` key in `.mdsmith.yml`.

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
