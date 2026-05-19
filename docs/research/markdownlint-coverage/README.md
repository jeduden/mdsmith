---
summary: >-
  Complete markdownlint MDxxx -> mdsmith coverage matrix:
  every active markdownlint rule, the mdsmith rule that
  covers it or the plan that schedules it, plus the
  mdsmith-only rules with no markdownlint analog.
---
# markdownlint coverage matrix

Every active markdownlint rule mapped to mdsmith. This is
the full rule-by-rule comparison; the
[linter comparison](../../background/markdown-linters.md)
links here instead of carrying a partial table that drifts.

Status legend:

- ✅ — implemented; the named `MDSxxx` rule covers it.
- partial — covered in part by the named rule.
- 🔲 plan N — not yet implemented; scheduled in `plan/N`.

Deprecated markdownlint numbers (MD002, MD006, MD008,
MD015-MD017) are omitted. As of 2026-05 mdsmith implements
**39 of the 52** active markdownlint rules (**38** fully, **1** partially); the
remaining **13** are scheduled in plans 172, 176, 179, and 181-182.

## Headings

| markdownlint                   | Checks              | mdsmith | Status      |
|--------------------------------|---------------------|---------|-------------|
| MD001 heading-increment        | one level at a time | MDS003  | ✅          |
| MD003 heading-style            | atx vs setext       | MDS002  | ✅          |
| MD018 no-missing-space-atx     | `#Heading`          | —       | 🔲 plan 176 |
| MD019 no-multiple-space-atx    | `#  Heading`        | —       | 🔲 plan 176 |
| MD020 no-missing-space-closed  | `#Heading#`         | —       | 🔲 plan 176 |
| MD021 no-multiple-space-closed | `# H  #`            | —       | 🔲 plan 176 |
| MD022 blanks-around-headings   | blank lines         | MDS013  | ✅          |
| MD023 heading-start-left       | no indent           | —       | 🔲 plan 176 |
| MD024 no-duplicate-heading     | unique text         | MDS005  | ✅          |
| MD025 single-title             | one H1              | MDS051  | ✅          |
| MD026 trailing-punctuation     | heading end         | MDS017  | ✅          |
| MD036 emphasis-as-heading      | bold as head        | MDS018  | ✅          |
| MD041 first-line-heading       | file starts H1      | MDS004  | ✅          |
| MD043 required-headings        | fixed structure     | MDS020  | ✅ schema   |

## Lists

| markdownlint              | Checks             | mdsmith | Status  |
|---------------------------|--------------------|---------|---------|
| MD004 ul-style            | bullet char        | MDS045  | ✅      |
| MD005 list-indent         | even indent        | MDS016  | partial |
| MD007 ul-indent           | nesting width      | MDS016  | ✅      |
| MD029 ol-prefix           | ordered numbering  | MDS046  | ✅      |
| MD030 list-marker-space   | space after marker | MDS061  | ✅      |
| MD032 blanks-around-lists | blank lines        | MDS014  | ✅      |

## Whitespace, blank lines, tabs

| markdownlint             | Checks          | mdsmith | Status |
|--------------------------|-----------------|---------|--------|
| MD009 no-trailing-spaces | line ends       | MDS006  | ✅     |
| MD010 no-hard-tabs       | tab chars       | MDS007  | ✅     |
| MD012 no-multiple-blanks | repeated blanks | MDS008  | ✅     |
| MD013 line-length        | max width       | MDS001  | ✅     |
| MD047 file-ends-newline  | trailing `\n`   | MDS009  | ✅     |
| MD027 multiple-space-bq  | `>  text`       | MDS059  | ✅     |
| MD028 blank-line-bq      | gap in quote    | MDS059  | ✅     |

## Code blocks and code spans

| markdownlint               | Checks           | mdsmith | Status      |
|----------------------------|------------------|---------|-------------|
| MD014 commands-show-output | `$` w/o output   | —       | 🔲 plan 182 |
| MD031 blanks-around-fences | blank lines      | MDS015  | ✅          |
| MD038 spaces-in-code-span  | `` ` x ` ``      | MDS052  | ✅          |
| MD040 fenced-code-language | info string      | MDS011  | ✅          |
| MD046 code-block-style     | fenced vs indent | —       | 🔲 plan 182 |
| MD048 code-fence-style     | ``` vs ~~~       | MDS010  | ✅          |

## Links and references

| markdownlint              | Checks           | mdsmith | Status      |
|---------------------------|------------------|---------|-------------|
| MD011 no-reversed-links   | `(t)[u]`         | —       | 🔲 plan 179 |
| MD034 no-bare-urls        | raw URL          | MDS012  | ✅          |
| MD039 spaces-in-link-text | `[ t ]`          | MDS049  | ✅          |
| MD042 no-empty-links      | empty target     | —       | 🔲 plan 179 |
| MD051 link-fragments      | `#anchor` exists | MDS027  | ✅ x-file   |
| MD052 reference-defined   | ref label set    | MDS054  | ✅          |
| MD053 reference-needed    | unused defs      | MDS053  | ✅          |
| MD054 link-image-style    | inline vs ref    | —       | 🔲 plan 172 |
| MD059 descriptive-link    | "click here"     | MDS063  | ✅          |

## Inline, emphasis, HTML

| markdownlint             | Checks         | mdsmith | Status |
|--------------------------|----------------|---------|--------|
| MD033 no-inline-html     | raw HTML       | MDS041  | ✅     |
| MD035 hr-style           | `---` vs `***` | MDS044  | ✅     |
| MD037 spaces-in-emphasis | `* x *`        | MDS047  | ✅     |
| MD044 proper-names       | capitalization | MDS050  | ✅     |
| MD045 no-alt-text        | image alt      | MDS032  | ✅     |
| MD049 emphasis-style     | `_` vs `*`     | MDS042  | ✅     |
| MD050 strong-style       | `__` vs `**`   | MDS042  | ✅     |

## Tables

| markdownlint               | Checks      | mdsmith | Status      |
|----------------------------|-------------|---------|-------------|
| MD055 table-pipe-style     | edge pipes  | —       | 🔲 plan 181 |
| MD056 table-column-count   | equal cells | —       | 🔲 plan 181 |
| MD058 blanks-around-tables | blank lines | —       | 🔲 plan 181 |

## mdsmith-only rules (no markdownlint analog)

| mdsmith                               | What it adds                      |
|---------------------------------------|-----------------------------------|
| MDS019 catalog                        | generated index from front matter |
| MDS020 required-structure             | CUE schema beyond MD043           |
| MDS021 include                        | spliced, synced file inclusion    |
| MDS022 max-file-length                | file size budget                  |
| MDS023 paragraph-readability          | ARI grade limit                   |
| MDS024 paragraph-structure            | sentence/word limits              |
| MDS025 table-format                   | autofix align and pad             |
| MDS026 table-readability              | width/row heuristics              |
| MDS027 cross-file-reference-integrity | whole-repo link graph             |
| MDS028 token-budget                   | LLM context budget                |
| MDS029 conciseness-scoring            | prose density (experimental)      |
| MDS030 empty-section-body             | no empty sections                 |
| MDS031 unclosed-code-block            | unterminated fence                |
| MDS033 directory-structure            | where files may live              |
| MDS034 markdown-flavor                | restrict to a flavor              |
| MDS035 toc-directive                  | flag stray TOC tokens             |
| MDS036 max-section-length             | per-section size                  |
| MDS037 duplicated-content             | copy-paste across files           |
| MDS038 toc                            | generated heading TOC             |
| MDS039 build                          | artifact-in-sync directive        |
| MDS040 recipe-safety                  | shell-safety on build recipes     |
| MDS043 no-reference-style             | forbid reference links            |
| MDS048 git-hook-sync                  | merge-driver/hook install state   |
| MDS055 forbidden-paragraph-starts     | banned opening phrases            |
| MDS056 forbidden-text                 | banned substrings                 |
| MDS057 required-text-patterns         | mandated patterns                 |
| MDS058 required-mentions              | mandated references               |
