---
id: 240
title: "cuelite phase 4 — drop cuelang.org and enable tinygo"
status: "🔳"
model: opus
summary: >-
  With every surface flipped, delete cue/cuelite's CUE
  delegation and remove cuelang.org/go from go.mod; replace the
  tinygo-incompatible sync.Map.CompareAndDelete in
  internal/lint/runcache.go; get the standard-Go WASM build
  under the plan-215 budget (the tinygo build still fails on
  unimplemented os.* calls — criterion unmet); update the
  engine-api page and the layering map.
depends-on: [239]
---
# cuelite phase 4 — drop cuelang.org and enable tinygo

## Goal

Remove `cuelang.org/go` entirely. Make the WASM artifact fit
the plan-215 budgets on the standard-Go and tinygo toolchains.

## Context

Phase 4 of [plan 218](218_wasm-size-reduction.md). It is
reachable once surfaces A–D are flipped, so nothing delegates
to CUE anymore.

This phase is sized `opus`, not `sonnet`. The original `sonnet`
estimate assumed task 1 was a delegation deletion. The phase-2
audit corrected that: the standing `cuelang.org/...` dependency
is the parser and AST, so task 1 is now writing a hand-rolled
CUE-subset parser and lexer (the grammar plus the underscore and
escape rules) and re-pointing the evaluator at it. That is the
largest and most error-prone task in the cuelite series, well
above the `sonnet` band.

## Tasks

1. Replace the SYNTAX FRONTEND. After the phase-2/3 flips the only
   remaining non-test `cuelang.org/...` use is the parser and AST:
   `compile.go` imports `cue/parser`, `cue/ast`, `cue/literal`, and
   `cue/token`; `eval.go` imports `cue/ast` and `cue/token`; and
   `evalrow.go` (the surface-C row evaluator, plan 239) imports all
   four. The evaluator already walks an in-house value model, so this
   task swaps the AST it walks. Write a hand-rolled CUE-SUBSET parser
   (the front-matter/row-expr grammar only: structs, fields, lists, the
   bounds and disjunction operators, comparisons, the single `if`/`for`
   comprehensions, the row-expr `\(…)` interpolation across the plain,
   raw, and multiline dialects, the `+`/`*` operators, calls, and
   string/number/bool/null literals with CUE underscore and escape
   rules) producing an in-house syntax tree, then re-point
   `compileExpr`/`evalExpr` and `parseRowExpr`/`evalRow` at it.
   `literal.Unquote`, `literal.ParseQuotes`, and the `token` constants
   are replaced by the new parser's own lexer. This is the last and
   largest CUE dependency to remove; it is the bulk of the phase.
2. Delete the CUE delegation from `cue/cuelite` and remove
   `cuelang.org/go` from `go.mod` and `go.sum`. Confirm no
   non-test file imports `cuelang.org/...`.
3. Delete `internal/cuelitetest` (or port its corpus to pure
   in-house self-tests with no oracle). Its non-test files
   `cuelitetest.go` (the schema/data oracle) and `expr.go` (the
   surface-C row-expr oracle plan 239 added) import `cuelang.org/go`
   for the direct-CUE oracle path, so they must go before
   `cuelang.org/go` can leave `go.mod`. Once every surface is flipped,
   the in-house path and the oracle are no longer two implementations
   to diff — the oracle's whole purpose ends — so the harness is
   removed rather than kept. Its oracle-backed test files
   (`cuelitetest_test.go`, `fuzz_validate_test.go`, `bench_test.go`,
   `factor_gate_test.go`, and the surface-C `expr_test.go` /
   `expr_unit_test.go`) go with it.
4. Migrate or delete the remaining TEST-ONLY `cuelang.org/...`
   imports — `go.mod` removal needs the build graph clean of CUE
   in test files too, not only non-test files. The current set is:
   `internal/schema/shortcuts_test.go` (imports `cue/cuecontext`
   to compile its shortcut CUE — rewrite against the cuelite
   façade or drop the direct-CUE assertion), and `cue/cuelite`'s
   own oracle-backed tests
   (`value_test.go`, `coverage4_test.go`, `coverage5_test.go`,
   `coverage6_test.go` import `cue/cuecontext`, `cue/ast`,
   `cue/token`, or `cue/errors`). Port each to in-house assertions
   or the new parser's tree before the module leaves. Re-run
   `grep -rl cuelang.org/ --include=*_test.go` until it is empty.
5. Replace `sync.Map.CompareAndDelete` in
   [runcache.go](../internal/lint/runcache.go) with a
   mutex-guarded map, red/green.
6. Get the standard-Go and `tinygo build -target wasm
   ./cmd/mdsmith-wasm` builds passing; tighten
   [size_test.go](../cmd/mdsmith-wasm/size_test.go) to the new
   budgets.
7. Update the
   [engine-api page](../docs/background/concepts/engine-api.md)
   and the `cue/` entry in the
   [layering map](../docs/development/architecture/index.md).

## Acceptance Criteria

- [x] `cuelang.org/go` is absent from `go.mod` and `go.sum`;
      NO file imports `cuelang.org/...` — test files included
      (`internal/schema/shortcuts_test.go` and
      `internal/extract/cue_diff_test.go` migrated off CUE, and the
      `cue/cuelite` tests rebuilt on the in-house syntax tree), since
      a test-only import alone keeps the module in the build graph.
      `cockroachdb/apd` and protobuf are dropped as orphaned transitives.
- [x] `internal/cuelitetest` is deleted; its corpus is ported to
      oracle-free in-house self-tests (`cue/cuelite/corpus_test.go`,
      `rowcorpus_test.go`), its fuzzers to engine-only smoke fuzzers
      (`fuzz_test.go`), and its CUE-relative factor gate to an absolute
      allocs/op guard (`bench_test.go`). No package imports
      `cuelang.org/...` from a non-test file.
- [x] Standard-Go WASM artifact ≤ 18 MB — measured ~11.2 MB raw /
      ~2.8 MB gzipped; `cmd/mdsmith-wasm/size_test.go` asserts the
      tightened ceilings (14 MiB raw / 4 MiB gzip).
- [x] `tinygo build -target wasm ./cmd/mdsmith-wasm` succeeds and is
      ≤ 8 MB. The `os.Chmod`, `os.SameFile`, and
      `os.Symlink`/`filepath.EvalSymlinks` calls are now behind
      build-tagged seams (plan 247). `TestTinyGoWASMArtifactSizeBudget`
      asserts the 8 MB ceiling; the `tinygo-wasm` CI job enforces it.
- [x] `Capabilities()` is unchanged — `methods_test.go` /
      `smoke_test.go` assert the WASM proxy advertises the same
      capability set as the native session.
- [x] All tests pass: `go test ./...` (and `-race` clean on the
      affected packages).
- [🔳] `go tool golangci-lint run` reports no issues — the tools/go.mod
      golangci-lint needs Go ≥ 1.25.8; the dev container has 1.25.0, so
      this is CI-verified.

## Implementation Notes

The parser (task 1) emits an in-house syntax tree. It does not mimic
`cue/ast`. The `cue/cuelite/syntax` package defines node types. They use
the same names the three consumers already switched on. So re-pointing
`compile.go`, `eval.go`, and `evalrow.go` was a near-mechanical import
swap.

One divergence from CUE's tree is deliberate. `Interpolation.Elts`
carries already-decoded string fragments. The scanner decodes them
across the three dialects. So `evalRowInterpolation` reads them direct.
The `token` and `literal` calls map to the in-house token set,
`syntax.Unquote`, and `syntax.IsBytesLiteral`.

Conformance was proven before the oracle was deleted. The frontend was
flipped first. The `internal/cuelitetest` differential harness then ran
green over the corpus, the fuzz seeds, the real schemas, and the
row-expr suite. Only then was the harness deleted, with its corpus
ported to engine-only pinned tests.

## See also

- [Plan 218 — in-house CUE-subset engine](218_wasm-size-reduction.md)
