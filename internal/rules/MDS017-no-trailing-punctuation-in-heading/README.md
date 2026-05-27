---
id: MDS017
name: no-trailing-punctuation-in-heading
status: ready
description: Headings should not end with punctuation.
category: heading
nature: content
maintainability: null
markdownlint:
  - id: MD026
    name: no-trailing-punctuation
    default: true
rumdl:
  - id: MD026
    name: no-trailing-punctuation
    default: true
mado:
  - id: MD026
    name: no-trailing-punctuation
    default: true
panache: []
---
# MDS017: no-trailing-punctuation-in-heading

Headings should not end with punctuation.

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

## Meta-Information

- **ID**: MDS017
- **Name**: `no-trailing-punctuation-in-heading`
- **Status**: ready
- **Default**: enabled
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: heading
- **Markdownlint**: [MD026][mdl-md026] (no-trailing-punctuation)

[mdl-md026]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md026.md
