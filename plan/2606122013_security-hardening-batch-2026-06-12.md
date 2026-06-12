---
id: 2606122013
title: "Security hardening batch — 2026-06-12 git/LSP audit"
status: "🔲"
summary: >-
  Low/informational hardening from the 2026-06-12 audit: remove the
  GOPATH binary fallback in resolveInstalledBinary (S003), and add a
  unit test for shellQuote (S002).
model: sonnet
---
# Security hardening batch — 2026-06-12 git/LSP audit

## Goal

Close findings S002 and S003 from the [2026-06-12 git/LSP
audit report](../docs/security/2026-06-12-git-lsp-audit/report.md).

**S003 (low, tentative).** `resolveInstalledBinary` falls back to
`$GOPATH/bin/mdsmith` after `os.Executable()` and `exec.LookPath`
both fail. A poisoned `$GOPATH` in a hostile CI environment can steer
this to an attacker binary. That binary is then written into git config
and invoked on every subsequent `git merge`.

**S002 (info).** `shellQuote` is the load-bearing control preventing
shell injection when the driver path enters git config. No unit test
covers paths with single quotes, spaces, or dollar signs.

## Tasks

### S003 — GOPATH fallback

- [ ] **Red**: add a test that sets `GOPATH` to a directory with a fake
  `bin/mdsmith`, makes `os.Executable()` and `LookPath` fail, calls
  `resolveInstalledBinary`, and asserts the fake path is not returned.
- [ ] **Green**: remove the `goEnvPath` / `GOPATH` fallback. If it is
  kept for development, gate it: verify the resolved path is in the
  same directory as `os.Executable()` before accepting it.
- [ ] Run `go test ./cmd/mdsmith/...`; all pass.

### S002 — shellQuote unit test

- [ ] **Red/Green**: add a table-driven test in
  `cmd/mdsmith/mergedriver_test.go` covering a path with spaces, a
  path with single quotes, a path with `$VAR` and backticks, and the
  empty string. Each case: pass the `shellQuote` result to
  `/bin/sh -c 'echo <result>'` and confirm output matches the input.

## Acceptance Criteria

- `resolveInstalledBinary` no longer uses `go env GOPATH` as a search
  path (or the fallback is gated by a same-directory check).
- A unit test asserts `shellQuote` escapes single quotes, spaces, and
  shell metacharacters correctly.
- All existing tests pass.
