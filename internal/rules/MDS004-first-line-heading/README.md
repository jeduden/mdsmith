---
id: MDS004
name: first-line-heading
status: ready
description: First line of the file should be a heading.
---
# MDS004: first-line-heading

First line of the file should be a heading.

- **ID**: MDS004
- **Name**: `first-line-heading`
- **Status**: ready
- **Default**: enabled, level: 1
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: heading

## Settings

| Setting | Type | Default | Description                               |
|---------|------|---------|-------------------------------------------|
| `level`   | int  | 1       | Required heading level for the first line |

## Config

Enable (default):

```yaml
rules:
  first-line-heading:
    level: 1
```

Disable:

```yaml
rules:
  first-line-heading: false
```

Custom (require level 2):

```yaml
rules:
  first-line-heading:
    level: 2
```

## Examples

### Bad

<?include
file: bad/default.md
wrap: markdown
?>

```markdown
Some content here.

# Title
```

<?/include?>

### Good

<?include
file: good/default.md
wrap: markdown
?>

```markdown
# Title

Some content here.
```

<?/include?>

## Diagnostics

| Message                                        | Condition                                   |
|------------------------------------------------|---------------------------------------------|
| `first line should be a level {level} heading`   | Line 1 is missing or not a heading          |
| `first heading should be level {level}, got {n}` | First heading on line 1 has the wrong level |
