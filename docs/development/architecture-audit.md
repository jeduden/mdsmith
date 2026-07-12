---
title: Architecture audit log
summary: >-
  Running log of SOLID and clean-architecture
  findings on origin/main. The
  solid-architecture skill (audit mode)
  appends here; blockers are also filed as
  plans.
audit-from: 834b56092d85c815c01bd6a8e4d955f43396ae3e
---
# Architecture audit log

This file is maintained by the
solid-architecture skill in audit mode.
Entries from 2026-04-28 through 2026-06-24 moved
to [the archive](architecture-audit-archive.md)
this cycle to stay under the file-length budget;
every finding there is resolved.

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

## Audit 2026-06-26 (range: 3d35b77..fe7141b)

Go 1.25.11 + x/net CVE bumps; five perf
fixes (map→struct, fmt→strconv); type-6 tag
gap fix; plan-2606241814/15 test additions.
No new production functions, DIP, SRP, or
line-count violations.
Plans 2606260211, 2606260614, 2606260615 green.

### tax (2026-06-26)

- `internal/lint/layer0_html.go` — seven
  helpers lack dedicated tests. File entered
  the touched set via a perf commit. Tests doc
  §"every function by name" —
  [plan/2606260211][2606260211].

- `internal/lint/lineclass_scan.go` — 9
  unexported helpers lack tests.
  Sub-functions of `htmlType7Start` —
  [plan/2606260614][2606260614].

- `cue/cuelite/engine.go` — 7 unexported
  helpers lack tests —
  [plan/2606260615][2606260615].

[2606260211]: ../../plan/2606260211_arch-fix-layer0-html-helper-tests.md
[2606260614]: ../../plan/2606260614_arch-fix-lineclass-scan-helper-tests.md
[2606260615]: ../../plan/2606260615_arch-fix-cuelite-engine-helper-tests.md

## Audit 2026-06-26 (range: fe7141b..0ededb3)

Three test files added. No production
sources changed. No new functions,
DIP violations, SRP breaches, or
line-count crossings. Plans 2606260211,
2606260614, and 2606260615 closed.

No blockers, tax, or nice-to-have.

## Audit 2026-06-28 (range: 0ededb3..82583fc)

Plan 219 routes `cmd/mdsmith` and the LSP
through `pkg/mdsmith.Session`. Five perf
hot-path commits land. Plan 233 adds a
numeric heading-level option to `<?include?>`.
Plan 217 ships the Obsidian plugin WASM
runtime. VS Code and LSP TypeScript fixes.
479 Go sources; 26 TypeScript sources outside
fixtures.

No blockers. No rule-to-rule DIP violations.
No SRP breaches. No file crossed 1 000 lines.
All new production functions have dedicated
unit tests.

## Audit 2026-07-05 (range: 0ededb3..528ce4c)

Prior entry's range end (`82583fc`) predates
`0ededb3` and isn't its ancestor — a mislabel, not
a real rewind. Resumed from `0ededb3` instead; front
matter now points at this audit's end commit.

91 commits; 39 touching Go/TS (word-lists,
no-llm-tells, reflow auto-fix, perf passes).

Blocker:

- `linelength/reflow.go` imported `internal/punkt`
  directly. Only `internal/mdtext` may, per
  [go.md][go].
- It built a second trained Punkt `Storage`, so
  `reflow: true` parsed `english.json` twice.
- Fixed: added `mdtext.IsAbbrevToken` in a new,
  build-tag-independent `abbrev.go`, so both the
  fork and upstream builds share one `Storage`.
  `fastpunct_init.go`'s `forkTokenizer` now builds
  from that same `Storage` too. Exported
  `punkt.AbbrevTokenCutset` and re-exported it as
  `mdtext.AbbrevTrimCutset`, so `reflow.go` no
  longer keeps its own copy of that punkt-owned
  constant. `reflow.go` calls
  `mdtext.IsAbbrevToken` instead of loading its own
  model.

[go]: architecture/go.md

### tax (2026-07-05)

- `wordlist.dedup` / `config`'s `dedupStrings`, and
  `config`'s `stringsToAny` / `convention`'s
  `toAnySlice`, are identical pairs —
  [plan/2607051918][2607051918].
- New helpers with no dedicated test: `wordlist`
  (`allNames`, `flatten`); `config/wordlist_files.go`
  (`toWordlistMap`, `stripLists`,
  `resolveListEntries`, `expandRuleLists`);
  `convention/nollmtells.go` (four `llm*` builders);
  `linelength/fix.go`+`reflow.go` (four helpers);
  `classifier/model.go`'s `buildPhraseMarkers` —
  [plan/2607051919][2607051919].

### nice-to-have (2026-07-05)

- Perf commit `39a21e63` duplicated
  `countLeadingSpaces` and `isBlank`/`isBlankLine`
  across `listindent`, `orderedlistnumbering`,
  `noreferencestyle` instead of using
  `internal/rules/astutil` —
  [plan/2607051920][2607051920].
- `cmd/mdsmith/runInit` (64 lines) is just past the
  ~50-line guideline; logic already delegates
  cleanly, so no plan filed.

[2607051918]: ../../plan/2607051918_arch-fix-wordlist-config-dedup.md
[2607051919]: ../../plan/2607051919_arch-fix-new-helper-tests.md
[2607051920]: ../../plan/2607051920_arch-fix-rule-whitespace-helpers-astutil.md
