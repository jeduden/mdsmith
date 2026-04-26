---
id: MDS005
name: no-duplicate-headings
status: ready
description: No two headings should have the same text.
---
# MDS005: no-duplicate-headings

No two headings should have the same text.

## Config

Enable:

```yaml
rules:
  no-duplicate-headings: true
```

Disable:

```yaml
rules:
  no-duplicate-headings: false
```

## Examples

### Bad

<?include
file: bad/default.md
wrap: markdown
?>

```markdown
# Title

## Section

## Section
```

<?/include?>

### Good

<?include
file: good/default.md
wrap: markdown
?>

```markdown
# Title

## Section One

Body one.

## Section Two

Body two.
```

<?/include?>

## Meta-Information

- **ID**: MDS005
- **Name**: `no-duplicate-headings`
- **Status**: ready
- **Default**: enabled
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: heading
