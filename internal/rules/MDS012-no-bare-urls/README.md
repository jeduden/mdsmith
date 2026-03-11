---
id: MDS012
name: no-bare-urls
status: ready
description: URLs must be wrapped in angle brackets or as a link, not left bare.
---
# MDS012: no-bare-urls

URLs must be wrapped in angle brackets or as a link, not left bare.

- **ID**: MDS012
- **Name**: `no-bare-urls`
- **Status**: ready
- **Default**: enabled
- **Fixable**: yes
- **Implementation**:
  [source](./)
- **Category**: link

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
