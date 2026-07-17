---
id: MET009
name: sentences
description: Sentence count from extracted plain text.
---
# MET009: sentences

Sentence count from extracted plain text.

- **Scope**: file
- **Sort default**: descending
- **Type**: integer
- **Default**: false (opt-in with `--metrics sentences`)

## Notes

Sentences are detected by splitting on `.`, `!`, and `?` followed
by whitespace or end of text. Non-empty text with no terminal
punctuation counts as one sentence. Abbreviations and decimal
numbers that happen to contain a dot may affect the count.

Pair with `avg-words-per-sentence` (MET010) to spot files that
are verbose at the sentence level.
