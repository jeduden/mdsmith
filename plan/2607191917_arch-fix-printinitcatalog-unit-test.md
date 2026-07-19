---
id: 2607191917
title: >-
  Add a dedicated unit test for printInitCatalog
status: "🔲"
model: haiku
summary: >-
  printInitCatalog (cmd/mdsmith/init.go) has no
  TestPrintInitCatalog; only an e2e subprocess test
  exercises `mdsmith init --list`. Flagged by the
  2026-07-19 audit as an inverted-pyramid tax finding.
---
# Add a dedicated unit test for printInitCatalog

## Goal

Give `printInitCatalog` its own unit test so `mdsmith init
--list` behavior is locked at the unit layer, not only
through the e2e subprocess test.

## Background

The 2026-07-19 audit (see
[the audit log](../docs/development/architecture-audit.md))
found this gap:

- `printInitCatalog` in
  [init.go](../cmd/mdsmith/init.go) has no
  `TestPrintInitCatalog` in
  [init_unit_test.go](../cmd/mdsmith/init_unit_test.go).
- Its only exercise today is the e2e subprocess assertion
  for `mdsmith init --list` in
  [e2e_test.go](../cmd/mdsmith/e2e_test.go).
- [tests.md](../docs/development/architecture/tests.md)
  states: "A new function lands together with its
  dedicated unit test by name."
- The same doc also says an e2e test reachable through the
  integration layer should move down. `printInitCatalog`
  writes to an `io.Writer`. It needs no subprocess to
  exercise.

This is `tax`, not `blocker`. `printInitCatalog` is a
helper `runInit` calls. It is not itself a CLI subcommand
entry point, an LSP handler, or a `rule.Rule` method.

## Tasks

1. Add `TestPrintInitCatalog` in
   [init_unit_test.go](../cmd/mdsmith/init_unit_test.go),
   writing to a `bytes.Buffer` and asserting both sections
   ("Starters (mdsmith init --starter <name>):" and "Packs
   (mdsmith init --add <name>):") plus at least one known
   starter name and one known pack name appear in the output.
2. Leave the existing e2e `--list` test in place — it still
   covers the full CLI dispatch path — but do not duplicate the
   catalog-content assertions there beyond a smoke check.
3. `go build ./...` passes.
4. `go test ./cmd/mdsmith/...` passes.

## Acceptance Criteria

- [ ] `TestPrintInitCatalog` exists in
      [init_unit_test.go](../cmd/mdsmith/init_unit_test.go) and
      exercises `printInitCatalog` directly (no subprocess).
- [ ] `go test ./...` is green.
- [ ] `mdsmith check .` is green.
