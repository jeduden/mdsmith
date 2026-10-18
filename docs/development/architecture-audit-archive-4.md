---
title: Architecture audit log archive (4)
summary: >-
  Second overflow shard for architecture-audit-archive-2.md
  (2026-09-27, holding the 2026-06-26 through 2026-07-05
  entries), which hit the project's file-length budget a
  second time. Every finding here is resolved; the linked
  plans are the detailed record.
---
# Architecture audit log archive (4)

[The second audit log archive](architecture-audit-archive-2.md)
links here for entries it no longer has room for — this is
its second overflow shard; [the third
archive](architecture-audit-archive-3.md) is its first,
holding an unrelated set of entries. Entries below are
moved, not summarized — nothing was reworded.

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

## Audit 2026-08-30 (range: b706d76..0ca0d2f)

59 commits, ~130 files touched (~125 Go files, no
TypeScript). New packages this cycle:

- `internal/linkgraph` — link/wikilink target parsing and
  resolution, split out of the rules that used to inline it.
- `internal/schema` — a one-question-per-file split (compose,
  extend, filename, parse_file, parse_inline, validate) with
  no reverse-layer imports.
- `internal/gitattributes` and `internal/directivefiles`
  (dead; since deleted by [plan/2608301918][2608301918]) —
  the other two packages of the 2026-08-23 `internal/githooks`
  SRP split ([plan/2608021916][2608021916]); that plan
  merged in PR #815, closing its 2026-08-23 note.

Rule-ID collisions hit this project once before
([plan/2608091910][2608091910]). Checked for a repeat:

- `internal/foreignregion` claims `MDS074`.
- `internal/rules/overrepetition` claims `MDS075`.
- Checked against `internal/rules/all/all.go` and
  `internal/integration/testdata/rule_walk_audit.json`.
- No overlap. No repeat this cycle.

Clean surfaces, verified:

- No rule-to-rule imports. No reverse-layer imports. No
  Liskov breaks.
- `internal/linkgraph`, `internal/schema`,
  `internal/gitattributes`, `internal/directivefiles` (now
  deleted): each answers one question, has dedicated tests,
  and imports no `internal/rules/...`.
- `cmd/mdsmith/discover.go`: correction, it was dead code
  (no caller since #213); deleted by [plan/2608301918][2608301918].
- `internal/rules/catalog/rule.go` and
  `internal/rules/requiredstructure/rule.go`'s changes this
  cycle are perf-only (`RunCache.RawSchemaFile`, MDS019
  pre-check gating); no new imports, both ship dedicated
  tests.

### blockers (2026-08-30)

None.

### tax (2026-08-30)

- `cmd/mdsmith/backlinks.go` had ~430 of 585 lines carrying
  the backlink target-matching algorithm — link/wikilink
  resolution, workspace-relative path math — inside the CLI
  package. [go.md][go] §"Clean wiring in `cmd/mdsmith`":
  "Domain logic ... belongs in `pkg/mdsmith`,
  `internal/engine`, or their dependencies." This was the
  newest and most self-contained instance of the pattern
  (`mergedriver.go` carries some of the same weight but is
  out of this cycle's touched set). Fixed directly (not
  filed as a plan): extracted `Record`, `Collect`, and every
  private helper the matching algorithm needs into a new
  `internal/backlinks` package; `cmd/mdsmith/backlinks.go`
  now only parses flags, validates arguments, calls
  `backlinks.Collect`, and formats output —
  `runBacklinks` stays a thin dispatcher. `workspaceRelativePath`
  and `isAbsOrDriveOrUNC` stayed in `cmd/mdsmith` (shared by
  `deps.go` and `rename.go` too, not backlinks-specific); the
  new package carries its own small private duplicates for
  the two pure predicates it needs
  (`relPath`/`isAbsOrDriveOrUNC`) rather than importing
  `cmd/mdsmith`, which would invert the dependency direction.
  `go build ./...`, `go test ./...`, and
  `go tool golangci-lint run` are green; behavior is
  unchanged (existing unit and e2e tests moved/kept
  untouched). Superseded by the 2026-09-13 audit.
- `internal/mdtext/wordfreq.go`'s `WordFrequencyInto` and
  three helpers in `internal/directivefiles/directivefiles.go`
  (`openingFence`, `isClosingFence`, `isIndentedCodeBlock`)
  have no dedicated unit test by name — [tests.md][tests]
  requires one — [plan/2608301918][2608301918]. Correction:
  that file was dead code and is deleted, helpers and all;
  the package no longer exists.
- `internal/lint/runcache.go`'s `RunCache` caches state across
  every file in a whole `engine.Run` pass, which answers a
  different question than [go.md][go]'s stated charter for
  `internal/lint` ("model a parsed Markdown file"). Closer to
  `internal/engine`'s job ("orchestrate rules over files; owns
  the run loop"). Not an import-cycle or forbidden-import
  violation — a package-boundary tax per go.md's "Split a
  package by question," now 676 lines and ten cache slots —
  [plan/2608301919][2608301919]. Resolved: moved to the leaf
  package `internal/runcache` (`internal/engine` would cycle).

### nice-to-have (2026-08-30)

- `internal/rules/overrepetition/rule.go` and
  `internal/rules/occurrence/rule.go` independently reimplement
  the same file/section/paragraph scope-walking dispatch shape.
  No cross-import (clean per go.md's DIP rule); [go.md][go]'s
  refactor-moves precedent ("lift a shared dependency up ...
  once two rules needed the same shape") would apply to a
  future cleanup. No plan filed.
- `cmd/mdsmith/query.go`'s `readFrontMatterRaw` reimplements a
  slice of front-matter parsing that overlaps
  `internal/lint`'s charter. Worth lifting into
  `internal/lint` (e.g. `lint.FrontMatterMap`) alongside the
  existing `StripFrontMatter` next time that file is touched.
  No plan filed.

[tests]: architecture/tests.md
[2608021916]: ../../plan/2608021916_arch-fix-githooks-package-split.md
[2608091910]: ../../plan/2608091910_arch-fix-mds073-collision.md
[2608301918]: ../../plan/2608301918_arch-fix-touched-set-unit-tests-0830.md
[2608301919]: ../../plan/2608301919_arch-fix-runcache-package-placement.md

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
