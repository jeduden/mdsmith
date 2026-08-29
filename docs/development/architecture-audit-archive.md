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
as well.

## Audit 2026-05-31 (range: 4809097..37488a7)

Plans 200, 201, 202 green. Tax:
[plan/223][223] (`pkg/mdsmith` private helpers),
[plan/224][224] (`internal/lint` SRP, now 12 files).
`linkstyle` helpers — add tests in-place.

[223]: ../../plan/223_arch-fix-mdsmith-helper-tests.md
[224]: ../../plan/224_arch-fix-lint-srp.md

### nice-to-have (2026-06-02)

`internal/punkt` not in the layering map —
[plan/225][225]. Separately, [plan/224][224]
(`internal/lint` SRP) is now implemented:
`gitignore`, `bytelimit`, and `piparser`
split into sibling packages.

[225]: ../../plan/225_arch-fix-punkt-layering.md

## Audit 2026-06-07 (range: 37488a7..82583fc)

Plans 203–225 green. Blocker: `Session.CheckSource`
(public API) had no unit test. Fixed: added
`pkg/mdsmith/checksource_test.go` with 4 tests.
Tax: the [tablereadability dedup][2606071930] and
[include helper test][2606071931] plans.

[2606071930]: ../../plan/2606071930_arch-fix-tablereadability-dup.md
[2606071931]: ../../plan/2606071931_arch-fix-include-helper-tests.md

## Audit 2026-06-14 (range: 82583fc..aed18aa)

Tax: [build→rules DIP](../../plan/2606141910_arch-fix-build-rules-dip.md),
[engine wrappers](../../plan/2606141911_arch-fix-engine-deprecated-wrappers.md),
[secreview tests](../../plan/2606141912_arch-fix-secreview-report-tests.md).

## Audit 2026-06-16 (range: aed18aa..7793b97)

Lazy-parse series (plans 2606141901–2606141904).
Tax: [new-pkg-docs](../../plan/2606162213_arch-fix-new-pkg-docs.md),
[helper-tests](../../plan/2606162214_arch-fix-missing-helper-tests.md).

## Audit 2026-06-21 (range: 7793b97..e701b94)

Parity + Layer-0 parse-skip series.
Symlink containment; engine panic recovery.
VS Code `kinds` and `rule-doc` commands.
270 Go/TS sources. No blockers,
rule-to-rule imports, or DIP violations.

### tax (2026-06-21)

- `internal/engine/runner.go` (1 290 lines) —
  SRP: 7 concerns. Fixed this cycle: split into
  `runner_layer0.go`, `runner_cache.go`,
  `runner_log.go` — [plan/2606211907][2606211907].

- `internal/lint/layer0.go` (1 203 lines) —
  full Layer-0 block scanner. Fix: split along
  block-type sub-parsers —
  [plan/2606211908][2606211908].

- `internal/lsp/server.go` (1 007 lines) —
  crept back over 1 000 lines. Dispatch-group
  split — [plan/2606211909][2606211909].

### nice-to-have (2026-06-21)

- `pkg/mdsmith/workspace.go` trivial methods
  lack "// no test by design" exemptions —
  [plan/2606211910][2606211910].

[2606211907]: ../../plan/2606211907_arch-fix-runner-srp-split.md
[2606211908]: ../../plan/2606211908_arch-fix-layer0-split.md
[2606211909]: ../../plan/2606211909_arch-fix-lsp-server-split.md
[2606211910]: ../../plan/2606211910_arch-fix-workspace-exemptions.md

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
