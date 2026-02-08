# TM008: no-multiple-blanks

No more than one consecutive blank line.

- **ID**: TM008
- **Name**: `no-multiple-blanks`
- **Default**: enabled
- **Fixable**: yes
- **Implementation**: [`internal/rules/nomultipleblanks/`](../../internal/rules/nomultipleblanks/)

## Config

```yaml
rules:
  no-multiple-blanks: true
```

## Examples

### Bad

```markdown
First paragraph.



Second paragraph.
```

### Good

```markdown
First paragraph.

Second paragraph.
```
