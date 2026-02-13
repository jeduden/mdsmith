---
id: TM023
name: paragraph-readability
description: Paragraph readability grade must not exceed a threshold.
---
# TM023: paragraph-readability

Paragraph readability grade must not exceed a threshold.

- **ID**: TM023
- **Name**: `paragraph-readability`
- **Default**: enabled, max-grade: 14.0, min-words: 20
- **Fixable**: no
- **Implementation**:
  [source](../../internal/rules/paragraphreadability/)
- **Category**: meta

## Settings

| Setting | Type | Default | Description |
|---------|------|---------|-------------|
| `max-grade` | float | 14.0 | Maximum allowed ARI readability grade |
| `min-words` | int | 20 | Minimum word count to check a paragraph |

Paragraphs with fewer words than `min-words` are skipped.
The Automated Readability Index (ARI) maps to US grade levels:
a score of 14.0 roughly corresponds to college-level text.

## Config

```yaml
rules:
  paragraph-readability: true
```

Custom thresholds:

```yaml
rules:
  paragraph-readability:
    max-grade: 12.0
    min-words: 25
```

Disable:

```yaml
rules:
  paragraph-readability: false
```

## Examples

### Good

```markdown
The cat sat on the mat and the dog lay on the rug.
They were both very happy to be at home on a warm day.
```

### Bad

```markdown
The implementation of concurrent distributed systems
requires sophisticated understanding of fundamental
computational paradigms and synchronization mechanisms
that must guarantee linearizability across heterogeneous
processing environments and architectural configurations.
```
