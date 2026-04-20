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
- **Fixable**: yes
- **Implementation**:
  [source](./)
- **Category**: meta

## Settings

| Key    | Type   | Description                     |
|--------|--------|---------------------------------|
| flavor | string | Target flavor; see table below. |

The flavor name is case-sensitive. Supported
values:

- `commonmark` — strict CommonMark; rejects every
  tracked feature.
- `gfm` — GitHub Flavored Markdown; adds tables,
  task lists, strikethrough, and bare-URL
  autolinks.
- `goldmark` — mdsmith-defined profile; GFM plus
  heading IDs.
- `pandoc` — Pandoc's default markdown; GFM plus
  footnotes, definition lists, heading IDs,
  superscript, subscript, math block, and inline
  math. Rejects abbreviations (non-default
  extension).
- `phpextra` — PHP Markdown Extra; tables,
  footnotes, definition lists, heading IDs, and
  abbreviations. Rejects GFM features and math.
- `multimarkdown` — MultiMarkdown; PHP Extra plus
  math block and inline math.
- `myst` — MyST (Sphinx documentation flavor);
  tables, strikethrough, footnotes, definition
  lists, heading IDs, math block, and inline math.
- `any` — accepts every tracked feature. Use when
  the target renderer is unknown or permissive and
  you want to silence flavor diagnostics without
  disabling the rule.

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

MDS034 tracks thirteen syntax features whose
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

GitHub Alerts are detected from the CommonMark
AST. The detector matches the five GFM tokens
(`NOTE`, `TIP`, `IMPORTANT`, `WARNING`,
`CAUTION`) on the first line of a blockquote
paragraph. Matching is case-sensitive per the
GFM spec.

`flavor: any` accepts every feature and is omitted
from the table below.

| Feature            | commonmark | gfm | goldmark | pandoc | phpextra | multimarkdown | myst |
|--------------------|------------|-----|----------|--------|----------|---------------|------|
| tables             | no         | yes | yes      | yes    | yes      | yes           | yes  |
| task lists         | no         | yes | yes      | yes    | no       | no            | no   |
| strikethrough      | no         | yes | yes      | yes    | no       | no            | yes  |
| bare-URL autolinks | no         | yes | yes      | yes    | no       | no            | no   |
| footnotes          | no         | no  | no       | yes    | yes      | yes           | yes  |
| definition lists   | no         | no  | no       | yes    | yes      | yes           | yes  |
| heading IDs        | no         | no  | yes      | yes    | yes      | yes           | yes  |
| superscript        | no         | no  | no       | yes    | no       | no            | no   |
| subscript          | no         | no  | no       | yes    | no       | no            | no   |
| math blocks        | no         | no  | no       | yes    | no       | yes           | yes  |
| inline math        | no         | no  | no       | yes    | no       | yes           | yes  |
| abbreviations      | no         | no  | no       | no     | yes      | yes           | no   |
| github alerts      | no         | yes | no       | no     | no       | no            | no   |

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
