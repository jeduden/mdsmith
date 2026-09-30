---
command: move
summary: Move a Markdown file and rewrite every reference to it — incoming links and ref-def destinations, the moved file's own outbound relative links, and `[[stem]]` wikilinks when the basename changes — staging the rename with `git mv` when the file is tracked.
---
# `mdsmith move`

> **Unreleased.** `move` landed after v0.55.1, so the npm, PyPI,
> and GitHub release binaries at that version answer
> `unknown command "move"`. It ships with the next release; until
> then, build from `main` with
> `go install github.com/jeduden/mdsmith/cmd/mdsmith@main`.
> Delete this note when a release includes the command.

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
- **Outbound inline links and images inside the moved file.**
  Each inline `[x](path)` or `![x](path)` in `src` is recomputed
  so it still resolves from `dst`'s directory. Moving
  `docs/a.md` to `guide/a.md` fixes its own `[x](./b.md)` as
  well as the links pointing at it. Link-shaped text in a code
  span, a code block, or an HTML comment is not a link, so it
  stays as written.
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
links and images are recomputed; ref-defs elsewhere that point
at the file are still repointed.

## How the file is moved

When `<src>` is tracked in the current Git work tree, `move`
runs `git mv` so the rename is staged in the index. Otherwise
it falls back to a plain filesystem rename. The destination's
parent directory is created first. A failed `git mv` leaves
the source in place, and `move` restores every file it had
rewritten (see [Safety](#safety)).

The file moves last, after every text edit is written, so the
relocated file carries its rewritten body.

## Safety

`move` is all-or-nothing. It rewrites every file and moves
`<src>`, or it exits 2 and undoes what it wrote. It works in
two phases.

1. **Plan.** `move` reads every file it will rewrite and
   computes the new bytes in memory. An existing `<dst>`, an
   unreadable file, or an edit that does not apply exits 2
   here, before anything is written. `--dry-run` runs this
   phase too, so it reports the same errors. Then it prints
   the edits and the planned move, and stops.
2. **Write.** `move` writes the new bytes of each file to a
   temp file in the same directory, with the permission bits
   of the original. If this fails, for example on a full disk,
   `move` deletes the temp files and no file changes. Next,
   each temp file is renamed over its original. Last, `<src>`
   is moved. If a rename or the move fails, `move` writes the
   original bytes back to every file it had replaced.

After a failure, stderr names the cause. Its last line gives
the state of the workspace:

- `no file was changed`: the failure came before any file was
  replaced.
- `restored N file(s) to their original content`: the rollback
  put back every replaced file.
- `N file(s) keep the rewritten content: <files>`: the
  rollback could not restore these files. A `restoring <file>`
  line gives the reason for each one. Restore them by hand, or
  with `git restore` when they are tracked.

This guarantee covers the failures `move` can see. A crash or
a power loss in the write phase can leave some files rewritten
and others not. Each file is still whole: it holds the old
bytes or the new bytes, never a mix of both.

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

| Code | Meaning                                                               |
| ---- | --------------------------------------------------------------------- |
| 0    | Moved                                                                 |
| 1    | Source not found                                                      |
| 2    | Existing destination, traversal, or a failed edit, write, or `git mv` |

## See also

- [`mdsmith rename`](rename.md) — retitle a heading or a
  link-reference label inside a file.
- [`mdsmith deps`](deps.md) — the dependency edges a move walks
  to find incoming references.
- [`mdsmith lsp`](lsp.md) — the editor surface; an explorer
  rename fires `workspace/willRenameFiles`, which runs the same
  move engine.
