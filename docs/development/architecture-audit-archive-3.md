---
title: Architecture audit log archive (3)
summary: >-
  Overflow shard for
  architecture-audit-archive-2.md, which hit the
  project's file-length budget. Every finding
  here is resolved; the linked plans are the
  detailed record.
---
# Architecture audit log archive (3)

[The second audit log archive](architecture-audit-archive-2.md)
links here for entries it no longer has room for.
Entries below are moved, not summarized — nothing
was reworded.

## Audit 2026-08-02 (range: 6680ff5..2ab4b29)

148 touched files. Notable new surfaces: MDS073
slide-structure and MDS060 occurrence. MDS073 was
renamed from MDS072; no leftover duplication was
found. Also new: foreign-managed regions and
user-extensible wordlists.

No rule-to-rule imports. No reverse-layer imports. No
Liskov breaks. The prior cycle's `isClaimed` dedup
(plan 2607191918) is confirmed resolved — `schema.IsClaimed`
is now exported and `requiredstructure` calls it directly.

### blockers (2026-08-02)

- `cmd/mdsmith/check.go`'s `runCheck` and
  `cmd/mdsmith/fix.go`'s `runFix` — the `check` and `fix`
  CLI subcommand entry points — had no dedicated in-process
  unit test; only binary-spawn e2e tests
  (`internal/integration`, `cmd/mdsmith/e2e_*_test.go`)
  exercised them.
  [audit-checklist.md][audit-checklist]: "blocker if the
  function is on a public surface ... a CLI subcommand
  entry." Fixed: added `TestRunCheck_UnknownFlag_ExitsTwo`,
  `TestRunCheck_Stdin_ChecksSource`,
  `TestRunCheck_Files_ExitsOneOnDiagnostics`,
  `TestRunCheck_Discovered_ChecksConfiguredFiles`,
  `TestRunFix_UnknownFlag_ExitsTwo`,
  `TestRunFix_StdinArg_ExitsTwo`,
  `TestRunFix_Files_FixesGivenFile`, and
  `TestRunFix_Discovered_FixesConfiguredFiles` to
  `cmd/mdsmith/main_unit_test.go`, matching the
  `TestRunInit_*` precedent from the 2026-07-19 cycle.
  `go test ./cmd/mdsmith/...` and
  `go tool golangci-lint run` are green.

### tax (2026-08-02)

- `cmd/mdsmith/main.go` still carries the `list query`
  subcommand's full domain logic (`parseQueryFlags`,
  `runQuery`, `queryFiles`, `readFrontMatterRaw`) instead
  of a dedicated file, even though `list.go` dispatches to
  it the same way it dispatches to `backlinks.go`.
  [go.md][go] "Clean wiring in `cmd/mdsmith`" —
  [plan/2608021915][2608021915].
- `internal/githooks`'s package doc comment joins three
  responsibilities with "and" (hook-script generation,
  `.gitattributes` I/O, directive-file discovery) —
  [go.md][go] refactor-moves: "Split a package by
  question" — [plan/2608021916][2608021916].
- `internal/lint/files.go`'s CLI-path/glob resolution
  overlaps the question `internal/discovery` already
  answers for config-glob-driven discovery. No plan filed
  this cycle; flagged for the next package-boundary pass.
- A broad set of unexported, branching helper functions
  across the touched rule packages
  (`requiredstructure`, `crossfilereferenceintegrity`,
  `noreferencestyle`, `include`, `linkstyle`,
  `slidevstructure`, `occurrence`, `tablefmt`,
  `tocdirective`, `githooksync`, `build`) and
  `cmd/mdsmith` (`checkFiles`, `checkBatchOptions`,
  `batchMaxBytes`, `parseFixFlags`, `setFixUsage`,
  `fixFiles`, `readStdinLimited`, `regenDirectiveNames`)
  lack a dedicated `TestFuncName` symbol per
  [tests.md][tests], though each is exercised
  transitively through its caller's scenario tests. None
  sit on a public surface by themselves, so tax rather
  than blocker.
- Trivial one-line accessors (`FixTitle` across several
  rule packages, plus a few `rule.Rule` capability
  predicates) carry only an "implements rule.X" comment,
  not the exemption statement [tests.md][tests] requires
  to distinguish "no test by design" from "no test,
  forgotten."
- Similar untested-but-covered helper clusters exist in
  `internal/lsp/server_codeaction.go`,
  `internal/config/merge.go`, and `internal/fix/fix.go`.

[audit-checklist]: architecture/audit-checklist.md
[2608021915]: ../../plan/2608021915_arch-fix-query-subcommand-placement.md

### nice-to-have (2026-08-02)

- Several tests covering the flagged helper clusters use
  scenario names (e.g. `TestDriftParts_ResolvesHooksDirOnce`)
  rather than the literal `TestReceiver_Foo` binding —
  coverage exists, only the naming convention drifts.
- `stagingHelperShellFunc` (`internal/githooks/githooks.go`)
  contains "Helper" in its name — go.md flags this as a
  smell, though the constant is well-scoped. Rename on
  next touch.
- `Override.Patterns()` / `KindAssignmentEntry.Patterns()`
  (`internal/config/config.go`) are two-line branching
  methods exercised only inside larger config tests, not by
  a dedicated `Test*`. Low regression risk.
- `parseCheckFlags` / `parseFixFlags` cross go.md's ~50-line
  guidance for `cmd/mdsmith` handlers, but the body is pure
  flag-registration boilerplate, not domain logic.

[2608021916]: ../../plan/2608021916_arch-fix-githooks-package-split.md
[go]: architecture/go.md
[tests]: architecture/tests.md

## Audit 2026-07-12 (range: 528ce4c..834b560)

66 touched production Go files. No TypeScript changes.

Scope:

- Every `internal/rules/*` package modified since the
  last checkpoint.
- `cmd/mdsmith`, `cmd/mdsmith-release`, `cue/cuelite`.
- A dozen core `internal/*` packages: config,
  convention, fix, index, linkgraph, lint, mdpath,
  mdtext, punkt, release, schema, starter,
  structlayout, wordlist.
- The `pkg/goldmark`, `pkg/markdown/flavor`, and
  `pkg/mdsmith` public-API layer.

No rule-to-rule imports, no reverse-layer imports,
no circular-import tells anywhere in the touched
set.

### blockers (2026-07-12)

- `internal/rules/catalog/rule.go` and
  `internal/rules/build/rule.go` — `RuleID`,
  `RuleName`, `Validate` (both rules), and
  `Generate` (build) implement
  `gensection.Directive`, a generated-markers
  contract surface per CLAUDE.md's cross-system
  list. None had a dedicated unit test. Tests doc
  §"blocker if the function is on a public
  surface." Fixed: added `TestRule_RuleID`,
  `TestRule_RuleName`, `TestRule_Validate` (both
  rules) and `TestRule_Generate` (build); also
  added the same pair for `include/rule.go`'s
  `RuleID`/`RuleName`, which had the identical gap.
- `internal/rules/listindent/rule.go` — `ID()` and
  `Name()` (the `rule.Rule` contract) had no
  dedicated test. Same doc citation. Fixed: added
  `TestID`/`TestName`.
- `internal/index/index.go` — `File()` is the
  LSP/rename/deps query surface's primary lookup
  and had no dedicated test of its hit/miss,
  path-normalization, or copy-not-alias behavior.
  Fixed: added `TestNew`/`TestIndex_File`.
- `internal/linkgraph/wikilinks.go` —
  `NewWikilinkIndex`/`ResolveWikiLink` call
  `fs.WalkDir`, contradicting go.md's "Pure (no
  file reads, no workspace walks)" claim for
  `internal/linkgraph`. Go arch doc §"Single
  responsibility per package" —
  [plan/2607121915][2607121915].

[2607121915]: ../../plan/2607121915_arch-fix-linkgraph-wikilink-purity.md

### tax (2026-07-12)

- Widespread missing-dedicated-test debt on
  unexported helpers across nearly every touched
  rule package (`catalog`, `build`,
  `crossfilereferenceintegrity`, `duplicatedcontent`,
  `firstlineheading`, `headingincrement`, `include`,
  `linelength`, `linkstyle`, `listindent`, `listscan`,
  `noreferencestyle`, `orderedlistnumbering`,
  `propernames`, `requiredfrontmatter`,
  `requiredstructure`, `requiredtextpatterns`,
  `tableformat`, `tokenbudget`) and in `cue/cuelite`,
  `internal/index`, `internal/lint`, `internal/config`,
  `internal/release`, `pkg/mdsmith`, and
  `pkg/goldmark/{ast,internal/fieldorder}`. All are
  exercised transitively through `Check`/`Fix`/
  `ApplySettings` integration-style tests, so behavior
  is covered — only the by-name unit test is missing.
  Candidate for a batched cleanup plan next cycle.
- `internal/release/bench.go` — `fetchTool` and
  `installMarkdownlint` are genuinely untested (not
  just missing a name), despite the file already
  having the fake HTTP/runner seams needed to test
  them.
- `internal/mdtext/upstreampunct.go`'s
  `mdtext_punkt_upstream` build tag is never passed
  in any CI workflow, unlike the analogous
  `goldmark_upstream` tag — silent bit-rot risk for
  `neurosnap/sentences` API drift.
- `internal/config/deepmerge.go`'s `settingMergeMode`
  hardcodes `settingKey == "lists"` instead of going
  through the `rule.ListMerger` extension point built
  for this. Go arch doc §"Interface segregation."
- Cross-cutting duplication, all doing the same
  "recompute line count minus trailing empty split
  element" — `maxfilelength`, `maxsectionlength`,
  `requiredmentions`, `requiredtextpatterns`, and
  `metrics.Document.LineCount()` each carry their own
  copy.
- `internal/rules/crossfilereferenceintegrity/rule.go`'s
  `toStringSlice` is byte-for-byte identical to
  `internal/rules/settings.ToStringSlice`.
- `firstlineheading.headingLevelFromSpan` and
  `headingincrement.atxLevelFromLine`/
  `setextLevelFromSpan` duplicate the same ATX/setext
  leading-space-cap parsing verbatim; no shared home
  in `astutil` yet.
- `requiredfrontmatter.extractYAMLBody` and
  `requiredstructure.extractYAML` are near-identical
  front-matter-fence-stripping helpers, each written
  independently on top of `lint.StripFrontMatter`.
- `internal/fix.Fixer.effectiveWithCategories` and
  `internal/engine.Runner.effectiveWithCategories`
  duplicate the same `config.EffectiveAll` +
  `config.ApplyCategories` pattern; only `Runner`'s
  reuses a shared lookup helper.
- SRP file-size bloat past the point of easy review:
  `internal/rules/requiredstructure/rule.go` (2605
  lines, 5 concerns), `internal/rules/catalog/rule.go`
  (1760 lines, ~6 concerns beyond what the package's
  existing file split already covers),
  `internal/rules/crossfilereferenceintegrity/rule.go`
  (~1130 lines, 3 concerns), `internal/schema/validate.go`
  (1733 lines, 5 concerns), `internal/fix/fix.go` (838
  lines, 5 concerns), `cmd/mdsmith/mergedriver.go` (905
  lines, 4 concerns), `cmd/mdsmith/main.go` (911 lines,
  nearing the ~1000-line smell threshold, with real
  domain logic that could move to `internal/*`),
  `internal/release/bench.go` (700 lines, 5 concerns).
  None are DIP violations; all are readability/review-cost
  friction, split candidates along the seams each
  finding's source agent already named.
- `overlay.go`'s `cleanKey` path-normalizer is
  reimplemented inline five times in `MemWorkspace`
  (`workspace.go`) instead of shared.

### nice-to-have (2026-07-12)

- `pkg/mdsmith`'s `OSWorkspace.Glob` ignores `Root`
  while `OverlayWorkspace.Glob` joins against it —
  both documented individually, worth one line on the
  `Workspace` interface noting `Glob`'s
  root-relativity is implementation-defined.
- `internal/mdtext/mdtext.go`'s `CountSentences` (fast
  heuristic) and `SplitSentences` (trained Punkt) both
  exist with no doc cross-reference explaining the
  trade-off, unlike the file's other paired functions.
- `internal/rules/catalog/rule.go`'s `isGitignored` is
  dead in production, kept only as a test oracle for
  `isGitignoredMemo` — undocumented as intentional.
- `internal/rules/requiredstructure/rule.go` has two
  same-named-in-spirit symbols, package func
  `isSchemaFile` and method `(*Rule).isSchemaFile` —
  naming clash, not a coverage gap.
