---
id: MDS071
name: required-frontmatter
status: ready
description: >-
  Every file in the configured glob scope must declare each named
  front-matter field with a present, non-empty value.
category: structural
nature: structure
maintainability: null
markdownlint: []
rumdl: []
mado: []
panache: []
obsidian-linter: []
gomarklint: []
---
# MDS071: required-frontmatter

Every file in the configured glob scope must declare each named
front-matter field with a present, non-empty value.

A file in scope is flagged once per required field that is absent,
explicitly null, or empty. An empty string (including whitespace
only), an empty list, and an empty map all count as empty; any other
scalar — a non-blank string, a number, a boolean — satisfies the
field. The rule checks presence, not a particular type. A file with
no front matter at all is missing every required field.

The diagnostic anchors to the file's first line. With no fields
configured the rule reports nothing, so it ships registered and
opt-in: nothing fires until a convention, kind, or config names the
fields.

The canonical use is the Open Knowledge Format (OKF), where every
concept document must carry a non-empty `type` and the reserved
`index.md` and `log.md` files are excluded. The built-in `okf`
convention wires the rule for exactly that.

## Settings

| Setting   | Type   | Default | Description                                          |
| --------- | ------ | ------- | ---------------------------------------------------- |
| `fields`  | list   | `[]`    | front-matter keys that must be present and non-empty |
| `field`   | string | `""`    | convenience alias for a single-element `fields`      |
| `include` | list   | `[]`    | globs selecting the scope; empty means every file    |
| `exclude` | list   | `[]`    | globs removed from the scope after include matching  |

Setting both `field` and `fields` is a config error. With no fields
configured the rule is inert. Globs match the raw path, the cleaned
path, and the base name, so `exclude: [index.md]` removes every
`index.md` at any depth.

## Config

Require a non-empty `type` on every Markdown file except the OKF
reserved files:

```yaml
rules:
  required-frontmatter:
    fields: [type]
    exclude: [index.md, log.md]
```

Single field via the alias:

```yaml
rules:
  required-frontmatter:
    field: type
```

Disable:

```yaml
rules:
  required-frontmatter: false
```

## Examples

### Bad — the required field is missing

```yaml
---
title: Orders
---
```

```text
front-matter "type" is required but missing
```

### Bad — the required field is empty

```yaml
---
type: ""
---
```

```text
front-matter "type" is required but empty
```

### Good — the field is present and non-empty

```yaml
---
type: BigQuery Table
title: Orders
---
```

## See also

- [MDS069](../MDS069-unique-frontmatter/) — the cross-file companion:
  enforces that a field's values are distinct across files, where
  this rule enforces that the field exists and holds a value
- [MDS020](../MDS020-required-structure/) — validates front matter
  against a full schema (types, enums, sections); reach for it when a
  presence-and-non-empty check is not enough
- [OKF guide](../../../docs/guides/okf.md) — authoring and validating
  Open Knowledge Format bundles with mdsmith

## Meta-Information

- **ID**: MDS071
- **Name**: `required-frontmatter`
- **Status**: ready
- **Default**: disabled (opt-in)
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: structural
