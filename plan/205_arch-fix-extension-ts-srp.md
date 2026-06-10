---
id: 205
title: Move extension.ts concerns to wiring.ts
status: "✅"
summary: >-
  extension.ts is 509 lines and owns the LSP client
  lifecycle, config-file watcher, error handler, and
  command registrations. Move those concerns to
  wiring.ts per the TypeScript architecture doc.
model: ""
depends-on: []
---
# Move extension.ts concerns to wiring.ts

## Goal

[editors/vscode/src/extension.ts](../editors/vscode/src/extension.ts) is
509 lines and violates SRP. It runs the
LSP client, watches `.mdsmith.yml`,
handles errors, and registers commands.
The TypeScript architecture doc says
[extension.ts](../editors/vscode/src/extension.ts) should activate, construct
the wiring object, and hand control to
[wiring.ts](../editors/vscode/src/wiring.ts). Everything else moves.

## Tasks

1. ~~Move `LanguageClient` lifecycle into
   `wiring.ts`.~~ Done — the `Wiring` class owns the client lifecycle.
2. ~~Move the `WorkspaceFileSystemWatcher`
   for `.mdsmith.yml` into `wiring.ts`.~~ Done — `Wiring` owns the
   watcher slot.
3. ~~Move the `ErrorHandler` class into
   `wiring.ts` or `commands/error-handler.ts`.~~ Done — moved to
   `commands/error-handler.ts`.
4. ~~Move `registerPaletteCommands` and all
   `registerCommand` calls into `wiring.ts`.~~ Done — `Wiring`
   registers every command and both virtual-doc schemes.
5. ~~Reduce `extension.ts` to: import,
   construct wiring, hand over.~~ Done — `extension.ts` is 37 lines.
6. ~~Extend the existing `wiring.test.ts`
   with coverage for the moved concerns:
   LSP client lifecycle, config watcher,
   and command registration.~~ Done.
7. ~~Run `bun test` and the extension-host
   e2e suite.~~ Done — `bun test` 155 pass; e2e suite 2 passing.

## Acceptance Criteria

- [x] `extension.ts` is under 60 lines.
- [x] No `registerCommand` calls remain
  in `extension.ts`.
- [x] `wiring.test.ts` covers the moved
  LSP lifecycle, watcher, and command
  registration. All tests pass.
- [x] The extension-host e2e suite passes.
- [x] `bun run lint` reports no issues.
