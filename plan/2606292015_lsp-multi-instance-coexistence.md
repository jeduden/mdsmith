---
id: 2606292015
title: Scope the LSP workspace singleton per client so instances coexist
status: "🔳"
model: opus
summary: >-
  Key the newest-wins LSP workspace singleton on a
  client-supplied scope token, not the workspace root
  alone, and make the claim opt-in. VS Code sends a
  token stable across an extension-host reload
  (`vscode.env.sessionId`); other clients send none and
  run without the singleton. That lets the VS Code
  server, a Claude Code plugin server, and a second
  Claude in another terminal all run on one workspace at
  once, while the VS Code extension-upgrade hand-off
  (old server stops, new starts) keeps working.
---
# Scope the LSP workspace singleton per client so instances coexist

## Goal

Run one `mdsmith lsp` per client on the same workspace, all at
once. One behavior must survive the change. A VS Code extension
upgrade or reload still stops the old server and starts the new
one.

## Background

`mdsmith lsp` is a stdio server; each client spawns and owns its
own process. That part already supports many instances. The
[VS Code extension](../editors/vscode/src/wiring.ts), the
[Claude Code plugin](../editors/claude-code/.claude-plugin/plugin.json)
(`npx -y -p @mdsmith/cli mdsmith lsp`), and a second Claude in
another terminal each launch their own server.

One mechanism breaks that: the newest-wins singleton in
[singleton.go](../internal/lsp/singleton.go). It is turned on in
production by [`cmd/mdsmith/lsp.go`](../cmd/mdsmith/lsp.go). It
records one owner per workspace. The key is the root path alone:

```go
func workspaceKey(root string) string {
    sum := sha256.Sum256([]byte(filepath.Clean(root)))
    return hex.EncodeToString(sum[:])
}
```

Every server on one workspace contends for that single owner
record. The newest claim wins; older servers poll, see a
different owner, send `mdsmith/superseded`, and exit. The
extension's [`decideClose`](../editors/vscode/src/wiring.ts)
suppresses restart on that signal. So when a Claude plugin server
(or a second Claude terminal) initializes on the same repo, the
VS Code server steps aside and does not come back, and the editor
loses diagnostics.

The singleton exists for one real case, documented in the
[VS Code guide](../docs/guides/editors/vscode.md): a VS Code
extension update or reload can leave a leaked extension host alive
next to the new one. The orphaned host holds the old server's
stdin open, so no EOF arrives, and it stays alive by PID, so the
[`processId` watchdog](../internal/lsp/parentwatch.go) can't reap
it. The newest-wins claim is what stops that orphan from racing
the freshly spawned server. That hand-off must keep working.

No other client has this failure mode. A Claude instance that
dies closes its child server's stdin pipe, so EOF arrives and the
server exits normally. The singleton is, in practice, a
VS-Code-only safeguard that currently reaches across to every
client on the workspace.

## Non-Goals

- Removing the singleton or the `processId` watchdog. Both stay.
- Sharing parse or cross-file caches between processes. Each
  `mdsmith lsp` keeps its own in-process
  [`Session`](../pkg/mdsmith) caches; no shared cache is added.
- Coordinating fix-on-save writes across processes. Each editor
  writes its own buffer; concurrent on-disk writes are out of
  scope.
- Changing the LSP wire surface beyond reading
  `initializationOptions` on `initialize`.

## Design

### Opt-in scope token

Add a client opt-in. The client may send a `singletonScope`
string in its `initializationOptions`. When it does, the owner
key hashes the root *and* that token:

```text
key = sha256(filepath.Clean(root) + "\x00" + scope)
```

When the token is empty or absent, the server does not claim the
registry and does not start the watcher. The singleton is then a
no-op for that client. So a client opts in by sending a token and
opts out by sending nothing.

### What each client sends

| Client                      | `singletonScope` value | Effect                                         |
| --------------------------- | ---------------------- | ---------------------------------------------- |
| VS Code extension           | `vscode.env.sessionId` | Orphan and respawn share one slot; newest wins |
| Claude Code plugin          | none                   | No claim; never supersedes or is superseded    |
| Second Claude in a terminal | none                   | No claim; coexists with the first              |
| Neovim / Helix / JetBrains  | none                   | No claim; coexists                             |

`vscode.env.sessionId` is stable across an extension-host reload
and across an extension update within one VS Code application
session, and it is unique per application session. On a full app
upgrade VS Code restarts, the old process exits, its pipes close,
and there is no orphan to reap. So the token reaps exactly the
orphan the singleton targets.

### Why a client token, not an inferred identity

| Identity              | Reaps the upgrade orphan?      | Lets two Claude terminals coexist? |
| --------------------- | ------------------------------ | ---------------------------------- |
| Workspace root only   | yes                            | no — they supersede each other     |
| `processId`           | no — orphan and respawn differ | yes                                |
| `clientInfo.name`     | yes                            | no — both report `claude-code`     |
| Client-supplied token | yes                            | yes                                |

Only a client-supplied, reload-stable token reaps the orphan and
lets independent clients coexist. The mechanism is generic: any
future client with the leaked-host problem opts in with its own
stable token, with no name-specific branch in the server.

### Rollout

The VS Code extension bundles its own `mdsmith` binary. So the
server change and the extension change ship together for the
common path. Until the extension sends the token, its singleton
stays off. That only disables orphan reaping, a rare edge case.
EOF and the `processId` watchdog still handle normal exits. A
user who points `mdsmith.path` at a newer external binary gets
the same interim behavior until they update the extension.

### Documentation

Two docs change. [`docs/reference/cli/lsp.md`](../docs/reference/cli/lsp.md)
gains a "Multiple instances" section. It states that many servers
per workspace are supported, and that the singleton is opt-in via
`singletonScope`. The [VS Code guide](../docs/guides/editors/vscode.md)
note "Two mdsmith servers running" is updated. It says the scope
is per VS Code session. So a Claude plugin or another editor on
the same workspace is unaffected.

## Tasks

1. [ ] Capture `initializationOptions` in `initializeParams`
   ([protocol.go](../internal/lsp/protocol.go)); add a typed
   `singletonScope` string field. Unit-test the unmarshal.
2. [ ] Add `singletonKey(root, scope)` and route the claim and
   watcher through it. Make
   [`startSingletonWatch`](../internal/lsp/singleton.go) a no-op
   on an empty scope. Red/green: same `(root, scope)` supersedes;
   different scopes coexist; empty scope never claims.
3. [ ] Thread the scope from `handleInitialize`
   ([server_lifecycle.go](../internal/lsp/server_lifecycle.go))
   into the claim.
4. [ ] VS Code: send
   `initializationOptions: { singletonScope: env.sessionId }`
   ([extension.ts](../editors/vscode/src/extension.ts) /
   [wiring.ts](../editors/vscode/src/wiring.ts)). Test that the
   built client options carry it.
5. [ ] Add a test that the upgrade hand-off still works: two
   processes with the same scope, newest wins, older emits
   `mdsmith/superseded`. Leave `decideClose` unchanged.
6. [ ] Update [lsp.md](../docs/reference/cli/lsp.md) and
   [vscode.md](../docs/guides/editors/vscode.md).
7. [ ] Run `mdsmith fix PLAN.md` after the front-matter status
   flips to done.

## Acceptance Criteria

- [ ] Two `mdsmith lsp` servers on one workspace with different
      `singletonScope` tokens both stay alive; neither is
      superseded.
- [ ] Two servers with the same `singletonScope`: the newest
      wins, the older sends `mdsmith/superseded` and exits.
- [ ] A server that receives no `singletonScope` never writes the
      owner registry and is never superseded.
- [ ] The VS Code extension sends
      `initializationOptions.singletonScope = vscode.env.sessionId`;
      an extension test asserts it.
- [ ] The VS Code upgrade hand-off still works — old server stops,
      new server starts — verified by a test using one scope
      across two processes.
- [ ] [`docs/reference/cli/lsp.md`](../docs/reference/cli/lsp.md)
      documents multi-instance coexistence, and the
      [VS Code guide](../docs/guides/editors/vscode.md) note
      reflects the per-session scope.
- [ ] All tests pass: `go test ./...`.
- [ ] `go tool -modfile=tools/go.mod golangci-lint run` reports no
      issues.
- [ ] `mdsmith check .` passes.

## ...

<?allow-empty-section?>
