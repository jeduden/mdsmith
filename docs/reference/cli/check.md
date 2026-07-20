---
command: check
summary: Lint Markdown files for style issues.
---
# `mdsmith check`

Walks the workspace, parses each file once, runs every
configured rule, and prints diagnostics — without writing
anything back. Also covers structure and cross-file
integrity, not only line-level style.

```text
mdsmith check [flags] [files...]
```

Files can be paths, directories (walked recursively for
`*.md` and `*.markdown`), or glob patterns. Pass `-` to
read from stdin. With no file arguments, files are
discovered from `.mdsmith.yml` `files:` patterns
(default: `**/*.md`, `**/*.markdown`).

Only Markdown files are linted. A non-Markdown path
(such as `.gitattributes`) is skipped whether the walk
reaches it or you name it explicitly.

## Flags

| Flag                | Default | Description                            |
| ------------------- | ------- | -------------------------------------- |
| `-c`, `--config`    | auto    | Override config path (auto-discovers)  |
| `-f`, `--format`    | `text`  | `text`, `json`, or `sarif`             |
| `--max-input-size`  | `2MB`   | Max file size (e.g. `2MB`, `0`=none)   |
| `--no-color`        | false   | Plain output                           |
| `--follow-symlinks` | config  | Follow symlinks; tri-state — see below |
| `--no-gitignore`    | false   | Skip gitignore filtering               |
| `-q`, `--quiet`     | false   | Suppress non-error output              |
| `-v`, `--verbose`   | false   | Show config, files, and rules          |
| `--explain`         | false   | Attach per-leaf rule provenance        |

`--follow-symlinks` is tri-state. Omitted defers to the
config key (default: skip). `--follow-symlinks` or
`=true` opts in for this run. `=false` forces skip even
when config opts in.

Symlinks resolving to directories, FIFOs, devices, or
sockets are always skipped.

`--explain` adds an `explanation` field to each JSON diag
(or a `└─` trailer in text output) naming the rule and
the winning source for each leaf setting:

```json
"explanation": {"rule": "line-length", "leaves": [
  {"path": "enabled", "value": true, "source": "default"},
  {"path": "settings.max", "value": 30, "source": "kinds.short"}
]}
```

## Examples

```bash
mdsmith check docs/                  # lint a directory
mdsmith check -f json docs/          # JSON output
mdsmith check -f sarif docs/         # SARIF 2.1.0 output
mdsmith check --explain README.md    # provenance trailer
echo "# Hi" | mdsmith check -        # lint stdin
```

## GitHub Code Scanning

`-f sarif` emits a SARIF 2.1.0 document that GitHub's
Code Scanning dashboard ingests directly. Upload with
[`github/codeql-action/upload-sarif`](https://github.com/github/codeql-action):

```yaml
- name: Run mdsmith
  run: mdsmith check -f sarif . 2> report.sarif || true
- name: Upload SARIF
  uses: github/codeql-action/upload-sarif@v3
  with:
    sarif_file: report.sarif
    category: mdsmith
```

Each fired rule links back to its mdsmith.dev doc page via
`helpUri` in the SARIF `driver.rules` array.

## Pre-commit

```yaml
# lefthook.yml
pre-commit:
  commands:
    mdsmith:
      glob: "*.{md,markdown}"
      run: mdsmith check {staged_files}
```

## Exit codes

| Code | Meaning                        |
| ---- | ------------------------------ |
| 0    | No lint issues found           |
| 1    | Lint issues found              |
| 2    | Runtime or configuration error |

## See also

- [`mdsmith fix`](fix.md) — auto-fix the issues `check` reports
- [Output and JSON schema](../cli.md#output)
