---
id: MDS041
name: no-inline-html
status: ready
description: >-
  Flags raw HTML in Markdown (block and inline).
  Tags in the allow list and HTML comments with
  allow-comments enabled are skipped.
---
# MDS041: no-inline-html

Flags raw HTML in Markdown (block and inline).
Tags in the allow list and HTML comments with
allow-comments enabled are skipped.

The rule walks `*ast.HTMLBlock` and `*ast.RawHTML`
nodes. It skips mdsmith directives, autolinks, fenced
code blocks, and inline code spans.

## Settings

| Key            | Type         | Default | Description                                      |
|----------------|--------------|---------|--------------------------------------------------|
| allow          | list(string) | `[]`    | Tag names that are permitted (case-insensitive). |
| allow-comments | bool         | `true`  | Whether `<!-- ... -->` comments are allowed.     |

`allow` uses replace-mode merge: a later config layer
replaces the list wholesale. Teams that want to extend
the inherited list must restate all tags.

## Config

Enable with defaults (empty allowlist, comments
allowed):

```yaml
rules:
  no-inline-html: true
```

Enable and permit specific tags:

```yaml
rules:
  no-inline-html:
    allow: [kbd, sub, sup, details, summary]
    allow-comments: true
```

Flag HTML comments too:

```yaml
rules:
  no-inline-html:
    allow: []
    allow-comments: false
```

Disable (default):

```yaml
rules:
  no-inline-html: false
```

## Examples

### Bad — block HTML

<?include
file: bad/block-html.md
wrap: markdown
?>

```markdown
# Title

<div>block-level HTML content</div>
```

<?/include?>

### Bad — inline HTML

<?include
file: bad/inline-html.md
wrap: markdown
?>

```markdown
# Title

text <span>marked</span> text
```

<?/include?>

### Bad — self-closing tag

<?include
file: bad/self-closing.md
wrap: markdown
?>

```markdown
# Title

line break<br/>
```

<?/include?>

### Good — allowed tags

<?include
file: good/allowed-tags.md
wrap: markdown
?>

```markdown
# Document with Allowed Tags

The <kbd>Ctrl</kbd>+<kbd>C</kbd> shortcut copies.

Chemical formula: H<sub>2</sub>O.

Superscript: x<sup>2</sup> + y<sup>2</sup>.
```

<?/include?>

### Good — code blocks

<?include
file: good/code-blocks.md
wrap: markdown
?>

````markdown
# Code Blocks

HTML inside fenced code blocks is not flagged.

```html
<div class="container">
  <span>content</span>
</div>
```

Inline code like `<br>` is also safe.
````

<?/include?>

## What is not flagged

- Fenced and indented code blocks (`*ast.FencedCodeBlock`
  / `*ast.CodeBlock`).
- Inline code spans (`*ast.CodeSpan`).
- Autolinks such as `<https://example.com>` and email
  autolinks — these are `*ast.AutoLink` nodes.
- mdsmith directives (`<?name ... ?>`): block forms are
  `ProcessingInstruction` nodes; inline forms are skipped
  because their raw bytes start with `<?`.
- Closing tags (`</tag>`): the opening tag already
  produces a diagnostic.

## Meta-Information

- **ID**: MDS041
- **Name**: `no-inline-html`
- **Status**: ready
- **Default**: disabled (opt-in)
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: meta
