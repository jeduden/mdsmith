---
id: 239
title: "cuelite phase 3 — surface C (row-expr evaluator)"
status: "🔲"
model: opus
summary: >-
  Move internal/cuetemplate onto cue/cuelite (green), then flip
  catalog row-expr evaluation to an in-house tree-walking
  evaluator — string interpolation, for/if comprehensions, the
  ternary idiom, field selection, and the strings.Join and len
  builtins — checked against the CUE oracle on the real
  row-expr corpus.
depends-on: [238]
---
# cuelite phase 3 — surface C (row-expr evaluator)

## Goal

Move catalog `row-expr` evaluation onto `cue/cuelite`, then
flip it to an in-house tree-walking evaluator.

## Context

Phase 3 of [plan 218](218_wasm-size-reduction.md). Surface C
evaluates a CUE expression returning a string. It is the
richest surface but the narrowest in real use: the only live
builtin is `strings.Join`. See plan 218 for the evaluator
design.

## Tasks

1. Add the expression façade to `cue/cuelite`, delegating to
   CUE's evaluator.
2. Move [cuetemplate](../internal/cuetemplate/cuetemplate.go)
   onto the façade. The suite stays green.
3. Flip to an in-house tree-walking evaluator: string
   interpolation, `for`/`if` comprehensions, indexing, field
   selection, the ternary idiom, and a `strings.Join`/`len`
   builtin registry. Red/green per node and per builtin.
4. Gate it on the real `row-expr` in
   [markdownlint-coverage](../docs/research/markdownlint-coverage/README.md)
   plus unit tests, checked against the CUE oracle.

## Acceptance Criteria

- [ ] `internal/cuetemplate` imports `cue/cuelite`, not
      `cuelang.org/go`.
- [ ] The in-house evaluator matches CUE on every checked-in
      `row-expr`.
- [ ] `cue/cuelite` evaluator code keeps 100 % statement and
      branch coverage.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues.

## See also

- [Plan 218 — in-house CUE-subset engine](218_wasm-size-reduction.md)
