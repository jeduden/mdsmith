---
title: Architecture audit log
summary: >-
  Running log of SOLID and clean-architecture
  findings on origin/main. The
  solid-architecture skill (audit mode)
  appends here; blockers are also filed as
  plans.
audit-from: fe7141beb32f9f20d82476fd8d652e0f63d4e4ef
---
# Architecture audit log

This file is maintained by the
solid-architecture skill in audit mode.

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

Plan 153 kept the workspace symbol
index at `internal/lsp/index`. Its
stated non-goal: "only link/edge
extraction is in scope." Plan 174
supersedes that. The package is now
`internal/index`, a peer support
package.

The move is a pure `git mv`; no logic
changed. Two forces drove it.
`internal/schema` already imported the
index from outside `internal/lsp`. The
new `mdsmith rename` and `mdsmith deps`
surfaces need it too, and the layering
map forbids `cmd/mdsmith` →
`internal/lsp`. A peer package removes
the conflict. `internal/index` must
never import `internal/lsp`.

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

### nice-to-have (2026-05-19)

`cue/types` not in layering map — [plan/206][206].

## Audit 2026-05-31 (range: 4809097..37488a7)

Plans 200, 201, 202 green. Tax:
[plan/223][223] (`pkg/mdsmith` private helpers),
[plan/224][224] (`internal/lint` SRP, now 12 files).
`linkstyle` helpers — add tests in-place.

[223]: ../../plan/223_arch-fix-mdsmith-helper-tests.md
[224]: ../../plan/224_arch-fix-lint-srp.md

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

Parity + Layer-0 parse-skip series; symlink
containment; engine panic recovery; VS Code
`kinds` and `rule-doc` commands; security
hardening batch. 270 Go/TS sources outside
fixtures.

No blockers. New rule / convention packages
follow the OCP barrel pattern correctly; no
rule-to-rule imports added; no DIP violations
in the new `internal/rules/listscan` helper
(it follows the established `astutil` /
`fencepos` pattern).

### tax (2026-06-21)

- `internal/engine/runner.go` (1 290 lines) —
  SRP violation. Seven concerns in one file:
  file dispatch, Layer-0 skip gate,
  config-resolution cache, source-mode lint
  path, front-matter parsing, config-target
  rules, and logging. Go arch doc
  §"Common violations to flag" names engine
  as a dumping-ground risk. Fixed this cycle:
  split into `runner_layer0.go`,
  `runner_cache.go`, `runner_log.go` —
  [plan/2606211907][2606211907].

- `internal/lint/layer0.go` (1 203 lines) —
  the full Layer-0 block scanner in one file:
  types, scanner state machine, HTML-block
  detection (types 1–7), fence handling, ATX
  heading, indented code, paragraph. Maintenance
  risk at this size. Fix: split along
  block-type sub-parsers —
  [plan/2606211908][2606211908].

- `internal/lsp/server.go` (1 007 lines) —
  plan 203 was green but the file has crept
  back over 1 000 lines with the new `kinds`
  and `rule-doc` capability wiring. Checklist
  names it explicitly. Fix: apply the same
  dispatch-group split plan 203 described —
  [plan/2606211909][2606211909].

### nice-to-have (2026-06-21)

- `pkg/mdsmith/workspace.go` trivial methods
  (`memFile.Close`, `memDir.Close`,
  `memDirEntry.Name`, `memDirEntry.IsDir`,
  `memFileInfo.Name`, `memFileInfo.Size`) lack
  the one-line "no test by design" exemption
  comment the audit policy requires. Tests doc
  §"Exemptions" — [plan/2606211910][2606211910].

[2606211907]: ../../plan/2606211907_arch-fix-runner-srp-split.md
[2606211908]: ../../plan/2606211908_arch-fix-layer0-split.md
[2606211909]: ../../plan/2606211909_arch-fix-lsp-server-split.md
[2606211910]: ../../plan/2606211910_arch-fix-workspace-exemptions.md

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

### tax (2026-06-26)

- `internal/lint/layer0_html.go` — seven
  helpers lack dedicated tests; file entered
  touched set via perf commit:
  `openHTMLBlock`, `tagName.lowerInto`,
  `type7TagIsRawText`, `type7TagBytes`,
  `isTagByte`, `htmlBlockCloses`,
  `scanner.tryHTMLBlock`. Tests doc
  §"every function by name" —
  [plan/2606260211][2606260211].

[2606260211]: ../../plan/2606260211_arch-fix-layer0-html-helper-tests.md
