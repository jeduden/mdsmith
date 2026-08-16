---
id: MDS074
name: over-repetition
status: ready
description: >-
  A content word must not repeat more than `max` times within the
  configured scope unit. Counts prose only; fenced and indented code
  blocks are excluded.
category: prose
nature: content
maintainability: null
markdownlint: []
rumdl: []
mado: []
panache: []
obsidian-linter: []
gomarklint: []
---
# MDS074: over-repetition

A content word must not repeat more than `max` times within the
configured scope unit. Counts prose only; fenced and indented code
blocks are excluded.

The rule tokenizes prose into case-folded, punctuation-delimited words
and counts occurrences per scope unit (file, heading-bounded section, or
paragraph). Words shorter than `min-length` runes are excluded. Words in
the `stopwords` list (populated via `lists:`) are excluded before
counting.

A diagnostic is emitted at the scope unit's first line whenever any
surviving word's count exceeds `max`. The rule is opt-in and requires
`max` > 0 to fire.

## Settings

| Setting      | Type            | Default   | Description                                             |
| ------------ | --------------- | --------- | ------------------------------------------------------- |
| `scope`      | string          | `section` | `file`, `section`, or `paragraph`                       |
| `max`        | integer         | `-1`      | Maximum occurrences per scope unit; must be ≥ 1 to fire |
| `min-length` | integer         | `4`       | Ignore words shorter than N runes (by rune count)       |
| `stopwords`  | list of strings | `[]`      | Words never flagged. Populated by `lists:`.             |

## Config

Enable with a per-section ceiling of four repetitions:

```yaml
rules:
  over-repetition:
    enabled: true
    scope: section
    max: 4
```

Enable with a stopword list to exclude common domain terms:

```yaml
rules:
  over-repetition:
    enabled: true
    scope: section
    max: 4
    lists:
      - stopwords
```

Disable:

```yaml
rules:
  over-repetition: false
```

## Examples

### Good

<?include
file: good/default.md
wrap: markdown
?>

```markdown
# Introduction

This section explains how the system processes requests efficiently.
Each request goes through validation and then enters the queue.
Processing completes when all validations pass.

## Results

Results appear in the output after processing finishes.
```

<?/include?>

### Bad

<?include
file: bad/default.md
wrap: markdown
?>

```markdown
# Introduction

The process handles requests. Each process step validates input. The
process logs results after each process step, then the process ends.
```

<?/include?>

## Meta-Information

- **ID**: MDS074
- **Name**: `over-repetition`
- **Status**: ready
- **Default**: disabled
- **Fixable**: no
- **Implementation**: [source](./)
- **Category**: prose
