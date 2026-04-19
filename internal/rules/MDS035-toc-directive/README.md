---
id: MDS035
name: toc-directive
status: ready
description: Flag renderer-specific TOC directives that render as literal text on CommonMark and goldmark.
---
# MDS035: toc-directive

Flag renderer-specific TOC directives that
render as literal text on CommonMark and
goldmark.

- **ID**: MDS035
- **Name**: `toc-directive`
- **Status**: ready
- **Default**: disabled (opt-in)
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: meta

## What it detects

Four directive variants appear in the wild,
each expanded by a different renderer:

| Token       | Expanded by                            |
|-------------|----------------------------------------|
| `[TOC]`     | Python-Markdown, MultiMarkdown, Pandoc |
| `[[_TOC_]]` | GitLab Flavored Markdown, Azure DevOps |
| `[[toc]]`   | markdown-it-toc-done-right, VitePress  |
| `${toc}`    | some VitePress configurations          |

CommonMark and goldmark do not expand any of
them; authors see the directive token in the
rendered output instead of a table of
contents. The rule flags each token when it
appears on its own line inside a paragraph so
authors can replace it, delete it, or maintain
the list manually.

`[TOC]` alone is also valid CommonMark
shortcut reference link syntax. When the
document contains a matching
`[TOC]: <url>` definition, the rule
suppresses the diagnostic because the token
resolves to a real link rather than rendering
as literal text.

## Why not auto-fix

The right replacement depends on intent. For
file-index usage — an index page listing
sibling documents — mdsmith's
[`<?catalog?>`](../MDS019-catalog/README.md)
directive is the replacement. For in-document
heading TOCs, mdsmith has no equivalent; the
author must drop the directive or maintain a
manual list. The rule is detection-only and
names both cases in its message.

## Config

Enable:

```yaml
rules:
  toc-directive: true
```

Disable (default):

```yaml
rules:
  toc-directive: false
```

## Examples

### Good

<?include
file: good/default.md
wrap: markdown
?>

````markdown
# Document with no TOC directives

This document has no renderer-specific TOC
markers, so MDS035 stays silent.

Normal prose is unaffected, and inline code
like `[TOC]` or `${toc}` is not flagged because
the tokens are inside code spans.

```text
[TOC]
[[_TOC_]]
[[toc]]
${toc}
```

Even the fenced block above is a code block,
not a paragraph, so nothing is reported.
````

<?/include?>

### Bad

<?include
file: bad/bracketed.md
wrap: markdown
?>

```markdown
# Python-Markdown TOC directive

[TOC]

Everything below the directive renders fine,
but the directive itself appears as literal
text on CommonMark and goldmark renderers.
```

<?/include?>
