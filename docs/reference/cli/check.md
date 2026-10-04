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
`*.md` and `*.markdown`), or glob patterns. Pass `-`
alone to read from stdin: `-` next to file arguments is
a usage error (exit `2`). With no file arguments, files
are discovered from `.mdsmith.yml` `files:` patterns
(default: `**/*.md`, `**/*.markdown`).

The config comes from `.mdsmith.yml` or a `pyproject.toml`
`[tool.mdsmith]` table, found by
[config discovery](../config-discovery.md). A bad config
value prints as a `file:line:col config` diagnostic and
exits `2`.

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
| `--color`           | unset   | `auto`, `always`, or `never`           |
| `--no-color`        | false   | Same as `--color=never`                |
| `--follow-symlinks` | config  | Follow symlinks; tri-state — see below |
| `--no-gitignore`    | false   | Skip gitignore filtering               |
| `-q`, `--quiet`     | false   | Quiet the terminal; see below          |
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

Runtime errors always go to stderr, so an `-o` report
never contains them. On the default route they share
stderr with the report and print before it, so capture
a `json` or `sarif` report with `-o`, not `2>`. After a
runtime error during linting, such as a file over the
size limit, the report still covers the files that were
linted. The verbose log (`-v`) and the non-Markdown skip
warning stay on stderr too. An empty `-o` value is a
usage error (exit `2`).

`-o <path>` creates the file with mode `0644` (before the
umask), or truncates it if it exists. An existing file
keeps its mode. The file is opened only once linting
ends. A run that stops first, as on a bad config, exits
`2` and leaves the file untouched. Before any file is
linted, an `-o` path that is a directory, or whose
directory is missing, is a usage error (exit `2`).

`-o` never overwrites an input. A path that is one of
the run's inputs is a usage error (exit `2`), reported
before any file is linted. So is a missing path that the
run would pick up once the report created it:

- a Markdown path inside a directory argument, as in
  `check -o docs/report.md docs/`;
- a Markdown path that matches a glob argument;
- on a run with no file arguments, a path that matches
  a `files:` pattern, whatever its extension.

An existing path is compared with each input by file
identity, so another spelling, a symlink, or a hard link
still counts. A dangling symlink is judged by the file
the report would create at its final target. Names are
compared without regard to case. Ignore rules are not
consulted, so give a report inside the linted tree a
non-Markdown name such as `report.txt`.

On stdin, `-o` is compared with the file stdin reads
from, so `check - -o a.md < a.md` is refused too. A pipe
has no file identity, so a piped
`cat a.md | mdsmith check - -o a.md` cannot be detected,
and the report overwrites `a.md`.

A clean run still writes a valid document on every route:
`[]` for `json`, and a SARIF log with one run and no
results for `sarif`. This holds even when no Markdown
file is found. Text writes only the stats line, and
nothing when no file is found.

`-q` silences the terminal. It drops the report on
stderr and on `-o -`, and the verbose log and the skip
warning. An explicit `-o <path>` file still gets the
full report, so `-q -o report.json` runs quietly and
keeps the result. Runtime errors still print.

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

By default, text output uses ANSI color only when the
report's destination is a terminal. A file, a pipe, or a
redirected stream gets plain text. CI logs, where stderr
is usually a pipe, are plain as a result. The first of
these rules that applies decides:

1. `--color=always` turns color on and `--color=never`
   turns it off, wherever the report goes. `--no-color`
   is the same as `--color=never`. When several color
   flags are given, the last one wins. `--no-color=false`
   sets nothing, so an earlier color flag still holds.
2. `--color=auto` colors a terminal and nothing else,
   whatever the environment says.
3. With no color flag, a `NO_COLOR` variable that is set
   and not empty turns color off
   ([no-color.org](https://no-color.org)).
4. Next, a `FORCE_COLOR` variable that is set and is
   neither empty nor `0` turns color on
   ([force-color.org](https://force-color.org)), except
   in an `-o <path>` file, which only `--color=always`
   colors.
5. Otherwise color is on only on a terminal.

So a flag beats both variables, and `NO_COLOR` beats
`FORCE_COLOR`. `FORCE_COLOR=0` forces nothing: color
still follows the terminal. Only `--color=always` puts
escape codes in an `-o` file.

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
