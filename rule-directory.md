---
title: Rule Directory
summary: >-
  Complete list of all mdsmith rules with status and
  description, generated from rule READMEs.
---
# Rule Directory

All mdsmith rules. Each rule links to its full
README with parameters, examples, and diagnostics.

## Rules MDS001-MDS016

<?catalog
glob:
  - "internal/rules/MDS00*/README.md"
  - "internal/rules/MDS01[0-6]*/README.md"
sort: id
header: |
  | Rule | Name | Status | Description |
  |------|------|--------|-------------|
row: "| [{id}]({filename}) | `{name}` | {status} | {description} |"
?>
| Rule                                                                    | Name                            | Status | Description                                                           |
|-------------------------------------------------------------------------|---------------------------------|--------|-----------------------------------------------------------------------|
| [MDS001](internal/rules/MDS001-line-length/README.md)                   | `line-length`                   | ready  | Line exceeds maximum length.                                          |
| [MDS002](internal/rules/MDS002-heading-style/README.md)                 | `heading-style`                 | ready  | Heading style must be consistent.                                     |
| [MDS003](internal/rules/MDS003-heading-increment/README.md)             | `heading-increment`             | ready  | Heading levels should increment by one. No jumping from `#` to `###`. |
| [MDS004](internal/rules/MDS004-first-line-heading/README.md)            | `first-line-heading`            | ready  | First line of the file should be a heading.                           |
| [MDS005](internal/rules/MDS005-no-duplicate-headings/README.md)         | `no-duplicate-headings`         | ready  | No two headings should have the same text.                            |
| [MDS006](internal/rules/MDS006-no-trailing-spaces/README.md)            | `no-trailing-spaces`            | ready  | No trailing whitespace at the end of lines.                           |
| [MDS007](internal/rules/MDS007-no-hard-tabs/README.md)                  | `no-hard-tabs`                  | ready  | No tab characters. Use spaces instead.                                |
| [MDS008](internal/rules/MDS008-no-multiple-blanks/README.md)            | `no-multiple-blanks`            | ready  | No more than one consecutive blank line.                              |
| [MDS009](internal/rules/MDS009-single-trailing-newline/README.md)       | `single-trailing-newline`       | ready  | File must end with exactly one newline character.                     |
| [MDS010](internal/rules/MDS010-fenced-code-style/README.md)             | `fenced-code-style`             | ready  | Fenced code blocks must use a consistent delimiter.                   |
| [MDS011](internal/rules/MDS011-fenced-code-language/README.md)          | `fenced-code-language`          | ready  | Fenced code blocks must specify a language.                           |
| [MDS012](internal/rules/MDS012-no-bare-urls/README.md)                  | `no-bare-urls`                  | ready  | URLs must be wrapped in angle brackets or as a link, not left bare.   |
| [MDS013](internal/rules/MDS013-blank-line-around-headings/README.md)    | `blank-line-around-headings`    | ready  | Headings must have a blank line before and after.                     |
| [MDS014](internal/rules/MDS014-blank-line-around-lists/README.md)       | `blank-line-around-lists`       | ready  | Lists must have a blank line before and after.                        |
| [MDS015](internal/rules/MDS015-blank-line-around-fenced-code/README.md) | `blank-line-around-fenced-code` | ready  | Fenced code blocks must have a blank line before and after.           |
| [MDS016](internal/rules/MDS016-list-indent/README.md)                   | `list-indent`                   | ready  | List items must use consistent indentation.                           |
<?/catalog?>

## Rules MDS017-MDS033

<?catalog
glob:
  - "internal/rules/MDS01[7-9]*/README.md"
  - "internal/rules/MDS02*/README.md"
  - "internal/rules/MDS03*/README.md"
sort: id
header: |
  | Rule | Name | Status | Description |
  |------|------|--------|-------------|
row: "| [{id}]({filename}) | `{name}` | {status} | {description} |"
?>
| Rule                                                                         | Name                                 | Status    | Description                                                                             |
|------------------------------------------------------------------------------|--------------------------------------|-----------|-----------------------------------------------------------------------------------------|
| [MDS017](internal/rules/MDS017-no-trailing-punctuation-in-heading/README.md) | `no-trailing-punctuation-in-heading` | ready     | Headings should not end with punctuation.                                               |
| [MDS018](internal/rules/MDS018-no-emphasis-as-heading/README.md)             | `no-emphasis-as-heading`             | ready     | Don't use bold or emphasis on a standalone line as a heading substitute.                |
| [MDS019](internal/rules/MDS019-catalog/README.md)                            | `catalog`                            | ready     | Catalog content must reflect selected front matter fields from files matching its glob. |
| [MDS020](internal/rules/MDS020-required-structure/README.md)                 | `required-structure`                 | ready     | Document structure and front matter must match its schema.                              |
| [MDS021](internal/rules/MDS021-include/README.md)                            | `include`                            | ready     | Include section content must match the referenced file.                                 |
| [MDS022](internal/rules/MDS022-max-file-length/README.md)                    | `max-file-length`                    | ready     | File must not exceed maximum number of lines.                                           |
| [MDS023](internal/rules/MDS023-paragraph-readability/README.md)              | `paragraph-readability`              | ready     | Paragraph readability index must not exceed a threshold.                                |
| [MDS024](internal/rules/MDS024-paragraph-structure/README.md)                | `paragraph-structure`                | ready     | Paragraphs must not exceed sentence and word limits.                                    |
| [MDS025](internal/rules/MDS025-table-format/README.md)                       | `table-format`                       | ready     | Tables must have consistent column widths and padding.                                  |
| [MDS026](internal/rules/MDS026-table-readability/README.md)                  | `table-readability`                  | ready     | Tables must stay within readability complexity limits.                                  |
| [MDS027](internal/rules/MDS027-cross-file-reference-integrity/README.md)     | `cross-file-reference-integrity`     | ready     | Links to local files and heading anchors must resolve.                                  |
| [MDS028](internal/rules/MDS028-token-budget/README.md)                       | `token-budget`                       | ready     | File must not exceed a token budget.                                                    |
| [MDS029](internal/rules/MDS029-conciseness-scoring/README.md)                | `conciseness-scoring`                | not-ready | Paragraph conciseness score must not fall below a threshold.                            |
| [MDS030](internal/rules/MDS030-empty-section-body/README.md)                 | `empty-section-body`                 | ready     | Section headings must include meaningful body content.                                  |
| [MDS031](internal/rules/MDS031-unclosed-code-block/README.md)                | `unclosed-code-block`                | ready     | Fenced code blocks must have a closing fence delimiter.                                 |
| [MDS032](internal/rules/MDS032-no-empty-alt-text/README.md)                  | `no-empty-alt-text`                  | ready     | Images must have non-empty alt text for accessibility.                                  |
| [MDS033](internal/rules/MDS033-directory-structure/README.md)                | `directory-structure`                | ready     | Markdown files must exist only in explicitly allowed directories.                       |
<?/catalog?>
