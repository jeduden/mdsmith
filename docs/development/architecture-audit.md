---
title: Architecture audit log
summary: >-
  Running log of SOLID and clean-architecture
  findings on origin/main. The
  solid-architecture skill (audit mode)
  appends here; blockers are also filed as
  plans.
audit-from: 0ca0d2f7c95ab53e3e9ed8851150af98b95088fb
---
# Architecture audit log

This file is maintained by the
solid-architecture skill in audit mode.
The oldest entries have moved to the
[archive shards](architecture-audit-archive.md) to stay
under the file-length budget; every finding there is
resolved.

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
  domain logic in the handler.
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
  untouched).
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
[cross]: architecture/cross-system.md
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

## Audit 2026-08-16 (range: 2ab4b29..81f0d96)

185 commits, 227 files touched (187 Go files). No
TypeScript changes.

No rule-to-rule imports. No reverse-layer imports. No
Liskov breaks. `cmd/mdsmith/main.go` and
`internal/lsp/server.go`/`symbols.go` stayed well under
the ~1000-line threshold. The MDS073 collision from the
prior cycle stays resolved — `rule_id_uniqueness_test.go`
now guards it as a contract test.

Clean surfaces, verified:

- Every touched `internal/rules/*` package (catalog,
  duplicatedcontent, markdownflavor, occurrence,
  requiredstructure, slidevstructure, externallink, and
  17 more) imports only shared helper packages — zero
  rule-to-rule imports.
- `internal/engine/source_config_cache.go` (new): a
  cache hit returns a fresh `cloneRules` copy per caller,
  never the shared template pointer, pinned by a
  dedicated `-race` concurrency test.
- `internal/pack/apm.go` (new): scoped correctly, no
  cross-layer imports, registered via the existing
  `pack.register` plugin point.
- `internal/rules/externallink/probe_net.go`'s SSRF
  hardening (`isRestrictedIP`, `ssrfControl`,
  `ssrfCheckRedirect`): 9 dedicated tests, no
  architecture concerns.
- The two new `links:` settings
  (`external-allow-internal`, `external-max-probes`) live
  in MDS072's own settings struct — not a "field reachable
  from only one rule" violation, consistent with the
  existing `links:` precedent.

### blockers (2026-08-16)

None.

### tax (2026-08-16)

- `pkg/mdsmith/session.go`'s `readBoundedFrontMatterSource`
  — the bounded/fallback front-matter read path on the
  public `pkg/mdsmith` engine API — had no dedicated unit
  test; only exercised indirectly via
  `TestSessionKindsOversizedFile*`.
  [tests.md][tests] requires a test by the function's own
  name, and `pkg/mdsmith` is the highest-blast-radius
  surface touched this cycle. Fixed directly (not filed as
  a plan): added `TestReadBoundedFrontMatterSource`, a
  table-driven test in `session_test.go` covering the
  bounded (`OSWorkspace`) and fallback (`MemWorkspace`)
  paths, the missing-file error, and both `max<=0` and
  `math.MaxInt64` unbounded cases. `go test ./...` and
  `go tool golangci-lint run` are green.
- Six more functions across `cmd/mdsmith`, `pkg/mdsmith`,
  `internal/rules/duplicatedcontent`,
  `internal/rules/astutil`, `internal/bytelimit`, and
  `pkg/markdown/flavor` have no dedicated unit test by
  name, each covered only behaviorally —
  [plan/2608161914][2608161914].

### nice-to-have (2026-08-16)

- `internal/lsp/server_diagnostics.go`'s
  `surfaceForeignDiagnostics` changed the
  `window/logMessage` notification shape from one
  notification per diagnostic to at most two batched
  notifications grouped by severity — a behavior change on
  the LSP wire surface [cross-system.md][cross] tracks.
  Well-tested; worth a one-line changelog note per that
  page's "breaks must be deliberate and noted" policy. No
  plan filed.
- `internal/engine/source_config_cache.go`'s
  `NewSourceConfigCache` is a trivial one-line constructor
  without the "no test by design" exemption comment
  [tests.md][tests] asks for on untested trivial functions.
  Documentation nit; the type it constructs is otherwise
  exhaustively tested. No plan filed.

[2608161914]: ../../plan/2608161914_arch-fix-touched-set-unit-tests.md
