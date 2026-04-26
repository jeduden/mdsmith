---
id: MDS009
name: single-trailing-newline
status: ready
description: File must end with exactly one newline character.
---
# MDS009: single-trailing-newline

File must end with exactly one newline character.

## Config

Enable:

```yaml
rules:
  single-trailing-newline: true
```

Disable:

```yaml
rules:
  single-trailing-newline: false
```

## Examples

### Good

<?include
file: good/default.md
wrap: markdown
?>

```markdown
# Title

Content here.
```

<?/include?>

The file ends with exactly one `\n` after the last line.

### Bad

<?include
file: bad/default.md
wrap: markdown
?>

```markdown
# Title

Content here.
```

<?/include?>

The file has no trailing newline after the last line.
The `<?include?>` output looks identical because the
wrap always adds a newline, but the actual fixture file
(`bad/default.md`) ends without one — that missing byte
is what the rule detects.

## Meta-Information

- **ID**: MDS009
- **Name**: `single-trailing-newline`
- **Status**: ready
- **Default**: enabled
- **Fixable**: yes
- **Implementation**:
  [source](./)
- **Category**: whitespace
