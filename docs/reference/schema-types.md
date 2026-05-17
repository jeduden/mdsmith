---
weight: 40
summary: >-
  Named field-type shortcuts for inline schema
  frontmatter values — the registered names, the
  canonical CUE each one resolves to, and example
  usage.
---
# Schema field types

A **field-type shortcut** is a short name a schema
frontmatter value can use instead of spelling out a CUE
expression. `created: date` is shorter and harder to
typo than `created: '=~"^\\d{4}-\\d{2}-\\d{2}$"'`, and
every project using the shortcut gets the same regex —
no three projects land on three slightly different ISO
patterns.

Shortcuts work the same way for inline schemas
(`kinds.<name>.schema:`) and for proto.md frontmatter.
The library ships embedded in the mdsmith binary; no
network access is needed to resolve a shortcut.

## How a value is interpreted

A frontmatter value (right-hand side of `key: value` in
the schema's `frontmatter:` block) is classified by
shape:

| Shape                                       | Treatment                                                              |
|---------------------------------------------|------------------------------------------------------------------------|
| Bare identifier in the shortcut registry    | Substituted with the canonical CUE expression                          |
| Bare identifier that is a CUE built-in type | Passes through verbatim (`string`, `int`, `bool`, `float`, `bytes`, …) |
| Bare identifier that is neither             | **Config error** naming the field and the unknown name                 |
| Anything else (operators, quotes, …)        | Passes through verbatim as raw CUE                                     |

A "bare identifier" is a YAML scalar matching
`[a-zA-Z_][a-zA-Z0-9_-]*` — letters, digits,
underscores, and hyphens, starting with a letter or
underscore. Anything containing whitespace, operators,
quotes, brackets, or other punctuation is treated as
raw CUE.

The strict third row catches typos at config-load
time. A schema that writes `created: iso-date` errors
with `unknown shortcut "iso-date"` instead of sliding
through to an undefined-reference error deep in CUE
evaluation.

## Registered shortcuts

The following names are accepted in a schema
frontmatter value:

| Name       | Canonical CUE                                                             | Accepts                | Rejects               |
|------------|---------------------------------------------------------------------------|------------------------|-----------------------|
| `date`     | `=~"^\\d{4}-\\d{2}-\\d{2}$"`                                              | `2024-05-01`           | `2024-5-1`            |
| `datetime` | `=~"^\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}(Z\|[+-]\\d{2}:\\d{2})?$" ` | `2024-05-01T12:30:00Z` | `2024-05-01 12:30:00` |
| `time`     | `=~"^\\d{2}:\\d{2}(:\\d{2})?$"`                                           | `12:30`                | `12:3`                |
| `email`    | `=~"^[^@\\s]+@[^@\\s]+\\.[^@\\s]+$"`                                      | `user@example.com`     | `user@@example`       |
| `url`      | `=~"^https?://"`                                                          | `https://example.com`  | `ftp://example.com`   |
| `filename` | `=~"^[A-Za-z0-9._-]+\\.md$"`                                              | `notes.md`             | `notes.txt`           |
| `nonEmpty` | `string & !=""`                                                           | `hello`                | `""`                  |

The regex bodies are shape-only. `date` accepts
`2024-02-30` even though February never has 30 days;
semantic validation is out of scope.

## Inline schema example

```yaml
kinds:
  rfc:
    schema:
      frontmatter:
        created: date
        modified: datetime
        contact: email
        homepage: url
      require:
        filename: "RFC-[0-9][0-9][0-9][0-9].md"
```

`mdsmith check` validates every file in the `rfc` kind:
`created` must match the ISO-date regex, `contact` must
look like an email, and so on.

## proto.md example

The same shortcuts work in a proto.md schema's YAML
frontmatter:

```markdown
---
created: date
homepage: url
---
# ?

## Goal

## Tasks
```

Substitution happens at schema-load time. After that
the validator treats the canonical CUE the same way
whether a user typed it out or the loader expanded
the shortcut.

## Composing a shortcut with extra constraints

A value that needs both a shortcut and an extra
constraint is **not** a bare name. Write the canonical
CUE directly:

```yaml
frontmatter:
  created: '=~"^\\d{4}-\\d{2}-\\d{2}$" & >="2020-01-01"'
```

The shortcut form is only triggered when the value is
exactly the bare name. Anything more complex passes
through as raw CUE.

## Adding a shortcut

The library lives at `cue/types/types.cue` in this
repository. The runtime registry is in
`internal/schema/shortcuts.go`; a drift test pins the
two to each other. User-defined shortcuts are not yet
a configuration surface — a project that needs a
custom shortcut should write the canonical CUE in a
shared proto.md (or wait for a follow-up plan if the
need recurs).

## See also

- [Schemas](../guides/schemas.md) — the broader story
  on inline schemas, proto.md, and per-scope rule
  overrides.
- [MDS020](../../internal/rules/MDS020-required-structure/README.md)
  — the rule that surfaces schema diagnostics.
- [File kinds](../guides/file-kinds.md) — how kinds
  attach schemas (and other rule config) to file
  groups.
