---
id: MDS059
name: blockquote-whitespace
status: ready
description: >-
  Blockquote markers must not be followed by multiple spaces,
  and adjacent blockquote blocks must not be separated by blank lines.
category: whitespace
nature: style
maintainability: null
markdownlint:
  - id: MD027
    name: no-multiple-space-blockquote
    partial: false
    default: true
  - id: MD028
    name: no-blanks-blockquote
    partial: false
    default: true
rumdl:
  - id: MD027
    name: multiple-spaces-blockquote
    partial: false
    default: true
  - id: MD028
    name: blanks-blockquote
    partial: false
    default: true
mado:
  - id: MD027
    name: no-multiple-space-blockquote
    partial: false
    default: false
  - id: MD028
    name: no-blanks-blockquote
    partial: false
    default: true
panache: []
obsidian-linter:
  - id: blockquote-style
    name: blockquote-style
    partial: true
    default: false
  - id: empty-line-around-blockquotes
    name: empty-line-around-blockquotes
    partial: true
    default: false
gomarklint: []
---
# MDS059: blockquote-whitespace

Blockquote markers must not be followed by multiple spaces, and adjacent
blockquote blocks must not be separated by blank lines.

Two defects are flagged:

- **MD027** — more than one space after a `>` marker (`>  text`).
  The fix collapses the extra spaces to a single space.
- **MD028** — a blank line between two adjacent sibling blockquote
  blocks. Renderers disagree on whether the gap merges the blocks or
  keeps them separate, so the fix is flag-only.

## Config

Enable (default):

```yaml
rules:
  blockquote-whitespace: true
```

Disable:

```yaml
rules:
  blockquote-whitespace: false
```

## Examples

### Good

<?include
file: good/default.md
wrap: markdown
?>

```markdown
# Title

> This blockquote has one space after the marker.

Some text between the blockquotes.

> Another blockquote, separated by a paragraph.
```

<?/include?>

### Good — internal blank via marker

<?include
file: good/internal-blank.md
wrap: markdown
?>

```markdown
# Title

> First paragraph in the blockquote.
>
> Second paragraph in the same blockquote.
```

<?/include?>

### Bad — multiple spaces (MD027)

<?include
file: bad/multi-space.md
wrap: markdown
?>

```markdown
# Title

>  Two spaces after the blockquote marker.
```

<?/include?>

### Bad — blank line between blockquotes (MD028)

<?include
file: bad/blank-between.md
wrap: markdown
?>

```markdown
# Title

> First blockquote.

> Second blockquote.
```

<?/include?>

## Meta-Information

- **ID**: MDS059
- **Name**: `blockquote-whitespace`
- **Status**: ready
- **Default**: enabled
- **Fixable**: MD027 yes; MD028 no
- **Implementation**:
  [source](./)
- **Category**: whitespace
- **markdownlint**:
  - [MD027][mdl-md027] (no-multiple-space-blockquote)
  - [MD028][mdl-md028] (no-blanks-blockquote)
- **rumdl**:
  - [MD027][rumdl-md027] (multiple-spaces-blockquote)
  - [MD028][rumdl-md028] (blanks-blockquote)
- **mado**:
  - [MD027][mado-rules] (no-multiple-space-blockquote)
  - [MD028][mado-rules] (no-blanks-blockquote)
- **obsidian-linter**:
  - [blockquote-style] (partial)
  - [empty-line-around-blockquotes] (partial)

[mdl-md027]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md027.md
[mdl-md028]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md028.md
[rumdl-md027]: https://rumdl.dev/md027/
[rumdl-md028]: https://rumdl.dev/md028/
[mado-rules]: https://github.com/akiomik/mado#supported-rules
[blockquote-style]:
  https://platers.github.io/obsidian-linter/settings/content-rules/#blockquote-style
[empty-line-around-blockquotes]:
  https://platers.github.io/obsidian-linter/settings/spacing-rules/#empty-line-around-blockquotes
