---
title: Rule Directory
summary: >-
  Complete list of all mdsmith rules with status and
  description, generated from rule READMEs.
---
# Rule Directory

All mdsmith rules. Each rule links to its full
README with parameters, examples, and diagnostics.

<?catalog
glob: "MDS*/README.md"
sort: id
header: |
  | Rule | Name | Status | Description |
  |------|------|--------|-------------|
row: "| [{id}]({filename}) | `{name}` | {status} | {description} |"
?>
| Rule                                                          | Name                                 | Status    | Description                                                                                   |
|---------------------------------------------------------------|--------------------------------------|-----------|-----------------------------------------------------------------------------------------------|
| [MDS001](MDS001-line-length/README.md)                        | `line-length`                        | ready     | Line exceeds maximum length.                                                                  |
| [MDS002](MDS002-heading-style/README.md)                      | `heading-style`                      | ready     | Heading style must be consistent.                                                             |
| [MDS003](MDS003-heading-increment/README.md)                  | `heading-increment`                  | ready     | Heading levels should increment by one. No jumping from `#` to `###`.                         |
| [MDS004](MDS004-first-line-heading/README.md)                 | `first-line-heading`                 | ready     | First line of the file should be a heading.                                                   |
| [MDS005](MDS005-no-duplicate-headings/README.md)              | `no-duplicate-headings`              | ready     | No two headings should have the same text.                                                    |
| [MDS006](MDS006-no-trailing-spaces/README.md)                 | `no-trailing-spaces`                 | ready     | No trailing whitespace at the end of lines.                                                   |
| [MDS007](MDS007-no-hard-tabs/README.md)                       | `no-hard-tabs`                       | ready     | No tab characters. Use spaces instead.                                                        |
| [MDS008](MDS008-no-multiple-blanks/README.md)                 | `no-multiple-blanks`                 | ready     | No more than one consecutive blank line.                                                      |
| [MDS009](MDS009-single-trailing-newline/README.md)            | `single-trailing-newline`            | ready     | File must end with exactly one newline character.                                             |
| [MDS010](MDS010-fenced-code-style/README.md)                  | `fenced-code-style`                  | ready     | Fenced code blocks must use a consistent delimiter.                                           |
| [MDS011](MDS011-fenced-code-language/README.md)               | `fenced-code-language`               | ready     | Fenced code blocks must specify a language.                                                   |
| [MDS012](MDS012-no-bare-urls/README.md)                       | `no-bare-urls`                       | ready     | URLs must be wrapped in angle brackets or as a link, not left bare.                           |
| [MDS013](MDS013-blank-line-around-headings/README.md)         | `blank-line-around-headings`         | ready     | Headings must have a blank line before and after.                                             |
| [MDS014](MDS014-blank-line-around-lists/README.md)            | `blank-line-around-lists`            | ready     | Lists must have a blank line before and after.                                                |
| [MDS015](MDS015-blank-line-around-fenced-code/README.md)      | `blank-line-around-fenced-code`      | ready     | Fenced code blocks must have a blank line before and after.                                   |
| [MDS016](MDS016-list-indent/README.md)                        | `list-indent`                        | ready     | List items must use consistent indentation.                                                   |
| [MDS017](MDS017-no-trailing-punctuation-in-heading/README.md) | `no-trailing-punctuation-in-heading` | ready     | Headings should not end with punctuation.                                                     |
| [MDS018](MDS018-no-emphasis-as-heading/README.md)             | `no-emphasis-as-heading`             | ready     | Don't use bold or emphasis on a standalone line as a heading substitute.                      |
| [MDS019](MDS019-catalog/README.md)                            | `catalog`                            | ready     | Catalog content must reflect selected front matter fields from files matching its glob.       |
| [MDS020](MDS020-required-structure/README.md)                 | `required-structure`                 | ready     | Document structure and front matter must match its schema.                                    |
| [MDS021](MDS021-include/README.md)                            | `include`                            | ready     | Include section content must match the referenced file.                                       |
| [MDS022](MDS022-max-file-length/README.md)                    | `max-file-length`                    | ready     | File must not exceed maximum number of lines.                                                 |
| [MDS023](MDS023-paragraph-readability/README.md)              | `paragraph-readability`              | ready     | Paragraph readability index must not exceed a threshold.                                      |
| [MDS024](MDS024-paragraph-structure/README.md)                | `paragraph-structure`                | ready     | Paragraphs must not exceed sentence and word limits.                                          |
| [MDS025](MDS025-table-format/README.md)                       | `table-format`                       | ready     | Tables must have consistent column widths and padding.                                        |
| [MDS026](MDS026-table-readability/README.md)                  | `table-readability`                  | ready     | Tables must stay within readability complexity limits.                                        |
| [MDS027](MDS027-cross-file-reference-integrity/README.md)     | `cross-file-reference-integrity`     | ready     | Links to local files and heading anchors must resolve.                                        |
| [MDS028](MDS028-token-budget/README.md)                       | `token-budget`                       | ready     | File must not exceed a token budget.                                                          |
| [MDS029](MDS029-conciseness-scoring/README.md)                | `conciseness-scoring`                | not-ready | Paragraph conciseness score must not fall below a threshold.                                  |
| [MDS030](MDS030-empty-section-body/README.md)                 | `empty-section-body`                 | ready     | Section headings must include meaningful body content.                                        |
| [MDS031](MDS031-unclosed-code-block/README.md)                | `unclosed-code-block`                | ready     | Fenced code blocks must have a closing fence delimiter.                                       |
| [MDS032](MDS032-no-empty-alt-text/README.md)                  | `no-empty-alt-text`                  | ready     | Images must have non-empty alt text for accessibility.                                        |
| [MDS033](MDS033-directory-structure/README.md)                | `directory-structure`                | ready     | Markdown files must exist only in explicitly allowed directories.                             |
| [MDS034](MDS034-markdown-flavor/README.md)                    | `markdown-flavor`                    | ready     | Flags Markdown syntax that the declared target flavor does not render.                        |
| [MDS035](MDS035-toc-directive/README.md)                      | `toc-directive`                      | ready     | Flag renderer-specific TOC directives that render as literal text on CommonMark and goldmark. |
| [MDS036](MDS036-max-section-length/README.md)                 | `max-section-length`                 | ready     | Section length must not exceed per-level or per-heading limits.                               |
| [MDS037](MDS037-duplicated-content/README.md)                 | `duplicated-content`                 | ready     | Paragraphs should not repeat verbatim across Markdown files.                                  |
| [MDS038](MDS038-toc/README.md)                                | `toc`                                | ready     | Keep toc generated heading lists in sync with document headings.                              |
| [MDS045](MDS045-list-marker-style/README.md)                  | `list-marker-style`                  | ready     | Unordered list items must use the configured bullet marker character.                         |
| [MDS046](MDS046-ordered-list-numbering/README.md)             | `ordered-list-numbering`             | ready     | Ordered list items must be numbered in the configured style.                                  |
<?/catalog?>
