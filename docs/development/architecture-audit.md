---
title: Architecture audit log
summary: >-
  Running log of SOLID and clean-architecture
  findings on origin/main. The
  solid-architecture skill (audit mode)
  appends here; blockers are also filed as
  plans.
audit-from: 28af6a912eaa37361e3a6419363d43400498b9d7
---
# Architecture audit log

This file is maintained by the
solid-architecture skill in audit mode.
The oldest entries have moved to the
[archive shards](architecture-audit-archive.md) to stay
under the file-length budget; every finding there is
resolved.

## Audit 2026-10-18 (range: shallow clone, last 3 weeks)

The recorded `audit-from` SHA is absent from this shallow
clone, so the sweep used the Go and TypeScript files touched
since 2026-09-27 (633 files).

Clean surfaces, verified:

- No rule package imports another rule package; the only
  cross-rule imports are shared helper packages (`settings`,
  `tablefmt`, `fencepos`) that register no rule.
- No lower layer (`lint`, `mdtext`, `index`, `schema`,
  `config`, `rule`, `refactor`) imports `engine`, `lsp`, or a
  rule package.
- `cmd/mdsmith/main.go` (585 lines), `internal/lsp/server.go`
  (592), and `internal/lsp/symbols.go` (496) are under the
  ~1000-line threshold.

### blockers (2026-10-18)

None.

### tax (2026-10-18)

- `cmd/mdsmith/mergedriver.go` (917 lines) carried the
  conflict-marker stripping and detection text logic.
  [go.md][go] §"Clean wiring in `cmd/mdsmith`" places domain
  logic outside `cmd`. Fixed directly: moved it to
  `internal/mergeconflict` with its unit tests;
  `regenDirectiveNames` stays in `cmd` because it reads the
  rule registry.
- `cmd/mdsmith/mergedriver.go` still holds install and hook
  logic (~800 lines): git-config registration, hooks-dir
  resolution, hook-file writing, and managed-block
  verification belong in `internal/githooks` or
  `internal/gitattributes`.
- `cmd/mdsmith/buildpass.go` (884 lines) holds build-target
  collection and dispatch; move it beside `internal/build`
  behind an options struct. No plans filed for these two:
  `PLAN.md` sits at the 300-line `max-file-length` cap, so a
  new plan needs the limit raised, which needs maintainer
  consent per CLAUDE.md.

### nice-to-have (2026-10-18)

- `cmd/mdsmith-wasm/main.go` (1121 lines, 50 functions) is
  the largest `cmd` file. Not a named budget; worth a split
  along the binding groups if it keeps growing. No plan filed.

[go]: architecture/go.md
[tests]: architecture/tests.md

## Audit 2026-09-27 (range: 0ca0d2f..979bb7f)

39 production files touched (Go plus one TypeScript file
in the Obsidian editor).

Clean surfaces, verified:

- Line-count budgets, checked directly against the
  ~1000-line threshold [audit-checklist.md][audit-checklist]
  names: `cmd/mdsmith/main.go` (566 lines),
  `internal/lsp/server.go` (558 lines), and
  `internal/lsp/symbols.go` (511 lines).
- No rule package imports another rule package.
- Test coverage, spot-checked on `cmd/mdsmith/main.go`,
  `internal/rules/astutil`, `internal/rules/catalog`, and
  `internal/rules/linkvalidity`: every production function
  has a matching `TestFoo`/`TestReceiver_Foo`, and trivial
  accessors carry the required exemption comment.

### blockers (2026-09-27)

None.

### tax (2026-09-27)

- `internal/lsp/rename.go` and `internal/refactor/heading.go`
  independently carried byte-for-byte identical
  implementations of the six ATX-heading-text-range parsing
  helpers (`atxHeadingTextByteRange`, `atxHeadingTextStart`,
  `trimTrailingHashRun`, `skipLeadingSpaces`,
  `trimRightSpace`, `trimmedRange`). [go.md][go]'s "Refactor
  moves we have used" calls for pushing a duplicated helper
  into the shared package a caller already imports;
  `internal/lsp/rename.go` already imports `internal/refactor`
  for `refactor.Heading` and `refactor.FindHeadingLine`, so
  nothing justified re-deriving the same CommonMark edge-case
  logic (tab handling, trailing `#` runs, the zero-width
  empty-heading case) a second time. A future goldmark parsing
  fix applied to one copy and not the other would let
  `textDocument/prepareRename`'s highlighted range silently
  disagree with the range `textDocument/rename` actually edits.
  Fixed directly (not filed as a plan): exported the six
  helpers from `internal/refactor/heading.go`
  (`AtxHeadingTextByteRange`, `AtxHeadingTextStart`,
  `TrimTrailingHashRun`, `SkipLeadingSpaces`,
  `TrimRightSpace`, `TrimmedRange`) and pointed
  `internal/lsp/rename.go`'s `headingPrepareRange` at them;
  merged every edge case from the two packages' test suites
  into `internal/refactor/heading_test.go` and deleted the
  now-redundant direct-unit tests from `internal/lsp`.
  `go build ./...`, `go test ./...`, and
  `go tool -modfile=tools/go.mod golangci-lint run` are green;
  behavior is unchanged.
- `cmd/mdsmith/rename.go`'s `detectRenameMode` and `pkg/mdsmith/refactor.go`'s
  `detectRenameKind` both wrap the same `refactor.FindHeadingLine` /
  `refactor.HasLinkRef` pair in an identical "both → error, heading, label,
  neither → error" switch. [go.md][go]'s "Common violations to flag": logic
  reimplemented per host surface instead of living once on the shared engine
  both the CLI and the public `pkg/mdsmith` API call —
  [engine-api.md][engine-api] documents the two as mirroring one-to-one, so a
  future refinement to the ambiguous/no-match messaging is likely to drift
  between them — [plan/2609271912][2609271912]. Resolved: both hosts call
  `refactor.Rename` (shared detection and dispatch) and keep only their own
  message and exit-code wrapping.
- `internal/index/build.go`'s `frontMatterSymbols`,
  `frontMatterScalar`, and `frontMatterStringList` are no
  longer called by any production path; `frontMatterAll`
  replaced them, and the only remaining callers are in
  `internal/index/coverage_test.go` — the function's own
  comment says they're "kept ... for the targeted coverage
  test." [tests.md][tests]'s per-function unit-test rule
  exists to get production code tested, not to justify keeping
  ~130 lines of dead production code alive as a test subject;
  the duplicate YAML-parse logic can also drift from
  `frontMatterAll`'s behavior with nothing in the real build
  path to catch it — [plan/2609271913][2609271913].
  Resolved: the helpers and their coverage-only tests are gone.

### nice-to-have (2026-09-27)

- `internal/rules/requiredstructure/rule.go` is 2700 lines,
  the largest touched file. Not a named budget in the docs
  (only `cmd/mdsmith/main.go` and `internal/lsp/server.go`
  are called out by name), and the package is already split
  into `fieldpatterncache.go`, `runcache_wiring.go`, and
  `scope_rules.go` beside it — worth a maintainer's eye if it
  keeps growing, not a violation today. No plan filed.

[audit-checklist]: architecture/audit-checklist.md
[engine-api]: ../background/concepts/engine-api.md
[2609271912]: ../../plan/2609271912_arch-fix-shared-rename-mode-detection.md
[2609271913]: ../../plan/2609271913_arch-fix-remove-dead-frontmatter-helpers.md

## Audit 2026-09-13 (range: 0ca0d2f..b48e90c)

The 2026-09-27 sweep above re-covers this range. Only
the findings it does not record are kept here.

### blockers (2026-09-13)

None.

### tax (2026-09-13)

- The absolute-path predicate had three copies: in `cmd/mdsmith`,
  in `internal/linkgraph`, and in `internal/backlinks`, inside a
  full copy of `linkgraph.ResolveRelTarget` ([go.md][go]). Fixed directly:
  `cmd/mdsmith` and `internal/linkgraph` call `internal/pathutil` (`\` is a
  separator on every host); `internal/backlinks` calls `ResolveRelTarget`.
  Left separate: `internal/refactor`'s `workspaceRelative` (`path.IsAbs`
  only, so `Session.Move` and the LSP accept a `C:/x.md` destination the
  CLI rejects), `internal/lsp`'s `isAbsPath`, and the drive-letter checks
  in `internal/schema`, `internal/rules/build`, and `internal/lsp`.
- `cliRenameWorkspace` and `sessionRefactorWorkspace` share
  four matching `Workspace` pass-throughs (`Resolve` differs by
  design) — [plan/2609131911][2609131911]. Resolved: they and
  `lspRenameWorkspace` embed `refactor.IndexEdges`.
- `internal/refactor/move.go`'s `recomputeToken`, `encodePathToken`,
  `pathEdit`, `countFilesWithStem` lack tests by name ([tests.md][tests])
  — [plan/2609131913][2609131913]. Resolved: the first three became the
  tested `destEdit` and `encodeLike`; `countFilesWithStem` has a test.

### nice-to-have (2026-09-13)

- Four `cmd/mdsmith` move/rename helpers lack test
  symbols but are covered indirectly; optional —
  [plan/2609131913][2609131913]. Resolved: all four
  have named tests or were replaced by tested code.

[2609131911]: ../../plan/2609131911_arch-fix-refactor-workspace-duplication.md
[2609131913]: ../../plan/2609131913_arch-fix-move-helper-unit-tests.md
