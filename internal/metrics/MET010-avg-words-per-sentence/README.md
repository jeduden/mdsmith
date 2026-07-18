---
id: MET010
name: avg-words-per-sentence
description: Average words per sentence (words ÷ sentences). Zero when there are no sentences.
---
# MET010: avg-words-per-sentence

Average number of words per sentence, computed as
`words / sentences` over the file's extracted plain text.

- **Scope**: file
- **Sort default**: descending
- **Type**: float (1 decimal place)
- **Default**: no (opt in with `--metrics avg-words-per-sentence`)

## Notes

Returns 0 when the file has no sentences (empty file or a file
whose content is entirely code blocks with no plain text).
