---
title: Rule Directory
weight: 40
summary: >-
  Complete list of all mdsmith rules with category, status, and
  description, generated from rule READMEs.
---
# Rule Directory

All mdsmith rules. Each rule links to its full
README with parameters, examples, and diagnostics.

<?catalog
glob: "MDS*/README.md"
sort: id
header: |
  | Rule | Name | Category | Status | Description |
  |------|------|----------|--------|-------------|
row: "| [{id}]({filename}) | `{name}` | {category} | {status} | {description} |"
?>
| Rule                                                          | Name                                 | Category      | Status    | Description                                                                                                                                        |
|---------------------------------------------------------------|--------------------------------------|---------------|-----------|----------------------------------------------------------------------------------------------------------------------------------------------------|
| [MDS001](MDS001-line-length/README.md)                        | `line-length`                        | line          | ready     | Line exceeds maximum length.                                                                                                                       |
| [MDS002](MDS002-heading-style/README.md)                      | `heading-style`                      | heading       | ready     | Heading style must be consistent.                                                                                                                  |
| [MDS003](MDS003-heading-increment/README.md)                  | `heading-increment`                  | heading       | ready     | Heading levels should increment by one. No jumping from `#` to `###`.                                                                              |
| [MDS004](MDS004-first-line-heading/README.md)                 | `first-line-heading`                 | heading       | ready     | First line of the file should be a heading.                                                                                                        |
| [MDS005](MDS005-no-duplicate-headings/README.md)              | `no-duplicate-headings`              | heading       | ready     | No two headings should have the same text.                                                                                                         |
| [MDS006](MDS006-no-trailing-spaces/README.md)                 | `no-trailing-spaces`                 | whitespace    | ready     | No trailing whitespace at the end of lines.                                                                                                        |
| [MDS007](MDS007-no-hard-tabs/README.md)                       | `no-hard-tabs`                       | whitespace    | ready     | No tab characters. Use spaces instead.                                                                                                             |
| [MDS008](MDS008-no-multiple-blanks/README.md)                 | `no-multiple-blanks`                 | whitespace    | ready     | No more than one consecutive blank line.                                                                                                           |
| [MDS009](MDS009-single-trailing-newline/README.md)            | `single-trailing-newline`            | whitespace    | ready     | File must end with exactly one newline character.                                                                                                  |
| [MDS010](MDS010-fenced-code-style/README.md)                  | `fenced-code-style`                  | code          | ready     | Fenced code blocks must use a consistent delimiter.                                                                                                |
| [MDS011](MDS011-fenced-code-language/README.md)               | `fenced-code-language`               | code          | ready     | Fenced code blocks must specify a language.                                                                                                        |
| [MDS012](MDS012-no-bare-urls/README.md)                       | `no-bare-urls`                       | link          | ready     | URLs must be wrapped in angle brackets or as a link, not left bare.                                                                                |
| [MDS013](MDS013-blank-line-around-headings/README.md)         | `blank-line-around-headings`         | heading       | ready     | Headings must have a blank line before and after.                                                                                                  |
| [MDS014](MDS014-blank-line-around-lists/README.md)            | `blank-line-around-lists`            | list          | ready     | Lists must have a blank line before and after.                                                                                                     |
| [MDS015](MDS015-blank-line-around-fenced-code/README.md)      | `blank-line-around-fenced-code`      | code          | ready     | Fenced code blocks must have a blank line before and after.                                                                                        |
| [MDS016](MDS016-list-indent/README.md)                        | `list-indent`                        | list          | ready     | List items must use consistent indentation.                                                                                                        |
| [MDS017](MDS017-no-trailing-punctuation-in-heading/README.md) | `no-trailing-punctuation-in-heading` | heading       | ready     | Headings should not end with punctuation.                                                                                                          |
| [MDS018](MDS018-no-emphasis-as-heading/README.md)             | `no-emphasis-as-heading`             | heading       | ready     | Don't use bold or emphasis on a standalone line as a heading substitute.                                                                           |
| [MDS019](MDS019-catalog/README.md)                            | `catalog`                            | directive     | ready     | Catalog content must reflect selected front matter fields from files matching its glob.                                                            |
| [MDS020](MDS020-required-structure/README.md)                 | `required-structure`                 | structural    | ready     | Document structure and front matter must match its schema.                                                                                         |
| [MDS021](MDS021-include/README.md)                            | `include`                            | directive     | ready     | Include section content must match the referenced file.                                                                                            |
| [MDS022](MDS022-max-file-length/README.md)                    | `max-file-length`                    | structural    | ready     | File must not exceed maximum number of lines.                                                                                                      |
| [MDS023](MDS023-paragraph-readability/README.md)              | `paragraph-readability`              | prose         | ready     | Paragraph readability index must not exceed a threshold.                                                                                           |
| [MDS024](MDS024-paragraph-structure/README.md)                | `paragraph-structure`                | prose         | ready     | Paragraphs must not exceed sentence and word limits.                                                                                               |
| [MDS025](MDS025-table-format/README.md)                       | `table-format`                       | table         | ready     | Tables must have consistent column widths and padding.                                                                                             |
| [MDS026](MDS026-table-readability/README.md)                  | `table-readability`                  | table         | ready     | Tables must stay within readability complexity limits.                                                                                             |
| [MDS027](MDS027-cross-file-reference-integrity/README.md)     | `cross-file-reference-integrity`     | link          | ready     | Links to local files and heading anchors must resolve.                                                                                             |
| [MDS028](MDS028-token-budget/README.md)                       | `token-budget`                       | prose         | ready     | File must not exceed a token budget.                                                                                                               |
| [MDS029](MDS029-conciseness-scoring/README.md)                | `conciseness-scoring`                | prose         | not-ready | Paragraph conciseness score must not fall below a threshold.                                                                                       |
| [MDS030](MDS030-empty-section-body/README.md)                 | `empty-section-body`                 | heading       | ready     | Section headings must include meaningful body content.                                                                                             |
| [MDS031](MDS031-unclosed-code-block/README.md)                | `unclosed-code-block`                | code          | ready     | Fenced code blocks must have a closing fence delimiter.                                                                                            |
| [MDS032](MDS032-no-empty-alt-text/README.md)                  | `no-empty-alt-text`                  | accessibility | ready     | Images must have non-empty alt text for accessibility.                                                                                             |
| [MDS033](MDS033-directory-structure/README.md)                | `directory-structure`                | structural    | ready     | Markdown files must exist only in explicitly allowed directories.                                                                                  |
| [MDS034](MDS034-markdown-flavor/README.md)                    | `markdown-flavor`                    | structural    | ready     | Flags Markdown syntax that the declared target flavor does not render.                                                                             |
| [MDS035](MDS035-toc-directive/README.md)                      | `toc-directive`                      | directive     | ready     | Flag renderer-specific TOC directives that render as literal text on CommonMark and goldmark.                                                      |
| [MDS036](MDS036-max-section-length/README.md)                 | `max-section-length`                 | heading       | ready     | Section length must not exceed per-level, per-heading, word, or paragraph limits.                                                                  |
| [MDS037](MDS037-duplicated-content/README.md)                 | `duplicated-content`                 | prose         | ready     | Paragraphs should not repeat verbatim across Markdown files.                                                                                       |
| [MDS038](MDS038-toc/README.md)                                | `toc`                                | directive     | ready     | Keep toc generated heading lists in sync with document headings.                                                                                   |
| [MDS039](MDS039-build/README.md)                              | `build`                              | directive     | ready     | Validate `<?build?>` directive parameters and keep the section body in sync with the recipe's rendered `body-template`.                            |
| [MDS040](MDS040-recipe-safety/README.md)                      | `recipe-safety`                      | directive     | ready     | Validate each build.recipes command for shell-safety at lint time; the rule never executes any binary.                                             |
| [MDS041](MDS041-no-inline-html/README.md)                     | `no-inline-html`                     | structural    | ready     | Raw HTML tags in Markdown are not allowed; use a Markdown construct or an mdsmith directive instead.                                               |
| [MDS042](MDS042-emphasis-style/README.md)                     | `emphasis-style`                     | prose         | ready     | Enforces a single delimiter character for bold and italic emphasis, and optionally forbids cross-delimiter nesting.                                |
| [MDS043](MDS043-no-reference-style/README.md)                 | `no-reference-style`                 | link          | ready     | Reference-style links and footnotes require global definition resolution; flag them in favor of inline links.                                      |
| [MDS044](MDS044-horizontal-rule-style/README.md)              | `horizontal-rule-style`              | whitespace    | ready     | Thematic breaks must use a consistent delimiter style, exact length, and blank-line spacing.                                                       |
| [MDS045](MDS045-list-marker-style/README.md)                  | `list-marker-style`                  | list          | ready     | Unordered list items must use the configured bullet marker character.                                                                              |
| [MDS046](MDS046-ordered-list-numbering/README.md)             | `ordered-list-numbering`             | list          | ready     | Ordered list items must be numbered in the configured style.                                                                                       |
| [MDS047](MDS047-ambiguous-emphasis/README.md)                 | `ambiguous-emphasis`                 | prose         | ready     | Forbid emphasis sequences whose meaning a human cannot predict at a glance.                                                                        |
| [MDS048](MDS048-git-hook-sync/README.md)                      | `git-hook-sync`                      | structural    | ready     | Git artifacts must match the canonical glob-based template derived from .mdsmith.yml.                                                              |
| [MDS049](MDS049-no-space-in-link-text/README.md)              | `no-space-in-link-text`              | link          | ready     | Link text and image alt text must not have leading or trailing whitespace inside the brackets.                                                     |
| [MDS050](MDS050-proper-names/README.md)                       | `proper-names`                       | prose         | ready     | Configured proper names (e.g. JavaScript, GitHub) must appear with their canonical casing.                                                         |
| [MDS051](MDS051-single-h1/README.md)                          | `single-h1`                          | heading       | ready     | At most one H1 heading is allowed per file.                                                                                                        |
| [MDS052](MDS052-no-space-in-code-spans/README.md)             | `no-space-in-code-spans`             | whitespace    | ready     | Inline code spans with leading or trailing whitespace inside the backticks are almost always typos; flag them.                                     |
| [MDS053](MDS053-no-unused-link-definitions/README.md)         | `no-unused-link-definitions`         | link          | ready     | Every `[label]: url` definition must be consumed by at least one reference-style link or image; duplicate labels are flagged.                      |
| [MDS054](MDS054-no-undefined-reference-labels/README.md)      | `no-undefined-reference-labels`      | link          | ready     | Reference-style links and images must have a matching link reference definition in the same file.                                                  |
| [MDS055](MDS055-forbidden-paragraph-starts/README.md)         | `forbidden-paragraph-starts`         | prose         | ready     | Paragraphs must not begin with any configured prefix.                                                                                              |
| [MDS056](MDS056-forbidden-text/README.md)                     | `forbidden-text`                     | prose         | ready     | Paragraphs must not contain any configured substring.                                                                                              |
| [MDS057](MDS057-required-text-patterns/README.md)             | `required-text-patterns`             | prose         | ready     | Heading-bounded sections must match every configured regex.                                                                                        |
| [MDS058](MDS058-required-mentions/README.md)                  | `required-mentions`                  | prose         | ready     | Heading-bounded sections must contain every configured substring.                                                                                  |
| [MDS061](MDS061-list-marker-space/README.md)                  | `list-marker-space`                  | list          | ready     | Each list marker must be followed by the configured number of spaces.                                                                              |
| [MDS063](MDS063-descriptive-link-text/README.md)              | `descriptive-link-text`              | prose         | ready     | Link text must be descriptive. Non-descriptive phrases like "click here", "here", "link", and "more" fail screen readers and link-list navigation. |
<?/catalog?>

## Directive rules

Rules whose `nature: directive` front matter
marks them as `<?...?>` directive
implementations. Filtered from the same source as
the table above.

<?catalog
glob: "MDS*/README.md"
where: 'nature: "directive"'
sort: id
header: |
  | Rule | Name | Description |
  |------|------|-------------|
row: "| [{id}]({filename}) | `{name}` | {description} |"
?>
| Rule                               | Name      | Description                                                                                                             |
|------------------------------------|-----------|-------------------------------------------------------------------------------------------------------------------------|
| [MDS019](MDS019-catalog/README.md) | `catalog` | Catalog content must reflect selected front matter fields from files matching its glob.                                 |
| [MDS021](MDS021-include/README.md) | `include` | Include section content must match the referenced file.                                                                 |
| [MDS038](MDS038-toc/README.md)     | `toc`     | Keep toc generated heading lists in sync with document headings.                                                        |
| [MDS039](MDS039-build/README.md)   | `build`   | Validate `<?build?>` directive parameters and keep the section body in sync with the recipe's rendered `body-template`. |
<?/catalog?>
