---
title: Architecture audit log archive
summary: >-
  Older entries moved out of
  architecture-audit.md to keep it under the
  project's file-length budget. Every finding
  here is resolved; the linked plans are the
  detailed record.
---
# Architecture audit log archive

[The current audit log](architecture-audit.md)
links here once its own length approaches the
300-line file budget. Entries below are moved,
not summarized — nothing was reworded.

This archive itself hit the budget on 2026-07-19.
Entries from 2026-07-12 through 2026-07-05 moved
on to
[the second archive](architecture-audit-archive-2.md).
It hit the budget again on 2026-08-23; entries
from 2026-05-13 through 2026-05-19 moved on to
[the second archive](architecture-audit-archive-2.md)
as well. It hit the budget again on 2026-08-30;
entries from 2026-05-31 through 2026-06-21 moved
on to
[the second archive](architecture-audit-archive-2.md)
too.

## Audit 2026-06-23 (range: e701b94..1599c9f)

Performance + struct-alignment series;
inline scanner refinements; benchmark
additions. No TypeScript changes. 273 Go
sources outside fixtures.

No blockers. No rule-to-rule imports added.
No DIP violations. New files are under 800
lines. Struct alignment and `map[string]struct{}`
changes are mechanical rewrites with no
layering impact.

### tax (2026-06-23)

- `internal/lint/inline_scan.go` — 13
  unexported helpers lack dedicated unit
  tests. Tests doc §"every function by
  name" — [plan/2606231013][2606231013].

- `internal/rules/samefileanchor/rule.go`
  — 12 unexported helpers lack dedicated
  unit tests — [plan/2606231014][2606231014].

[2606231013]: ../../plan/2606231013_arch-fix-inline-scan-helper-tests.md
[2606231014]: ../../plan/2606231014_arch-fix-samefileanchor-helper-tests.md

## Audit 2026-06-24 (range: 1599c9f..09f22d3)

Perf series (struct-alignment, Sprintf→strconv,
`[]byte` FindSubmatch, Builder). Plans 2606231013
and 2606231014 closed. Benchmark docs and security
SARIF retired. No TypeScript changes. 273 Go
sources outside fixtures.

No blockers. No rule-to-rule imports. No DIP
violations. No file crossed 1 000 lines.

### tax (2026-06-24)

- `internal/index/locate.go` — 12 unexported
  helpers lack dedicated unit tests. Tests doc
  §"every function by name" —
  [plan/2606240211][2606240211].

- `internal/lsp/rename.go` — 15 unexported
  helpers lack dedicated unit tests. Tests doc
  §"every function by name" —
  [plan/2606240212][2606240212].

- `internal/export/export.go` — 11 unexported
  helpers lack dedicated unit tests. Tests doc
  §"every function by name" —
  [plan/2606240213][2606240213].

- `internal/lsp/rename.go` and
  `internal/rename/rename.go` — `normalizedLabel`
  and `refDefBracketBytes` are duplicated. Both
  have identical bodies. Hub §"Anti-patterns" —
  [plan/2606240214][2606240214].

- `internal/rules/concisenessscoring/rule.go`
  and `internal/rename/rename.go` —
  `countClassifierTokens` and
  `contentBlockLines` lack dedicated unit tests.
  Batched into [plan/2606240213][2606240213].

### nice-to-have (2026-06-24)

- `internal/index/locate.go` —
  `isGlobPattern` is a trivial one-liner with no
  branch. Add "// no test by design" so the audit
  can distinguish it from forgotten test debt.

[2606240211]: ../../plan/2606240211_arch-fix-locate-helper-tests.md
[2606240212]: ../../plan/2606240212_arch-fix-lsp-rename-helper-tests.md
[2606240213]: ../../plan/2606240213_arch-fix-export-helper-tests.md
[2606240214]: ../../plan/2606240214_arch-fix-rename-dedup.md

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

[tests]: architecture/tests.md
[go]: architecture/go.md

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
[2608091910]: ../../plan/2608091910_arch-fix-mds073-collision.md
[2608091911]: ../../plan/2608091911_arch-fix-missing-unit-tests.md

### nice-to-have (2026-08-09)

None found this cycle beyond the tax items above.
