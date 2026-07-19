---
id: MET008
name: readability
description: Automated Readability Index (ARI) over the file's plain text. Higher means harder to read.
---
# MET008: readability

Automated Readability Index (ARI) computed over the file's
extracted plain text. Higher values mean harder to read; the
score approximates a US grade level.

- **Scope**: file
- **Sort default**: descending
- **Type**: float (1 decimal place)
- **Default**: no (opt in with `--metrics readability`)

## Notes

Formula: `4.71 × (characters/words) + 0.5 × (words/sentences) − 21.43`
where *characters* counts letters and digits only.

Returns `null` / `-` when the file has no words (empty file or
a file whose content is entirely code blocks with no plain text).
