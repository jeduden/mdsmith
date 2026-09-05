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

This archive itself hit the budget on 2026-08-23;
the 2026-07-12 entry moved on to
[the third archive](architecture-audit-archive-3.md).

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

## Audit 2026-05-31 (range: 4809097..37488a7)

Plans 200, 201, 202 green. Tax:
[plan/223][223] (`pkg/mdsmith` private helpers),
[plan/224][224] (`internal/lint` SRP, now 12 files).
`linkstyle` helpers — add tests in-place.

[223]: ../../plan/223_arch-fix-mdsmith-helper-tests.md

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
