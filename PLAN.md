# Plans

<?catalog
glob:
  - "plan/*.md"
  - "!plan/proto.md"
sort: numeric:id
header: |

  | ID  | Status | Model | Title |
  |-----|--------|-------|-------|
row: "| {id} | {status} | {model} | [{title}]({filename}) |"
footer: |

?>

| ID         | Status | Model  | Title                                                                                                                                   |
| ---------- | ------ | ------ | --------------------------------------------------------------------------------------------------------------------------------------- |
| 52         | ✅     |        | [Archetype / Template Library for Agentic Patterns](plan/52_archetype-template-library.md)                                              |
| 61         | ✅     |        | [Required Structure Rule Hardening](plan/61_required-structure-hardening.md)                                                            |
| 65         | ✅     |        | [Spike WASM-Embedded Weasel Inference](plan/65_spike-wasm-embedded-inference.md)                                                        |
| 78         | ✅     |        | [Query subcommand for front-matter filtering](plan/78_query-command.md)                                                                 |
| 83         | ✅     |        | [Security hardening batch](plan/83_security-hardening-batch.md)                                                                         |
| 84         | ✅     |        | [Symlink default-deny for file discovery](plan/84_symlink-default-deny.md)                                                              |
| 85         | ✅     |        | [Increase test coverage to 95% by extracting shared rule helpers](plan/85_coverage-to-95-percent.md)                                    |
| 86         | ✅     |        | [Markdown flavor validation](plan/86_markdown-flavor-validation.md)                                                                     |
| 89         | ✅     |        | [TOC generator directive and MDS035 auto-fix](plan/89_toc-generator-directive.md)                                                       |
| 90         | ✅     |        | [Isolate corpus test git config from host signing](plan/90_corpus-test-git-config-isolation.md)                                         |
| 91         | ✅     |        | [MDS037 skips paragraphs inside generated sections](plan/91_mds037-skip-generated-sections.md)                                          |
| 92         | ✅     | sonnet | [File kinds — config schema, assignment, merge](plan/92_file-kinds.md)                                                                  |
| 93         | ✅     | sonnet | [Placeholder grammar — opt-in token vocabulary](plan/93_placeholder-grammar.md)                                                         |
| 94         | ✅     | sonnet | [Lint-once for `<?include?>` and `<?catalog?>` embeds](plan/94_lint-once-for-embeds.md)                                                 |
| 95         | ✅     | opus   | [Kind/rule resolution observability via `kinds` subcommand](plan/95_kind-rule-resolution-cli.md)                                        |
| 96         | ✅     | sonnet | [Adopt kinds in mdsmith repo and ship the docs](plan/96_kinds-adoption-and-docs.md)                                                     |
| 97         | ✅     | opus   | [Deep-merge for kinds and overrides](plan/97_deep-merge-config.md)                                                                      |
| 98         | ✅     | sonnet | [Replace `archetypes` with `kinds`](plan/98_replace-archetypes-with-kinds.md)                                                           |
| 100        | ✅     | sonnet | [build config block and MDS040 recipe-safety rule](plan/100_build-config-and-mds040.md)                                                 |
| 101        | ✅     | sonnet | [build directive and MDS039 lint rule](plan/101_build-directive-mds039.md)                                                              |
| 102        | ✅     | opus   | [Multi-output `<?build?>` directive](plan/102_build-subcommand.md)                                                                      |
| 103        | 🔲     | opus   | [Build target staleness and dependency tracking](plan/103_build-staleness-and-deps.md)                                                  |
| 104        | 🔲     | sonnet | [Build lifecycle hooks (before/after)](plan/104_build-lifecycle-hooks.md)                                                               |
| 105        | ✅     | sonnet | [No inline HTML rule](plan/105_no-inline-html.md)                                                                                       |
| 106        | ✅     | sonnet | [Emphasis style rule](plan/106_emphasis-style.md)                                                                                       |
| 107        | ✅     | opus   | [No reference-style links rule](plan/107_no-reference-style.md)                                                                         |
| 108        | ✅     | sonnet | [Horizontal rule style rule](plan/108_horizontal-rule-style.md)                                                                         |
| 109        | ✅     | sonnet | [List marker style rule](plan/109_list-marker-style.md)                                                                                 |
| 110        | ✅     | sonnet | [Ordered list numbering rule](plan/110_ordered-list-numbering.md)                                                                       |
| 111        | ✅     | sonnet | [Ambiguous emphasis rule](plan/111_ambiguous-emphasis.md)                                                                               |
| 112        | ✅     | opus   | [Markdown convention bundles for MDS034](plan/112_flavor-profiles.md)                                                                   |
| 113        | ✅     | sonnet | [User-defined Markdown conventions](plan/113_user-defined-profiles.md)                                                                  |
| 114        | ✅     | sonnet | [MDS034 message clarity and flavor-vs-rule docs](plan/114_mds034-message-and-flavor-vs-rule-docs.md)                                    |
| 120        | ✅     | sonnet | [Unify glob matcher and field naming across mdsmith](plan/120_glob-unification.md)                                                      |
| 121        | ✅     | opus   | [Expose mdsmith to VS Code via Language Server Protocol](plan/121_vscode-integration.md)                                                |
| 122        | ✅     | sonnet | [VS Code palette commands](plan/122_vscode-hover-and-palette.md)                                                                        |
| 124        | ✅     | sonnet | [No space inside code spans rule](plan/124_no-space-in-code-spans.md)                                                                   |
| 125        | ✅     | sonnet | [No space inside link text rule](plan/125_no-space-in-link-text.md)                                                                     |
| 126        | ✅     | sonnet | [Proper-name capitalization rule](plan/126_proper-names.md)                                                                             |
| 127        | ✅     | sonnet | [Single H1 per file rule](plan/127_single-h1.md)                                                                                        |
| 128        | ✅     | sonnet | [Reject undefined reference-link labels](plan/128_no-undefined-reference-labels.md)                                                     |
| 129        | ✅     | sonnet | [Flag unused or duplicate link reference definitions](plan/129_no-unused-link-definitions.md)                                           |
| 130        | ✅     | opus   | [Distribute mdsmith binaries via npm, PyPI, and the VS Code marketplaces](plan/130_binary-distribution-and-versioning.md)               |
| 131        | ✅     | opus   | [LSP symbol navigation for agents (Claude)](plan/131_lsp-symbol-navigation.md)                                                          |
| 132        | ✅     | sonnet | [Package mdsmith LSP as a Claude Code plugin](plan/132_claude-code-plugin.md)                                                           |
| 133        | ✅     | sonnet | [LSP hover for rule and directive docs](plan/133_lsp-hover.md)                                                                          |
| 134        | ✅     | sonnet | [LSP completion for anchors, refs, kinds, and directive args](plan/134_lsp-completion.md)                                               |
| 135        | ✅     | sonnet | [Schema inheritance via `extends`](plan/135_schema-extends.md)                                                                          |
| 136        | ✅     | sonnet | [Field deprecation flag in schemas](plan/136_field-deprecation-flag.md)                                                                 |
| 137        | ✅     | sonnet | [`mdsmith fix --dry-run`](plan/137_fix-dry-run.md)                                                                                      |
| 138        | ✅     | sonnet | [`mdsmith list backlinks` subcommand](plan/138_backlinks-subcommand.md)                                                                 |
| 139        | ✅     | sonnet | [Field-presence kind assignment](plan/139_field-presence-kind-assignment.md)                                                            |
| 140        | ✅     | sonnet | [Per-kind `path-pattern` for filename validation](plan/140_kind-path-pattern.md)                                                        |
| 142        | ✅     | sonnet | [Content rules for prose constraints](plan/142_schema-content-constraints.md)                                                           |
| 143        | ✅     | sonnet | [Schema cross-references, acronyms, and index](plan/143_schema-cross-refs-acronyms-index.md)                                            |
| 144        | ✅     | sonnet | [Numeric sort for `<?catalog?>` directive](plan/144_catalog-numeric-sort.md)                                                            |
| 145        | 🔳     | opus   | [Publish mdsmith via asdf and mise registry submissions](plan/145_asdf-mise-registry-submissions.md)                                    |
| 146        | ✅     | opus   | [Schema engine — sources, scope tree, per-scope rules](plan/146_inline-schema-in-kinds.md)                                              |
| 147        | ✅     | opus   | [Actionable schema diagnostics for MDS020](plan/147_actionable-schema-diagnostics.md)                                                   |
| 148        | ✅     | sonnet | [Named field-type shortcuts for inline schemas](plan/148_named-field-type-shortcuts.md)                                                 |
| 149        | ✅     | opus   | [Section content schema for non-heading AST nodes](plan/149_section-content-schema.md)                                                  |
| 151        | ✅     | opus   | [LSP rename for headings and link-reference labels](plan/151_lsp-rename.md)                                                             |
| 153        | ✅     | opus   | [Unify linkgraph and the LSP symbol index](plan/153_unify-linkgraph-and-lsp-index.md)                                                   |
| 154        | ✅     | sonnet | [arch-fix: extract cross-rule helpers](plan/154_arch-fix-rule-helper-extraction.md)                                                     |
| 155        | ✅     | sonnet | [arch-fix: relocate convention types out of markdownflavor](plan/155_arch-fix-convention-config-ownership.md)                           |
| 156        | ✅     | opus   | [Composable required-structure schemas across multiple kinds](plan/156_kind-schema-composition.md)                                      |
| 157        | ✅     | sonnet | [Catalog filter by front matter property](plan/157_catalog-where-filter.md)                                                             |
| 160        | ✅     | sonnet | [Claude Code plugin extensions — skills, agents, hooks](plan/160_claude-code-skills-agents-hooks.md)                                    |
| 161        | ✅     | sonnet | [Expose rule maintainability patterns via CLI help and LSP](plan/161_rule-pattern-metadata.md)                                          |
| 162        | ✅     | sonnet | [Split the overloaded `meta` rule category](plan/162_rule-category-cleanup.md)                                                          |
| 163        | ✅     |        | [Extract mdsmith Markdown parse/produce as a public Go library](plan/163_public-markdown-library.md)                                    |
| 164        | ✅     |        | [GitHub-UI-triggered releases and a split website deploy](plan/164_github-ui-releases-and-split-website.md)                             |
| 165        | ✅     | opus   | [Portable Markdown export (mdsmith export)](plan/165_portable-markdown-export.md)                                                       |
| 166        | ✅     | opus   | [Schema-driven data extraction (mdsmith extract)](plan/166_schema-driven-data-extraction.md)                                            |
| 167        | ✅     | opus   | [Custom binding overrides for mdsmith extract](plan/167_custom-binding-overrides.md)                                                    |
| 168        | ✅     | sonnet | [Obsidian Flavored Markdown support](plan/168_obsidian-markdown-support.md)                                                             |
| 169        | ✅     | opus   | [Enforce terminal Meta-Information and render it from frontmatter](plan/169_rule-readme-meta-information-sync.md)                       |
| 170        | ✅     | opus   | [Audit link handling across mdsmith and the website](plan/170_link-handling-audit.md)                                                   |
| 171        | ✅     | opus   | [MDS027 link-integrity hardening](plan/171_mds027-link-integrity-hardening.md)                                                          |
| 172        | ✅     | opus   | [Link-style rule and shared links config](plan/172_link-style-rule-and-config.md)                                                       |
| 173        | ✅     | sonnet | [Website rewriter tolerates titled links](plan/173_rewriter-titled-links.md)                                                            |
| 174        | ✅     | opus   | [Expose rename and dependency-graph as CLI subcommands and feature docs](plan/174_expose-rename-and-deps-cli.md)                        |
| 175        | ✅     | opus   | [CI performance gate for mdsmith check, modelled on the LSP latency gate](plan/175_check-performance-gate.md)                           |
| 176        | ✅     | sonnet | [ATX heading whitespace and indentation rule](plan/176_atx-heading-whitespace.md)                                                       |
| 177        | ✅     | sonnet | [Blockquote whitespace rule](plan/177_blockquote-whitespace.md)                                                                         |
| 178        | ✅     | sonnet | [List marker space rule](plan/178_list-marker-space.md)                                                                                 |
| 179        | ✅     | opus   | [Reversed and empty link rule](plan/179_link-validity.md)                                                                               |
| 180        | ✅     | sonnet | [Descriptive link text rule](plan/180_descriptive-link-text.md)                                                                         |
| 181        | ✅     | opus   | [Table structure rules](plan/181_table-structure.md)                                                                                    |
| 182        | ✅     | sonnet | [Code block convention rules](plan/182_code-block-conventions.md)                                                                       |
| 183        | ✅     | sonnet | [Skip DedupeDiagnostics via an audited rule.RepoScoped marker](plan/183_dedupe-diagnostics-repo-scoped-skip.md)                         |
| 184        | ✅     | opus   | [Automate the cross-tool benchmark on merge to main and publish numbers to the assets branch](plan/184_release-benchmark-automation.md) |
| 185        | ✅     |        | [Expose extended-syntax parsers and the flavor model in pkg/markdown](plan/185_public-markdown-flavor-library.md)                       |
| 186        | ✅     |        | [Centralize UTF-16 column helpers in internal/mdtext](plan/186_arch-fix-utf16-centralize.md)                                            |
| 187        | ✅     | opus   | [Neutral-corpus engine lever — shared AST walk and Punkt cost](plan/187_neutral-corpus-engine-lever.md)                                 |
| 188        | ✅     | opus   | [Regex-over-source rules — inventory and AST-resident replacements](plan/188_regex-vs-ast-inventory.md)                                 |
| 189        | ✅     | opus   | [Finish the multiplex migration for pure per-node rules](plan/189_multiplex-finish.md)                                                  |
| 190        | ✅     | opus   | [Intra-file rule parallelism for non-NodeChecker rules](plan/190_intra-file-rule-parallelism.md)                                        |
| 191        | ✅     | opus   | [Hand-rolled DFA for Punkt's `reAbbr` to skip regex backtracking](plan/191_punkt-reabbr-dfa.md)                                         |
| 192        | ✅     | opus   | [Run-scoped read cache for catalog cross-host redundancy](plan/192_catalog-run-scoped-readcache.md)                                     |
| 193        | ✅     | opus   | [Rework MDS024 to fit the per-rule allocation budget (≤ 10 allocs/op)](plan/193_mds024-allocation-budget.md)                            |
| 194        | ✅     | opus   | [Frontpage persona audit — reduce AI-first framing, surface non-AI path](plan/194_frontpage-persona-audit.md)                           |
| 195        | ✅     | opus   | [Enforce the ≤ 10 allocs/op per-rule budget across every registered rule](plan/195_per-rule-alloc-budget.md)                            |
| 196        | ✅     | opus   | [Lazy SectionParagraph text — defer ExtractPlainText until a caller asks](plan/196_lazy-section-paragraph-text.md)                      |
| 197        | ✅     | opus   | [PoC — review goldmark's allocation architecture, then pool the best lever](plan/197_fork-goldmark-for-allocs.md)                       |
| 198        | ✅     | opus   | [Fork goldmark with a per-parse arena for the four structural allocators](plan/198_goldmark-arena-fork.md)                              |
| 200        | ✅     |        | [Move docs/ embed out of internal/lsp/hover.go](plan/200_arch-fix-hover-embed.md)                                                       |
| 201        | ✅     |        | [Rename internal/testutil to internal/testsymlink](plan/201_arch-fix-testutil-rename.md)                                                |
| 202        | ✅     |        | [Split cmd/mdsmith/main.go into per-subcommand files](plan/202_arch-fix-main-split.md)                                                  |
| 203        | ✅     |        | [Split internal/lsp/server.go and symbols.go](plan/203_arch-fix-lsp-server-split.md)                                                    |
| 204        | ✅     |        | [Fix internal/fix importing internal/engine](plan/204_arch-fix-fix-engine-inversion.md)                                                 |
| 205        | ✅     |        | [Move extension.ts concerns to wiring.ts](plan/205_arch-fix-extension-ts-srp.md)                                                        |
| 206        | ✅     |        | [Document cue/ in architecture layering map](plan/206_arch-fix-cue-types-docs.md)                                                       |
| 207        | ✅     | sonnet | [LSP fix preview via ChangeAnnotation](plan/207_lsp-fix-preview.md)                                                                     |
| 208        | ✅     | opus   | [Kind-per-file config under `.mdsmith/kinds/`](plan/208_kind-files.md)                                                                  |
| 209        | ✅     | opus   | [Convention-per-file config under `.mdsmith/conventions/`](plan/209_convention-files.md)                                                |
| 210        | ✅     | opus   | [Single source of truth for product messaging via `mdsmith extract`](plan/210_messaging-source-of-truth.md)                             |
| 211        | ✅     | opus   | [`<?include?>` projects any typed value of any kind via `extract`](plan/211_include-extract-value.md)                                   |
| 212        | ✅     | opus   | [`mdsmith extract` projects paragraph inline spans as data](plan/212_extract-inline-spans.md)                                           |
| 213        | ✅     | opus   | [Built-in `no-llm-tells` convention with append-mode forbidden lists](plan/213_anti-slop-convention.md)                                 |
| 214        | ⛔     | opus   | [Obsidian plugin via hand-rolled LSP bridge](plan/214_obsidian-plugin.md)                                                               |
| 215        | ✅     | opus   | [mdsmith public engine API and WASM bindings](plan/215_engine-api-wasm.md)                                                              |
| 216        | ✅     | opus   | [Per-document parse cache for the LSP, keyed by version](plan/216_lsp-parse-cache.md)                                                   |
| 217        | ✅     | opus   | [Obsidian plugin (WASM runtime)](plan/217_obsidian-plugin.md)                                                                           |
| 218        | 🔳     | opus   | [In-house CUE-subset engine for WASM size and tinygo](plan/218_wasm-size-reduction.md)                                                  |
| 219        | ✅     | opus   | [Route cmd/mdsmith and the LSP through pkg/mdsmith.Session](plan/219_session-cli-lsp-migration.md)                                      |
| 220        | ✅     | opus   | [Harden the git-index writers against a transient index.lock](plan/220_git-index-lock-retry.md)                                         |
| 221        | ✅     | opus   | [Ship mdsmith as a self-hosted Flatpak bundle](plan/221_flatpak-bundle-distribution.md)                                                 |
| 222        | ✅     | opus   | [Single source of truth for distribution channels](plan/222_distribution-channel-ssot.md)                                               |
| 223        | ✅     |        | [Add unit tests for pkg/mdsmith private helpers](plan/223_arch-fix-mdsmith-helper-tests.md)                                             |
| 224        | ✅     |        | [Split internal/lint along question boundaries](plan/224_arch-fix-lint-srp.md)                                                          |
| 225        | ✅     |        | [Add internal/punkt to the architecture layering map](plan/225_arch-fix-punkt-layering.md)                                              |
| 230        | ✅     | opus   | [Navigable schema diagnostics in the editor](plan/230_navigable-schema-diagnostics.md)                                                  |
| 231        | ✅     |        | [LSP newest-wins workspace singleton](plan/231_lsp-workspace-singleton.md)                                                              |
| 232        | ✅     | opus   | [include heading-offset parameter](plan/232_include-heading-offset.md)                                                                  |
| 233        | ✅     | opus   | [numeric heading-level target for include](plan/233_include-heading-level-target.md)                                                    |
| 234        | 🔳     | sonnet | [Distribute mdsmith on Windows via Scoop and WinGet](plan/234_windows-package-managers.md)                                              |
| 235        | ✅     | sonnet | [Playwright end-to-end tests for the website, runnable by CI and agents](plan/235_playwright-site-e2e.md)                               |
| 236        | ✅     | opus   | [cuelite phase 0 — package, façade, and differential harness](plan/236_cuelite-package-harness.md)                                      |
| 237        | ✅     | sonnet | [cuelite phase 1 — surface D (placeholder paths)](plan/237_cuelite-surface-d.md)                                                        |
| 238        | ✅     | opus   | [cuelite phase 2 — surfaces A + B (schema, query)](plan/238_cuelite-surfaces-ab.md)                                                     |
| 239        | ✅     | opus   | [cuelite phase 3 — surface C (row-expr evaluator)](plan/239_cuelite-surface-c.md)                                                       |
| 240        | 🔳     | opus   | [cuelite phase 4 — drop cuelang.org and enable tinygo](plan/240_cuelite-drop-cue.md)                                                    |
| 241        | ✅     | opus   | [Schema-per-file config under `.mdsmith/schemas/`](plan/241_schema-files.md)                                                            |
| 242        | ✅     | opus   | [proto.md schemas declare content entries via `<?content?>`](plan/242_proto-content-entries.md)                                         |
| 243        | ✅     | sonnet | [`mdsmith extract` projects the document H1 as `title`](plan/243_extract-h1-title.md)                                                   |
| 244        | ✅     | opus   | [Structured list projection; fix nested-item text corruption](plan/244_structured-list-projection.md)                                   |
| 245        | ✅     | sonnet | [Table projection modes: `records` and `rows`](plan/245_table-projection-modes.md)                                                      |
| 246        | ✅     | opus   | [Typed block projection and full-document extract](plan/246_block-projection-full-extract.md)                                           |
| 2606022122 | ✅     | sonnet | [Review and centralize YAML handling](plan/2606022122_yaml-handling-review.md)                                                          |
| 2606022123 | ✅     | sonnet | [Catalog directive — accept `..` globs within project root](plan/2606022123_catalog-dotdot-globs.md)                                    |
| 2606022124 | ✅     | opus   | [Section schema — unify entry shape under `heading:` discriminator](plan/2606022124_schema-entry-unification.md)                        |
| 2606022125 | ✅     | sonnet | [MDS019 catalog: CUE-expression row templates](plan/2606022125_catalog-cue-row-expressions.md)                                          |
| 2606022126 | ✅     | opus   | [Audit AST-walking rules and rewrite the ones that only need f.Lines](plan/2606022126_lines-only-rule-audit.md)                         |
| 2606022127 | ✅     | sonnet | [Finish MD054 link-image-style coverage in MDS068](plan/2606022127_finish-md054-link-image-style.md)                                    |
| 2606022128 | ✅     | opus   | [Multiplexed AST walk to close the parity gap to mado](plan/2606022128_multiplexed-ast-walk.md)                                         |
| 2606071930 | ✅     | sonnet | [Consolidate duplicated table-parse helpers in tablereadability](plan/2606071930_arch-fix-tablereadability-dup.md)                      |
| 2606071931 | ✅     | haiku  | [Unit tests for include-rule private validation helpers](plan/2606071931_arch-fix-include-helper-tests.md)                              |
| 2606092208 | ✅     | sonnet | [Recover from panics in the LSP lint pipeline](plan/2606092208_lsp-panic-recovery.md)                                                   |
| 2606092209 | ✅     | sonnet | [Security hardening batch — 2026-06-09 audit](plan/2606092209_secreview-2026-06-09-hardening.md)                                        |
| 2606100533 | ✅     | sonnet | [Coordination-free plan ids from UTC creation timestamps](plan/2606100533_timestamp-plan-ids.md)                                        |
| 2606100534 | ✅     | sonnet | [Workspace-unique front-matter fields (unique-frontmatter rule)](plan/2606100534_unique-frontmatter-rule.md)                            |
| 2606101546 | 🔲     | opus   | [Builder execution wired into `mdsmith fix`](plan/2606101546_builder-execution-in-fix.md)                                               |
| 2606101547 | 🔲     | opus   | [Build execution UX (stdout/stderr, debug, parallel)](plan/2606101547_build-execution-ux.md)                                            |
| 2606101548 | 🔲     | opus   | [Build execution hardening](plan/2606101548_build-execution-hardening.md)                                                               |
| 2606110517 | 🔲     | sonnet | [tinygo wasm build under the 8 MiB budget](plan/247_tinygo-wasm-build.md)                                                               |
| 2606110639 | ✅     |        | [Audience split for website-published docs](plan/2606110639_website-docs-audience-split.md)                                             |
<?/catalog?>
