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

| Flag                | Default | Description                             |
|---------------------|---------|-----------------------------------------|
| `-c`, `--config`    | auto    | Override config path (auto-discovers)   |
| `-f`, `--format`    | `text`  | `text` or `json`                        |
| `--max-input-size`  | `2MB`   | Max file size (e.g. `2MB`, `0`=none)    |
| `--no-color`        | false   | Plain output                            |
| `--follow-symlinks` | config  | Follow symlinks; tri-state — see below  |
| `--no-gitignore`    | false   | Skip gitignore filtering                |
| `-q`, `--quiet`     | false   | Suppress non-error output               |
| `-v`, `--verbose`   | false   | Show config, files, and rules           |
| `--explain`         | false   | Attach per-leaf rule provenance         |
| `--dry-run`         | false   | Preview fixes without writing any files |

`--follow-symlinks` semantics match
[`mdsmith check`](check.md#flags).

## Examples

```bash
mdsmith fix README.md            # fix a single file
mdsmith fix docs/                # fix a tree
mdsmith fix --explain plan/      # show provenance for unfixed leftovers
mdsmith fix --dry-run docs/      # preview what would change
```

## Dry run

`--dry-run` runs the full fix pipeline but writes nothing to disk.
For each file that would change, it prints one summary line.
The line names each rule that would fire and its violation count:

```text
docs/api.md: would fix 3 violations (MDS001, MDS006)
```

The trailing stats line includes a `would-fix=N` field.
`fixed=0` is always literal zero on a dry run — nothing was written:

```text
stats: checked=12 fixed=0 failures=0 unfixed=4 would-fix=8
```

With `--format json` each file record includes `would_fix` (integer)
and `rules` (array of rule IDs):

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

The exit code matches what a real `fix` run would return:
`0` when every issue is fixable, `1` when unfixable diagnostics remain.
This lets a CI step use `--dry-run` as a gate without applying changes.

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
