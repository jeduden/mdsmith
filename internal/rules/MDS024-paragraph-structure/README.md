---
id: MDS024
name: paragraph-structure
status: ready
description: Paragraphs must not exceed sentence and word limits.
---
# MDS024: paragraph-structure

Paragraphs must not exceed sentence and word limits.

- **ID**: MDS024
- **Name**: `paragraph-structure`
- **Status**: ready
- **Default**: enabled, max-sentences: 6, max-words: 40
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: meta

## Settings

| Setting         | Type | Default | Description                     |
|-----------------|------|---------|---------------------------------|
| `max-sentences` | int  | 6       | Maximum sentences per paragraph |
| `max-words`     | int  | 40      | Maximum words per sentence      |

Markdown tables and code blocks are skipped.

## Config

```yaml
rules:
  paragraph-structure:
    max-sentences: 6
    max-words: 40
```

Disable:

```yaml
rules:
  paragraph-structure: false
```

## Examples

### Good

<?include
file: good.md
wrap: markdown
?>

```markdown
# Well Structured Document

The sun rose over the hills. Birds began to sing.
A gentle breeze swept through the valley.
```

<?/include?>

### Bad

<?include
file: bad.md
wrap: markdown
?>

```markdown
# Overly Long Paragraph

Dogs bark. Cats meow. Birds sing. Fish swim. Frogs croak. Snakes hiss. Bees buzz. Ants march.
```

<?/include?>

## Diagnostics

| Condition          | Message                                    |
|--------------------|--------------------------------------------|
| too many sentences | `paragraph has too many sentences (8 > 6)` |
| sentence too long  | `sentence too long (45 > 40 words)`        |
