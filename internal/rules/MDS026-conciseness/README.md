---
id: MDS026
name: conciseness
description: Paragraph conciseness score must not fall below a threshold.
---
# MDS026: conciseness

Paragraph conciseness score must not fall below a threshold.

- **ID**: MDS026
- **Name**: `conciseness`
- **Default**: enabled, min-score: 55.0, min-words: 20
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: meta

## Settings

| Setting               | Type         | Default  | Description                                   |
|-----------------------|--------------|----------|-----------------------------------------------|
| `min-score`             | float        | `55.0`     | Minimum allowed conciseness score             |
| `min-words`             | int          | `20`       | Minimum words before the rule evaluates       |
| `min-content-ratio`     | float        | `0.45`     | Minimum ratio of content-bearing words        |
| `filler-weight`         | float        | `1.0`      | Penalty multiplier for filler-word ratio      |
| `hedge-weight`          | float        | `0.8`      | Penalty multiplier for hedge-word ratio       |
| `verbose-phrase-weight` | float        | `4.0`      | Penalty multiplier for verbose phrase density |
| `content-weight`        | float        | `1.2`      | Penalty multiplier for low content ratio      |
| `filler-words`          | list[string] | built-in | Words treated as filler                       |
| `hedge-words`           | list[string] | built-in | Words treated as hedging                      |
| `verbose-phrases`       | list[string] | built-in | Multi-word verbose phrase patterns            |

The rule uses paragraph boundaries from Goldmark paragraph nodes.
Text tokens come from `mdtext.ExtractPlainText` and whitespace word splitting.
Markdown tables are skipped.

## Config

```yaml
rules:
  conciseness: true
```

Custom threshold and heuristic weights:

```yaml
rules:
  conciseness:
    min-score: 60.0
    min-words: 24
    min-content-ratio: 0.50
    filler-weight: 1.1
    hedge-weight: 0.9
    verbose-phrase-weight: 5.0
    content-weight: 1.3
```

Override by path:

```yaml
overrides:
  - files: ["guides/*.md"]
    rules:
      conciseness:
        min-score: 48.0
  - files: ["specs/*.md"]
    rules:
      conciseness:
        min-score: 62.0
        min-content-ratio: 0.55
```

Disable:

```yaml
rules:
  conciseness: false
```

## Examples

### Good

```markdown
Shard leaders persist monotonic commit indices, reject stale lease epochs,
and replicate snapshots with bounded retries across each region.
```

### Bad

```markdown
In order to make sure that we are all on the same page, it is important to
note that we might possibly make changes in most cases if the timeline shifts.
```

## Diagnostics

| Condition             | Message                                                                                               |
|-----------------------|-------------------------------------------------------------------------------------------------------|
| score below threshold | `paragraph conciseness score too low (42.3 < 55.0; filler 10.0%, hedge 8.0%, content 32.0%, phrases 2)` |
