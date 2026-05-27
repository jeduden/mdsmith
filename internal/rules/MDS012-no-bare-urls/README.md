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
    default: true
rumdl:
  - id: MD034
    name: no-bare-urls
    default: true
mado:
  - id: MD034
    name: no-bare-urls
    default: true
panache: []
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
- **Markdownlint**: [MD034][mdl-md034] (no-bare-urls)

[mdl-md034]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md034.md
