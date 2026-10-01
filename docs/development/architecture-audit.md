---
title: Architecture audit log
summary: >-
  Running log of SOLID and clean-architecture
  findings on origin/main. The
  solid-architecture skill (audit mode)
  appends here; blockers are also filed as
  plans.
audit-from: 979bb7fbfc7379d628b029336f3fc075dd16edab
---
# Architecture audit log

This file is maintained by the
solid-architecture skill in audit mode.
The oldest entries have moved to the
[archive shards](architecture-audit-archive.md) to stay
under the file-length budget; every finding there is
resolved.

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
- `cmd/mdsmith/rename.go`'s `detectRenameMode` and
  `pkg/mdsmith/refactor.go`'s `detectRenameKind` both wrap the
  same `refactor.FindHeadingLine` / `refactor.HasLinkRef` pair
  in an identical "both → error, heading, label, neither →
  error" switch. [go.md][go]'s "Common violations to flag":
  logic reimplemented per host surface instead of living once
  on the shared engine both the CLI and the public
  `pkg/mdsmith` API call —
  [engine-api.md][engine-api] documents the two as mirroring
  one-to-one, so a future refinement to the
  ambiguous/no-match messaging is likely to drift between them
  — [plan/2609271912][2609271912].
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
  four matching `Workspace` pass-throughs (`Resolve`
  differs by design) — [plan/2609131911][2609131911].
- `internal/refactor/move.go`'s `recomputeToken`,
  `encodePathToken`, `pathEdit`, `countFilesWithStem`
  lack tests by name ([tests.md][tests]) —
  [plan/2609131913][2609131913].

### nice-to-have (2026-09-13)

- Four `cmd/mdsmith` move/rename helpers lack test
  symbols but are covered indirectly; optional —
  [plan/2609131913][2609131913].

[2609131911]: ../../plan/2609131911_arch-fix-refactor-workspace-duplication.md
[2609131913]: ../../plan/2609131913_arch-fix-move-helper-unit-tests.md

## Audit 2026-08-30 (range: b706d76..0ca0d2f)

59 commits, ~130 files touched (~125 Go files, no
TypeScript). New packages this cycle:

- `internal/linkgraph` — link/wikilink target parsing and
  resolution, split out of the rules that used to inline it.
- `internal/schema` — a one-question-per-file split (compose,
  extend, filename, parse_file, parse_inline, validate) with
  no reverse-layer imports.
- `internal/gitattributes` and `internal/directivefiles` —
  the two other packages the 2026-08-23 `internal/githooks`
  SRP split ([plan/2608021916][2608021916]) produced. That
  plan is now fully merged (PR #815); its "picked up as this
  cycle's fix" note from 2026-08-23 is closed.

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
  `internal/gitattributes`, `internal/directivefiles`: each
  answers one question, each has dedicated tests, none
  imports `internal/rules/...`.
- `cmd/mdsmith/discover.go` is an exemplary thin shim over
  `internal/directivefiles.DiscoverFilesForInstall` — no
  domain logic in the handler. (Correction, 2026-10-01: the
  shim had no production caller since PR #213; it and
  `internal/directivefiles` were deleted under
  [plan/2608301918][2608301918].)
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
  have no dedicated unit test by name, only behavior-level
  coverage via their callers — [tests.md][tests] requires a
  test by the function's own name —
  [plan/2608301918][2608301918].
- `internal/lint/runcache.go`'s `RunCache` caches state across
  every file in a whole `engine.Run` pass, which answers a
  different question than [go.md][go]'s stated charter for
  `internal/lint` ("model a parsed Markdown file"). Closer to
  `internal/engine`'s job ("orchestrate rules over files; owns
  the run loop"). Not an import-cycle or forbidden-import
  violation — a package-boundary tax per go.md's "Split a
  package by question," now 676 lines and ten cache slots —
  [plan/2608301919][2608301919].

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

[go]: architecture/go.md
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
