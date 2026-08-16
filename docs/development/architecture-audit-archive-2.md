---
title: Architecture audit log archive (2)
summary: >-
  Overflow shard for architecture-audit-archive.md,
  which hit the project's file-length budget. Every
  finding here is resolved; the linked plans are the
  detailed record.
---
# Architecture audit log archive (2)

[The audit log archive](architecture-audit-archive.md)
links here for entries it no longer has room for.
Entries below are moved, not summarized — nothing
was reworded.

This shard itself hit the budget on 2026-08-16.
The 2026-06-28 and 2026-07-05 entries moved on to
[the third archive](architecture-audit-archive-3.md).

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

## Audit 2026-06-24 (range: 09f22d3..3d35b77)

Plans 2606241814/2606241815 green.
No DIP, SRP, or line-count violations.

## Audit 2026-07-19 (range: 834b560..6680ff5)

89 touched files. Notable new surfaces: SARIF output,
the `internal/pack` init-scaffold package, and a
wasm/net-split `internal/rules/externallink` rule.

No rule-to-rule imports. No reverse-layer imports. No
Liskov breaks. No missing tests on a public surface
(`rule.Rule` method, LSP handler, CLI subcommand entry).

Clean surfaces, verified:

- `internal/output/sarif.go` implements the same
  `Formatter` interface as `JSONFormatter` and
  `TextFormatter`. It is documented in
  `docs/reference/cli/check.md` and `fix.md`
  (`-f sarif`). 16 tests lock its shape.
- `internal/pack` is a small registry package,
  mirroring `internal/starter`.
- `internal/rules/externallink`'s wasm/net build-tag
  split preserves Liskov substitutability. Both
  `probe()` implementations share one signature and
  one documented tri-state contract. The split is
  thoroughly tested.

### blockers (2026-07-19)

None.

### tax (2026-07-19)

- `cmd/mdsmith/main.go` crossed the project's documented
  ~1000-line threshold (1018 lines), almost entirely from
  the new `mdsmith init --add` pack machinery
  (`runInit`, `printInitCatalog`, `normalizePackNames`,
  `applyPacks`, `writeScaffolds`, `refuseSymlinkedParents`,
  `validatePackPath`, `statTarget`).
  `docs/development/architecture/audit-checklist.md`
  names this exact smell: "`cmd/mdsmith/main.go` past
  ~1000 lines — handler bodies have crept in; relocate to
  `internal/engine` or a per-subcommand file." Fixed: moved
  the init-subcommand logic (`setInitUsage` through
  `convertedConfigBytes`) into a new `cmd/mdsmith/init.go`,
  matching the existing per-subcommand file pattern
  (`check.go`, `fix.go`, `deps.go`, …), with its tests in a
  matching `init_unit_test.go`. `main.go` is now 678 lines.
  No behavior change; `go test ./...` and
  `go tool golangci-lint run` are green.
- `printInitCatalog` and `setInitUsage`
  (`cmd/mdsmith/init.go`) have no dedicated unit test —
  only e2e subprocess tests (`mdsmith init --list` and
  `--help`) exercise them.
  [tests.md][tests]: "A new function lands together with
  its dedicated unit test by name," and an e2e test
  reachable without the process boundary is an inverted
  pyramid. Neither is a public surface itself (both are
  `runInit` helpers), so tax not blocker. The
  `setInitUsage` half of this surfaced during this cycle's
  3x code-review pass on the fix, not the original sweep —
  [plan/2607191917][2607191917].
- `internal/rules/requiredstructure/rule.go`'s `isClaimed`
  is a byte-for-byte copy of
  `internal/schema/validate.go`'s `isClaimed`, despite
  `requiredstructure` already importing `internal/schema`
  — not an import-cycle workaround, a plain copy.
  [go.md][go] refactor-moves: "Lift a shared dependency up
  to an interface... once two rules needed the same shape"
  — [plan/2607191918][2607191918].

[tests]: architecture/tests.md
[go]: architecture/go.md
[2607191917]: ../../plan/2607191917_arch-fix-printinitcatalog-unit-test.md
[2607191918]: ../../plan/2607191918_arch-fix-isclaimed-dedup.md

### nice-to-have (2026-07-19)

- `runInit` (`cmd/mdsmith/init.go`) is 62 lines, over
  go.md's "~50 lines is a smell" guidance for `cmd/mdsmith`
  wiring — it already delegates cleanly to single-purpose
  helpers, so no plan filed.
- `internal/pack/pack.go`'s `Pack.Files()` and unexported
  `register()` are trivial one-line, no-branch functions
  with no dedicated test and no exemption comment per
  tests.md's exemption clause; both are exercised
  indirectly. Naming/documentation nit, not a coverage gap.
