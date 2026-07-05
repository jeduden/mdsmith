---
title: Architecture audit log
summary: >-
  Running log of SOLID and clean-architecture
  findings on origin/main. The
  solid-architecture skill (audit mode)
  appends here; blockers are also filed as
  plans.
audit-from: 528ce4cf7df4f2fe7d15051e5a39390c27fdc82d
---
# Architecture audit log

This file is maintained by the
solid-architecture skill in audit mode.
Entries from 2026-04-28 through 2026-06-21 moved
to [the archive](architecture-audit-archive.md)
this cycle to stay under the file-length budget;
every finding there is resolved.

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
- Fixed: added `mdtext.IsAbbrevToken`, backed by
  the existing singleton. `reflow.go` now calls
  that.

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
