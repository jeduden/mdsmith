---
id: MDS049
name: no-space-in-link-text
status: ready
description: Link text and image alt text must not have leading or trailing whitespace inside the brackets.
nature: style
maintainability: null
---
# MDS049: no-space-in-link-text

Link text and image alt text must not have leading or trailing whitespace
inside the brackets.

`[ click here ](url)` renders the spaces as part of the underlined text
in most renderers.

## Config

Enable:

```yaml
rules:
  no-space-in-link-text: true
```

Disable:

```yaml
rules:
  no-space-in-link-text: false
```

Disable image alt-text checking:

```yaml
rules:
  no-space-in-link-text:
    check-images: false
```

## Examples

### Bad

<?include
file: bad/leading-and-trailing.md
wrap: markdown
strip-frontmatter: "true"
?>

```markdown
# Title

[ text ](https://example.com)
```

<?/include?>

### Good

<?include
file: good/default.md
wrap: markdown
?>

```markdown
# Title

Visit [example](https://example.com) for more.

![logo](good/logo.png)
```

<?/include?>

## Meta-Information

- **ID**: MDS049
- **Name**: `no-space-in-link-text`
- **Status**: ready
- **Default**: disabled (opt-in)
- **Fixable**: yes
- **Implementation**:
  [source](./)
- **Category**: link
