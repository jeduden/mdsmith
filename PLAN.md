# Plans

<?catalog
glob:
  - "plan/*.md"
  - "!plan/proto.md"
sort: id
header: |

  | ID  | Status | Title |
  |-----|--------|-------|
row: "| {id} | {status} | [{title}]({filename}) |"
footer: |

?>

| ID  | Status | Title                                                                                                |
|-----|--------|------------------------------------------------------------------------------------------------------|
| 52  | ✅     | [Archetype / Template Library for Agentic Patterns](plan/52_archetype-template-library.md)           |
| 61  | 🔳     | [Required Structure Rule Hardening](plan/61_required-structure-hardening.md)                         |
| 65  | ✅     | [Spike WASM-Embedded Weasel Inference](plan/65_spike-wasm-embedded-inference.md)                     |
| 78  | ✅     | [Query subcommand for front-matter filtering](plan/78_query-command.md)                              |
| 83  | ✅     | [Security hardening batch](plan/83_security-hardening-batch.md)                                      |
| 84  | 🔲     | [Symlink default-deny for file discovery](plan/84_symlink-default-deny.md)                           |
| 85  | 🔳     | [Increase test coverage to 95% by extracting shared rule helpers](plan/85_coverage-to-95-percent.md) |
| 86  | 🔳     | [Markdown flavor validation](plan/86_markdown-flavor-validation.md)                                  |
| 89  | 🔲     | [TOC generator directive and MDS035 auto-fix](plan/89_toc-generator-directive.md)                    |
| 90  | ✅     | [Isolate corpus test git config from host signing](plan/90_corpus-test-git-config-isolation.md)      |
| 91  | 🔲     | [MDS037 skips paragraphs inside generated sections](plan/91_mds037-skip-generated-sections.md)       |
<?/catalog?>
