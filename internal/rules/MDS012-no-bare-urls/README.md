---
id: MDS012
name: no-bare-urls
description: URLs must be wrapped in angle brackets or as a link, not left bare.
---
# MDS012: no-bare-urls

URLs must be wrapped in angle brackets or as a link, not left bare.

- **ID**: MDS012
- **Name**: `no-bare-urls`
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

```markdown
Visit https://example.com for more info.
```

### Good

```markdown
Visit <https://example.com> for more info.

Visit [example](https://example.com) for more info.
```
