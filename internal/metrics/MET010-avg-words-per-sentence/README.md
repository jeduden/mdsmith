---
id: MET010
name: avg-words-per-sentence
description: Average words per sentence from extracted plain text. Zero when there are no sentences.
---
# MET010: avg-words-per-sentence

Average words per sentence from extracted plain text. Zero when
there are no sentences.

- **Scope**: file
- **Sort default**: descending
- **Type**: float (1 decimal place)
- **Default**: false (opt-in with `--metrics avg-words-per-sentence`)

## Notes

Computed as `words / sentences` where sentences is from MET009 and
words is from MET003. Returns `0` for empty files or files whose
plain text has no sentence-ending punctuation and no words.

Long average sentence length often correlates with higher ARI
(MET008) and can be a signal for overly complex prose.
