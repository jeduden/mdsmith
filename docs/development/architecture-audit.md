---
title: Architecture audit log
summary: >-
  Running log of SOLID and clean-architecture
  findings on origin/main. The
  solid-architecture skill (audit mode)
  appends here; blockers are also filed as
  plans.
audit-from: b706d7618cca472108b80ccd837877fad80230b3
---
# Architecture audit log

This file is maintained by the
solid-architecture skill in audit mode.
The oldest entries have moved to the
[archive shards](architecture-audit-archive.md) to stay
under the file-length budget; every finding there is
resolved.

## Audit 2026-08-23 (range: 2ab4b29..b706d76)

211 commits, ~200 files touched. ~140 are Go,
mostly new alloc/race/bench tests — a healthy
sign, not flagged. Notable production additions:

- `internal/pack` — APM kind-pack scaffolding.
- `internal/index/lineindex.go` — a shared
  newline index.
- `internal/engine/source_config_cache.go`.
- An SSRF guard in
  `internal/rules/externallink`.
- A vendored `pkg/runewidth` fork replacing the
  eager LUT — exempt from the test-coverage rule
  as vendored code, like `pkg/goldmark`.

Clean surfaces, verified:

- No rule-to-rule imports. No reverse-layer
  imports. No Liskov breaks.
- `internal/pack` is a leaf consumed only by
  `cmd/mdsmith/init.go`.
- `internal/engine/source_config_cache.go` and
  `internal/index/lineindex.go` both resolve to
  the directions [go.md][go] requires and ship
  dedicated tests.
- No `Helper`/`Util`/`Misc` symbols. No
  `cmd/mdsmith` handler crossed ~50 lines with
  domain logic left uninlined.

### blockers (2026-08-23)

None.

### tax (2026-08-23)

None new this cycle.
[plan/2608021916][2608021916] (`internal/githooks`
SRP split, flagged 2026-08-02) had no open PR yet
after two cycles — picked up as this cycle's fix;
see the linked PR once opened.

### nice-to-have (2026-08-23)

None found this cycle.

[go]: architecture/go.md
[2608021916]: ../../plan/2608021916_arch-fix-githooks-package-split.md

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
