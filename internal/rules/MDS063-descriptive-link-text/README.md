---
id: MDS063
name: descriptive-link-text
status: ready
description: >-
  Link text must be descriptive. Non-descriptive phrases like "click here",
  "here", "link", and "more" fail screen readers and link-list navigation.
category: prose
nature: style
maintainability: null
markdownlint:
  - id: MD059
    name: descriptive-link-text
---
# MDS063: descriptive-link-text

Link text must be descriptive. Non-descriptive phrases like "click here",
"here", "link", and "more" fail screen readers and link-list navigation.

The comparison is case- and whitespace-insensitive. Two link patterns are
exempt: links whose sole content is a single inline code span (an API
symbol), and links whose sole content is an image (a linked logo or badge
where the image itself carries the meaning).

## Config

Enable:

```yaml
rules:
  descriptive-link-text: true
```

Disable:

```yaml
rules:
  descriptive-link-text: false
```

Replace the default banned list:

```yaml
rules:
  descriptive-link-text:
    banned: ["read more", "learn more"]
```

## Examples

### Bad

<?include
file: bad/default.md
wrap: markdown
strip-frontmatter: "true"
?>

```markdown
# Title

[click here](https://example.com)
```

<?/include?>

### Good

<?include
file: good/default.md
wrap: markdown
?>

```markdown
# Title

Visit [the documentation](https://example.com) for more information.

See [`SomeAPI`](https://example.com/api) for details.

[![logo](logo.png)](https://example.com)
```

<?/include?>

## Meta-Information

- **ID**: MDS063
- **Name**: `descriptive-link-text`
- **Status**: ready
- **Default**: disabled (opt-in)
- **Fixable**: no
- **Implementation**:
  [source](../descriptivelinktext/)
- **Category**: prose
- **Markdownlint**: [MD059][mdl-md059] (descriptive-link-text)

[mdl-md059]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md059.md
