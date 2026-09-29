---
command: move
summary: Move a Markdown file and rewrite every reference to it — incoming links and ref-def destinations, the moved file's own outbound relative links, and `[[stem]]` wikilinks when the basename changes — staging the rename with `git mv` when the file is tracked.
---
# `mdsmith move`

Relocate a Markdown file and rewrite every reference in one
step, so no link breaks in either direction. `move` and
[`rename`](rename.md) are two verbs over one refactor engine:
`move` changes a file's path, `rename` changes a heading slug
or a link-reference label inside a file.

```text
mdsmith move [flags] <src> <dst>
```

`<src>` and `<dst>` are workspace-relative. Absolute paths and
parent-traversal entries (`../foo.md`) are rejected with exit
code 2.

## What it rewrites

- **Incoming links.** Every `[text](src)` and
  `[text](src#anchor)` in the workspace is repointed to `dst`;
  the `#anchor` fragment is kept. The path token is recomputed
  relative to each referencing file's own directory, preserving
  its spelling — an explicit `./x` keeps the prefix.
- **Ref-def destinations.** A `[label]: src` definition line is
  repointed the same way.
- **Outbound inline links inside the moved file.** Each inline
  `[x](path)` in `src` is recomputed so it still resolves from
  `dst`'s directory. Moving `docs/a.md` to `guide/a.md` fixes
  its own `[x](./b.md)` as well as the links pointing at it.
- **Wikilinks.** `[[old-stem]]` becomes `[[new-stem]]` only when
  the basename stem changes. A move that keeps the basename
  (`docs/api.md` → `ref/api.md`) leaves wikilinks alone, because
  a stem still resolves to the file at its new path — an
  asymmetry with path links that `--dry-run` makes visible.

Absolute URLs, `mailto:`, and root-anchored `/x` paths do not
resolve to a workspace file, so a move never touches them.

Some references inside the moved file are not yet recomputed. A
cross-directory move can leave them stale. Two kinds need a
manual fix. One is `<?include?>`, `<?build?>`, and `<?catalog?>`
directive paths. The other is a reference definition the file
declares itself, such as `[label]: ../other.md`. Only inline
links are recomputed; ref-defs elsewhere that point at the file
are still repointed.

## How the file is moved

When `<src>` is tracked in the current Git work tree, `move`
runs `git mv` so the rename is staged in the index. Otherwise
it falls back to a plain filesystem rename. The destination's
parent directory is created first. A tracked file whose
`git mv` fails aborts the whole operation — the source stays in
place and no edit is written.

The text edits apply before the file is moved, so the relocated
file carries its rewritten body.

## Safety

An existing `<dst>` aborts with exit code 2 and nothing is
moved or written. `--dry-run` prints the edits and the planned
move and changes nothing.

## Flags

| Flag                | Default | Description                                     |
| ------------------- | ------- | ----------------------------------------------- |
| `--dry-run`         | false   | Print the edits and planned move; write nothing |
| `-c`, `--config`    | auto    | Override config path                            |
| `-f`, `--format`    | `text`  | Output format: `text` or `json`                 |
| `--no-gitignore`    | false   | Disable `.gitignore` filtering during walk      |
| `--follow-symlinks` | config  | Follow symlinks; tri-state — see below          |
| `--max-input-size`  | `2MB`   | Max file size (e.g. `2MB`, `0`=none)            |

`--follow-symlinks` and file discovery (the `files:` and
`ignore:` patterns in `.mdsmith.yml`) match
[`mdsmith check`](check.md#flags).

## Output

The rewritten files, one per line, then the move.

**text** (default):

```text
docs/index.md: 2 edit(s)
guide/api.md: 1 edit(s)
moved docs/api.md -> guide/api.md
```

A `--dry-run` writes `would move` in place of `moved`.

**json**:

```json
{
  "files": [
    { "file": "docs/index.md", "edits": 2 }
  ],
  "move": { "from": "docs/api.md", "to": "guide/api.md" }
}
```

## Examples

Move a file and fix every reference:

```bash
mdsmith move docs/api.md reference/api.md
```

Preview the edits without touching disk:

```bash
mdsmith move guide.md reference/guide.md --dry-run
```

## Exit codes

| Code | Meaning                                             |
| ---- | --------------------------------------------------- |
| 0    | Moved                                               |
| 1    | Source not found                                    |
| 2    | Existing destination, traversal, or `git mv` failed |

## See also

- [`mdsmith rename`](rename.md) — retitle a heading or a
  link-reference label inside a file.
- [`mdsmith deps`](deps.md) — the dependency edges a move walks
  to find incoming references.
- [`mdsmith lsp`](lsp.md) — the editor surface; an explorer
  rename fires `workspace/willRenameFiles`, which runs the same
  move engine.
