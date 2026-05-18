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

| Flag                | Default | Description                            |
|---------------------|---------|----------------------------------------|
| `-c`, `--config`    | auto    | Override config path (auto-discovers)  |
| `-f`, `--format`    | `text`  | `text` or `json`                       |
| `--dry-run`         | false   | Preview fixes without writing to disk  |
| `--max-input-size`  | `2MB`   | Max file size (e.g. `2MB`, `0`=none)   |
| `--no-color`        | false   | Plain output                           |
| `--follow-symlinks` | config  | Follow symlinks; tri-state — see below |
| `--no-gitignore`    | false   | Skip gitignore filtering               |
| `-q`, `--quiet`     | false   | Suppress non-error output              |
| `-v`, `--verbose`   | false   | Show config, files, and rules          |
| `--explain`         | false   | Attach per-leaf rule provenance        |

`--follow-symlinks` semantics match
[`mdsmith check`](check.md#flags).

## Examples

```bash
mdsmith fix README.md            # fix a single file
mdsmith fix docs/                # fix a tree
mdsmith fix --explain plan/      # show provenance for unfixed leftovers
mdsmith fix --dry-run docs/      # preview what would change
```

## Dry-run mode

`--dry-run` builds the fixed content but skips writing. Each file
with at least one fixable violation is reported:

```text
docs/api.md: would fix 3 violations (MDS001 ×2, MDS006)
docs/index.md: would fix 1 violations (MDS006)
stats: checked=12 fixed=0 failures=0 unfixed=4 would-fix=8
```

- `fixed=0` is always literal zero — nothing was written.
- `would-fix=N` is the number of violations a real run would
  auto-fix.
- The exit code matches what a real run would return, so a CI step
  can gate on dry-run output without risk of modifying files.

With `--format json`, each file-with-fixes produces one record:

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
