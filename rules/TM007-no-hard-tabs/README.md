---
id: TM007
name: no-hard-tabs
description: No tab characters. Use spaces instead.
---
# TM007: no-hard-tabs

No tab characters. Use spaces instead.

- **ID**: TM007
- **Name**: `no-hard-tabs`
- **Default**: enabled
- **Fixable**: yes
- **Implementation**:
  [source](../../internal/rules/nohardtabs/)
- **Category**: whitespace

## Config

```yaml
rules:
  no-hard-tabs: true
```

## Examples

### Bad

```markdown
→Indented with a tab.
```

### Good

```markdown
  Indented with spaces.
```
