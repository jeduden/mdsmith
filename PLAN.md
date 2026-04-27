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
| 94  | 🔲     | sonnet | [Lint-once for `<?include?>` and `<?catalog?>` embeds](plan/94_lint-once-for-embeds.md)              |
| 95  | 🔲     | opus   | [Kind/rule resolution observability via `kinds` subcommand](plan/95_kind-rule-resolution-cli.md)     |
| 96  | 🔲     | sonnet | [Adopt kinds in mdsmith repo and ship the docs](plan/96_kinds-adoption-and-docs.md)                  |
| 97  | 🔲     | opus   | [Deep-merge for kinds and overrides](plan/97_deep-merge-config.md)                                   |
| 98  | 🔲     | sonnet | [Replace `archetypes` with `kinds`](plan/98_replace-archetypes-with-kinds.md)                        |
<?/catalog?>
