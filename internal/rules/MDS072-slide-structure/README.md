---
id: MDS072
name: slide-structure
status: ready
description: >-
  Flags Slidev slide-structure errors: unknown
  layouts, missing or orphaned slot separators,
  missing layout-required fields, and misspelled
  per-slide frontmatter keys.
nature: structure
maintainability: null
markdownlint: []
rumdl: []
mado: []
panache: []
obsidian-linter: []
gomarklint: []
category: structural
---
# MDS072: slide-structure

Flags Slidev slide-structure errors: unknown
layouts, missing or orphaned slot separators,
missing layout-required fields, and misspelled
per-slide frontmatter keys.

[Slidev](https://sli.dev) renders a single Markdown
file as a slide deck: `---` separates slides and
each slide may carry its own frontmatter block. Its
parser is permissive — an unmatched `::slot::`
drops its content, a misspelled `layout:` renders
blank, and an unknown frontmatter key passes
through as data. None of these error. MDS072 splits
the deck into slides and reports the failures
Slidev never does.

The rule is opt-in. Select the
[`slidev` convention](../../../docs/reference/conventions.md)
to enable it, or turn it on directly. It owns the
Markdown/structural layer only: it does not resolve
theme packages, render Vue, or validate UnoCSS
classes.

## Settings

| Key            | Type | Description                                           |
| -------------- | ---- | ----------------------------------------------------- |
| custom-layouts | list | Theme or addon layout names to treat as known layouts |

Declare a theme's layouts so they are not flagged
as unknown:

```yaml
rules:
  slide-structure:
    custom-layouts: [my-cover, quote-dark]
```

## Config

Enable:

```yaml
rules:
  slide-structure: true
```

Disable:

```yaml
rules:
  slide-structure: false
```

## Examples

### Bad

A `two-cols` layout with no `::right::` separator —
the right column renders empty.

<?include
file: bad/missing-right-slot.md
wrap: markdown
?>

```markdown
# Intro

Body text.

---
layout: two-cols
---

# Left column

Left body.
```

<?/include?>

### Good

A clean deck: blank-padded separators and unique,
well-formed headings.

<?include
file: good/clean-deck.md
wrap: markdown
?>

```markdown
# Opening

Welcome to the deck.

---

## Agenda

Three topics today.

---

## Summary

Thanks for watching.
```

<?/include?>

## Meta-Information

- **ID**: MDS072
- **Name**: `slide-structure`
- **Status**: ready
- **Default**: disabled
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: structural
