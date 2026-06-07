---
id: MDS012
name: no-bare-urls
status: ready
description: URLs must be wrapped in angle brackets or as a link, not left bare.
category: link
nature: content
maintainability: null
markdownlint:
  - id: MD034
    name: no-bare-urls
    partial: false
    default: true
rumdl:
  - id: MD034
    name: no-bare-urls
    partial: false
    default: true
mado:
  - id: MD034
    name: no-bare-urls
    partial: false
    default: true
panache: []
obsidian-linter:
  - id: no-bare-urls
    name: no-bare-urls
    partial: false
    default: false
---
# MDS012: no-bare-urls

URLs must be wrapped in angle brackets or as a link, not left bare.

## Config

Enable:

```yaml
rules:
  no-bare-urls: true
```

Disable:

```yaml
rules:
  no-bare-urls: false
```

## Examples

### Bad

<?include
file: bad/default.md
wrap: markdown
?>

```markdown
# Title

Visit https://example.com for more.
```

<?/include?>

### Good

<?include
file: good/default.md
wrap: markdown
?>

```markdown
# Title

Visit [example](https://example.com) for more.
```

<?/include?>

## Meta-Information

- **ID**: MDS012
- **Name**: `no-bare-urls`
- **Status**: ready
- **Default**: enabled
- **Fixable**: yes
- **Implementation**:
  [source](./)
- **Category**: link
- **markdownlint**: [MD034][mdl-md034] (no-bare-urls)
- **rumdl**: [MD034][rumdl-md034] (no-bare-urls)
- **mado**: [MD034][mado-rules] (no-bare-urls)
- **obsidian-linter**: [no-bare-urls]

[mdl-md034]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md034.md
[rumdl-md034]: https://rumdl.dev/md034/
[mado-rules]: https://github.com/akiomik/mado#supported-rules
[no-bare-urls]:
  https://platers.github.io/obsidian-linter/settings/content-rules/#no-bare-urls
