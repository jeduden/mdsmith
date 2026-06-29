---
id: 2606292015
title: Scope the LSP workspace singleton per client so instances coexist
status: "🔲"
model: opus
summary: >-
  Make the newest-wins LSP workspace singleton opt-in.
  Key its owner record on the workspace root plus a
  client-supplied scope token, not the root alone. The
  VS Code extension sends a per-workspace UUID it
  persists in `workspaceState`; the id is read from
  disk, so it is stable across an extension reload or
  update. Other clients send no token and run without
  the singleton. That lets the VS Code server, a Claude
  Code plugin server, and a second Claude in another
  terminal all run on one workspace at once, while the
  VS Code upgrade hand-off (old server stops, new one
  starts) keeps working.
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
- Re-keying mid-session. The root is read once at `initialize`
  (from `workspaceFolders[0]`), exactly as today. A multi-root
  folder add/remove does not re-claim. This matches current
  behavior and is unchanged here.
- Changing the LSP wire surface beyond reading
  `initializationOptions` on `initialize`.

## Design

### Opt-in scope token

Add a client opt-in. The client may send a `singletonScope`
string under `initializationOptions.mdsmith`. The namespace keeps
the key off the generic top level and leaves room to add sibling
fields later. When the scope is non-empty, the owner key hashes
the root *and* that token:

```text
key = sha256(filepath.Clean(root) + "\x00" + scope)
```

When the scope is empty or absent, the server does not claim the
registry and does not start the watcher. The singleton is then a
no-op for that client. So a client opts in by sending a scope and
opts out by sending nothing. An empty scope also hashes to the
legacy root-only key, so a no-token client and an old root-only
binary still agree on the same key (see Backward compatibility).

### What each client sends

| Client                      | `singletonScope` value       | Effect                                         |
| --------------------------- | ---------------------------- | ---------------------------------------------- |
| VS Code extension           | persisted per-workspace UUID | Orphan and respawn share one slot; newest wins |
| Claude Code plugin          | none                         | No claim; never supersedes or is superseded    |
| Second Claude in a terminal | none                         | No claim; coexists with the first              |
| Neovim / Helix / JetBrains  | none                         | No claim; coexists                             |

### The VS Code token must be disk-backed

The orphan and its respawn are two different extension-host
processes. The token must be identical for both, or the new
server never reaps the orphan. So the token must survive an
extension-host restart.

The extension generates a UUID once with `crypto.randomUUID()`
and stores it in `context.workspaceState`. That store is written
to disk and read back unchanged after any reload or update. So
both hosts read the same id. The id is per workspace on that
machine, which is the grain the key needs.

`vscode.env.sessionId` is the obvious shortcut, but it is not
safe here. The API documents it as changing "each time the editor
is started," and it is injected per extension-host process. So a
leaked host and a fresh host may hold different session ids. That
would silently break reaping — the exact case the singleton
exists for. A disk-backed id removes that doubt.

One known limit follows from the per-workspace grain. Two VS Code
windows on the *same* folder read the same stored id, so the
newest still wins between them. VS Code already focuses an open
folder instead of opening a duplicate window, and today's
root-only key behaves the same way, so this is not a regression.

### Why a client token, not an inferred identity

| Identity                     | Reaps the upgrade orphan?           | Two Claude terminals coexist?  |
| ---------------------------- | ----------------------------------- | ------------------------------ |
| Workspace root only          | yes                                 | no — they supersede each other |
| `processId`                  | no — orphan and respawn differ      | yes                            |
| `clientInfo.name`            | yes                                 | no — both report `claude-code` |
| `vscode.env.sessionId`       | unclear — may change on host reload | yes (VS Code only)             |
| Persisted per-workspace UUID | yes — read from disk                | yes                            |

Only a disk-backed, client-supplied token reaps the orphan and
lets independent clients coexist. The mechanism is generic: any
future client with the leaked-host problem opts in with its own
stable token, with no name-specific branch in the server.

### One key function, one gate

Keep a single key function. Extend `workspaceKey` to take the
scope rather than adding a sibling. An empty scope hashes the
root alone, so every call site agrees on one derivation and the
legacy behavior is preserved.

Keep one switch for "is the singleton active." `EnableWorkspaceSingleton`
stays the process capability: it wires the registry seams and
keeps unit tests hermetic. The scope is only the key input and
the claim gate. The claim fires only when the scope is non-empty.
The server never treats the scope as content; any client that
sends one has opted in by definition.

### Backward compatibility

The key format changes from `sha256(root)` to
`sha256(root + "\x00" + scope)`. An empty scope reproduces the
old key byte for byte. So a no-token client, and an older
root-only binary, still key the same way and still see each
other. Only the VS Code (now UUID-keyed) path moves to a new key.

Old `.owner` records written under the legacy root-only key are
not migrated. They are tiny files in the user cache dir, and
nothing reads them once VS Code keys by UUID. They are harmless;
the plan does not add a prune.

### Rollout

The VS Code extension bundles its own `mdsmith` binary. So the
server change and the extension change ship together for the
common path. There is no skew there.

Skew is possible only when a user points `mdsmith.path` at a
newer external binary while running an older extension that sends
no token. In that window the singleton is off, so the leaked-host
orphan is not reaped — and that orphan is the one case the
subsystem exists for. EOF and the `processId` watchdog still
handle normal exits. The honest cost is: for the override case
only, the "Two mdsmith servers running" note can recur until the
extension updates.

### Documentation

Two docs change. [`docs/reference/cli/lsp.md`](../docs/reference/cli/lsp.md)
gains a "Multiple instances" section. It states that many servers
per workspace are supported, and that the singleton is opt-in via
`singletonScope`. The [VS Code guide](../docs/guides/editors/vscode.md)
note "Two mdsmith servers running" is updated. It says the scope
is per VS Code workspace. So a Claude plugin or another editor on
the same workspace is unaffected.

## Tasks

1. [ ] Capture `initializationOptions` in `initializeParams`
   ([protocol.go](../internal/lsp/protocol.go)); read
   `mdsmith.singletonScope` as an optional string. Unit-test the
   unmarshal, including the absent / `null` / non-object case,
   which must decode to an empty scope (the opt-out path).
2. [ ] Extend `workspaceKey` to take the scope and fold the
   empty-scope case in (empty scope hashes the root alone).
   Add `TestWorkspaceKey…` cases: same root + different scope →
   different key; same root + same scope → same key; empty scope
   → the legacy key. Do not add a second key function.
3. [ ] Gate the claim and watcher on a non-empty scope in
   [`startSingletonWatch`](../internal/lsp/singleton.go), and
   thread the scope from `handleInitialize`
   ([server_lifecycle.go](../internal/lsp/server_lifecycle.go)).
   Drive the new empty-scope no-op red/green, distinct from the
   existing empty-root / empty-instanceID guard.
4. [ ] VS Code: generate a UUID once and persist it in
   `context.workspaceState`; send it as
   `initializationOptions.mdsmith.singletonScope`
   ([extension.ts](../editors/vscode/src/extension.ts) /
   [wiring.ts](../editors/vscode/src/wiring.ts)). Widen the
   injected context type to expose `workspaceState`. A `bun:test`
   asserts the built client options carry the token and that the
   id is stable across two activations.
5. [ ] Unit-test the supersede logic on the existing seams
   (`watchSingleton` / the key): same scope → newest wins, older
   emits `mdsmith/superseded`; different scopes → both stay; no
   scope → never claims. Leave `decideClose` unchanged.
6. [ ] Update [lsp.md](../docs/reference/cli/lsp.md) and
   [vscode.md](../docs/guides/editors/vscode.md).
7. [ ] On completion, flip the front-matter status and run
   `mdsmith fix PLAN.md`.

## Acceptance Criteria

- [ ] Two `mdsmith lsp` servers on one workspace with different
      `singletonScope` tokens both stay alive; neither is
      superseded.
- [ ] Two servers with the same `singletonScope`: the newest
      wins, the older sends `mdsmith/superseded` and exits.
- [ ] A server that receives an empty or absent `singletonScope`
      never writes the owner registry and is never superseded;
      the absent / `null` `initializationOptions` decode is
      covered by a test.
- [ ] The empty-scope no-op is driven red/green and is distinct
      from the pre-existing empty-root guard.
- [ ] There is exactly one key function; an empty scope yields
      the legacy root-only key (a unit test pins this).
- [ ] The VS Code extension sends
      `initializationOptions.mdsmith.singletonScope` = a UUID it
      persists in `workspaceState`; a `bun:test` asserts the
      token is sent and is stable across activations.
- [ ] The VS Code upgrade hand-off still works — old server
      stops, new server starts — verified by a unit test that
      feeds one scope to two server instances.
- [ ] [`docs/reference/cli/lsp.md`](../docs/reference/cli/lsp.md)
      documents multi-instance coexistence, and the
      [VS Code guide](../docs/guides/editors/vscode.md) note
      reflects the per-workspace scope.
- [ ] All tests pass: `go test ./...` and the extension
      `bun:test` suite.
- [ ] `go tool -modfile=tools/go.mod golangci-lint run` reports no
      issues.
- [ ] `mdsmith check .` passes.

## ...

<?allow-empty-section?>
