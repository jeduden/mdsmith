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

[tests]: architecture/tests.md
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
  smell, though the function is well-scoped. Rename on next
  touch.
- `Override.Patterns()` / `KindAssignmentEntry.Patterns()`
  (`internal/config/config.go`) are two-line branching
  methods exercised only inside larger config tests, not by
  a dedicated `Test*`. Low regression risk.
- `parseCheckFlags` / `parseFixFlags` cross go.md's ~50-line
  guidance for `cmd/mdsmith` handlers, but the body is pure
  flag-registration boilerplate, not domain logic.

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
