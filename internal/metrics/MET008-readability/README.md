---
id: MET008
name: readability
description: Automated Readability Index (ARI) over the file's plain text. Higher values indicate harder-to-read text; the number approximates US grade level.
---
# MET008: readability

Automated Readability Index (ARI) over the file's plain text.
Higher values indicate harder-to-read text; the number approximates
US grade level.

- **Scope**: file
- **Sort default**: descending
- **Type**: float (1 decimal place)
- **Default**: false (opt-in with `--metrics readability`)

## Formula

```text
ARI = 4.71 × (characters / words) + 0.5 × (words / sentences) − 21.43
```

Characters counts letters and digits only (no spaces or punctuation).
Returns `0` when the file has no words.

## Notes

The ARI formula was designed for typed text in US English. It works
as a rough proxy for prose complexity across technical documentation
but penalises long technical terms regardless of whether the audience
knows them. Use it as a relative signal across a corpus, not as an
absolute grade-level gate.

See also `MDS023 paragraph-readability` for a per-paragraph ARI rule
that flags paragraphs above a configurable threshold.
