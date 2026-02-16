---
id: MDS030
name: empty-section-body
status: ready
description: Section headings must include meaningful body content.
---
# MDS030: empty-section-body

Section headings must include meaningful body content.

- **ID**: MDS030
- **Name**: `empty-section-body`
- **Status**: ready
- **Default**: enabled, `min-level: 2`, `max-level: 6`
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: heading

This rule reports sections that only contain whitespace,
comments, or nested headings without any body content.

## Semantics

A section starts at a heading and ends at the next heading
of the same or higher level.

Within that range, meaningful content includes:

- paragraphs
- lists
- tables
- code blocks
- other non-comment HTML blocks

Ignored content includes:

- blank lines
- HTML comments
- nested headings by themselves

Use an explicit allow marker comment for intentional empty sections:

```markdown
<!-- allow-empty-section -->
```

## Prior Art

- `remark-lint-no-empty-sections` reports heading sections without content.
- `markdownlint`, Vale, and textlint do not provide a core rule with this
  exact section-empty semantic, so this rule adds explicit heading-scope
  behavior to mdsmith.

## Settings

| Setting      | Type   | Default             | Description                           |
|--------------|--------|---------------------|---------------------------------------|
| `min-level`    | int    | `2`                   | minimum heading level to check        |
| `max-level`    | int    | `6`                   | maximum heading level to check        |
| `allow-marker` | string | `allow-empty-section` | comment marker that exempts a section |

## Config

```yaml
rules:
  empty-section-body:
    min-level: 2
    max-level: 6
    allow-marker: allow-empty-section
```

Disable:

```yaml
rules:
  empty-section-body: false
```

## Examples

### Good

```markdown
## Overview

This section explains what the command does.
```

### Good -- intentional empty section

```markdown
## Compatibility

<!-- allow-empty-section -->
```

### Bad

```markdown
## Overview

<!-- TODO -->
```

## Diagnostics

- `section "## Heading" has no meaningful body content; add paragraph, list,
  table, or code content, or add "<!-- marker -->" for an intentional empty
  section`

## Edge Cases

- Parent headings are considered non-empty when nested subsections contain
  meaningful content.
- Heading-only nesting with no body content is reported.
- End-of-file sections are checked the same way as middle sections.
