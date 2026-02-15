---
id: MDS029
name: conciseness-scoring
description: Paragraph conciseness score must not fall below a threshold.
---
# MDS029: conciseness-scoring

Paragraph conciseness score must not fall below a threshold.

- **ID**: MDS029
- **Name**: `conciseness-scoring`
- **Default**: enabled, min-score: 0.20, min-words: 20
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: meta

## Settings

| Setting         | Type         | Default  | Description                            |
|-----------------|--------------|----------|----------------------------------------|
| `min-score`       | number       | `0.20`     | Minimum allowed conciseness score      |
| `min-words`       | int          | `20`       | Skip paragraphs shorter than this      |
| `filler-words`    | list[string] | built-in | Words that reduce score when repeated  |
| `hedge-phrases`   | list[string] | built-in | Phrases that indicate weak assertions  |
| `verbose-phrases` | list[string] | built-in | Long phrases with shorter alternatives |

The score combines content-word ratio with penalties for filler words,
hedge phrases, and verbose phrases. Markdown tables are skipped.

## Config

```yaml
rules:
  conciseness-scoring:
    min-score: 0.20
    min-words: 20
    filler-words:
      - "actually"
      - "basically"
      - "just"
    hedge-phrases:
      - "it seems"
      - "i think"
    verbose-phrases:
      - "in order to"
      - "due to the fact that"
```

Disable:

```yaml
rules:
  conciseness-scoring: false
```

## Examples

### Good

```markdown
The deployment pipeline validates release notes, signs artifacts,
and uploads checksums so reviewers can verify each package quickly.
```

### Bad

```markdown
Basically, it seems that we are just trying to explain things
in order to make them very clear, and it appears that we are
really not adding many concrete details.
```

## Diagnostics

| Condition             | Message                                                                                                              |
|-----------------------|----------------------------------------------------------------------------------------------------------------------|
| score below threshold | `conciseness score too low (0.12 < 0.20); target >= 0.20; reduce filler or hedge cues (e.g., "basically", "it seems")` |
