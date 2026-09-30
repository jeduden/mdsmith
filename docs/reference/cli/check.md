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
reaches it or you name it explicitly. Naming one
explicitly prints a `skipping …: not a Markdown file`
warning on stderr, so the skip is never a silent no-op;
`--quiet` suppresses it. The `json`/`sarif` formats also
suppress it while their report goes to stderr, since a
prose line would corrupt the document there; with `-o`
the warning shows.

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
| `-o`, `--output`    | stderr  | Report to a file; `-` is stdout        |

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

## Report output

The report is the diagnostics plus the text-format stats
line. It goes to stderr by default. `-o <path>` writes it
to a file and `-o -` writes it to stdout, where a plain
redirect captures it:

```bash
mdsmith check -f json -o diagnostics.json docs/
mdsmith check -f json -o - docs/ > diagnostics.json
```

Runtime errors always stay on stderr, so they never land
in the report. The verbose log (`-v`) and the
non-Markdown skip warning stay on stderr too. An empty
`-o` value is a usage error (exit `2`).

`-o <path>` creates the file with mode `0644` (before the
umask), or truncates it if it exists. An existing file
keeps its mode. The file is opened only once linting
ends. A run that stops first, as on a bad config, exits
`2` and leaves the file untouched.

A clean run still writes a valid document on every route:
`[]` for `json`, and a SARIF log with one run and no
results for `sarif`. This holds even when no Markdown
file is found. Text writes only the stats line, and
nothing when no file is found. `-q` writes no report at
all; with `-o <path>` the file is left empty.

A report that cannot be written is a runtime error. The
file may fail to open, or a write or close may fail.
`mdsmith: error writing output: …` goes to stderr and the
exit code is `2`. A write that fails partway leaves an
incomplete report. When the report goes elsewhere, a
failed write of an error message to stderr is ignored,
and the report still arrives. On Unix, a write to a
closed pipe, as in `| head`, ends the process on
`SIGPIPE` instead.

### Color

Text output uses ANSI color only when the report's
destination is a terminal. A file, a pipe, or a
redirected stream gets plain text. CI logs, where stderr
is usually a pipe, are plain as a result. `--no-color`,
or a `NO_COLOR` environment variable that is set and not
empty, turns color off on a terminal as well.

## Examples

```bash
mdsmith check docs/                  # lint a directory
mdsmith check -f json docs/          # JSON output
mdsmith check -f sarif docs/         # SARIF 2.1.0 output
mdsmith check -o report.txt docs/    # report to a file
mdsmith check --explain README.md    # provenance trailer
echo "# Hi" | mdsmith check -        # lint stdin
```

## GitHub Code Scanning

`-f sarif` emits a SARIF 2.1.0 document that GitHub's
Code Scanning dashboard ingests directly. Upload with
[`github/codeql-action/upload-sarif`](https://github.com/github/codeql-action):

```yaml
- name: Run mdsmith
  run: mdsmith check -f sarif -o report.sarif . || true
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

| Code | Meaning                                                    |
| ---- | ---------------------------------------------------------- |
| 0    | No lint issues found                                       |
| 1    | Lint issues found                                          |
| 2    | Runtime or configuration error, or the report write failed |

## See also

- [`mdsmith fix`](fix.md) — auto-fix the issues `check` reports
- [Output and JSON schema](../cli.md#output)
