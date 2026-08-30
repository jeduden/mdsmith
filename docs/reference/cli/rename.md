---
command: rename
summary: Retitle a heading or rename a link-reference label and rewrite every dependent edit; the kind is auto-detected or forced with `--as`, and a path-shaped request is steered to `mdsmith move`.
---
# `mdsmith rename`

The CLI surface for the same rename engine the LSP server
drives, so a script or agent with no editor reaches it too.
Every dependent edit across the workspace is rewritten in
place.

`rename` changes a symbol *inside* a file — a heading slug or a
link-reference label. To relocate a *file*, use
[`mdsmith move`](move.md).

```text
mdsmith rename [flags] <file> <old> <new>
```

`<file>` is workspace-relative. Absolute paths and
parent-traversal entries (`../foo.md`) are rejected with exit
code 2.

## What it rewrites

When `<old>` names a **heading** (its current visible text),
mdsmith rewrites the heading line. It also rewrites every
workspace `[text](file.md#slug)` anchor link that resolved to
it. The matching `[label]: file.md#slug` ref-defs update the
same way. Same-file `(#slug)` references are included, and a
shifted duplicate-name disambiguator updates too.

When `<old>` names a **link-reference label**, the
`[label]: url` definition moves with every `[text][label]` and
shortcut `[label]` use in the file. The label is matched after
the lowercase / whitespace-collapse normalization links use.

## Choosing heading vs label

The kind is auto-detected from `<old>`: a heading whose visible
text is `<old>`, or a label normalizing to `<old>`. Pass
`--as heading` or `--as label` to force it. When both match,
the command exits 2 and asks for `--as`.

A path-shaped `<old>` or `<new>` — one with a slash or a `.md` /
`.markdown` suffix — that matches no symbol exits 2 and points
you at `mdsmith move`. Renaming a *file* is a move.

| You want to…                                   | Command  |
| ---------------------------------------------- | -------- |
| Relocate or rename a *file* (path or basename) | `move`   |
| Retitle a *heading* and fix its anchors        | `rename` |
| Rename a *link-ref label* and its uses         | `rename` |

## Safety

The rename refuses to corrupt the workspace. It fails when the
new heading slug collides with another heading, when the label
collides with another definition, or when the text slugifies to
nothing or carries a newline or a stray bracket. Each failure
exits 2 and names the conflict. No partial edit is written.
`--dry-run` prints the edits and changes nothing.

## Flags

| Flag                | Default | Description                                |
| ------------------- | ------- | ------------------------------------------ |
| `--as`              | auto    | Force the kind: `heading` or `label`       |
| `--dry-run`         | false   | Print the edits without writing them       |
| `-c`, `--config`    | auto    | Override config path                       |
| `-f`, `--format`    | `text`  | Output format: `text` or `json`            |
| `--no-gitignore`    | false   | Disable `.gitignore` filtering during walk |
| `--follow-symlinks` | config  | Follow symlinks; tri-state — see below     |
| `--max-input-size`  | `2MB`   | Max file size (e.g. `2MB`, `0`=none)       |

`--follow-symlinks` and file discovery (the `files:` and
`ignore:` patterns in `.mdsmith.yml`) match
[`mdsmith check`](check.md#flags).

## Output

The rewritten files, one per line.

**text** (default):

```text
docs/guide.md: 1 edit(s)
docs/index.md: 2 edit(s)
```

**json**:

```json
{
  "files": [
    { "file": "docs/guide.md", "edits": 1 }
  ]
}
```

Rows are sorted by path. Keys are stable. The `move` and
`dryRun` fields appear only for [`mdsmith move`](move.md) and a
dry run.

## Examples

Rename a heading and fix every link that pointed at it:

```bash
mdsmith rename docs/guide.md "Old Title" "New Title"
```

Force a link-reference label rename:

```bash
mdsmith rename docs/guide.md --as label oldlabel newlabel
```

JSON summary for a release script:

```bash
mdsmith rename --format json docs/guide.md --as heading "Setup" "Install"
```

## Exit codes

| Code | Meaning                                                         |
| ---- | --------------------------------------------------------------- |
| 0    | Rewritten                                                       |
| 1    | No matching heading or label (with an explicit `--as`)          |
| 2    | Conflict, invalid input, ambiguous kind, or move-shaped request |

## See also

- [`mdsmith move`](move.md) — relocate a file and rewrite every
  reference to it.
- [`mdsmith deps`](deps.md) — the dependency edges the rename
  walks to find dependent anchors.
- [`mdsmith lsp`](lsp.md) — the editor surface for the same
  rename engine (prepare-range, collision data).
