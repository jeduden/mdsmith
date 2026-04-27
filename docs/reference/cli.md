---
summary: CLI commands, flags, exit codes, and output format.
---
# CLI Design

## Usage

```text
mdsmith <command> [flags] [files...]
```

## Commands

| Command        | Description                                    |
|----------------|------------------------------------------------|
| `check`        | Lint files                                     |
| `fix`          | Auto-fix issues in place                       |
| `query`        | Select files by CUE expression on front matter |
| `help`         | Show help for rules/topics                     |
| `metrics`      | List and rank shared metrics                   |
| `merge-driver` | Git merge driver for regenerable sections      |
| `archetypes`   | Scaffold, list, show, and locate archetypes    |
| `kinds`        | Inspect declared kinds and resolve config      |
| `init`         | Generate `.mdsmith.yml`                        |
| `version`      | Print version, exit                            |

The `check` and `fix` commands accept file paths,
directories, and glob patterns as positional arguments.
Pass `-` to read from stdin.

When no file arguments are given, `check` and `fix`
discover files using the `files` glob patterns from config
(default: `["**/*.md", "**/*.markdown"]`). If no files
match, exits 0.

## Subcommand Flags (check, fix)

| Flag                | Default | Description                             |
|---------------------|---------|-----------------------------------------|
| `-c`, `--config`    | auto    | Config path (auto-discovers by default) |
| `-f`, `--format`    | `text`  | `text` or `json`                        |
| `--max-input-size`  | `2MB`   | Max file size (e.g. `2MB`, `0`=none)    |
| `--no-color`        | false   | Plain output                            |
| `--follow-symlinks` | config  | Follow symlinks; tri-state — see below  |
| `--no-gitignore`    | false   | Skip gitignore                          |
| `-q`, `--quiet`     | false   | Quiet mode                              |
| `-v`, `--verbose`   | false   | Verbose output                          |

Symlinks are skipped by default. This blocks a malicious
symlink from redirecting `check` or `fix` to files outside
the project. The rule applies to directory walks, glob
expansion, and explicit file or directory arguments:
`mdsmith check ./linked.md` silently skips a symlink named
on the command line. Opt-in follows only symlinks that
resolve to regular files; directory / FIFO / device / socket
targets are always skipped.

`--follow-symlinks` is tri-state:

- omitted — fall back to `follow-symlinks:` in
  `.mdsmith.yml` (default: skip)
- `--follow-symlinks` or `--follow-symlinks=true` — opt in
  for this run
- `--follow-symlinks=false` — force deny for this run, even
  when the loaded config has `follow-symlinks: true`

The old `no-follow-symlinks:` config key still parses and
emits a deprecation warning on stderr.

## Other Subcommand Flags

`query` accepts `-c`/`--config`, `-v`/`--verbose`,
`-0`/`--null`, and `--max-input-size`.

`metrics list` accepts `-f`/`--format` (`text` or `json`)
and `--scope` (only `file` is supported; defaults to
`file`).

`metrics rank` accepts `-c`/`--config`, `-f`/`--format`,
`--no-gitignore`, `--follow-symlinks`,
`--max-input-size`, plus `--metrics`, `--by`, `--order`,
`--top`.

`metrics rank` counts only **authored bytes** for each
file: content between the `<?include?>` and `<?catalog?>`
markers is excluded. This matches the lint-once model —
embedded content is measured against the source file,
not the host that pulls it in.

## `kinds` Subcommands

| Subcommand          | Description                                        |
|---------------------|----------------------------------------------------|
| `list`              | Print declared kinds and their merged bodies       |
| `show <name>`       | Print one kind's merged body                       |
| `path <name>`       | Print resolved schema path of `required-structure` |
| `resolve <file>`    | Resolved kind list and per-leaf provenance summary |
| `why <file> <rule>` | Full per-rule merge chain, including no-op layers  |

Each subcommand accepts `--json` for stable structured
output. Unknown kinds and unresolved schemas exit `2`.

Provenance layer sources:

- `default` — built-ins plus top-level `rules:`.
- `kinds.<name>` — one per kind in the effective list.
- `overrides[<i>]` — one per matching glob override
  (zero-indexed).
- `front-matter override` — reserved for future
  per-file rule overrides.

## `--explain` on `check` / `fix`

`--explain` attaches per-leaf rule provenance to each
diagnostic. Text output prints a `└─` trailer naming
the rule and the winning source for each leaf setting;
JSON adds an `explanation` field (see schema below).

## JSON Schemas

Stable shapes for LSP / tool consumption. `leaves[]`
always lists every leaf of the final rule config:
`enabled` plus one entry per `settings.<key>`. Output
never elides leaves.

`check --explain` adds an `explanation` to each diag
(omitted without `--explain`):

```json
"explanation": {
  "rule": "line-length",
  "leaves": [
    {"path": "enabled", "value": true, "source": "default"},
    {"path": "settings.max", "value": 30, "source": "kinds.short"}
  ]
}
```

`kinds list` → `{"kinds": [<body>...]}`; `show` → one
body. Body shape: `{"name", "rules", "categories"}`.
`rules[<name>]` follows the YAML rule-cfg union:
`false`, `true`, or the settings map.

`kinds resolve <file>`:

```json
{
  "file": "plan/95.md",
  "kinds": [{"name": "plan", "source": "kind-assignment[0]"}],
  "categories": {"meta": true},
  "rules": {
    "max-file-length": {
      "final": {"max": 500},
      "leaves": [
        {"path": "enabled", "value": true, "source": "default"},
        {"path": "settings.max", "value": 500, "source": "kinds.plan"}
      ]
    }
  }
}
```

`kinds[].source` is `front-matter` or
`kind-assignment[<i>]`.

`kinds why <file> <rule>` (`leaves[].chain` records
every layer that set the leaf):

```json
{
  "file": "plan/9_big.md",
  "rule": "max-file-length",
  "final": {"max": 900},
  "layers": [
    {"source": "default", "set": true, "value": {"max": 300}},
    {"source": "kinds.plan", "set": true, "value": {"max": 500}},
    {"source": "overrides[0]", "set": true, "value": {"max": 900}}
  ],
  "leaves": [
    {"path": "settings.max", "value": 900,
     "source": "overrides[0]",
     "chain": [
       {"source": "default", "value": 300},
       {"source": "kinds.plan", "value": 500},
       {"source": "overrides[0]", "value": 900}
     ]}
  ]
}
```

No-op layers carry `"set": false` and omit `value`.

## `archetypes` Subcommands

| Subcommand    | Description                                  |
|---------------|----------------------------------------------|
| `init [dir]`  | Scaffold archetype dir with example + README |
| `list`        | Print discovered archetypes (name + path)    |
| `show <name>` | Print raw schema source to stdout            |
| `path <name>` | Print resolved filesystem path               |

Archetype roots come from the top-level
`archetypes.roots` key in `.mdsmith.yml`. When absent,
`./archetypes` is used. `init` never mutates the
config file; it prints the snippet to add manually.
`list` exits `1` when no archetypes are discovered;
`show` and `path` exit `2` on unknown names.

## `--max-input-size`

Sets the byte-size cap for any input file read by
commands that support the flag (`check`, `fix`, `query`,
`metrics rank`). Behavior on oversize input:

- `check` / `fix`: file is skipped and reported as a
  runtime error on stderr. Exit code follows the usual
  precedence — `1` when lint diagnostics are found (even
  if some files were skipped), `2` when only runtime
  errors occur.
- `query`: file is skipped; the per-file error is only
  printed on stderr when `--verbose` is set. Exit code
  `1` if no files matched, `0` on a match.
- `metrics rank`: the whole run fails with exit code
  `2` on the first oversize file.

An invalid `--max-input-size` value always exits `2`.
Accepts `KB`, `MB`, `GB` suffixes (binary:
1 MB = 1,048,576 bytes), bare integers (bytes), or `0`
to disable the limit. Default: `2MB`. The CLI flag
overrides the `max-input-size` key in `.mdsmith.yml`.

## Merge Semantics

Rule settings come from a chain of layers. The chain
starts with the built-in defaults. Then every `kinds:`
block whose name matches the file applies. Front-matter
`kinds:` come first; `kind-assignment:` entries follow
in config order. Last, every `overrides:` block whose
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
- A **bool-only** layer (e.g. `line-length: false`)
  toggles `enabled` for that rule but preserves the
  inherited settings. A later layer that re-enables the
  rule sees the original settings still in place.

A layer that fully restates a rule's body still wins on
every key, so the previous block-replacement behavior is
a special case of deep-merge.

## Global Flags

| Flag     | Short | Description |
|----------|-------|-------------|
| `--help` | `-h`  | Show help   |

Use `--` to separate flags from filenames starting with `-`.

## Exit Codes

| Code | Meaning                        |
|------|--------------------------------|
| 0    | No lint issues found           |
| 1    | Lint issues found              |
| 2    | Runtime or configuration error |

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

Each diagnostic prints a header line (`file:line:col rule message`).
When source context is available, up to 5 surrounding lines appear
with a dot path (`····^`) pointing to the exact column.

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

The `source_lines` and `source_start_line` fields are omitted when
source context is unavailable (e.g., empty diagnostics).

## Pre-commit (lefthook)

```yaml
# lefthook.yml
pre-commit:
  commands:
    mdsmith:
      glob: "*.{md,markdown}"
      run: mdsmith check {staged_files}
      # To auto-fix and re-stage:
      # run: mdsmith fix {staged_files}
      # stage_fixed: true
```
