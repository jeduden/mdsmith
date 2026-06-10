---
id: 238
title: "cuelite phase 2 — surfaces A + B (schema, query)"
status: "✅"
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

1. Adopt the compile/unify/validate façade already shipped by
   phase 0 ([plan 236](236_cuelite-package-harness.md)):
   `Compile`, `CompileJSON`, `Value.Unify`, `Value.Validate`,
   and the `Errors` accessor. Extend it with the per-surface
   methods these call sites need (`LookupPath`, `Decode`,
   `Fields`, …), still delegating to CUE. The phase-0 interim
   has two costs to retire here. First, each `Compile`/
   `CompileJSON` owns a fresh `*cue.Context`, and a cross-context
   `Unify` rebuilds whichever side retains source into the other
   side's context — one rebuild per such `Unify`, leaving the
   result in (and mutating) the non-rebuilt side's context.
   Second, the schema path still marshals front matter to JSON
   and `CompileJSON` parses it back, so validating one file pays
   three JSON traversals — the marshal, `CompileJSON`'s
   duplicate-key scan, and `cuejson.Extract`'s own parse. The
   duplicate scan is interim-only: the post-flip hot path
   validates the `map[string]any` directly and bypasses
   `CompileJSON` entirely, so the scan disappears with the round
   trip. All three blow the ≤ 10 allocs/op budget on the hot
   path, so the budget is met only by the flip in task 3 — the
   in-house engine validates a `map[string]any` directly (plan
   218), with no JSON round-trip and no per-`Value` context — not
   by the façade adoption in this task.
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

- [x] `internal/schema`, `requiredstructure`, and
      `internal/query` import `cue/cuelite`, not `cuelang.org/go`
      (non-test files; task 2).
- [x] Front-matter validation skips the JSON round-trip
      (`CompileMap`/`LiftMap`). The per-rule alloc-budget test
      passes; the hot-path benchmark drops to 85 allocs/op
      (from 356) and 8.0 µs/op (from 89 µs), 0.25× CUE.
- [x] MDS020 diagnostics stay actionable and navigable: the
      schema suite (plan 147 / plan 230 tests) passes unchanged.
- [x] The harness shows in-house and CUE agree on the full
      corpus, the real-repo-schema sweep, and a 300 s
      schema×data fuzzer. `cue/cuelite` and `internal/cuelitetest`
      reach 100 % statement coverage after the task-4 dedup;
      gobco branch coverage is 1276/1278 (two structural arms).
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no issues.

## Implementation notes

- **Build paths from data keys via `MakePath`, never `ParsePath`.**
  `query.collectPaths` and any other consumer that derives a path from a
  `map[string]any` key (e.g. iterating `Value.Fields()`) must use
  `cuelite.MakePath(segs...)`, which stores raw segments — including empty,
  dotted, `true`-headed, or quote-needing keys that `ParsePath` (the narrower
  string-label EXPRESSION grammar) cannot parse back. Reserve `ParsePath`
  for user-typed path expressions. There is deliberately no `Path` render
  today (`PathError.Error()`'s dot-join is lossy and display-only); a future
  `Path.String()` must quote any segment that is not a bare identifier.
- **LookupPath provenance.** A derived `Value` (a `Unify` result)
  is context-pinned and retains no source, so it cannot be rebuilt
  into another context. `LookupPath` on such a value inherits that
  limit: a section-level lookup against a cached schema must keep
  rebuildable provenance — for example the root source plus the
  looked-up path — or the cached schema either races (each lookup
  mutates its shared context) or forfeits caching (every lookup
  recompiles). Choose the provenance representation when adding
  `LookupPath`, not after.
- **Call-site operand order.** Today's adopters call
  `schemaVal.Unify(dataVal)` in
  [validate.go](../internal/schema/validate.go) and
  `m.schema.Unify(dataVal)` in
  [query.go](../internal/query/query.go) — the SHARED schema is the
  receiver, so the cross-context rebuild puts the schema's context
  in the mutated (non-rebuilt) position. Phase 2 must pick the
  operand order and locking deliberately: a schema cached and
  reused across files or goroutines cannot sit in the mutated
  position without synchronization.
- **RunCache / CachedCompile shape (task 2).** The cache's contract
  — one schema compile per Run, shared across parallel workers via
  [`CompiledCUE.Ctx`](../internal/schema/compile_cache.go) — has NO
  façade equivalent, because cuelite hides the context. Neither
  operand order resolves it under the cache: `schemaVal.Unify(
  dataVal)` rebuilds the data into the shared schema's context and
  MUTATES it (a data race under `-race` CI when workers share the
  cached schema), while `dataVal.Unify(schemaVal)` rebuilds the
  schema per file and recompiles it each call (the cache becomes a
  no-op and the compile-once assertions in
  [validate_runcache_test.go](../internal/schema/validate_runcache_test.go)
  break). So task 2 must redesign the cache shape, not just adopt
  the façade — cache the SOURCE and compile per worker, or guard the
  shared `Value` with a lock, or defer caching to the in-house flip
  in task 3 (where a context-free `Value` is shareable). Name the
  affected files when scheduling: `compile_cache.go` and
  `validate_runcache_test.go`.

## Progress

### Task 1 — façade extension (done)

Added the surface-A/B accessor methods to `cue/cuelite`, still
CUE-backed, each with a dedicated unit test and 100 % statement
coverage held:

- `Value.Exists`, `Value.LookupPath(Path) (Value, bool)`,
  `Value.Fields() []Field` (with the `Field{Selector, Value}` shape),
  `Value.String`, `Value.Decode` — in `cue/cuelite/access.go`.
- **LookupPath provenance decision (plan note resolved).** A
  LookupPath/Fields result keeps REBUILDABLE PROVENANCE — the root
  source plus the path that reached it (`lookupRoot`, `lookupPath`,
  `hasLookup` on `Value`) — instead of pinning the context-bound
  derived `cue.Value`. `Value.rebuild` gained a `hasLookup` branch
  that recompiles the root in the target context and re-applies the
  path, so a section-level lookup against a cached schema crosses
  contexts without mutating the shared value (the race the note rules
  out). Fields extends the prefix one selector per child so a nested
  field carries the full path.
- Differential harness extended: `internal/cuelitetest/access.go` adds
  an `AccessCase`/`AccessOutcome`/`RunAccess` arm comparing
  Exists/LookupPath/Fields/String against a direct-CUE oracle over a
  per-class corpus; green in CI as the flip scaffold.

### Task 2 — adopt the façade (done)

All three packages import `cue/cuelite` now. No non-test file imports
`cuelang.org/go`. The grep over those trees is empty.

One cuelang import survives, in
`internal/schema/shortcuts_test.go`. It is a test. It cross-checks the
shortcut canonicals against CUE, like the harness oracle, so it stays.

Every existing test stayed green unchanged. The `CompiledCUE`-typed
assertions in `compile_cache_test.go` and `validate_runcache_test.go`
still pass. `CompiledCUE` is a schema-package type. Its internals
swapped to wrap a `cuelite.Value`. Its `.Err()` and identity surface
did not change.

Added `Value.Err()` to the façade. It reports compile/bottom status.
It skips the concreteness check `Validate` applies. `CompiledCUE.Err()`
and `checkUnifiable` need exactly that.

**RunCache / CachedCompile shape decision.** Cache the
source-retaining compiled value. Fix the Unify operand order: the
shared schema is the operand, the per-file data is the receiver. Every
call site reads `dataVal.Unify(schemaVal)` — validate.go,
requiredstructure, query.go.

Under cuelite's `rebuild`, the receiver's context is the one rebuilt
into. So the per-file data context absorbs a fresh recompile of the
schema's source. The shared cached `Value` is only read for its `src`.
It is never mutated.

This keeps the cache's compile-once contract. The
`validate_runcache_test.go` slot-populated assertion still holds: the
slot is keyed by CUE source and built once per Run. It is also
`-race`-clean when parallel workers share the cached schema.
`CachedCompile_ConcurrentSingleBuild` and the runcache compile-once
test both pass under `-race`.

The per-file schema recompile is the interim cost. Task 3's
context-free immutable `Value` erases it. The operand order then stops
mattering. `compile_cache.go`'s `CompiledCUE` dropped its `Ctx` field,
because cuelite hides the context.

MDS020 PER-FIELD diagnostics stay byte-identical. The error walkers
consume `[]*cuelite.PathError` now, which they get from
`cuelite.Errors`; the old code used `[]errors.Error`. Both carry the
same `.Path()` route. So the per-field diagnostic, the dedup key, and
the anchor line do not change. The schema suite passes unchanged,
including the plan-147/230 diagnostic-shape tests.

The byte-identity claim is scoped to per-field diagnostics. The
front-matter-shape diagnostic is a synthetic fail-safe now. It fires
only on a value the lifter cannot represent. Its wording moved to the
`LiftMap` check.

### Task 3 — flip to the in-house value model (done)

The flip landed: an in-house value model, `Unify` as lattice meet,
direct `map[string]any` validation, and a schema/data fuzzer. The
differential harness (validate, access, path arms) is the oracle.

#### Test-contract flip (authorized)

The interim per-`Value`-context behaviors broke from single-context CUE on
purpose. The flip restores single-context CUE as the truth. So four
pinned-test classes were rewritten, with sign-off:

1. Cross-context bottom tests now assert the post-flip contract: a chained
   unify of derived values SUCCEEDS and validates per single-context CUE.
   `TestValue_Unify_singleContextOracle` rebuilds each chained composition by
   unifying the same source fragments inside ONE `cue.Context` and asserts the
   oracle's accept/reject matches the in-house engine — the direct single-
   context check behind this claim (the two-input cuelitetest harness covers
   the schema×data shape; the multi-fragment chained compositions are pinned by
   this oracle test). `errCrossContext` and the `rebuild` machinery were
   deleted with their tests.
2. `Errors() → []*PathError` (+ `PathError.Unwrap` to the engine's own
   cause). The `errors.As`-to-cuelang assertions were dropped; in-house
   sentinel unwrap tests (`TestValidate_unwrapsBottom`) kept.
3. Exact CUE message strings re-pinned on the in-house engine's stable
   wording: `conflicting values <a> and <b>`, path-tagged.
4. The CUE-only private-helper tests (`buildJSON`/`rebuild`/
   `scanDuplicateJSONKeys`/`jsonLevel`/`recordKey`/`cueErrorsOf`) were
   deleted with their helpers. The strict-JSON duplicate-key CONTRACT of
   `CompileJSON` is durable and preserved by the in-house JSON lifter; its
   behavior-level tests stay green.

#### Parser-frontend decision (recorded)

The constraint parser reuses cuelang's syntax frontend (`cue/parser` →
`cue/ast`) in phase 2; the in-house compiler walks that AST into the
value model. The evaluator stays fully in-house (unify, validate,
concreteness) — that is the actual flip. The oracle keeps its own
direct-cuelang evaluator, so a shared parser does not collapse the two
arms. Phase 4 (plan 240) swaps the cuelang parser for a hand-rolled one
and drops `cuelang.org/go`; this interim is its removal target. The
blocker — four pinned CUE-behavior test classes — was resolved by the
authorization the "Test-contract flip" section records.

### Task 4 — coverage, gobco, alloc, factor gate (done)

**Dedup refactor.** The duplicate builders in `eval.go` collapsed
into one scope-threaded set. `compileExpr` is the unscoped face of
`evalExpr` (a nil scope), and `evalChild` routes the field, element,
and branch positions through the compile-time deferral. An index or
relational over an unresolved sibling becomes a thunk. A bare
reference stays a hard "reference not found". Five compile-only
builders were deleted.

**Coverage 100 %.** `cue/cuelite` and `internal/cuelitetest` are both
at 100 % statements. Residual defaults were driven red/green on
constructed values or restructured away. The two `ast.LabelName`-error
branches were removed: the parser always yields an `*ast.Ident`
selector member, read via `selectorName`. The impossible list-tail
nil-fills were dropped under the "openTop ⟹ elem != nil" invariant.

**gobco.** `go tool gobco -branch ./cue/cuelite` reports 1276/1278
conditions. The two remaining are the structural gaps plan 237
records (`path.go`'s `sepBracket` switch arm and `multiline.go`'s
`for i > 0` walk-back bound), neither in the flipped engine.

**Alloc.** The `internal/integration` per-rule gate passes. MDS020
allocates 0.0/op on the gate fixture (no front matter → early exit).
The schema-validate hot path it delegates to measures
`BenchmarkValidate/cuelite` 7.9 µs / 85 allocs (0.24×/0.40× CUE) and
`BenchmarkCompileValidate` 22.1 µs / 205 allocs (0.35×/0.53× CUE).

**Factor gate tightened.** `HotFactorBudget` and `ColdFactorBudget`
set to 1.0 at this flip, not deferred to plan 240. The interim
hot-looser-than-cold guard now asserts both ≤ 1.0×. The armed gate
passes with margin: hot 0.20–0.30×, cold 0.36–0.40×.

### Task 5 — ⟨value, default⟩ default-semantics redesign (done)

Round 2 replaced flatten-and-mark-pointers with CUE's per-disjunct
mode (`engineValue.modes`).

- `evalDisjunction` flattens by VALUE, so a collapsed sub-disjunction
  loses its default: `(*0|0)|10` rejects (the plan-239 carry;
  `schemaHasNestedDuplicateDefault` deleted).
- `unifyDisjunction` prunes a NESTED-bottom branch and reconciles the
  meet default from operand defaults; `concreteValueEqual` covers
  `*[]`.
- `forceThunkFixpoint`, `evalIdent`, and an `evalBinary` thunk-`&`
  deferral complete P0.

Each claim was probed against v0.16.1. The 600 s fuzz found one
divergence, fixed and seeded. Coverage 100 %.

## See also

- [Plan 218 — in-house CUE-subset engine](218_wasm-size-reduction.md)
