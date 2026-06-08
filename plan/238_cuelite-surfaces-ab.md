---
id: 238
title: "cuelite phase 2 — surfaces A + B (schema, query)"
status: "🔲"
model: opus
summary: >-
  Move internal/schema, requiredstructure, and internal/query
  onto cue/cuelite's compile/unify/validate façade (green),
  then flip to the in-house value model and validator —
  validating front matter directly without the JSON round-trip,
  preserving per-field MDS020 diagnostics, checked against the
  CUE oracle across the whole corpus.
depends-on: [237]
---
# cuelite phase 2 — surfaces A + B (schema, query)

## Goal

Move schema validation (MDS020) and `query`/`where:` onto
`cue/cuelite`, then flip them to the in-house value model and
validator.

## Context

Phase 2 of [plan 218](218_wasm-size-reduction.md), the largest
migration. Surface A is
[internal/schema](../internal/schema/validate.go) and
[requiredstructure](../internal/rules/requiredstructure/rule.go);
surface B is [internal/query](../internal/query/query.go), a
strict subset of A. See plan 218 for the value model and the
unification rules.

## Tasks

1. Add the compile/unify/validate façade to `cue/cuelite`,
   delegating to CUE.
2. Move [internal/schema](../internal/schema),
   [requiredstructure](../internal/rules/requiredstructure/rule.go),
   and [internal/query](../internal/query/query.go) onto the
   façade. The suite stays green.
3. Flip to the in-house value model, `Unify`, and `Validate` —
   red/green per rule and per ⊥/error path. Validate
   front-matter maps directly, with no JSON marshal.
4. Hold the differential harness green across every
   `frontmatter:` constraint, the
   [file-kinds conflict table](../docs/guides/file-kinds.md),
   and the query/`where:` examples.

## Acceptance Criteria

- [ ] `internal/schema`, `requiredstructure`, and
      `internal/query` import `cue/cuelite`, not `cuelang.org/go`.
- [ ] Front-matter validation skips the JSON round-trip and
      stays within the ≤ 10 allocs/op budget.
- [ ] MDS020 diagnostics stay actionable and navigable (plan
      147 / plan 230 behavior preserved).
- [ ] The harness shows in-house and CUE agree on the full
      corpus; `cue/cuelite` keeps 100 % statement and branch
      coverage.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues.

## See also

- [Plan 218 — in-house CUE-subset engine](218_wasm-size-reduction.md)
