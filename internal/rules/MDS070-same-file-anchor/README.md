---
id: MDS070
name: same-file-anchor
status: ready
description: >-
  Every same-file #fragment link must resolve to a heading present in the
  same file.
category: link
nature: structure
maintainability: null
markdownlint:
  - id: MD051
    name: link-fragments
    partial: false
    default: true
rumdl:
  - id: MD051
    name: link-fragments
    partial: false
    default: true
mado: []
panache: []
obsidian-linter: []
gomarklint:
  - id: link-fragments
    name: link-fragments
    partial: false
    default: true
---
# MDS070: same-file-anchor

Every same-file #fragment link must resolve to a heading present in the same file.

A same-file fragment link has a destination beginning with `#` and no path
component — for example `[text](#my-heading)`.

The rule computes GitHub-flavored Markdown heading slugs: lowercase, spaces
become `-`, and non-alphanumeric characters except `-` are removed. Any
fragment that does not match a slug in the file is reported.

This rule is parse-skip-safe: it does not require a goldmark AST. It works
on the Layer 0 block-span projection and the shared inline-block parser.

## Config

Enable (default):

```yaml
rules:
  same-file-anchor: true
```

Disable:

```yaml
rules:
  same-file-anchor: false
```

## Examples

### Bad

```markdown
# My Heading

See [link](#nonexistent-section).
```

Reports: `same-file anchor #nonexistent-section does not match any heading in this file`

### Good

```markdown
# My Heading

See [link](#my-heading).

## Another Section

See [another link](#another-section).
```

## See also

- [MDS027](../MDS027-cross-file-reference-integrity/) — cross-file link and
  anchor resolution; this rule handles only same-file `#fragment` links

## Meta-Information

- **ID**: MDS070
- **Name**: `same-file-anchor`
- **Status**: ready
- **Default**: enabled
- **Fixable**: no
- **Implementation**: [source](./)
- **Category**: link
- **markdownlint**: [MD051][mdl-md051] (link-fragments)
- **rumdl**: [MD051][rumdl-md051] (link-fragments)
- **gomarklint**: [link-fragments][gomarklint-rules]

[mdl-md051]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md051.md
[rumdl-md051]: https://rumdl.dev/md051/
[gomarklint-rules]: https://shinagawa-web.github.io/gomarklint/docs/rules/
