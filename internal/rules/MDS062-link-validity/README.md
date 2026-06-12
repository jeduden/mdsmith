---
id: MDS062
name: link-validity
status: ready
description: >-
  Links must not use the reversed `(text)[url]` form, and every link
  or image must have a non-empty destination; a link must also have
  visible text.
nature: structure
category: link
maintainability: null
markdownlint:
  - id: MD011
    name: no-reversed-links
    partial: false
    default: true
  - id: MD042
    name: no-empty-links
    partial: false
    default: true
rumdl:
  - id: MD011
    name: reversed-link
    partial: false
    default: true
  - id: MD042
    name: no-empty-links
    partial: false
    default: true
mado: []
panache: []
obsidian-linter: []
gomarklint:
  - id: no-empty-links
    name: no-empty-links
    partial: false
    default: true
---
# MDS062: link-validity

Links must not use the reversed `(text)[url]` form, and every link
or image must have a non-empty destination; a link must also have
visible text.

This rule closes the markdownlint MD011 (`no-reversed-links`) and
MD042 (`no-empty-links`) gap. Both shapes are correctness defects —
the link silently does not work — so the rule is enabled by default.

- **Reversed syntax** `(text)[url]`: CommonMark renders this as
  literal text, so the intended link is lost. `mdsmith fix`
  rewrites it to `[text](url)`.
- **Empty destination** `[text]()`, `[text](<>)`, `[text](#)`, or a
  whitespace-only target: the link goes nowhere. The same check
  applies to images (`![alt]()`).
- **Empty link text** `[](url)`: the link has nothing to click.
  An image-only link such as `[![logo](logo.png)](url)` is not
  empty — the image is its visible content.

A reversed pattern inside a code span, a code block, or a
directive-generated body is left alone. An empty image destination
is reported here; empty image *alt text* with a valid destination
is MDS032's concern, not this rule's.

## Config

Enabled by default:

```yaml
rules:
  link-validity: true
```

Disable:

```yaml
rules:
  link-validity: false
```

## Examples

### Bad -- reversed link

<?include
file: bad/reversed-link.md
wrap: markdown
strip-frontmatter: "true"
?>

```markdown
# Reversed link

(example)[https://example.com] points the wrong way.
```

<?/include?>

### Bad -- empty destination

<?include
file: bad/empty-target.md
wrap: markdown
strip-frontmatter: "true"
?>

```markdown
# Empty target

The [docs link]() has no destination.
```

<?/include?>

### Bad -- empty link text

<?include
file: bad/empty-text.md
wrap: markdown
strip-frontmatter: "true"
?>

```markdown
# Empty text

A link with [](https://example.com) no text is broken.
```

<?/include?>

### Good -- valid links

<?include
file: good/normal-and-autolink.md
wrap: markdown
?>

```markdown
# Valid links

A [normal link](https://example.com) works as written.

The autolink <https://example.com> is also fine.
```

<?/include?>

### Good -- reversed shape shown as code

<?include
file: good/code-span.md
wrap: markdown
?>

```markdown
# Code span

The reversed form `(text)[url]` is shown as code, not as a link.
```

<?/include?>

## Diagnostics

| Condition               | Message                                                 |
| ----------------------- | ------------------------------------------------------- |
| reversed `(text)[url]`  | `reversed link: use [text](url) instead of (text)[url]` |
| empty link destination  | `empty link destination`                                |
| empty link text         | `empty link text`                                       |
| empty image destination | `empty image destination`                               |

## Meta-Information

- **ID**: MDS062
- **Name**: `link-validity`
- **Status**: ready
- **Default**: enabled
- **Fixable**: reversed links only
- **Implementation**:
  [source](../linkvalidity/)
- **Category**: link
- **markdownlint**:
  - [MD011][mdl-md011] (no-reversed-links)
  - [MD042][mdl-md042] (no-empty-links)
- **rumdl**:
  - [MD011][rumdl-md011] (reversed-link)
  - [MD042][rumdl-md042] (no-empty-links)
- **gomarklint**: [no-empty-links][gomarklint-rules]

[mdl-md011]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md011.md
[mdl-md042]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md042.md
[rumdl-md011]: https://rumdl.dev/md011/
[rumdl-md042]: https://rumdl.dev/md042/
[gomarklint-rules]: https://shinagawa-web.github.io/gomarklint/docs/rules/
