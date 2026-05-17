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

### Dry-run preview

`--dry-run` computes the same fixes as a real run but writes nothing
to disk. Files with no fixes are silent. Files that would change each
get one line, and the trailing summary includes `would-fix=N`:

```text
$ mdsmith fix --dry-run docs/
docs/api.md: would fix 3 violations (MDS006, MDS017 ×2)
docs/index.md: would fix 1 violation (MDS017)
stats: checked=12 fixed=0 failures=0 unfixed=0 would-fix=2
```

With `--format json`, stdout is a JSON array — one record per file
that has fixes:

```json
[
  {
    "path": "docs/api.md",
    "would_fix": 3,
    "rules": ["MDS006", "MDS017"],
    "diagnostics": []
  }
]
```

`diagnostics` carries any unfixable violations that would remain
after the fix pass. Exit code is the same as a real run on the same
input: `0` when every violation is fixable, `1` when unfixable issues
remain.

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
