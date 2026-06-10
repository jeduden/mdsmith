---
command: deps
summary: List a file's dependency-graph edges (includes, links, catalogs, builds).
---
# `mdsmith deps`

Print the dependency edges of one Markdown file: the
includes, catalogs, build inputs, and links it points
at. With `--incoming`, print every workspace file that
points at it instead. This is the CLI surface for the
same workspace graph the LSP call-hierarchy walks.

```text
mdsmith deps [flags] <file>
```

`<file>` is workspace-relative. Absolute paths and
parent-traversal entries (`../foo.md`) are rejected
with exit code 2.

## Flags

| Flag                | Default | Description                                |
| ------------------- | ------- | ------------------------------------------ |
| `-c`, `--config`    | auto    | Override config path                       |
| `-f`, `--format`    | `text`  | Output format: `text` or `json`            |
| `--incoming`        | false   | List files that depend on `<file>` instead |
| `--no-gitignore`    | false   | Disable `.gitignore` filtering during walk |
| `--follow-symlinks` | config  | Follow symlinks; tri-state — see below     |
| `--max-input-size`  | `2MB`   | Max file size (e.g. `2MB`, `0`=none)       |

`--follow-symlinks` and file discovery (the `files:` and
`ignore:` patterns in `.mdsmith.yml`) match
[`mdsmith check`](check.md#flags).

## Output

**text** (default), one row per edge:

```text
docs/index.md:14: file-link docs/api.md#authentication
docs/index.md:20: include docs/frag.md
```

Each row carries the source path, the 1-based source
line, the edge kind, and the target. Outgoing rows are
sorted by line; `--incoming` rows are sorted by source
path then line.

Edge kinds are `anchor-link`, `file-link`, `ref-link`,
`include`, `catalog`, and `build`. An unresolved
`<?catalog?>` glob renders its target as `(glob)`.

**json**:

```json
[
  {
    "source": "docs/index.md",
    "line": 14,
    "kind": "file-link",
    "target": "docs/api.md#authentication"
  }
]
```

Keys are stable. Empty results emit `[]`, not `null`.

## Examples

What does this file pull in?

```bash
mdsmith deps docs/index.md
```

What depends on this file (impact analysis before a
move or delete)?

```bash
mdsmith deps docs/api.md --incoming
```

JSON for a CI dependency check:

```bash
mdsmith deps --format json docs/api.md --incoming
```

## Exit codes

| Code | Meaning             |
| ---- | ------------------- |
| 0    | At least one edge   |
| 1    | No edges, no errors |
| 2    | Runtime/parse error |

## See also

- [`mdsmith list backlinks`](backlinks.md) — the
  reverse-link query scoped to direct Markdown links.
- [`mdsmith lsp`](lsp.md) — the editor surface for the
  same graph (call hierarchy, references).
