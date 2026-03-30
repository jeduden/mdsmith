---
id: MDS023
name: paragraph-readability
status: ready
description: Paragraph readability index must not exceed a threshold.
---
# MDS023: paragraph-readability

Paragraph readability index must not exceed a threshold.

- **ID**: MDS023
- **Name**: `paragraph-readability`
- **Status**: ready
- **Default**: enabled, max-index: 14.0, min-words: 20
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: meta

## Settings

| Setting     | Type  | Default | Description                             |
|-------------|-------|---------|-----------------------------------------|
| `max-index` | float | 14.0    | Maximum allowed readability index (ARI) |
| `min-words` | int   | 20      | Minimum word count to check a paragraph |

Paragraphs with fewer words than `min-words` are skipped.
Markdown tables and code blocks are skipped.
The Automated Readability Index (ARI) maps to US grade levels:
a score of 14.0 roughly corresponds to college-level text.

> `max-grade` is accepted as a deprecated alias for `max-index`.
> Update configurations to use `max-index` instead.

## Config

```yaml
rules:
  paragraph-readability: true
```

Custom thresholds:

```yaml
rules:
  paragraph-readability:
    max-index: 12.0
    min-words: 25
```

Disable:

```yaml
rules:
  paragraph-readability: false
```

## Examples

### Good

<?include
file: good.md
wrap: markdown
?>

```markdown
# Simple Document

The cat sat on the mat and the dog lay on the rug and they
were both very happy to be at home on a warm and sunny day
in the middle of the summer.
```

<?/include?>

### Bad

<?include
file: bad.md
wrap: markdown
?>

```markdown
# Complex Document

The implementation of concurrent distributed systems requires sophisticated understanding of fundamental computational paradigms and synchronization mechanisms that must guarantee linearizability across heterogeneous processing environments and architectural configurations.
```

<?/include?>
