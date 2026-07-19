---
id: MET009
name: sentences
description: Sentence count from extracted plain text.
---
# MET009: sentences

Sentence count over the file's extracted plain text, estimated
by counting terminal punctuation (`.`, `!`, `?`) followed by
whitespace or end of string.

- **Scope**: file
- **Sort default**: descending
- **Type**: integer
- **Default**: no (opt in with `--metrics sentences`)

## Notes

Returns at least 1 for any non-empty plain text, matching the
behavior of `CountSentences` in `internal/mdtext`.
