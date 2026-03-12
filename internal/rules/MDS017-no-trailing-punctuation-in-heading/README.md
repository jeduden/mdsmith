---
id: MDS017
name: no-trailing-punctuation-in-heading
status: ready
description: Headings should not end with punctuation.
---
# MDS017: no-trailing-punctuation-in-heading

Headings should not end with punctuation.

- **ID**: MDS017
- **Name**: `no-trailing-punctuation-in-heading`
- **Status**: ready
- **Default**: enabled
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: heading

Flags headings that end with `.`, `,`, `:`, `;`, or `!`.

## Config

Enable:

```yaml
rules:
  no-trailing-punctuation-in-heading: true
```

Disable:

```yaml
rules:
  no-trailing-punctuation-in-heading: false
```

## Examples

### Bad

<?include
file: bad/default.md
wrap: markdown
?>

```markdown
# Title.

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

## Section

Body text.
```

<?/include?>
