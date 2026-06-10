---
id: MDS069
name: unique-frontmatter
status: ready
description: >-
  Every file in the configured glob scope must hold a distinct
  value in the configured front-matter field.
category: structural
nature: structure
maintainability: null
markdownlint: []
rumdl: []
mado: []
panache: []
obsidian-linter: []
---
# MDS069: unique-frontmatter

Every file in the configured glob scope must hold a distinct
value in the configured front-matter field.

The first holder in ascending path order stays clean; every
later file with the same value gets one diagnostic at its
front-matter line, naming the field, the value, and the first
holder. Files without the field are skipped. Values compare as
canonical scalars: `id: 7` and `id: "7"` collide, and so do
`id: 0x10` and `id: 16`; sequence, mapping, and null values
never participate.

Symlinked files and directories are skipped, so front matter
outside the workspace cannot join the scope. The index reads
files as saved on disk; in an editor the verdict refreshes on
save.

The canonical use is plan ids. Two branches that each add a
plan can merge a silent id collision. This rule turns the
collision into a `mdsmith check` failure on the merged
result.

## Settings

| Setting   | Type   | Default | Description                                                     |
| --------- | ------ | ------- | --------------------------------------------------------------- |
| `field`   | string | `""`    | front-matter key whose values must be distinct within the scope |
| `include` | list   | `[]`    | glob patterns selecting the scope; empty disables the rule      |
| `exclude` | list   | `[]`    | glob patterns removed from the scope after include matching     |

With no `include` globs the rule reports nothing, so it ships
enabled and inert until a scope is configured.

## Config

Enable for plan ids:

```yaml
rules:
  unique-frontmatter:
    field: id
    include: ["plan/*.md"]
    exclude: ["plan/proto.md"]
```

Disable:

```yaml
rules:
  unique-frontmatter: false
```

## Examples

### Bad — two files share an id

```yaml
# plan/a.md front matter
---
id: 7
---
```

```yaml
# plan/b.md front matter
---
id: 7
---
```

`plan/b.md` sorts after `plan/a.md`, so `plan/b.md` is flagged:

```text
front-matter "id": value 7 already used by plan/a.md
```

### Good — distinct ids

```yaml
# plan/a.md front matter
---
id: 7
---
```

```yaml
# plan/b.md front matter
---
id: 8
---
```

## See also

- [MDS020](../MDS020-required-structure/) — validates each
  file's front matter against a schema; this rule adds the
  cross-file uniqueness a per-file schema cannot express
- [MDS027](../MDS027-cross-file-reference-integrity/) — the
  include/exclude glob-scoping idiom this rule mirrors

## Meta-Information

- **ID**: MDS069
- **Name**: `unique-frontmatter`
- **Status**: ready
- **Default**: enabled
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: structural
