---
id: 2608091910
title: >-
  Resolve the MDS073 rule-ID collision between
  slidevstructure and foreignregion
status: "✅"
model: sonnet
summary: >-
  internal/rules/slidevstructure and
  internal/foreignregion both independently claim
  MDS073 — an undetected rebase/merge collision from
  2026-07-19. Renumber the foreign-region diagnostic to
  MDS074 and add a contract test that fails on any future
  rule-ID collision. Flagged by the 2026-08-09 audit.
---
# Resolve the MDS073 rule-ID collision between slidevstructure and foreignregion

## Goal

Give the foreign-region malformed-marker-pair diagnostic
its own unused rule ID. Add a contract test so a future ID
collision fails CI instead of shipping silently.

## Background

The 2026-08-09 audit (see [the audit log][audit-log]) found
that `MDS073` is claimed by two unrelated, independently
registered diagnostics:

- [slidevstructure][slidev] line 52:
  `func (r *Rule) ID() string { return "MDS073" }` — the
  Slidev slide-structure rule. Registered via
  [all.go][all] and documented at
  [MDS073-slide-structure/README.md][slide-readme].
- [foreignregion/scan.go][scan] line 25:
  `RuleID = "MDS073"` — the malformed foreign-marker-pair
  diagnostic. Wired directly into
  [internal/engine/runner.go][runner] and
  [internal/fix/fix.go][fix], and documented at
  [docs/reference/foreign-regions.md][foreign-regions-doc]
  line 76 (`## Malformed regions (MDS073)`).

Both landed the same day (2026-07-19). Slide-structure was
renumbered `MDS072 -> MDS073` in commit `7e4925a`, while the
foreign-region feature independently claimed `MDS073` for
its own diagnostic in parallel commits.

Nothing caught the collision. The foreign-region diagnostic
deliberately is not a registered `rule.Rule` — per its own
comment in `scan.go`, it "rides along" the fix/check
pipeline like generated-section ranges. It sits outside
whatever ad hoc process normally prevents ID reuse.

[cross-system.md][cross] treats rule IDs as part of the
public `.mdsmith.yml`/CLI/docs contract. Users enable and
disable rules by ID, `--explain` looks up by ID, and
per-rule docs key off the ID. Two diagnostics sharing one ID
breaks that contract for anyone who disables or enables
`MDS073` expecting only one behavior.

The foreign-region diagnostic is the newer of the two by
feature intent — it was still being built out across
several commits after slide-structure's rename. It is the
one renumbered here, to `MDS074`, the next unused ID (the
highest claimed ID across `internal/rules/*/rule.go` is
`MDS073`).

## Tasks

1. Rename `RuleID = "MDS073"` to `RuleID = "MDS074"` in
   [foreignregion/scan.go][scan].
2. Update every reference to the old ID in
   `internal/foreignregion/` (diagnostic messages, tests,
   fixtures).
3. Update [internal/engine/runner.go][runner] and
   [internal/fix/fix.go][fix] references to the
   foreign-region rule ID, if any hardcode the string
   rather than importing the constant.
4. Update [foreign-regions.md][foreign-regions-doc]: rename
   the `## Malformed regions (MDS073)` heading and body
   text to `MDS074`, and update the summary line that reads
   "a start with no matching end reports MDS073."
5. Search the rest of the docs tree
   (`docs/reference/cli.md`, any rule-coverage tables) for
   other `MDS073` references tied to the foreign-region
   feature specifically, not the slide-structure rule, and
   update them.
6. Add a contract test — e.g.
   `internal/integration/rule_id_uniqueness_test.go` —
   that collects every ID from `rule.All()` plus the known
   out-of-band IDs (currently just `foreignregion.RuleID`)
   and fails if any ID appears more than once.
7. `go build ./...` passes.
8. `go test ./...` passes.
9. `mdsmith check .` passes.

## Acceptance Criteria

- [x] `internal/foreignregion` reports `MDS074`, not
      `MDS073`, for malformed marker pairs.
- [x] `internal/rules/slidevstructure` still reports
      `MDS073`, unchanged.
- [x] `docs/reference/foreign-regions.md` reflects the new
      ID with no stale `MDS073` references to the
      foreign-region feature.
- [x] A new contract test fails if any two registered rule
      IDs (including out-of-band IDs like
      `foreignregion.RuleID`) collide, and passes on the
      fixed code.
- [x] `go test ./...` is green.
- [x] `go tool -modfile=tools/go.mod golangci-lint run`
      reports no issues.
- [x] `mdsmith check .` is green.

[audit-log]: ../docs/development/architecture-audit.md
[slidev]: ../internal/rules/slidevstructure/rule.go
[all]: ../internal/rules/all/all.go
[slide-readme]: ../internal/rules/MDS073-slide-structure/README.md
[scan]: ../internal/foreignregion/scan.go
[runner]: ../internal/engine/runner.go
[fix]: ../internal/fix/fix.go
[foreign-regions-doc]: ../docs/reference/foreign-regions.md
[cross]: ../docs/development/architecture/cross-system.md
