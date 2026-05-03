---
summary: CLI commands, flags, exit codes, and output format.
---
# CLI Reference

```text
mdsmith <command> [flags] [files...]
```

## Commands

<?catalog
glob:
  - "cli/*.md"
sort: command
header: |
  | Command | Description |
  |---------|-------------|
row: "| [`{command}`]({filename}) | {summary} |"
?>
| Command                                       | Description                                                                          |
|-----------------------------------------------|--------------------------------------------------------------------------------------|
| [`check`](cli/check.md)                       | Lint Markdown files for style issues.                                                |
| [`fix`](cli/fix.md)                           | Auto-fix lint issues in Markdown files in place.                                     |
| [`help`](cli/help.md)                         | Show built-in documentation for rules, metrics, and concept pages.                   |
| [`init`](cli/init.md)                         | Generate a default `.mdsmith.yml` config in the current directory.                   |
| [`kinds`](cli/kinds.md)                       | Inspect declared file kinds and resolve effective rule config per file.              |
| [`merge-driver`](cli/merge-driver.md)         | Git merge driver that resolves conflicts inside generated sections.                  |
| [`metrics`](cli/metrics.md)                   | List and rank shared Markdown metrics (file length, token estimate, readability, …). |
| [`pre-merge-commit`](cli/pre-merge-commit.md) | Install / manage a pre-merge-commit hook that runs `mdsmith fix` after a merge.      |
| [`query`](cli/query.md)                       | Select Markdown files by a CUE expression on front matter.                           |
| [`version`](cli/version.md)                   | Print the mdsmith build version and exit.                                            |
<?/catalog?>

The `check`, `fix`, and `query` commands accept file
paths, directories, and glob patterns as positional
arguments. `check` and `query` also accept `-` to read
from stdin.

With no file arguments:

- `check` and `fix` discover files using `files:` from
  `.mdsmith.yml` (default: `["**/*.md", "**/*.markdown"]`).
- `query` and `metrics rank` default to the current
  directory (`.`) and walk it recursively.

## Global flags

| Flag     | Short | Description |
|----------|-------|-------------|
| `--help` | `-h`  | Show help   |

Use `--` to separate flags from filenames starting with `-`.

## `--max-input-size`

Sets the byte-size cap for any input file read by commands
that support the flag (`check`, `fix`, `query`,
`metrics rank`). Behavior on oversize input:

- `check` / `fix`: file is skipped and reported as a
  runtime error on stderr. Exit code follows the usual
  precedence — `1` when lint diagnostics are found (even
  if some files were skipped), `2` when only runtime
  errors occur.
- `query`: file is skipped; the per-file error prints on
  stderr only when `--verbose` is set. Exit code `1` if no
  files matched, `0` on a match.
- `metrics rank`: the whole run fails with exit code `2`
  on the first oversize file.

An invalid `--max-input-size` value always exits `2`.
Accepts `KB`, `MB`, `GB` suffixes (binary: 1 MB =
1,048,576 bytes), bare integers (bytes), or `0` to disable
the limit. Default: `2MB`. The CLI flag overrides the
`max-input-size:` key in `.mdsmith.yml`.

## Configuration merge semantics

Rule settings come from a chain of layers. The chain
starts with the built-in defaults. Then every `kinds:`
block whose name matches the file applies. Front-matter
`kinds:` come first; `kind-assignment:` entries follow in
config order. Last, every `overrides:` block whose
`files:` glob matches the file applies.

Each layer **deep-merges** onto the accumulator:

- **Maps** are merged key by key; nested maps recurse on
  shared keys.
- **Scalars** at a leaf are replaced by the later layer.
- **Lists** are replaced wholesale by default. Selected
  list settings opt into **append** so a later kind or
  override extends the inherited list rather than
  replacing it. The placeholder vocabulary
  (`placeholders:` on `first-line-heading`,
  `required-structure`, `paragraph-structure`,
  `paragraph-readability`, `heading-increment`,
  `no-emphasis-as-heading`, and
  `cross-file-reference-integrity`) appends.
- A **bool-only** layer (e.g. `line-length: false`) toggles
  `enabled` for that rule but preserves the inherited
  settings. A later layer that re-enables the rule sees
  the original settings still in place.

A layer that fully restates a rule's body still wins on
every key, so the previous block-replacement behavior is a
special case of deep-merge. Use [`mdsmith kinds why`](cli/kinds.md)
to see the full chain on a single rule.

## Exit codes

| Code | Meaning                        |
|------|--------------------------------|
| 0    | No lint issues found           |
| 1    | Lint issues found              |
| 2    | Runtime or configuration error |

Per-command exits may vary; see the per-command pages.

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

Each diagnostic prints a header line
(`file:line:col rule message`). When source context is
available, up to 5 surrounding lines appear with a dot
path (`····^`) pointing to the exact column.

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

The `source_lines` and `source_start_line` fields are
omitted when source context is unavailable (e.g., empty
diagnostics). With `--explain`, each diag also gains an
`explanation` field — see [`mdsmith check`](cli/check.md).

## See also

- [Configuration globs](globs.md) — pattern syntax across
  config, directives, and CLI args
- [Conventions](conventions.md) — built-in rule presets
