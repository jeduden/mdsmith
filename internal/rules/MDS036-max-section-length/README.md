---
id: MDS036
name: max-section-length
status: ready
description: Section length must not exceed per-level, per-heading, word, or paragraph limits.
nature: content
maintainability: null
---
# MDS036: max-section-length

Section length must not exceed per-level, per-heading, word, or paragraph
limits.

A section spans from its heading line up to (but not including) the next
heading line of any level, or the end of file. Nested subsections are
measured separately from their parent, so the limit applies to the
content written directly under each heading.

## Settings

| Setting          | Type | Default | Description                                                   |
|------------------|------|---------|---------------------------------------------------------------|
| `max`            | int  | 0       | Default line limit; zero disables the global line check.      |
| `per-level`      | map  | `{}`    | Map from heading level (1-6) to line limit.                   |
| `per-heading`    | list | `[]`    | Regex patterns with per-heading line limits.                  |
| `max-words`      | int  | 0       | Maximum word count for paragraphs directly under the heading. |
| `min-words`      | int  | 0       | Minimum word count for paragraphs directly under the heading. |
| `max-paragraphs` | int  | 0       | Maximum number of paragraphs directly under the heading.      |

Lookup order for the line limit: `per-heading` (first matching regex
wins), then `per-level`, then `max`. A resolved limit of zero disables
the check for that heading. The word and paragraph caps apply to every
section uniformly; zero disables each one.

Word and paragraph counts cover paragraph nodes whose start line falls
in the section's line range. Sub-section paragraphs belong to the
sub-section, not the parent.

## Config

Enable with a default limit:

```yaml
rules:
  max-section-length:
    max: 100
```

Per-level and per-heading overrides:

```yaml
rules:
  max-section-length:
    max: 100
    per-level:
      1: 200
      2: 80
    per-heading:
      - pattern: "^Changelog$"
        max: 500
```

Cap each section's prose by word count, require a minimum, and limit
paragraph counts:

```yaml
rules:
  max-section-length:
    max-words: 200
    min-words: 20
    max-paragraphs: 5
```

Disable:

```yaml
rules:
  max-section-length: false
```

## Examples

### Good

<?include
file: good/default.md
wrap: markdown
?>

```markdown
# Title

Short section within the limit.

## Subsection

Also short. Each section is bounded by the next heading of any level.

## Another

Stays under the limit.
```

<?/include?>

### Bad

<?include
file: bad/default.md
wrap: markdown
?>

```markdown
# Too Long

line a
line b
line c
line d
```

<?/include?>

## Meta-Information

- **ID**: MDS036
- **Name**: `max-section-length`
- **Status**: ready
- **Default**: disabled
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: heading
