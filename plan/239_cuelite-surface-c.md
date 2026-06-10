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

The façade shape differs from surfaces A/B.
[cuetemplate.Compile](../internal/cuetemplate/cuetemplate.go) is
PARSE-ONLY (`parser.ParseFile`). References like `fm.id` resolve
only at `Render`, when `buildSource` injects the frontmatter
scope. `cuelite.Compile` instead EVALUATES. Feeding it a bare
row-expr would reject the unresolved references with "reference
not found". So the façade needs two new entries surface C alone
uses: a parse-without-evaluate entry (the `Compile` analogue), and
an evaluate-expression-against-a-scope entry (the `Render`
analogue that takes the frontmatter map / alias bindings).

`Template` also caches one `*cue.Context` at Compile and reuses
it across `Render` calls. That reuse does not map onto cuelite's
per-call fresh-context model. So the interim façade pays a fresh
context per catalog row until the context-free in-house evaluator
lands.

### What phase-2 `eval.go` already provides

Plan 238's flip landed a scope-threaded tree-walking evaluator,
`evalExpr(ast.Expr, scope)`. Surface C extends it, not replaces
it. It already walks these nodes:

- basic literals,
- identifiers (sibling-field references and the type keywords),
- index expressions,
- binary expressions (comparison, `&`, `|` disjunction),
- parens,
- list literals with the open tail,
- struct literals (fields, embeds, ellipsis),
- unary expressions,
- the `close` and `strings.MinRunes` calls,
- the single-clause `if` comprehension.

References resolve against `scope`. An unresolved one becomes a
thunk (`deferToThunk`) the force pass re-runs once a sibling binds.

### What surface C still has to add

The row-expr language needs four constructs `evalExpr` does NOT
yet handle:

- string INTERPOLATION (`"\(fm.id)"`) — no `*ast.Interpolation`
  case exists,
- `for` comprehension clauses — `evalComprehension` rejects
  anything but a single `if` clause,
- general SELECTOR evaluation (`fm.id`) — a bare `*ast.SelectorExpr`
  is rejected outside a builtin call, so a frontmatter field
  access does not resolve,
- a BUILTIN REGISTRY — only `close` and `strings.MinRunes` are
  wired; the live row-expr builtin `strings.Join` and `len` over a
  string/list are missing.

### Resolved in phase 2: nested-disjunction default

The phase-2 fuzzer found a disjunction-default class the in-house
engine got wrong. It was a parenthesized nested disjunction whose
value collapses to one branch (`{A: (*0|0)|10}`). Plan 238's review
round 2 fixed it in the ⟨value, default⟩ redesign. The evaluator now
keeps the sub-disjunction boundary (`flattenDisjunct`). A collapsed
sub-disjunction loses its default. The skip is deleted; surface C
inherits the corrected semantics.

## Tasks

1. Add the expression façade to `cue/cuelite`, delegating to
   CUE's evaluator: a parse-without-evaluate entry and an
   evaluate-against-a-scope entry (see "façade shape" above).
2. Move [cuetemplate](../internal/cuetemplate/cuetemplate.go)
   onto the façade. The suite stays green.
3. EXTEND `evalExpr` (the phase-2 tree-walking evaluator) with the
   four missing constructs, red/green per node and per builtin:

  - an `*ast.Interpolation` case that evaluates each embedded
     expression against scope and concatenates,
  - a `for`-clause arm in `evalComprehension` (and its
     multi-clause shape),
  - a scoped `*ast.SelectorExpr` case so `fm.id` resolves a
     frontmatter field against scope, not just inside a call,
  - a builtin registry replacing the ad-hoc `compileCall` switch,
     adding `strings.Join` and `len`.

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
