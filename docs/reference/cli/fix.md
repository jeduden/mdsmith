---
command: fix
summary: Auto-fix lint issues in Markdown files in place.
---
# `mdsmith fix`

Multi-pass fixing resolves cascading changes in one
invocation, so a single run leaves the worktree clean.

```text
mdsmith fix [flags] [files...]
```

Files can be paths, directories (walked recursively for
`*.md` and `*.markdown`), or glob patterns. Stdin is
rejected — files must be writable. With no file arguments,
files are discovered from `.mdsmith.yml` `files:` patterns.

## Flags

| Flag                | Default | Description                            |
| ------------------- | ------- | -------------------------------------- |
| `-c`, `--config`    | auto    | Override config path (auto-discovers)  |
| `-f`, `--format`    | `text`  | `text` or `json`                       |
| `--max-input-size`  | `2MB`   | Max file size (e.g. `2MB`, `0`=none)   |
| `--no-color`        | false   | Plain output                           |
| `--follow-symlinks` | config  | Follow symlinks; tri-state — see below |
| `--no-gitignore`    | false   | Skip gitignore filtering               |
| `-q`, `--quiet`     | false   | Suppress non-error output              |
| `-v`, `--verbose`   | false   | Show config, files, and rules          |
| `--explain`         | false   | Attach per-leaf rule provenance        |
| `--dry-run`         | false   | Preview changes; write nothing         |

`--follow-symlinks` semantics match
[`mdsmith check`](check.md#flags).

## Examples

```bash
mdsmith fix README.md            # fix a single file
mdsmith fix docs/                # fix a tree
mdsmith fix --explain plan/      # show provenance for unfixed leftovers
mdsmith fix --dry-run docs/      # preview without writing
```

## `--dry-run`

`mdsmith fix --dry-run` runs the full fix pipeline but
writes nothing back to disk. Use it to preview which files
would change and which rules would fire, then gate a CI
step on the resulting exit code.

```text
$ mdsmith fix --dry-run docs/
docs/api.md: would fix 3 violations (MDS001 ×2, MDS006)
stats: checked=12 fixed=0 failures=3 unfixed=0 would-fix=3
```

The summary keeps the `checked=` / `fixed=` /
`failures=` / `unfixed=` fields machine-parsable.
`fixed=` is always `0` on a dry run (nothing was
written); the additive `would-fix=N` field counts the
violations a real run would have auto-fixed. The exit
code matches what `mdsmith fix` would have returned on
the same input:

- `0` — every diagnostic is fixable; a real run would
  leave the worktree clean.
- non-zero — at least one unfixable diagnostic remains.

`--format json` emits one record per file whose bytes
or diagnostic counts would change:

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

The `diagnostics` array carries the same per-diagnostic
fields `check --format json` returns. Like every other
lint output, the JSON goes to **stderr**. The text-mode
`stats:` summary is suppressed in JSON mode; the
machine-readable counts live inside each record's
`would_fix` field.

Some rules fix by writing a sibling file rather than
the markdown itself. MDS048 `git-hook-sync` is the
example today: it regenerates `.gitattributes`. On
`--dry-run` these rules return early to honor the
no-disk-writes contract and instead declare which
diagnostics a real run would have cleared (via the
`DryRunPredictor` rule interface), so the dry-run
exit code still matches a real run.

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
| ---- | ------------------------------ |
| 0    | No remaining issues            |
| 1    | Issues remain after fixing     |
| 2    | Runtime or configuration error |

## See also

- [`mdsmith check`](check.md) — read-only sibling
- [`mdsmith merge-driver`](merge-driver.md) — Git merge
  driver that uses `fix` to resolve generated-section
  conflicts
