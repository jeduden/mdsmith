---
title: "Size and readability limits"
summary: >-
  Cap file, section, and token-budget size; enforce reading grade and
  sentence count; flag verbatim copy-paste across files.
icon: ruler
link: "/guides/metrics-tradeoffs/"
rules: ["MDS022", "MDS023", "MDS024", "MDS028", "MDS037"]
weight: 18
---
# Size and readability limits

mdsmith enforces five rules that cap the size and readability of a
Markdown corpus. They flag long files, dense paragraphs, oversized
context-window loads, and verbatim copy-paste between files.

These rules help any team catch bloat early. They are useful whether
prose is hand-written or machine-generated.

## Default state

Three rules are **on by default**. Two are **opt-in**.

| Rule     | Config key              | Default state | Default threshold                       |
|----------|-------------------------|---------------|-----------------------------------------|
| `MDS022` | `max-file-length`       | on            | `max: 300` lines                        |
| `MDS023` | `paragraph-readability` | on            | `max-index: 14.0` ARI, `min-words: 20`  |
| `MDS024` | `paragraph-structure`   | opt-in        | `max-sentences: 6`, 40 words / sentence |
| `MDS028` | `token-budget`          | on            | `max: 8000`, `mode: heuristic`          |
| `MDS037` | `duplicated-content`    | opt-in        | verbatim paragraph match (≥ 200 chars)  |

Turn an opt-in rule on by naming it in `.mdsmith.yml`:

```yaml
rules:
  paragraph-structure:
    max-sentences: 6
    max-words-per-sentence: 40
  duplicated-content: true
```

Disable all five at once — covers cases where a convention or kind
turned on `paragraph-structure` or `duplicated-content`:

```yaml
rules:
  max-file-length: false
  paragraph-readability: false
  paragraph-structure: false
  token-budget: false
  duplicated-content: false
```

A bare `false` toggles `enabled` only. Inherited settings stay
intact. A later layer can re-enable the rule without re-stating its
thresholds.

## Rule reference

`MDS022` caps lines per file. `MDS023` scores paragraph reading grade
using ARI. `MDS024` flags paragraphs with too many sentences or
over-long sentences. `MDS028` estimates the token load of a file with
either a fast heuristic or a tokenizer. `MDS037` hashes each
normalized paragraph (default: ≥ 200 characters) and flags
verbatim duplicates that appear in another file in the corpus.

See the [metrics trade-offs guide](../guides/metrics-tradeoffs.md)
for threshold guidance and the [rule directory](/rules/) for the full
reference for each rule.
