---
id: 231
title: LSP newest-wins workspace singleton
status: "✅"
summary: >-
  A leaked editor extension host can outlive its
  window while staying alive and holding the LSP
  server's stdin open, so neither the stdin-EOF
  exit nor the processId watchdog fires and an
  orphaned `mdsmith lsp` keeps running beside the
  one a reload spawned. Add a newest-wins workspace
  singleton: each server records its workspace root
  in a shared registry under a per-process id; once
  a newer server claims the same root, the older one
  sends `mdsmith/superseded` and exits, and the VS
  Code extension suppresses the restart so exactly
  one server stays live per workspace.
model: ""
depends-on: []
---
# LSP newest-wins workspace singleton

## Goal

The processId watchdog
([internal/lsp/parentwatch.go](../internal/lsp/parentwatch.go))
reaps an `mdsmith lsp` server whose editor host has
*died*. It cannot reap one whose host is *alive but
orphaned* — a VS Code / code-server extension host
that survives its own window after an abrupt
reload. That host keeps the server's stdin pipe open
(so no EOF arrives) and still reports as alive (so
the watchdog stays quiet), and its language client
even respawns the server if it is killed. The result
is two servers for one workspace where the user wants
one.

Add a newest-wins workspace singleton so the older
server steps aside on its own, leaving exactly one
live server per workspace root.

## Tasks

1. Add the registry and watcher in
   [internal/lsp/singleton.go](../internal/lsp/singleton.go):
   a per-workspace owner file under the OS cache dir
   (atomic claim, plain read), a `watchSingleton`
   poll loop mirroring `watchParentProcess`, and a
   `startSingletonWatch` that claims the workspace and
   steps aside when a newer owner appears.
2. On step-aside, send the `mdsmith/superseded`
   server-to-client notification, then exit — so the
   client knows the close is intentional.
3. Gate it behind `Options.EnableWorkspaceSingleton`
   in [internal/lsp/server.go](../internal/lsp/server.go);
   call `startSingletonWatch` from `handleInitialize`;
   enable it in
   [cmd/mdsmith/lsp.go](../cmd/mdsmith/lsp.go).
4. In the VS Code extension, suppress the restart on
   `mdsmith/superseded`: a `decideClose` policy in
   [editors/vscode/src/wiring.ts](../editors/vscode/src/wiring.ts)
   and a `markSuperseded` hook on the error handler in
   [editors/vscode/src/extension.ts](../editors/vscode/src/extension.ts).
5. Document the behavior and the `mdsmith/superseded`
   contract in the troubleshooting section of
   [the VS Code guide](../docs/guides/editors/vscode.md).

## Acceptance Criteria

- [x] A second server started for the same workspace
  root makes the older one send `mdsmith/superseded`
  and exit, leaving one live server.
- [x] The feature is off by default so unit tests
  neither write to the real cache dir nor leak a
  watcher goroutine; the singleton tests drive the
  seams directly.
- [x] The VS Code error handler does not restart the
  server after `mdsmith/superseded`.
- [x] `go test ./...` and
  `go tool golangci-lint run` pass.
- [x] `go run ./cmd/mdsmith check .` passes.
