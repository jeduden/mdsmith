---
title: Architecture audit log
summary: >-
  Running log of SOLID and clean-architecture
  findings on origin/main. The
  solid-architecture skill (audit mode)
  appends here; blockers are also filed as
  plans.
audit-from: 2ab4b2949e9420b2eb73f4239091fd647c4b5e80
---
# Architecture audit log

This file is maintained by the
solid-architecture skill in audit mode.
Entries from 2026-06-26 through 2026-07-12 moved
to [the archive](architecture-audit-archive.md)
this cycle to stay under the file-length budget;
every finding there is resolved.

## Audit 2026-07-26 (range: 6680ff5..2ab4b29)

90+ touched files. Notable new surfaces: the
`foreign-regions:` scanner and the `MDS073`
Slidev slide-structure rule, both added this
cycle, plus the `cmd/mdsmith/init.go` split that
closes the prior cycle's tax finding.

No rule-to-rule imports. No reverse-layer
imports. No Liskov breaks. No inverted test
pyramid (checked every `internal/integration/`
test in the touched set and the one e2e
subprocess test added). No missing unit tests on
a public surface — every touched function has a
`TestFoo`/`TestReceiver_Foo` in a sibling
`_test.go`, or is exercised indirectly through
its public caller's test on the established
project convention.

### blockers (2026-07-26)

- Two unrelated diagnostics both claimed
  `MDS073`: the registered `slide-structure`
  rule (`internal/rules/slidevstructure/rule.go`)
  and the `internal/foreignregion` scanner's
  engine-emitted malformed-region diagnostic
  (`internal/foreignregion/scan.go`), which rides
  along the fix/check pipeline outside the
  `rule.Rule` registry. `slide-structure` and the
  foreign-region scanner landed independently in
  the same window (see `git log` on
  `internal/rules/slidevstructure` and
  `internal/foreignregion`); neither checked
  whether the ID was still free.
  [cross-system.md][cross] — a rule ID is the
  addressable unit of the `.mdsmith.yml` `rules:`
  map, CLI output, and inline-suppression
  comments, so an ambiguous ID breaks per-rule
  disabling for whichever diagnostic isn't
  routed through the registry. Fixed directly in
  this cycle rather than filed as a plan: renamed
  the foreign-region diagnostic to `MDS074` (the
  next free ID) across
  `internal/foreignregion/scan.go`,
  `internal/config/foreignregion.go`,
  `internal/engine/runner.go`,
  `internal/fix/fix_foreign_region_test.go`,
  `internal/config/foreignregion_more_test.go`,
  and `docs/reference/foreign-regions.md`, and
  added
  `internal/integration/rule_id_uniqueness_test.go`
  — a contract test that walks every registered
  `rule.Rule.ID()` plus `foreignregion.RuleID` and
  fails on any duplicate, so a repeat collision
  fails CI instead of shipping silently.

### tax (2026-07-26)

None.

### nice-to-have (2026-07-26)

- `cmd/mdsmith/mergedriver.go` (916 lines) and
  `internal/engine/runner.go` (949 lines) are
  approaching the ~1000-line split threshold
  [go.md][go] calls out for `main.go` and
  `internal/lsp/server.go`/`symbols.go`. Neither
  crossed it this cycle and neither is named in
  the checklist's threshold list, so no plan
  filed; revisit if either grows further.
  `mergedriver.go` in particular mixes CLI flag
  parsing, `.gitattributes` docstring content,
  and glob-resolution logic that could split the
  same way `main.go`'s `init` subcommand did last
  cycle.

[cross]: architecture/cross-system.md

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
