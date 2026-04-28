# Plans

<?catalog
glob:
  - "plan/*.md"
  - "!plan/proto.md"
sort: id
header: |

  | ID  | Status | Model | Title |
  |-----|--------|-------|-------|
row: "| {id} | {status} | {model} | [{title}]({filename}) |"
footer: |

?>

| ID  | Status | Model  | Title                                                                                                |
|-----|--------|--------|------------------------------------------------------------------------------------------------------|
| 100 | 🔲     | sonnet | [build config block and MDS040 recipe-safety rule](plan/100_build-config-and-mds040.md)              |
| 101 | 🔲     | sonnet | [build directive and MDS039 lint rule](plan/101_build-directive-mds039.md)                           |
| 102 | 🔲     | opus   | [Builder interface and mdsmith build subcommand](plan/102_build-subcommand.md)                       |
| 103 | 🔲     | opus   | [Build target staleness and dependency tracking](plan/103_build-staleness-and-deps.md)               |
| 104 | 🔲     | sonnet | [Build lifecycle hooks (before/after)](plan/104_build-lifecycle-hooks.md)                            |
| 105 | 🔲     | sonnet | [No inline HTML rule](plan/105_no-inline-html.md)                                                    |
| 106 | 🔲     | sonnet | [Emphasis style rule](plan/106_emphasis-style.md)                                                    |
| 107 | 🔲     | opus   | [No reference-style links rule](plan/107_no-reference-style.md)                                      |
| 108 | 🔲     | sonnet | [Horizontal rule style rule](plan/108_horizontal-rule-style.md)                                      |
| 109 | 🔲     | sonnet | [List marker style rule](plan/109_list-marker-style.md)                                              |
| 110 | 🔲     | sonnet | [Ordered list numbering rule](plan/110_ordered-list-numbering.md)                                    |
| 111 | 🔲     | sonnet | [Ambiguous emphasis rule](plan/111_ambiguous-emphasis.md)                                            |
| 112 | 🔲     | opus   | [Flavor profiles refactor](plan/112_flavor-profiles.md)                                              |
| 113 | 🔲     | sonnet | [User-defined flavor profiles](plan/113_user-defined-profiles.md)                                    |
| 120 | 🔲     | sonnet | [Unify glob matcher and field naming across mdsmith](plan/120_glob-unification.md)                   |
| 52  | ✅     |        | [Archetype / Template Library for Agentic Patterns](plan/52_archetype-template-library.md)           |
| 61  | ✅     |        | [Required Structure Rule Hardening](plan/61_required-structure-hardening.md)                         |
| 65  | ✅     |        | [Spike WASM-Embedded Weasel Inference](plan/65_spike-wasm-embedded-inference.md)                     |
| 78  | ✅     |        | [Query subcommand for front-matter filtering](plan/78_query-command.md)                              |
| 83  | ✅     |        | [Security hardening batch](plan/83_security-hardening-batch.md)                                      |
| 84  | ✅     |        | [Symlink default-deny for file discovery](plan/84_symlink-default-deny.md)                           |
| 85  | ✅     |        | [Increase test coverage to 95% by extracting shared rule helpers](plan/85_coverage-to-95-percent.md) |
| 86  | ✅     |        | [Markdown flavor validation](plan/86_markdown-flavor-validation.md)                                  |
| 89  | ✅     |        | [TOC generator directive and MDS035 auto-fix](plan/89_toc-generator-directive.md)                    |
| 90  | ✅     |        | [Isolate corpus test git config from host signing](plan/90_corpus-test-git-config-isolation.md)      |
| 91  | ✅     |        | [MDS037 skips paragraphs inside generated sections](plan/91_mds037-skip-generated-sections.md)       |
| 92  | ✅     | sonnet | [File kinds — config schema, assignment, merge](plan/92_file-kinds.md)                               |
| 93  | ✅     | sonnet | [Placeholder grammar — opt-in token vocabulary](plan/93_placeholder-grammar.md)                      |
| 94  | ✅     | sonnet | [Lint-once for `<?include?>` and `<?catalog?>` embeds](plan/94_lint-once-for-embeds.md)              |
| 95  | ✅     | opus   | [Kind/rule resolution observability via `kinds` subcommand](plan/95_kind-rule-resolution-cli.md)     |
| 96  | ✅     | sonnet | [Adopt kinds in mdsmith repo and ship the docs](plan/96_kinds-adoption-and-docs.md)                  |
| 97  | ✅     | opus   | [Deep-merge for kinds and overrides](plan/97_deep-merge-config.md)                                   |
| 98  | 🔳     | sonnet | [Replace `archetypes` with `kinds`](plan/98_replace-archetypes-with-kinds.md)                        |
<?/catalog?>
