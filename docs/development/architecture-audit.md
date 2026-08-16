---
title: Architecture audit log
summary: >-
  Running log of SOLID and clean-architecture
  findings on origin/main. The
  solid-architecture skill (audit mode)
  appends here; blockers are also filed as
  plans.
audit-from: 81f0d9659e2593f225371f9277460b58fa959756
---
# Architecture audit log

This file is maintained by the
solid-architecture skill in audit mode.
Entries from 2026-06-26 through 2026-07-12 moved
to [the archive](architecture-audit-archive.md)
this cycle to stay under the file-length budget;
every finding there is resolved. The 2026-07-19
entry moved there too this cycle for the same
reason.

## Audit 2026-08-16 (range: 2ab4b29..81f0d96)

185 commits, 227 files touched (187 Go files). No
TypeScript changes.

No rule-to-rule imports. No reverse-layer imports. No
Liskov breaks. `cmd/mdsmith/main.go` and
`internal/lsp/server.go`/`symbols.go` stayed well under
the ~1000-line threshold. The MDS073 collision from the
prior cycle stays resolved — `rule_id_uniqueness_test.go`
now guards it as a contract test.

Clean surfaces, verified:

- Every touched `internal/rules/*` package (catalog,
  duplicatedcontent, markdownflavor, occurrence,
  requiredstructure, slidevstructure, externallink, and
  17 more) imports only shared helper packages — zero
  rule-to-rule imports.
- `internal/engine/source_config_cache.go` (new): a
  cache hit returns a fresh `cloneRules` copy per caller,
  never the shared template pointer, pinned by a
  dedicated `-race` concurrency test.
- `internal/pack/apm.go` (new): scoped correctly, no
  cross-layer imports, registered via the existing
  `pack.register` plugin point.
- `internal/rules/externallink/probe_net.go`'s SSRF
  hardening (`isRestrictedIP`, `ssrfControl`,
  `ssrfCheckRedirect`): 9 dedicated tests, no
  architecture concerns.
- The two new `links:` settings
  (`external-allow-internal`, `external-max-probes`) live
  in MDS072's own settings struct — not a "field reachable
  from only one rule" violation, consistent with the
  existing `links:` precedent.

### blockers (2026-08-16)

None.

### tax (2026-08-16)

- `pkg/mdsmith/session.go`'s `readBoundedFrontMatterSource`
  — the bounded/fallback front-matter read path on the
  public `pkg/mdsmith` engine API — had no dedicated unit
  test; only exercised indirectly via
  `TestSessionKindsOversizedFile*`.
  [tests.md][tests] requires a test by the function's own
  name, and `pkg/mdsmith` is the highest-blast-radius
  surface touched this cycle. Fixed directly (not filed as
  a plan): added `TestReadBoundedFrontMatterSource`, a
  table-driven test in `session_test.go` covering the
  bounded (`OSWorkspace`) and fallback (`MemWorkspace`)
  paths, the missing-file error, and both `max<=0` and
  `math.MaxInt64` unbounded cases. `go test ./...` and
  `go tool golangci-lint run` are green.
- Six more functions across `cmd/mdsmith`, `pkg/mdsmith`,
  `internal/rules/duplicatedcontent`,
  `internal/rules/astutil`, `internal/bytelimit`, and
  `pkg/markdown/flavor` have no dedicated unit test by
  name, each covered only behaviorally —
  [plan/2608161914][2608161914].

### nice-to-have (2026-08-16)

- `internal/lsp/server_diagnostics.go`'s
  `surfaceForeignDiagnostics` changed the
  `window/logMessage` notification shape from one
  notification per diagnostic to at most two batched
  notifications grouped by severity — a behavior change on
  the LSP wire surface [cross-system.md][cross] tracks.
  Well-tested; worth a one-line changelog note per that
  page's "breaks must be deliberate and noted" policy. No
  plan filed.
- `internal/engine/source_config_cache.go`'s
  `NewSourceConfigCache` is a trivial one-line constructor
  without the "no test by design" exemption comment
  [tests.md][tests] asks for on untested trivial functions.
  Documentation nit; the type it constructs is otherwise
  exhaustively tested. No plan filed.

[2608161914]: ../../plan/2608161914_arch-fix-touched-set-unit-tests.md

## Audit 2026-08-09 (range: 6680ff5..2ab4b29)

100 commits, 148 files touched (92 Go files).

New packages this cycle:

- `internal/foreignregion` — foreign managed-region
  protection for check/fix.
- `internal/convention` — extracted convention model.

New opt-in rules this cycle:

- `internal/rules/occurrence` (MDS060).
- `internal/rules/slidevstructure` (MDS073).

No rule-to-rule imports. No reverse-layer imports. No
Liskov breaks. `cmd/mdsmith/main.go` (709 lines) stayed well
under the ~1000-line threshold, as did
`internal/lsp/server.go`/`symbols.go` (554/511 lines).

Clean surfaces, verified:

- `internal/mdpath` consolidation: `IsMarkdownPath` grew into
  a single source of truth (`Extensions`, `FileGlobs`,
  `RecursiveGlobs`, `HasMarkdownExt`) now consumed by
  `internal/lsp`, `internal/config`, and the merge-driver
  include set — SRP fix per go.md's "package answers one
  question." Every function has an exact-name test.
- `cmd/mdsmith/init.go` split: extracted from `main.go` with
  full dedicated-test coverage — 39 `Test*` functions cover
  all 13 functions, including error paths.
- `internal/rules/crossfilereferenceintegrity`'s
  double-checked-locking memoization
  (`cachedGlobSettingsErr`) is pinned by dedicated tests
  following the `TestReceiver_Foo` naming convention.

### blockers (2026-08-09)

- `MDS073` is claimed by two unrelated, independently
  registered diagnostics. `internal/rules/slidevstructure/rule.go:52`
  returns `"MDS073"` for the Slidev slide-structure rule
  (registered in `internal/rules/all/all.go`, documented at
  `internal/rules/MDS073-slide-structure/README.md`).
  `internal/foreignregion/scan.go:25` independently defines
  `RuleID = "MDS073"` for the malformed-marker-pair
  diagnostic (wired into `internal/engine/runner.go` and
  `internal/fix/fix.go`, documented at
  `docs/reference/foreign-regions.md:76`). Both landed the
  same day (2026-07-19) — slide-structure was renumbered
  `MDS072 -> MDS073` in `7e4925a` while the foreign-region
  feature independently claimed `MDS073` in parallel commits
  — an undetected rebase/merge collision.
  [cross-system.md][cross] treats rule IDs as part of the
  public `.mdsmith.yml`/CLI/docs contract; no test in
  `internal/integration/` or `internal/rule/` enforces
  uniqueness across `rule.All()` plus out-of-band IDs like
  foreign-region's. Fixed by renumbering the foreign-region
  diagnostic to the next free ID and adding a uniqueness
  contract test — [plan/2608091910][2608091910].

### tax (2026-08-09)

- `internal/rules/occurrence/rule.go` has 18 non-trivial
  private helpers with no dedicated unit test by name
  (`countCombinedInRange`, `searchText`, `applyScope`, …).
  All are covered indirectly by 47 behavior-level
  `TestCheck_*`/`TestApplySettings_*` cases, but
  [tests.md][tests] requires a test by the function's own
  name, and the pre-existing sibling rule
  `paragraphstructure` follows that convention for every
  helper — [plan/2608091911][2608091911].
- `internal/rules/slidevstructure/rule.go` has 11
  non-trivial private helpers (`checkSlide`, `slideAnchor`,
  `nearest`, …) in the same shape — covered behaviorally by
  35 `Test*` cases but not by name —
  [plan/2608091911][2608091911].

[cross]: architecture/cross-system.md
[tests]: architecture/tests.md
[2608091910]: ../../plan/2608091910_arch-fix-mds073-collision.md
[2608091911]: ../../plan/2608091911_arch-fix-missing-unit-tests.md

### nice-to-have (2026-08-09)

None found this cycle beyond the tax items above.

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

[go]: architecture/go.md
[audit-checklist]: architecture/audit-checklist.md
[2608021915]: ../../plan/2608021915_arch-fix-query-subcommand-placement.md
[2608021916]: ../../plan/2608021916_arch-fix-githooks-package-split.md

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
