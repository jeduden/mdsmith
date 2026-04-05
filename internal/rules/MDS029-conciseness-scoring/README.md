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
- **Default**: disabled
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: meta

## Settings

| Setting     | Type   | Default | Description                       |
|-------------|--------|---------|-----------------------------------|
| `min-score` | number | `0.20`  | Minimum allowed conciseness score |
| `min-words` | int    | `20`    | Skip paragraphs shorter than this |

The score is produced by a pure-Go linear classifier with 15
features. The classifier outputs a risk score via sigmoid; the
conciseness score is `1 - risk_score`, so 1.0 = maximally
concise. Markdown tables are skipped.

## Config

Enable:

```yaml
rules:
  conciseness-scoring: true
```

Enable with custom threshold:

```yaml
rules:
  conciseness-scoring:
    min-score: 0.20
    min-words: 20
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

| Condition             | Message                                                                                                           |
|-----------------------|-------------------------------------------------------------------------------------------------------------------|
| score below threshold | `conciseness score too low (0.08 < 0.20); target >= 0.20; reduce verbose cues (e.g., "basically", "in order to")` |
