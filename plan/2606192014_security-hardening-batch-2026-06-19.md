---
id: 2606192014
title: "Security hardening batch — 2026-06-19 LSP/VS Code audit"
status: "🔳"
summary: >-
  Low-severity Workspace Trust gaps from the 2026-06-19 audit: gate the
  mdsmith.kinds.resolve/why palette commands and the mdsmith-rule:
  content provider on isWorkspaceTrusted, matching the pattern already
  used by the three mutating commands.
model: sonnet
---
# Security hardening batch — 2026-06-19 LSP/VS Code audit

## Goal

Add Workspace Trust gates to two read-only VS Code paths. Closes S006
and S007 from the [2026-06-19 LSP/VS Code audit
report](../docs/security/2026-06-19-lsp-vscode-audit/report.md).

**S006 (low).** `mdsmith.kinds.resolve` and `mdsmith.kinds.why` lack
`isWorkspaceTrusted` in their `when` conditions (package.json:125-130).
Their handlers also lack an `isTrusted()` guard. A hostile
`.mdsmith.yml` can inject text into the virtual document pane.

**S007 (low).** The `mdsmith-rule:` TextDocumentContentProvider
(wiring.ts:906-910) spawns `mdsmith help rule <id>` without a trust
gate. Risk is very low. The pattern is inconsistent with the trust model.

## Tasks

### S006 — kinds palette trust gate

- [x] Add `&& isWorkspaceTrusted` to the `when` expression for
  `mdsmith.kinds.resolve` and `mdsmith.kinds.why` in
  [editors/vscode/package.json](../editors/vscode/package.json)
  lines 125-130.
- [x] Add an `isTrusted()` early-return guard to `runKindsResolve`
  and `runKindsWhy` in [editors/vscode/src/commands/kinds.ts](
  ../editors/vscode/src/commands/kinds.ts) — matching the pattern in
  fix-workspace.ts:62 and init.ts:23.
- [x] Pass `isTrusted` through the wiring.ts call sites at lines 829
  and 838 in
  [editors/vscode/src/wiring.ts](../editors/vscode/src/wiring.ts).

### S007 — rule-doc content provider trust gate

- [x] Add a trust check in `fetchRuleDocContent` in
  [editors/vscode/src/commands/rule-doc.ts](
  ../editors/vscode/src/commands/rule-doc.ts).
  Return empty content when the workspace is untrusted. Trust gate
  added as optional `isTrusted?: () => boolean` 5th parameter on both
  `fetchRuleDocContent` and `provideRuleDocContent`; wiring.ts passes
  `isTrusted` via the 5th slot (`undefined` for spawn uses the
  default).

## Acceptance Criteria

- [x] `mdsmith.kinds.resolve` and `mdsmith.kinds.why` are hidden in the
  palette in an untrusted workspace.
- [x] `runKindsResolve` and `runKindsWhy` return early when
  `isTrusted()` is false.
- [x] The `mdsmith-rule:` provider does not spawn in untrusted mode.
- [x] All existing VS Code extension tests pass.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
