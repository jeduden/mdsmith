---
title: Choose between inline and file-based kinds
weight: 22
summary: >-
  Decide whether to declare a kind inline in
  `.mdsmith.yml` or as its own file under
  `.mdsmith/kinds/`. Selection criteria and the
  rules that separate the two forms.
---
# Choose between inline and file-based kinds

A kind can live in either of two places:

- Inline as `kinds.<name>:` in `.mdsmith.yml`.
- As a standalone YAML file at
  `.mdsmith/kinds/<name>.yaml` (or `.yml`).

Both forms accept the same `KindBody` keys
(`schema:`, `rules:`, `path-pattern:`, `extends:`,
`categories:`). For the same body, the two forms
produce byte-equal diagnostics. The choice is a
maintenance decision, not a behavioral one. Pick
per kind based on edit history, body size, and
how reviewers read the diff.

## The two forms side by side

Same kind, two homes. Declaring the same name in
both is a config error at load.

```yaml
# Inline — .mdsmith.yml
kinds:
  audit-log:
    schema:
      frontmatter:
        title: 'string & != ""'
        audit-from: '=~"^[0-9a-f]{7,40}$"'
      closed: false
    rules:
      max-file-length:
        max: 600
```

```yaml
# File-based — .mdsmith/kinds/audit-log.yaml
schema:
  frontmatter:
    title: 'string & != ""'
    audit-from: '=~"^[0-9a-f]{7,40}$"'
  closed: false
rules:
  max-file-length:
    max: 600
```

The file body drops the wrapper key. The basename
(`audit-log`) becomes the kind name and must match
`[a-z][a-z0-9-]*`. That constraint applies only to
filenames; inline kind names are not validated.

## When to keep a kind inline

Inline is the default. Keep a kind inline when:

- The body fits on a screen and the project's
  `kinds:` block is under about 50 lines. Scanning
  every kind at once is faster in one file.
- The kind is new and still under iteration.
  Editing in place is faster than alt-tabbing
  between two files.
- The kind sits next to a related override or
  `kind-assignment:` entry, and the proximity
  helps a reader follow the wiring.
- The project has fewer than six kinds. The
  review cost of one extra file per kind
  outweighs the history isolation.

## When to lift a kind into its own file

Move the kind to `.mdsmith/kinds/<name>.yaml`
when:

- The `kinds:` block has grown past about 150
  lines or six kinds and per-kind edits dirty
  unrelated config history. This repo's
  `.mdsmith.yml` was 558 lines with the `kinds:`
  block alone at 295–480 when plan 208 shipped.
  That is exactly the case file-based kinds
  solve.
- A reviewer should see which kind changed from
  the file list alone. A PR that touches
  `.mdsmith/kinds/secret-rotation.yaml` names
  itself; a PR that touches `.mdsmith.yml`
  forces the reviewer to open the diff to learn
  which kind moved.
- The kind body is large. A long frontmatter
  schema and many rule overrides bloat the
  inline view of every other kind.
- The kind is reused across repos. Copying a
  standalone file is cleaner than excising a
  named block from a shared `.mdsmith.yml`.

For the mechanics of the move, see
[Split a kind into its own file](file-kinds.md#split-a-kind-into-its-own-file).

## What stays inline regardless

Four config sections have no canonical per-kind
name and remain in `.mdsmith.yml`:

- `kind-assignment:` — entries are glob-keyed,
  not name-keyed.
- `overrides:` — glob-keyed.
- `ignore:` — glob-keyed.
- Top-level `rules:` — one project-wide block,
  not a per-kind body.

A `kind-assignment:` entry references a
file-defined kind by name with no extra wiring.
The resolver merges file and inline kinds into
one map before assignment runs.

## Side-by-side comparison

| Property             | Inline                       | File-based                                                                             |
| -------------------- | ---------------------------- | -------------------------------------------------------------------------------------- |
| Where to read it     | One spot in `.mdsmith.yml`   | One file per kind under `.mdsmith/kinds/`                                              |
| Edit history         | Mixed with every config edit | Isolated per kind                                                                      |
| Files to open        | One                          | Two: the kind file plus `.mdsmith.yml` for any matching `kind-assignment:` or override |
| Name rule            | Any YAML key                 | Basename must match `[a-z][a-z0-9-]*`                                                  |
| Copy to another repo | Find and excise the block    | Copy the file                                                                          |
| Conflict semantics   | One source                   | Declaring the same name in both is a config error                                      |

`extends:` works across the two sources. A
file-defined kind can extend an inline kind and
the reverse. The cycle detector runs on the
merged kinds map.

## Mixing both forms

A project can use both forms in the same
`.mdsmith.yml`. The conventional split is:

- File-based for long-lived structural kinds:
  `audit-log`, `secret-rotation`,
  `architecture-doc`.
- Inline for small project-glue kinds: a one-off
  `proto` kind that disables structural rules
  on `proto.md`, or a `rule-readme` kind with
  three rule toggles.

`mdsmith kinds resolve <file>` prints the
defining-source path for each kind in the file's
effective list, so a mixed configuration stays
auditable from one command. `mdsmith kinds show
<name>` adds a `defined-in:` line to the body
output.

## See also

- [File kinds](file-kinds.md) — declaring,
  assigning, and merging kinds.
- [Kind files reference](../reference/kind-files.md)
  — directory layout, basename rule, JSON shape.
- [Schemas](schemas.md) — declaring a
  document-structure schema inline on a kind or
  in a `proto.md` file.
