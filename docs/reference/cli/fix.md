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

| Flag                | Default | Description                              |
|---------------------|---------|------------------------------------------|
| `-c`, `--config`    | auto    | Override config path (auto-discovers)    |
| `-f`, `--format`    | `text`  | `text` or `json`                         |
| `--max-input-size`  | `2MB`   | Max file size (e.g. `2MB`, `0`=none)     |
| `--no-color`        | false   | Plain output                             |
| `--follow-symlinks` | config  | Follow symlinks; tri-state — see below   |
| `--no-gitignore`    | false   | Skip gitignore filtering                 |
| `-q`, `--quiet`     | false   | Suppress non-error output                |
| `-v`, `--verbose`   | false   | Show config, files, and rules            |
| `--explain`         | false   | Attach per-leaf rule provenance          |
| `--dry-run`         | false   | Preview fixes without writing; see below |

`--follow-symlinks` semantics match
[`mdsmith check`](check.md#flags).

## Dry-run mode

`--dry-run` reports which files would change and which rules would fire,
without writing any bytes to disk. The exit code matches what `fix` would
return on the same input, so a CI step can gate on dry-run output.

```bash
mdsmith fix --dry-run docs/
```

Text output — one line per file with at least one fixable violation:

```text
docs/api.md: would fix 3 violations (MDS001 ×2, MDS006)
docs/index.md: would fix 1 violations (MDS004)
stats: checked=12 fixed=0 failures=0 unfixed=4 would-fix=8
```

`--format json` emits one record per file:

```json
[
  {"path":"docs/api.md","would_fix":3,"rules":["MDS001","MDS006"],"diagnostics":[]},
  {"path":"docs/index.md","would_fix":1,"rules":["MDS004"],"diagnostics":[]}
]
```

## Examples

```bash
mdsmith fix README.md                  # fix a single file
mdsmith fix docs/                      # fix a tree
mdsmith fix --dry-run docs/            # preview without writing
mdsmith fix --dry-run --format json .  # machine-readable preview
mdsmith fix --explain plan/            # show provenance for unfixed leftovers
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
