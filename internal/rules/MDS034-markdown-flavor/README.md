---
id: MDS034
name: markdown-flavor
status: ready
description: >-
  Flags Markdown syntax that the declared target
  flavor does not render.
---
# MDS034: markdown-flavor

Flags Markdown syntax that the declared target
flavor does not render.

- **ID**: MDS034
- **Name**: `markdown-flavor`
- **Status**: ready
- **Default**: disabled
- **Fixable**: no (fix pipeline lands in a follow-up)
- **Implementation**:
  [source](./)
- **Category**: meta

## Settings

| Key    | Type   | Description                                       |
|--------|--------|---------------------------------------------------|
| flavor | string | Target flavor: `commonmark`, `gfm`, or `goldmark` |

The flavor name is case-sensitive. The `goldmark`
profile is mdsmith-defined. It accepts GFM features
plus heading IDs. It does not accept optional
footnote, definition-list, or math extensions.

## Config

Enable with a target flavor:

```yaml
rules:
  markdown-flavor:
    flavor: gfm
```

Disable (default):

```yaml
rules:
  markdown-flavor: false
```

## Detected features

MDS034 tracks twelve syntax features whose
support varies across Markdown flavors.

Eleven features are detected from the goldmark AST
of a dual parse. That parse enables five built-in
extensions: table, strikethrough, task list,
footnote, and definition list. It also enables the
heading-ID attribute parser. Five custom parsers
add superscript, subscript, math block, inline
math, and abbreviations.

Bare-URL autolinks are detected separately. The
detector scans text nodes from the main parse for
URL-shaped text. It skips links, autolinks, code
spans, and code blocks.

| Feature            | commonmark | gfm | goldmark |
|--------------------|------------|-----|----------|
| tables             | no         | yes | yes      |
| task lists         | no         | yes | yes      |
| strikethrough      | no         | yes | yes      |
| bare-URL autolinks | no         | yes | yes      |
| footnotes          | no         | no  | no       |
| definition lists   | no         | no  | no       |
| heading IDs        | no         | no  | yes      |
| superscript        | no         | no  | no       |
| subscript          | no         | no  | no       |
| math blocks        | no         | no  | no       |
| inline math        | no         | no  | no       |
| abbreviations      | no         | no  | no       |

## Examples

### Good

<?include
file: good/gfm.md
wrap: markdown
?>

```markdown
# Heading

Text with ~~old~~ markup and a task list:

- [x] done
- [ ] todo
```

<?/include?>

### Bad

<?include
file: bad/commonmark-table.md
wrap: markdown
?>

```markdown
# Heading

| a | b |
| - | - |
| 1 | 2 |
```

<?/include?>
