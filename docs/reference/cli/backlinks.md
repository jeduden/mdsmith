---
command: list backlinks
summary: List workspace links that point at a file.
---
# `mdsmith list backlinks`

Print every workspace file that links to `<target>`,
one row per incoming link. The link graph is the same
one MDS027 (cross-file-reference-integrity) walks for
broken-link checking, queried in reverse.

```text
mdsmith list backlinks [flags] <target>
```

`<target>` is workspace-relative. Append `#anchor` to
restrict matches to links whose anchor resolves to the
named heading. Absolute paths and parent-traversal
entries (`../foo.md`) are rejected with exit code 2.

## Flags

| Flag                | Default | Description                                         |
|---------------------|---------|-----------------------------------------------------|
| `-c`, `--config`    | auto    | Override config path                                |
| `-f`, `--format`    | `text`  | Output format: `text` or `json`                     |
| `--include GLOB`    | none    | Restrict sources to paths matching glob; repeatable |
| `--limit N`         | `0`     | Cap output at N rows (`0` = no cap)                 |
| `--no-gitignore`    | false   | Disable `.gitignore` filtering during walk          |
| `--follow-symlinks` | config  | Follow symlinks; tri-state — see below              |
| `--max-input-size`  | `2MB`   | Max file size (e.g. `2MB`, `0`=none)                |

`--follow-symlinks` semantics match
[`mdsmith check`](check.md#flags). Omit the flag to
defer to the `follow-symlinks:` config key (default
skip). `--follow-symlinks` (or `=true`) opts in for
this run. `=false` forces skip even when config opts
in.

File discovery follows the `files:` patterns in
`.mdsmith.yml` and the same `ignore:` rules `check` and
`fix` use.

## Output

**text** (default):

```text
docs/index.md:14: [API reference](api.md)
docs/getting-started.md:42: [api docs](./api.md)
plan/045_api-overhaul.md:8: [api](../docs/api.md)
```

Each row carries the source path, the 1-based source
line, the visible link text, and the link target as it
appears in the source. Output is sorted by source path
then line, so `--limit` paginates deterministically.

**json**:

```json
[
  {
    "source": "docs/index.md",
    "line": 14,
    "text": "API reference",
    "target": "api.md"
  },
  {
    "source": "plan/045_api-overhaul.md",
    "line": 8,
    "text": "api",
    "target": "../docs/api.md"
  },
  {
    "source": "vault/notes.md",
    "line": 3,
    "text": "API reference",
    "target": "api",
    "kind": "wikilink",
    "alias": "API reference"
  }
]
```

Four base keys are always present. They are `source`,
`line`, `text`, and `target`. Three more appear only
on Obsidian-style wikilink records:

| Key     | Type   | Set when                                            |
|---------|--------|-----------------------------------------------------|
| `kind`  | string | `"wikilink"`; standard links omit the key entirely. |
| `alias` | string | Source carried a `\|alias` half; otherwise omitted. |
| `embed` | bool   | `true` for `![[…]]` embeds; otherwise omitted.      |

Existing consumers that read only the four base keys
see the same shape as before. Empty results emit
`[]`, not `null`.

## Examples

List everything that points at `docs/api.md`:

```bash
mdsmith list backlinks docs/api.md
```

Filter to a specific anchor on the target:

```bash
mdsmith list backlinks docs/api.md#authentication
```

Limit to the plan directory and the first ten rows:

```bash
mdsmith list backlinks --include "plan/**" --limit 10 docs/api.md
```

## Scope

The command resolves the same direct Markdown links
MDS027 sees: `[text](path)` and `[text](path#anchor)`.

It also resolves Obsidian-style wikilinks
(`[[page]]`, `[[page#anchor]]`, `[[page|alias]]`,
`![[file.png]]`) by walking the workspace for a
shortest-path match. The wikilink rows carry
`"kind": "wikilink"` in JSON output. Text output
renders them as `[[target]]` (or `[[target|alias]]`,
`![[target]]`).

Reference-style links (`[text][label]`) and external
URLs are still out of scope.

## Exit codes

| Code | Meaning               |
|------|-----------------------|
| 0    | At least one match    |
| 1    | No matches, no errors |
| 2    | Runtime/parse error   |

Per-file read failures are printed to stderr and never
abort the walk: matches from the surviving files still
appear on stdout. The exit code reflects the worst
outcome — `0` when any record was emitted, `2` when
nothing matched but errors occurred, `1` only on a
clean empty result.
