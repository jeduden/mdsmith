---
title: Kind files under `.mdsmith/kinds/`
weight: 25
summary: >-
  Each file under `.mdsmith/kinds/` declares one
  kind. The basename is the kind name; the file body
  carries the full `KindBody` — schema, rules,
  `path-pattern:`, `extends:`. Sits alongside inline
  `kinds.<name>:` in `.mdsmith.yml`.
---
# Kind files under `.mdsmith/kinds/`

A **kind file** is a YAML file under
`.mdsmith/kinds/` whose basename is the kind's name
and whose body is the full kind definition. One file
per kind, no nesting. The directory sits next to
`.mdsmith.yml` at the workspace root.

```text
.mdsmith.yml                   # unchanged
.mdsmith/
  kinds/
    audit-log.yaml
    secret-rotation.yaml
    architecture-doc.yaml
```

Use kind files when the `kinds:` block has grown
large. Each rule edit dirties the same
`.mdsmith.yml` as every other config change.
Splitting kinds into one file each isolates the
history. The read path shortens too: open
`audit-log.yaml` to see the whole `audit-log`
kind.

## File shape

The file body matches the inline
`kinds.<name>:` body. It accepts every
[`KindBody`](../guides/file-kinds.md) key:
`extends:`, `path-pattern:`, `categories:`,
`schema:`, `rules:`. A key outside that set is
a config error.

```yaml
# .mdsmith/kinds/audit-log.yaml
schema:
  frontmatter:
    title: 'string & != ""'
    "summary?": 'string'
    audit-from: '=~"^[0-9a-f]{7,40}$"'
  filename: "architecture-audit.md"
  closed: false
  sections:
    - heading: null
    - heading:
        regex: '.+'
        repeat: { min: 0 }
rules:
  max-file-length:
    max: 600
```

## Basename rule

The kind's name is the basename minus extension.
The basename must match `[a-z][a-z0-9-]*` — lower
case, starting with a letter, with optional
hyphen-separated segments. The rule applies only to
filenames (OS case folding, path safety); inline
`kinds.<name>:` keys stay unvalidated.

Both `*.yaml` and `*.yml` are scanned. Two kind
files with the same basename across the two
extensions is a config error naming both files.

Subdirectories under `.mdsmith/kinds/` are
rejected. One kind per file, flat layout.

## Composition with `.mdsmith.yml`

`kinds.<name>:` blocks inside `.mdsmith.yml`
remain a first-class source. A project can mix
inline and file-defined kinds freely.

The same kind name declared in **both** a file and
inline is a config error naming both sources. The
two sources do **not** merge — a merged kind would
defeat the "read one file to know one kind"
property kind files ship.

`kind-assignment:`, `overrides:`, and `ignore:` are
glob-keyed and stay in `.mdsmith.yml`. A
kind-assignment entry references a kind by name —
inline or file kind — with no extra wiring.

## Schema sources

A kind file accepts the same three schema sources
as an inline kind, and they remain mutually
exclusive (acceptance criterion #6 of plan 208):

- inline `schema:` block
- `rules.required-structure.schema:` path to a
  `proto.md`
- legacy `rules.required-structure.inline-schema:`
  map

Setting two on the same kind errors at config load
with both source names.

A schema shared across kinds is shared via
`extends:` — the inheritance chain works
seamlessly across sources. A file kind may extend
an inline kind and the reverse. The cycle detector
runs on the merged kinds map.

## Audit

`mdsmith kinds resolve <file>` prints the
defining-source path next to each kind it
reports. A mixed resolution shows the path each
kind came from. You can jump straight to the
right file:

```text
file: docs/audit.md
effective kinds:
  - audit-log (from kind-assignment[3]: glob docs/**/*.md) defined-in .mdsmith/kinds/audit-log.yaml
```

`mdsmith kinds show <name>` adds a `defined-in:`
line to the body output, so the same info is
available without going through a target file.

The JSON shape (`--json`) carries a
`source-path:` key on each kind body and on every
resolved-kind entry so editor integrations can
key off a stable field.
