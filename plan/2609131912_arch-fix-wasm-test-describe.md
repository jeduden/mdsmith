---
id: 2609131912
title: >-
  Wrap wasm-runtime.test.ts cases in describe blocks
status: "🔲"
model: haiku
summary: >-
  editors/obsidian/src/wasm-runtime.test.ts uses bare test(...)
  calls with no describe(...) wrapper, unlike every sibling
  *.test.ts file in editors/obsidian/src/. Flagged by the
  2026-09-13 audit as tax.
---
# Wrap wasm-runtime.test.ts cases in describe blocks

## Goal

Wrap every test case in [wasm-runtime.test.ts][test-file] in
a `describe(...)` block.

This matches every sibling `*.test.ts` file in
`editors/obsidian/src/`.

## Background

The 2026-09-13 audit (see [the audit log][audit-log]) found
this gap.

[wasm-runtime.test.ts][test-file] already imports `describe`
from `bun:test` but never calls it.

Its twelve `test(...)` cases cover `check`, `fix`,
`invalidate`, `capabilities`, `rename`, `move`, and the
engine cache lifecycle.

The TypeScript binding in [audit-checklist.md][audit]
requires a `describe("name")` block around one or more
`test(...)` cases.

Every sibling `*.test.ts` file under
`editors/obsidian/src/` (`actions.test.ts`,
`wiring.test.ts`, `config.test.ts`, …) already follows
this shape.

This is a mechanical, no-behavior-change fix: purely a test
file reorganization.

## Tasks

1. Read [wasm-runtime.test.ts][test-file] in full and group its
   `test(...)` cases into `describe(...)` blocks by the
   `MdsmithRuntime` method or concern they exercise — e.g.
   `describe("check", ...)`, `describe("fix", ...)`,
   `describe("capabilities", ...)`, `describe("rename", ...)`,
   `describe("move", ...)`, `describe("engine cache lifecycle",
   ...)` — following the grouping style already used in
   `actions.test.ts` or `wiring.test.ts`.
2. Keep every `beforeAll`/`afterAll` hook and helper function at
   the scope it needs (top-level vs. inside a `describe` block)
   so setup/teardown still runs correctly.
3. Run the test file locally: `bun test
   editors/obsidian/src/wasm-runtime.test.ts`.
4. Confirm no test case moved, was renamed in a way that loses
   its meaning, or was skipped.

## Acceptance Criteria

- [ ] Every `test(...)` case in `wasm-runtime.test.ts` lives
      inside a `describe(...)` block.
- [ ] `bun test editors/obsidian/src/wasm-runtime.test.ts`
      passes with the same 12 cases (by count and name) as
      before.
- [ ] No behavior change to `wasm-runtime.ts` itself.

[audit-log]: ../docs/development/architecture-audit.md
[audit]: ../docs/development/architecture/audit-checklist.md
[test-file]: ../editors/obsidian/src/wasm-runtime.test.ts
