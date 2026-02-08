# TM014: blank-line-around-lists

Lists must have a blank line before and after.

- **ID**: TM014
- **Name**: `blank-line-around-lists`
- **Default**: enabled
- **Fixable**: yes
- **Implementation**: [`internal/rules/blanklinearoundlists/`](../../internal/rules/blanklinearoundlists/)

## Config

```yaml
rules:
  blank-line-around-lists: true
```

## Examples

### Bad

```markdown
Some text.
- Item 1
- Item 2
More text.
```

### Good

```markdown
Some text.

- Item 1
- Item 2

More text.
```
