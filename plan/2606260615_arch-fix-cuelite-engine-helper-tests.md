---
id: 2606260615
title: >-
  Add dedicated unit tests for unexported
  helpers in cue/cuelite/engine.go
status: "✅"
summary: >-
  Seven unexported helpers in
  cue/cuelite/engine.go lack dedicated
  unit tests. Adds TestCombineMode,
  TestMkBottom, TestTopValue,
  TestEngineValue_IsBottomV,
  TestEngineValue_DefaultValue,
  TestEngineValue_DescribeBound,
  and TestBound_Describe.
model: sonnet
---
# arch-fix: cuelite engine helper tests

## Goal

Seven unexported helpers in
`cue/cuelite/engine.go` lack dedicated
unit tests. Add a `TestFoo` for each one.
Closes the 2026-06-26 audit finding.

## Context

Audit 2026-06-26 (range: 3d35b77..fe7141b)
flagged seven unexported helpers. The file
was touched in the perf commit `e7cb8b0`
(fmt.Sprintf → strconv in `describe`).

Functions lacking dedicated tests:

- `combineMode(a, b defaultMode) defaultMode`
- `mkBottom(path []string, format string, args ...any) *engineValue`
- `topValue() *engineValue`
- `(v *engineValue) isBottomV() bool`
- `(v *engineValue) defaultValue() (*engineValue, bool)`
- `(v *engineValue) describeBound() string`
- `(b bound) describe() string`

Several are used in existing coverage
tests (`mkBottom`, `topValue`, `isBottomV`)
but lack a `TestFunctionName`-style test.
The architecture tests doc §"every function
by name" requires one.

## Tasks

1. [x] Add `TestCombineMode` in
   `cue/cuelite/engine_test.go` or a new
   `engine_helpers_test.go`. Cover all
   four `combineMode` table entries.
2. [x] Add `TestMkBottom`. Confirm the
   returned value satisfies `isBottomV()`
   and that `describe()` includes the
   formatted message.
3. [x] Add `TestTopValue`. Confirm
   `describe() == "_"` and
   `isBottomV() == false`.
4. [x] Add `TestEngineValue_IsBottomV`.
   Cover `nil` receiver (false), `kBottom`
   (true), and a non-bottom value (false).
5. [x] Add `TestEngineValue_DefaultValue`.
   Cover a value with a default (returns
   default and `true`) and a value with no
   default (returns `false`).
6. [x] Add `TestEngineValue_DescribeBound`.
   Cover a bounded integer (`>=1 & <=10`)
   and a string match constraint.
7. [x] Add `TestBound_Describe`. Cover each
   operator (`>=`, `<=`, `>`, `<`, `!=`,
   `=~`, `!~`) and `strings.MinRunes`.

## Acceptance Criteria

- [x] Each of the seven functions has a
  dedicated top-level test.
- [x] `go test ./cue/cuelite/...` green.
- [x] `go vet ./...` clean.
- [x] No production code changed; tests only.
