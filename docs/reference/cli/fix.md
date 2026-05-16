---
command: fix
summary: Auto-fix lint issues in Markdown files in place.
---
# `mdsmith fix`

Auto-fix lint issues in Markdown files in place. Multi-pass
fixing resolves cascading changes in one invocation.

```text
mdsmith fix [flags] [files...]
```

Files can be paths, directories (walked recursively for
`*.md` and `*.markdown`), or glob patterns. Stdin is
rejected — files must be writable. With no file arguments,
files are discovered from `.mdsmith.yml` `files:` patterns.

## Flags

| Flag                | Default | Description                                    |
|---------------------|---------|------------------------------------------------|
| `-c`, `--config`    | auto    | Override config path (auto-discovers)          |
| `-f`, `--format`    | `text`  | `text` or `json`                               |
| `--dry-run`         | false   | Preview fixes without writing; see [Dry run][] |
| `--max-input-size`  | `2MB`   | Max file size (e.g. `2MB`, `0`=none)           |
| `--no-color`        | false   | Plain output                                   |
| `--follow-symlinks` | config  | Follow symlinks; tri-state — see below         |
| `--no-gitignore`    | false   | Skip gitignore filtering                       |
| `-q`, `--quiet`     | false   | Suppress non-error output                      |
| `-v`, `--verbose`   | false   | Show config, files, and rules                  |
| `--explain`         | false   | Attach per-leaf rule provenance                |

[Dry run]: #dry-run

`--follow-symlinks` semantics match
[`mdsmith check`](check.md#flags).

## Examples

```bash
mdsmith fix README.md            # fix a single file
mdsmith fix docs/                # fix a tree
mdsmith fix --explain plan/      # show provenance for unfixed leftovers
mdsmith fix --dry-run docs/      # preview without writing
```

## Dry run

`--dry-run` runs the full fix pipeline but writes nothing to disk. Every
candidate file is left byte-identical to its on-disk state.

Output — one line per file with at least one fixable violation:

```text
docs/api.md: would fix 3 violations (MDS001, MDS006)
docs/index.md: would fix 1 violations (MDS042)
stats: checked=12 fixed=0 failures=0 unfixed=4 would-fix=4
```

`fixed=0` is always literal-zero on a dry run. `would-fix=N` carries the
count of violations a real run would auto-fix. A CI step can gate on the
summary line without accidentally applying changes.

With `--format json`, each file is emitted as one record:

```json
[
  {
    "path": "docs/api.md",
    "would_fix": 3,
    "rules": ["MDS001", "MDS006"],
    "diagnostics": []
  }
]
```

`diagnostics` lists any remaining (unfixable) violations for that file.

The exit code matches what a real `fix` run would return on the same
input: `0` when every violation is fixable, `1` when unfixable
diagnostics remain.

## Pre-commit

```yaml
# lefthook.yml
pre-commit:
  commands:
    mdsmith:
      glob: "*.{md,markdown}"
      run: mdsmith fix {staged_files}
      stage_fixed: true
```

## Exit codes

| Code | Meaning                        |
|------|--------------------------------|
| 0    | No remaining issues            |
| 1    | Issues remain after fixing     |
| 2    | Runtime or configuration error |

## See also

- [`mdsmith check`](check.md) — read-only sibling
- [`mdsmith merge-driver`](merge-driver.md) — Git merge
  driver that uses `fix` to resolve generated-section
  conflicts
