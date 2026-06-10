---
weight: 35
summary: >-
  Every markdownlint rule and the mdsmith rule that covers
  it, generated from the rule README front matter — the same
  data `mdsmith init --from-markdownlint` reads.
---
# markdownlint rule mapping

Each row pairs an mdsmith rule with the markdownlint rule or
rules it covers, ordered by mdsmith rule id. `(partial)`
marks a mapping that covers only part of the markdownlint
check. A markdownlint name follows its id when the two
differ.

The table is generated from the `markdownlint:` front matter
in each rule README. `mdsmith init --from-markdownlint`
reads the same data, so the table and the converter cannot
drift apart. `mdsmith fix` regenerates the table.

<?catalog
source-dir: "internal/rules"
glob: "MDS*/README.md"
where: 'markdownlint: [_, ...]'
sort: id
header: |
  | markdownlint | mdsmith |
  | ------------ | ------- |
row-expr: |
  "| " +
  strings.Join([for m in markdownlint {
    "\(m.id)" +
    [if m.id != m.name {" \(m.name)"},
     if m.id == m.name {""}][0] +
    [if m.partial {" (partial)"},
     if !m.partial {""}][0]
  }], ", ") +
  " | [\(id)](../../internal/rules/\(id)-\(name)/README.md) \(name) |"
?>
| markdownlint                                                                                                                                                       | mdsmith                                                                                                               |
| ------------------------------------------------------------------------------------------------------------------------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------- |
| MD013 line-length                                                                                                                                                  | [MDS001](../../internal/rules/MDS001-line-length/README.md) line-length                                               |
| MD003 heading-style                                                                                                                                                | [MDS002](../../internal/rules/MDS002-heading-style/README.md) heading-style                                           |
| MD001 heading-increment                                                                                                                                            | [MDS003](../../internal/rules/MDS003-heading-increment/README.md) heading-increment                                   |
| MD041 first-line-h1                                                                                                                                                | [MDS004](../../internal/rules/MDS004-first-line-heading/README.md) first-line-heading                                 |
| MD024 no-duplicate-heading                                                                                                                                         | [MDS005](../../internal/rules/MDS005-no-duplicate-headings/README.md) no-duplicate-headings                           |
| MD009 no-trailing-spaces                                                                                                                                           | [MDS006](../../internal/rules/MDS006-no-trailing-spaces/README.md) no-trailing-spaces                                 |
| MD010 no-hard-tabs                                                                                                                                                 | [MDS007](../../internal/rules/MDS007-no-hard-tabs/README.md) no-hard-tabs                                             |
| MD012 no-multiple-blanks                                                                                                                                           | [MDS008](../../internal/rules/MDS008-no-multiple-blanks/README.md) no-multiple-blanks                                 |
| MD047 single-trailing-newline                                                                                                                                      | [MDS009](../../internal/rules/MDS009-single-trailing-newline/README.md) single-trailing-newline                       |
| MD048 code-fence-style                                                                                                                                             | [MDS010](../../internal/rules/MDS010-fenced-code-style/README.md) fenced-code-style                                   |
| MD040 fenced-code-language                                                                                                                                         | [MDS011](../../internal/rules/MDS011-fenced-code-language/README.md) fenced-code-language                             |
| MD034 no-bare-urls                                                                                                                                                 | [MDS012](../../internal/rules/MDS012-no-bare-urls/README.md) no-bare-urls                                             |
| MD022 blanks-around-headings                                                                                                                                       | [MDS013](../../internal/rules/MDS013-blank-line-around-headings/README.md) blank-line-around-headings                 |
| MD032 blanks-around-lists                                                                                                                                          | [MDS014](../../internal/rules/MDS014-blank-line-around-lists/README.md) blank-line-around-lists                       |
| MD031 blanks-around-fences                                                                                                                                         | [MDS015](../../internal/rules/MDS015-blank-line-around-fenced-code/README.md) blank-line-around-fenced-code           |
| MD005 list-indent (partial), MD007 ul-indent                                                                                                                       | [MDS016](../../internal/rules/MDS016-list-indent/README.md) list-indent                                               |
| MD026 no-trailing-punctuation                                                                                                                                      | [MDS017](../../internal/rules/MDS017-no-trailing-punctuation-in-heading/README.md) no-trailing-punctuation-in-heading |
| MD036 no-emphasis-as-heading                                                                                                                                       | [MDS018](../../internal/rules/MDS018-no-emphasis-as-heading/README.md) no-emphasis-as-heading                         |
| MD043 required-headings                                                                                                                                            | [MDS020](../../internal/rules/MDS020-required-structure/README.md) required-structure                                 |
| MD055 table-pipe-style, MD056 table-column-count (partial), MD058 blanks-around-tables                                                                             | [MDS025](../../internal/rules/MDS025-table-format/README.md) table-format                                             |
| MD051 link-fragments                                                                                                                                               | [MDS027](../../internal/rules/MDS027-cross-file-reference-integrity/README.md) cross-file-reference-integrity         |
| MD045 no-alt-text                                                                                                                                                  | [MDS032](../../internal/rules/MDS032-no-empty-alt-text/README.md) no-empty-alt-text                                   |
| MD033 no-inline-html                                                                                                                                               | [MDS041](../../internal/rules/MDS041-no-inline-html/README.md) no-inline-html                                         |
| MD049 emphasis-style, MD050 strong-style                                                                                                                           | [MDS042](../../internal/rules/MDS042-emphasis-style/README.md) emphasis-style                                         |
| MD035 hr-style                                                                                                                                                     | [MDS044](../../internal/rules/MDS044-horizontal-rule-style/README.md) horizontal-rule-style                           |
| MD004 ul-style                                                                                                                                                     | [MDS045](../../internal/rules/MDS045-list-marker-style/README.md) list-marker-style                                   |
| MD029 ol-prefix                                                                                                                                                    | [MDS046](../../internal/rules/MDS046-ordered-list-numbering/README.md) ordered-list-numbering                         |
| MD037 no-space-in-emphasis                                                                                                                                         | [MDS047](../../internal/rules/MDS047-ambiguous-emphasis/README.md) ambiguous-emphasis                                 |
| MD039 no-space-in-links                                                                                                                                            | [MDS049](../../internal/rules/MDS049-no-space-in-link-text/README.md) no-space-in-link-text                           |
| MD044 proper-names                                                                                                                                                 | [MDS050](../../internal/rules/MDS050-proper-names/README.md) proper-names                                             |
| MD025 single-h1                                                                                                                                                    | [MDS051](../../internal/rules/MDS051-single-h1/README.md) single-h1                                                   |
| MD038 no-space-in-code                                                                                                                                             | [MDS052](../../internal/rules/MDS052-no-space-in-code-spans/README.md) no-space-in-code-spans                         |
| MD053 link-image-reference-definitions                                                                                                                             | [MDS053](../../internal/rules/MDS053-no-unused-link-definitions/README.md) no-unused-link-definitions                 |
| MD052 reference-links-images                                                                                                                                       | [MDS054](../../internal/rules/MDS054-no-undefined-reference-labels/README.md) no-undefined-reference-labels           |
| MD027 no-multiple-space-blockquote, MD028 no-blanks-blockquote                                                                                                     | [MDS059](../../internal/rules/MDS059-blockquote-whitespace/README.md) blockquote-whitespace                           |
| MD030 list-marker-space                                                                                                                                            | [MDS061](../../internal/rules/MDS061-list-marker-space/README.md) list-marker-space                                   |
| MD011 no-reversed-links, MD042 no-empty-links                                                                                                                      | [MDS062](../../internal/rules/MDS062-link-validity/README.md) link-validity                                           |
| MD059 descriptive-link-text                                                                                                                                        | [MDS063](../../internal/rules/MDS063-descriptive-link-text/README.md) descriptive-link-text                           |
| MD018 no-missing-space-atx, MD019 no-multiple-space-atx, MD020 no-missing-space-closed-atx (partial), MD021 no-multiple-space-closed-atx, MD023 heading-start-left | [MDS064](../../internal/rules/MDS064-atx-heading-whitespace/README.md) atx-heading-whitespace                         |
| MD046 code-block-style                                                                                                                                             | [MDS065](../../internal/rules/MDS065-code-block-style/README.md) code-block-style                                     |
| MD014 commands-show-output                                                                                                                                         | [MDS066](../../internal/rules/MDS066-commands-show-output/README.md) commands-show-output                             |
| MD054 link-image-style                                                                                                                                             | [MDS068](../../internal/rules/MDS068-link-style/README.md) link-style                                                 |
<?/catalog?>

## See also

- [`mdsmith init`](cli/init.md) —
  `--from-markdownlint` converts a config through this
  mapping.
- [Migrate from markdownlint](../guides/migrate-from-markdownlint.md)
  — the migration workflow.
- [Markdown linters comparison](../background/markdown-linters.md)
  — feature-level comparison across peer linters.
