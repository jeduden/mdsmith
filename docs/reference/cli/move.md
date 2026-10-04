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

- **Incoming links.** Every inline `[text](src)` or
  `![alt](src)` in the workspace is repointed to `dst`. A
  `?query` or `#anchor` after the path is kept. The path token
  is recomputed relative to each referencing file's own
  directory, preserving its spelling — an explicit `./x` keeps
  the prefix.
- **Ref-def destinations.** A `[label]: src` definition is
  repointed the same way, including one with a `?query`.
- **Outbound links, images, and ref-defs inside the moved
  file.** Each inline `[x](path)`, `![x](path)`, and
  `[label]: path` in `src` is recomputed so it still resolves
  from `dst`'s directory. Moving `docs/a.md` to `guide/a.md`
  fixes its own `[x](./b.md)` as well as the links pointing at
  it. A destination that still resolves, such as `sub/../b.md`
  after a move within one directory, keeps its spelling. A link
  to a directory, such as `sub/`, keeps its trailing `/`. A
  moved file that mdsmith does not lint as Markdown, such as an
  image, keeps its bytes. One with another extension that the
  `files:` patterns match, such as `x.mdx`, is recomputed too.
- **Wikilinks.** `[[old-stem]]` becomes `[[new-stem]]` only when
  the basename stem changes. A move that keeps the basename
  (`docs/api.md` → `ref/api.md`) leaves wikilinks alone, because
  a stem still resolves to the file at its new path — an
  asymmetry with path links that `--dry-run` makes visible. A
  new stem that holds a dot keeps its extension, so a move to
  `v1.3.md` writes `[[v1.3.md]]`: a bare `[[v1.3]]` would look
  for a file named exactly `v1.3`. A stem that ends in a space
  keeps it too, so `guide .md` gives `[[guide .md]]`, since a
  link target is trimmed. A new name with another extension is
  written whole, as in `[[guide.mdx]]`.

Each destination is found in the parsed document, so each one is
rewritten exactly once. These forms are all handled:

- a link with empty text, such as `[](a.md)`;
- a label that spans rows, or a destination on the row after
  its `(` (or after a ref-def's `:`), in a block quote too;
- an angle-bracketed destination, such as `<my file.md>`;
- a titled destination, such as `[t](a.md "title")`.

Link-shaped text in a code span, a code block, or an HTML
comment is not a destination, so it stays as written, even
inside a link's own label.

A percent-escaped destination such as `my%20file.md` is decoded
before it is compared. The new path is escaped the way the old
one was. `my%20file.md` stays escaped, and `<my file.md>` keeps
its literal space.

A character that would break the link is always escaped: a space
in a bare destination, and `%`, `?`, `#`, `<`, `>`, `&`, `\`, or
`"`. So a move to `what?.md` writes `what%3F.md`. A bare `?` would
start a query string and name the file `what`, and a literal
`&amp;` would be read as `&`. A bare destination also escapes its parens
when one has no partner. A move to `a).md` writes `a%29.md`, while
`a(1).md` stays as written. A new path whose first segment holds a
`:` gets a `./` prefix, so a move to `a:b.md` writes `./a:b.md`. A
bare `a:b.md` would read as a URL with the scheme `a:`.

A literal `?` is read as the start of a query unless the whole
path names the moved file, of any type, or a Markdown file in the
workspace, and the part before the `?` does not. Then
`[x](what?.md)` is matched as the file `what?.md`, and the rewrite
writes it as `what%3F.md`.

Absolute URLs, `mailto:`, and root-anchored `/x` paths do not
resolve to a workspace file, so a move never touches them.

## What needs a manual fix

A move does not rewrite these references yet. After a
cross-directory move, check them by hand.

- **Directive paths.** A `file:` path in `<?include?>` or an
  `inputs:` path in `<?build?>`, in the moved file or in a file
  that points at it, and a `<?catalog?>` glob in the moved
  file.
- **Raw HTML links.** `<a href="a.md">` and `<img src="a.png">`
  are not Markdown destinations.
- **Links above the workspace root.** A relative link in the
  moved file that climbs out of the workspace, such as
  `[p](../README.md)` in `a.md`, is never recomputed. After a move
  to `guide/a.md`, it names the workspace's own `README.md`.
- **Backslash escapes and entities.** A path spelled with one,
  such as `a\_b.md` or `a&amp;b.md`, is left as written, in the
  moved file and in the files that point at it. A renderer reads
  it as `a_b.md` or `a&b.md`, which the move does not decode.
  Write the name out, or percent-escape it as in `a%26b.md`. A
  `\` just before the `#` or `?` that ends the path, as in
  `a.md\#x`, only escapes that byte, so such a link is repointed
  and keeps its `\`.
- **Ambiguous wikilinks.** A `[[stem]]` reads the basename
  alone, folder prefix or not. When several Markdown files share
  the stem, it reaches the shallowest one, then the first by
  path, with capitals before lowercase letters, so `Docs/a.md`
  comes before `archive/a.md`.
  If that file is not the one you move, no `[[stem]]` is
  rewritten, because each one still reaches that other file. If
  it is, every `[[stem]]` follows the move, except one whose
  folder prefix names the folder of another file with the stem,
  such as `[[ref/guide]]` for `ref/guide.md`. That link already
  names its file, and it reaches that file once yours is gone.
  The new name gets the same check: when a file that already has
  the new stem comes before the destination, no `[[stem]]` is
  rewritten, because the new name would reach that file. When the
  destination comes first, the move also takes over that file's
  links. For a new name with another extension, such as
  `guide.mdx`, the check reads files with that exact name, since
  `[[guide.mdx]]` reads it by name. The check reads the files a
  wikilink resolves against: every file under the workspace root
  except `.git` and `node_modules`, whatever the `files:`
  patterns or `.gitignore` say. So a gitignored
  `archive/guide.md` blocks the rewrite for a move of
  `docs/guide.md`, and an unlisted `logo.png` blocks it for a
  move to `docs/logo.png`. A `node_modules/pkg/README.md` does
  not block a move of `docs/api/v1/readme.md`, though it is
  shallower. When another file has the stem, a source path typed
  in other letter case than on disk, such as `Docs/guide.md` for
  `docs/guide.md`, also blocks the rewrite: the check cannot tell
  it is the same file. A move that keeps the stem rewrites
  nothing, but it can still change which file the stem reaches.
  Moving the file a `[[guide]]` reaches below another `guide.md`
  hands its links to that file. Check such a move with `--dry-run`.
- **Wikilinks to a name they cannot reach.** A move to a name
  with no extension, such as `COPYING`, leaves every `[[stem]]`
  as written, because a bare `[[name]]` finds only Markdown
  files. So does a new name that a wikilink cannot spell: an
  empty stem such as `.md`, a `#`, `|`, `[`, or `]` as in `C#.md`,
  a backtick, a line break, or a name that ends with a space.
  So does a move into `.git` or `node_modules`, which a wikilink
  never searches. Those links break. A move out of one of them
  also leaves `[[stem]]` as written, since no wikilink reached
  the file there. A name that starts with a space, or reads as
  a drive letter such as `C:x.md`, is written as `[[./C:x]]`.
- **Footnote text that is a lone link.** mdsmith reads
  `[^1]: [z](a.md)` as a footnote definition and leaves its text
  as written, so the link inside it is not repointed. Longer
  footnote text, such as `[^1]: See [z](a.md).`, is repointed.
- **Embeds of a renamed non-Markdown file.** Moving an image
  repoints the links and images that name it. A wikilink embed
  such as `![[diagram.png]]` is left alone, so it goes stale
  when the move changes the file name.

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
   original bytes back to every file it had replaced. Any
   directory it created for `<dst>` stays behind, empty. A
   rewritten file that was a symlink becomes a regular file,
   and a rollback puts the old bytes in that file, not the link.

An edit does not apply when another edit on its line
claims some of the same bytes, or when it falls outside
its line. It also fails when it ends on another line than
it starts, or when it starts or ends between the two
UTF-16 units of a character such as 😀. The error names
the file and the line.

A plan failure prints only its cause. Nothing was written.
After a failed write, stderr names the cause. Its last line
gives the state of the workspace:

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
bytes or the new bytes, never a mix of both. Temp files named
`<file>.<digits>.tmp` may be left next to the files; delete
them.

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
- [`mdsmith lsp`](lsp.md) — the editor surface; an explorer rename fires
  `workspace/willRenameFiles`, which runs the same move engine. A rename of
  several files is planned as one batch: a link between two moved files gets
  one rewrite, from the holder's new folder to the target's new path. A link
  to a refused move from a moved file, a link in a refused move that leaves
  its folder, and a `[[stem]]` whose new stem another moved file takes get
  none. The warning counts each that may miss its file, and every link to a
  refused move whose path a moved file takes, but not a link from a refused
  move to an unmoved file; MDS027 flags any of these that stop resolving.
