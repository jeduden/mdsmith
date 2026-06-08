---
command: rename
summary: Rename a heading or link-reference label and rewrite every dependent edit.
---
# `mdsmith rename`

The CLI surface for the same rename engine the LSP server
drives, so a script or agent with no editor reaches it too.
Every dependent edit across the workspace is rewritten in
place.

```text
mdsmith rename [flags] <file> <old> <new>
```

`<file>` is workspace-relative. Absolute paths and
parent-traversal entries (`../foo.md`) are rejected with
exit code 2. Exactly one of `--heading` or `--link-ref`
is required.

With `--heading`, `<old>` is the heading's current visible
text. mdsmith rewrites the heading line. It also rewrites
every workspace `[text](file.md#slug)` anchor link and
every `[label]: file.md#slug` ref-def whose target resolved
to it. Same-file `(#slug)` references are included. A
duplicate-name disambiguator that shifts as a result is
updated too.

With `--link-ref`, `<old>` is the label. It is matched
after the lowercase / whitespace-collapse normalization
links already use. The `[label]: url` definition moves
with every `[text][label]` and shortcut `[label]` use in
the file.

The rename refuses to corrupt the workspace. It fails when
the new heading slug collides with another heading. It
fails when the label collides with another definition. It
fails when the text slugifies to nothing, or carries a
newline or a stray bracket. Each failure exits 2 and names
the conflict. No partial edit is written.

## Flags

| Flag                | Default | Description                                |
| ------------------- | ------- | ------------------------------------------ |
| `--heading`         | false   | Rename a heading and its workspace anchors |
| `--link-ref`        | false   | Rename a link-ref label: def + uses        |
| `-c`, `--config`    | auto    | Override config path                       |
| `-f`, `--format`    | `text`  | Output format: `text` or `json`            |
| `--no-gitignore`    | false   | Disable `.gitignore` filtering during walk |
| `--follow-symlinks` | config  | Follow symlinks; tri-state — see below     |
| `--max-input-size`  | `2MB`   | Max file size (e.g. `2MB`, `0`=none)       |

`--follow-symlinks` and file discovery (the `files:` and
`ignore:` patterns in `.mdsmith.yml`) match
[`mdsmith check`](check.md#flags).

## Output

A summary of the rewritten files, one per line.

**text** (default):

```text
docs/guide.md: 1 edit(s)
docs/index.md: 2 edit(s)
```

**json**:

```json
[
  {
    "file": "docs/guide.md",
    "edits": 1
  }
]
```

Rows are sorted by path. Keys are stable.

## Examples

Rename a heading and fix every link that pointed at it:

```bash
mdsmith rename docs/guide.md --heading "Old Title" "New Title"
```

Rename a link-reference label:

```bash
mdsmith rename docs/guide.md --link-ref oldlabel newlabel
```

JSON summary for a release script:

```bash
mdsmith rename --format json docs/guide.md --heading "Setup" "Install"
```

## Exit codes

| Code | Meaning                        |
| ---- | ------------------------------ |
| 0    | Rewritten                      |
| 1    | No matching heading or label   |
| 2    | Conflict, invalid input, error |

## See also

- [`mdsmith deps`](deps.md) — the dependency edges the
  rename walks to find dependent anchors.
- [`mdsmith lsp`](lsp.md) — the editor surface for the
  same rename engine (prepare-range, collision data).
