---
title: "Rename and move without breaking links"
summary: >-
  Rename a heading and every workspace anchor link that points at it is
  rewritten in one atomic edit. Link-reference labels rename with their
  uses. Move a whole file and every incoming link, ref-def, and
  outbound relative link is rewritten while `git mv` stages the rename.
  A colliding slug fails loudly instead of silently breaking cross-file
  links.
icon: replace
weight: 9
group: "A connected docs tree"
link: "/reference/cli/lsp/"
---
# Rename without breaking links

Renaming a heading normally breaks every
`[text](file.md#old-slug)` link that pointed at it. The
links still parse, so nothing complains until a reader hits
a dead anchor — or until MDS027 flags it on the next lint
pass, after the damage is committed.

mdsmith renames the whole graph at once. Rename a heading
and the editor rewrites the heading line plus every
workspace anchor link that resolved to its slug, in a
single atomic edit. Same-file `(#slug)` references are
included. When a duplicate-name disambiguator shifts —
renaming the first "Setup" changes the second's slug from
`setup-1` to `setup` — the affected links update too.

Link-reference labels rename the same way. The `[label]:
url` definition and every `[text][label]` and shortcut
`[label]` use in the file move together.

The rename refuses to corrupt the workspace. If the new
heading text slugifies to a slug another heading already
owns, the rename fails and names the colliding heading
rather than silently shifting numbered suffixes. A label
that collides with another definition fails the same way.
Text that slugifies to nothing, or that contains a newline
or a stray bracket, is rejected before any edit applies.

Any LSP-aware editor, and the Claude Code agent, can
drive this over the wire. See the
[LSP reference](../reference/cli/lsp.md) for the
prepare-range table and the collision-error contract.

## Move a whole file

Renaming a symbol fixes links *inside* the graph; moving a
file fixes the links *to* it. `mdsmith move` relocates a file
and rewrites every reference in one step, repointing every
incoming `[text](path)` link and `[label]: path` ref-def. It
also recomputes the moved file's own outbound relative links,
so `docs/a.md` → `guide/a.md` keeps its `[x](./b.md)` working.
`[[stem]]` wikilinks follow when the basename changes. A tracked
file is staged with `git mv`; otherwise it is a plain move. Any
LSP-aware editor fires the same engine on an explorer rename.

```bash
mdsmith move docs/api.md reference/api.md
mdsmith move guide.md reference/guide.md --dry-run
```

## Which command?

| You want to…                                   | Command  |
| ---------------------------------------------- | -------- |
| Relocate or rename a *file* (path or basename) | `move`   |
| Retitle a *heading* and fix its anchors        | `rename` |
| Rename a *link-ref label* and its uses         | `rename` |

## From the command line

The same refactor engine has a CLI surface, so a script or
an agent with no editor reaches it too:

```bash
mdsmith rename docs/guide.md "Old Title" "New Title"
mdsmith rename docs/guide.md --as label oldlabel newlabel
```

`rename` auto-detects whether `<old>` is a heading or a label;
`--as heading` or `--as label` forces it. A path-shaped request
is steered to `mdsmith move`. The command rewrites every
dependent edit in place and prints a per-file summary
(`--format text|json`, `--dry-run` to preview). A collision, an
empty or bracket-bearing name, or a missing target exits
non-zero and names the conflict, exactly like the editor path.
See the [`mdsmith rename`](../reference/cli/rename.md) and
[`mdsmith move`](../reference/cli/move.md) references for flags,
output, and exit codes.
