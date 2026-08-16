---
id: MET007
name: word-frequency
description: Highest repeat count of any single content word in the file.
---
# MET007: word-frequency

Highest repeat count of any single content word in the file.

- **Scope**: file
- **Sort default**: descending
- **Type**: integer

## Notes

Counts case-folded, whitespace/punctuation-delimited words of four or
more runes from the file's extracted plain text. Fenced and indented code
blocks are excluded because `ExtractPlainText` omits them. Inline code
spans are included.

The value is the single highest repeat count across all qualifying words.
A value of 1 means no word appears more than once; higher values flag
files that may over-repeat a single term.

Use `mdsmith metrics rank --metrics word-frequency` to find the most
repetitive files in a corpus. Pair with the `over-repetition` rule
(MDS075) to enforce a per-scope ceiling in CI.

**Text extraction**: `ExtractPlainText` recurses into all block nodes.
Adjacent blocks are concatenated without separators, so words at block
boundaries merge into unrecognised tokens. Heading-only repetition
therefore rarely raises MET007. MDS075 (which checks paragraph nodes
directly) also never flags heading text, so the two agree in practice.

**min-length**: The metric always applies a minimum word length of 4 runes,
matching MDS075's default. Files where the rule is configured with a
different `min-length` will observe a discrepancy between the metric value
and the rule's findings.
