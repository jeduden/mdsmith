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

This archive itself hit the budget on 2026-09-20;
the 2026-07-12 entry moved on to
[the fourth archive](architecture-audit-archive-4.md).

## Audit 2026-05-13 (range: 6af677fb..7464d273)

1 107 files; 425 Go/TS sources outside fixtures.

Resolved:

- Rule-to-rule imports —
  [plan/154](../../plan/154_arch-fix-rule-helper-extraction.md).
- Config-to-rule import —
  [plan/155](../../plan/155_arch-fix-convention-config-ownership.md).
- `internal/testutil` anti-pattern name —
  [plan/201](../../plan/201_arch-fix-testutil-rename.md).
- `hover.go` DIP — [plan/200][200].
- `main.go` > 1 000 lines — [plan/202][202].

Tax:

- `extension.ts` SRP — [plan/205][205].
- `internal/fix`→`internal/engine` DIP —
  [plan/204][204].
- `internal/lint` SRP — [plan/224][224].

## Audit 2026-05-17 (range: 7464d273..b5a6d72)

Covered `internal/rename`, `internal/index`,
`mdsmith deps`, `mdsmith export`. Tax:
`nonNegativeUTF16RuneLen` copied privately in
three packages; export from `internal/mdtext` —
[plan/186](../../plan/186_arch-fix-utf16-centralize.md).

## Decision 2026-05-17 (plan/174)

### plan/153 non-goal superseded

Plan 174 moved the workspace symbol index
from `internal/lsp/index` to `internal/index`.
Pure `git mv`; no logic changed.
`internal/schema` already imported it from
outside `internal/lsp`. `mdsmith rename` and
`mdsmith deps` need it. The layering map
forbids `cmd/mdsmith` → `internal/lsp`.
`internal/index` must never import `internal/lsp`.

## Audit 2026-05-19 (range: 7464d273..41e61a5)

131 Go files. Plans 154, 155, 174 green.

### tax (2026-05-19)

- `server.go` (1 536) and `symbols.go` (1 385)
  exceed 1 000 lines — [plan/203][203].
- Five items from 2026-05-13 now scheduled:
  [hover][200], [testutil][201], [main.go][202],
  [fix→engine][204], [extension.ts][205].

[200]: ../../plan/200_arch-fix-hover-embed.md
[201]: ../../plan/201_arch-fix-testutil-rename.md
[202]: ../../plan/202_arch-fix-main-split.md
[203]: ../../plan/203_arch-fix-lsp-server-split.md
[204]: ../../plan/204_arch-fix-fix-engine-inversion.md
[205]: ../../plan/205_arch-fix-extension-ts-srp.md
[206]: ../../plan/206_arch-fix-cue-types-docs.md
[224]: ../../plan/224_arch-fix-lint-srp.md

### nice-to-have (2026-05-19)

`cue/types` not in layering map — [plan/206][206].

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
