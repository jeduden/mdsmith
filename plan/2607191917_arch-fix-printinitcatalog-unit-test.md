---
id: 2607191917
title: >-
  Add dedicated unit tests for printInitCatalog and
  setInitUsage
status: "✅"
model: haiku
summary: >-
  printInitCatalog and setInitUsage (cmd/mdsmith/init.go)
  have no dedicated unit test; only e2e subprocess tests
  exercise them. Flagged by the 2026-07-19 audit and its
  own 3x code-review pass as inverted-pyramid tax findings.
---
# Add dedicated unit tests for printInitCatalog and setInitUsage

## Goal

Give `printInitCatalog` and `setInitUsage` their own unit
tests. Lock `mdsmith init --list` and `--help` behavior at
the unit layer, not only through e2e subprocess tests.

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

A follow-up 3x code-review pass on the PR that filed this
plan found the identical shape one function over:

- `setInitUsage` in [init.go](../cmd/mdsmith/init.go) has
  no `TestSetInitUsage` anywhere in
  [init_unit_test.go](../cmd/mdsmith/init_unit_test.go).
- Its only exercise is the e2e subprocess assertion for
  `mdsmith init --help` in
  [e2e_coverage_test.go](../cmd/mdsmith/e2e_coverage_test.go).
- Same tests.md citation applies: `setInitUsage` installs a
  closure on a `*pflag.FlagSet` and needs no subprocess to
  exercise directly.

Both are `tax`, not `blocker`. Neither is itself a CLI
subcommand entry point, an LSP handler, or a `rule.Rule`
method — both are helpers `runInit` wires up.

## Tasks

1. Add `TestPrintInitCatalog` in
   [init_unit_test.go](../cmd/mdsmith/init_unit_test.go),
   writing to a `bytes.Buffer` and asserting both sections
   ("Starters (mdsmith init --starter <name>):" and "Packs
   (mdsmith init --add <name>):") plus at least one known
   starter name and one known pack name appear in the output.
2. Add `TestSetInitUsage` in
   [init_unit_test.go](../cmd/mdsmith/init_unit_test.go),
   building a `*pflag.FlagSet`, calling `setInitUsage`, then
   invoking `fs.Usage()` inside `captureStderr` (the closure
   writes to the hardcoded `os.Stderr`, so this is the only
   viable capture point — see the other `runInit` tests in
   this file for the pattern) and asserting the printed text
   names `--starter`, `--from-markdownlint`, `--add`,
   `--force`, and `--list`.
3. Leave the existing e2e `--list` and `--help` tests in
   place — they still cover the full CLI dispatch path — but
   do not duplicate the content assertions there beyond a
   smoke check.
4. `go build ./...` passes.
5. `go test ./cmd/mdsmith/...` passes.

## Acceptance Criteria

- [x] `TestPrintInitCatalog` exists in
      [init_unit_test.go](../cmd/mdsmith/init_unit_test.go) and
      exercises `printInitCatalog` directly (no subprocess).
- [x] `TestSetInitUsage` exists in
      [init_unit_test.go](../cmd/mdsmith/init_unit_test.go) and
      exercises `setInitUsage` directly (no subprocess).
- [x] `go test ./...` is green.
- [x] `mdsmith check .` is green.
