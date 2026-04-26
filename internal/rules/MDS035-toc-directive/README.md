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

## Auto-fix

`mdsmith fix` replaces each detected token with
the canonical empty generated-section block:

```text
<?toc?>
<?/toc?>
```

[MDS038 (toc)](../MDS038-toc/README.md) then
runs in the same fix pass. It populates the
block with a nested heading list. A single
`mdsmith fix` converts a `[TOC]` source into
a populated `<?toc?>` block.

`[TOC]` is left untouched when a matching
`[TOC]: <url>` link reference definition
exists. In that case the token is a valid
CommonMark link, not a renderer directive.

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

## Meta-Information

- **ID**: MDS035
- **Name**: `toc-directive`
- **Status**: ready
- **Default**: disabled (opt-in)
- **Fixable**: yes
- **Implementation**:
  [source](./)
- **Category**: meta
