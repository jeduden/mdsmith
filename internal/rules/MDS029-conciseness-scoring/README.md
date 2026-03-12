---
id: MDS029
name: conciseness-scoring
status: not-ready
description: Paragraph conciseness score must not fall below a threshold.
---
# MDS029: conciseness-scoring

Paragraph conciseness score must not fall below a threshold.

- **ID**: MDS029
- **Name**: `conciseness-scoring`
- **Status**: not-ready
- **Default**: disabled (experimental, not ready yet)
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

Enable (experimental):

```yaml
rules:
  conciseness-scoring: true
```

Enable with custom settings:

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

<?include
file: good/default.md
wrap: markdown
?>

```markdown
# Concise Paragraph

The release process validates links, updates version tags, and publishes
checksums so reviewers can verify artifacts before approving deployment.
```

<?/include?>

### Bad

<?include
file: bad/default.md
wrap: markdown
?>

```markdown
# Verbose Paragraph

Basically, it seems that we are just trying to explain the same idea in
order to make it very clear, and it appears that we are really adding very
little concrete information to the paragraph.
```

<?/include?>

## Diagnostics

| Condition             | Message                                                                                                              |
|-----------------------|----------------------------------------------------------------------------------------------------------------------|
| score below threshold | `conciseness score too low (0.12 < 0.20); target >= 0.20; reduce filler or hedge cues (e.g., "basically", "it seems")` |
