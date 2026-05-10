---
id: 148
title: Named field-type shortcuts for inline schemas
status: "🔲"
model: sonnet
depends-on: [146]
summary: >-
  Ship a small CUE library of common field
  patterns (`date`, `datetime`, `email`, `url`,
  `filename`) and let inline schemas (plan 146)
  reference them by short name. Schemas in
  `proto.md` files import the same library.
---
# Named field-type shortcuts for inline schemas

## Goal

Make inline schemas (plan 146) shorter and more
readable for the common case. A user writing
`created: date` should not have to re-derive the
ISO-date regex; nor should three projects each
land on subtly different versions of the same
pattern.

## Background

mdbase ships a fixed list of named scalar types
(`string`, `int`, `number`, `bool`, `date`,
`datetime`, `time`, `enum`, `link`). mdsmith uses
CUE, which is more expressive but asks each
project to spell out its own date and email
patterns. The mdbase research records this gap as
**S-2**: not a missing capability, but a missing
ergonomic layer over CUE.

Plan 146 (inline schemas) is the trigger. Before
plan 146 the patterns can live in a shared
`proto.md` and be reused by import; after plan
146 each inline `frontmatter:` block re-derives
them.

## Non-Goals

- Replacing CUE. The shortcuts resolve to CUE
  expressions; everything CUE accepts is still
  accepted.
- Custom user-defined shortcuts. A real project
  can already define one in its own `proto.md`
  schema. If user shortcuts become a real
  request, that is a separate plan.
- Validating against system locale or timezone.
  Date and datetime patterns match strings of the
  expected ISO shape; semantic checks (is the day
  valid for the month) are out of scope.

## Design

### The shortcut library

A package lives at a stable, importable module
path. The provisional path is
`github.com/jeduden/mdsmith/types`. The plan
settles it on landing. The initial vocabulary:

```cue
package types

#date:     =~"^\\d{4}-\\d{2}-\\d{2}$"
#datetime: =~"^\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}(Z|[+-]\\d{2}:\\d{2})?$"
#time:     =~"^\\d{2}:\\d{2}(:\\d{2})?$"
#email:    =~"^[^@\\s]+@[^@\\s]+\\.[^@\\s]+$"
#url:      =~"^https?://"
#filename: =~"^[A-Za-z0-9._-]+\\.md$"
#nonEmpty: string & != ""
```

The library source lives in this repo, likely
at `cue/types/types.cue`. The distribution
mechanism (vendored overlay, embedded asset,
public Go module) is settled in the
implementation. The user-visible contract is
the import path and the symbol names.

### File-schema use

A `proto.md` schema imports the package and
references the definitions:

```cue
package proto

import "github.com/jeduden/mdsmith/types"

created:  types.#date
modified: types.#datetime
homepage: types.#url
```

Existing `proto.md` schemas keep working. The
import is opt-in.

### Inline-schema shortcut form

Inline schemas (plan 146) accept the unprefixed
short name as a string value:

```yaml
kinds:
  rfc:
    schema:
      frontmatter:
        created: date
        modified: datetime
        contact: email
        homepage: url
```

The loader splits values into two cases:

- **Bare name** — a YAML scalar that is a single
  identifier (no quotes, no operators, no
  whitespace). Must resolve in the registry, or
  the loader rejects the config with an error
  naming the field and the unknown name.
- **Anything else** — values containing
  operators, quotes, parentheses, or whitespace
  — passes through as raw CUE. So
  `status: '"open" | "done"'` continues to work
  unchanged.

The split keeps intent unambiguous. A typo on a
bare shortcut surfaces early. A real CUE
expression is never silently re-read as a
shortcut.

### Composition with CUE

A field that needs both a shortcut and an extra
constraint uses inline CUE syntax:

```yaml
frontmatter:
  created: 'date & >="2020-01-01"'
```

The shortcut is parsed only when the value is
exactly the bare short name. Anything more
complex stays raw CUE.

### Surface

- A new public package import path for the
  library.
- A new bare-name shortcut in inline-schema
  `frontmatter:` values.
- A documentation page listing the registered
  names and what each resolves to.

## Tasks

1. Create the CUE source for the shortcut
   library at the chosen module path.
   Settle the path (publicly importable,
   versioned) and document the choice in the
   plan.
2. Wire mdsmith's CUE overlay so external
   `proto.md` schemas can resolve the import
   without network access.
3. In `internal/config/`, add a registry of
   short names → canonical CUE expressions.
4. In the inline-schema loader (plan 146), look
   up bare-string `frontmatter:` values in the
   registry; rewrite to the canonical expression
   before evaluation. Anything else passes
   through as raw CUE.
5. Add a documentation page at
   `docs/reference/schema-types.md` listing the
   registered names, the canonical CUE, and an
   example use.
6. Cross-link the page from the
   [file-kinds guide](../docs/guides/file-kinds.md)
   and the
   [MDS020 README](../internal/rules/MDS020-required-structure/README.md).
7. Tests:

  - each shortcut accepts the canonical inputs
     and rejects clear violations,
  - a value that is `date & >="2020-01-01"`
     parses as raw CUE, not as a shortcut,
  - an unknown bare name produces a clear
     config error naming the field and the
     unknown name.

## Acceptance Criteria

- [ ] The shortcut library defines `#date`,
      `#datetime`, `#time`, `#email`, `#url`,
      `#filename`, `#nonEmpty` at the chosen
      import path.
- [ ] An external `proto.md` schema resolves the
      import without internet access (covered by
      a fixture test using an offline CUE
      module cache).
- [ ] An inline schema with `created: date`
      validates `2024-05-01` and rejects
      `2024-5-1`.
- [ ] An inline-schema value that is not a
      registered bare name (`status: '"open" |
      "done"'`) is parsed as raw CUE without
      lookup.
- [ ] An inline-schema value that looks like a
      shortcut but is unknown (`created:
      iso-date`) produces a config error
      naming the field and the unknown name.
- [ ] A new `docs/reference/schema-types.md`
      page lists every registered name with the
      canonical CUE expression and an example.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no
      issues.
