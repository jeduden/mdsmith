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

| Setting               | Type         | Default  | Description                                              |
|-----------------------|--------------|----------|----------------------------------------------------------|
| `min-score`             | number       | `0.20`     | Heuristic fallback threshold for conciseness             |
| `min-words`             | int          | `20`       | Skip paragraphs shorter than this                        |
| `mode`                  | string       | `auto`     | Backend mode: `auto`, `classifier`, `heuristic`                |
| `threshold`             | number       | `0.60`     | Classifier risk threshold (`verbose` if `risk >= threshold`) |
| `classifier-timeout-ms` | int          | `25`       | Per-paragraph classifier timeout before fallback         |
| `classifier-model-path` | string       | unset    | External model JSON override path                        |
| `classifier-checksum`   | string       | unset    | SHA256 for external model override                       |
| `filler-words`          | list[string] | built-in | Words that reduce score when repeated                    |
| `hedge-phrases`         | list[string] | built-in | Phrases that indicate weak assertions                    |
| `verbose-phrases`       | list[string] | built-in | Long phrases with shorter alternatives                   |

The default embedded classifier is `cue-linear-v1` (`2026-02-15`) with
SHA256 `63132fdc0df4085dd056a49ae9d3e9287cd1014a0c5e8262b9ae05d21450a466`.
Markdown tables are skipped.

## Backend Selection and Fallback

Runtime selection order:

1. `mode: heuristic` always uses heuristic scoring.
2. `mode: classifier` and `mode: auto` attempt classifier loading first.
3. If classifier load fails (read, parse, checksum), fallback to heuristic.
4. If classifier inference exceeds `classifier-timeout-ms`, fallback to
   heuristic for remaining paragraphs in the file.

For a fixed file and config, output is deterministic when classifier
inference finishes within `classifier-timeout-ms`. Artifacts are local and
checksum-verified. If inference times out and the rule falls back to
heuristic mode, results may differ between runs.

## Config

Enable (experimental):

```yaml
rules:
  conciseness-scoring:
    mode: auto
```

Enable classifier mode with explicit external override:

```yaml
rules:
  conciseness-scoring:
    mode: classifier
    threshold: 0.58
    classifier-timeout-ms: 25
    classifier-model-path: ".mdsmith-models/cue-linear-v1.json"
    classifier-checksum: "63132fdc0df4085dd056a49ae9d3e9287cd1014a0c5e8262b9ae05d21450a466"
```

Force heuristic-only mode:

```yaml
rules:
  conciseness-scoring:
    mode: heuristic
    min-score: 0.22
    min-words: 20
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
