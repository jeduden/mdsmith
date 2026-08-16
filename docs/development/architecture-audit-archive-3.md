---
title: Architecture audit log archive (3)
summary: >-
  Overflow shard for architecture-audit-archive-2.md,
  which hit the project's file-length budget. Every
  finding here is resolved; the linked plans are the
  detailed record.
---
# Architecture audit log archive (3)

[The second audit log archive](architecture-audit-archive-2.md)
links here for entries it no longer has room for.
Entries below are moved, not summarized — nothing
was reworded.

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
