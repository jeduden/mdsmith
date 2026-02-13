---
id: TM024
name: paragraph-structure
description: Paragraphs must not exceed sentence and word limits.
---
# TM024: paragraph-structure

Paragraphs must not exceed sentence and word limits.

- **ID**: TM024
- **Name**: `paragraph-structure`
- **Default**: enabled, max-sentences: 6, max-words: 40
- **Fixable**: no
- **Implementation**:
  [source](../../internal/rules/paragraphstructure/)
- **Category**: meta

## Settings

| Setting | Type | Default | Description |
|---------|------|---------|-------------|
| `max-sentences` | int | 6 | Maximum sentences per paragraph |
| `max-words` | int | 40 | Maximum words per sentence |

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

```markdown
The sun rose over the hills. Birds began to sing.
A gentle breeze swept through the valley.
```

### Bad

```markdown
The sun rose. Birds sang. Wind blew. Rain fell.
Snow came. Ice formed. Fog rolled in. Stars appeared.
```

## Diagnostics

| Condition | Message |
|-----------|---------|
| too many sentences | `paragraph has too many sentences (8 > 6)` |
| sentence too long | `sentence too long (45 > 40 words)` |
