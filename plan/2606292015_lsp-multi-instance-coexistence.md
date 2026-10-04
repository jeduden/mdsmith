---
id: 2606292015
title: Scope the LSP workspace singleton per client so instances coexist
status: "✅"
model: opus
summary: >-
  Make the newest-wins LSP workspace singleton opt-in.
  Key its owner record on the workspace root plus a
  client-supplied scope token, not the root alone. The
  VS Code extension sends its per-workspace
  `storageUri`, which VS Code derives from the
  workspace identity, so it is stable across an
  extension reload or update. Other clients send no token and run without
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
records one owner per workspace, keyed on
`sha256(filepath.Clean(root))` alone.

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

No other client has this failure mode: a dying Claude closes
its server's stdin, so EOF arrives and the server exits.

## Non-Goals

- Removing the singleton or the `processId` watchdog. Both stay.
- Sharing parse or cross-file caches between processes. Each
  `mdsmith lsp` keeps its own in-process
  [`Session`](../pkg/mdsmith) caches; no shared cache is added.
- Coordinating fix-on-save writes across processes. Each editor
  writes its own buffer; concurrent on-disk writes are out of
  scope.
- Re-keying mid-session. The root is read once at `initialize`
  (from `workspaceFolders[0]`), as today.
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
legacy root-only key, so the one key function keeps the old
format (see Backward compatibility).

### What each client sends

| Client                      | `singletonScope` value | Effect                                         |
| --------------------------- | ---------------------- | ---------------------------------------------- |
| VS Code extension           | workspace `storageUri` | Orphan and respawn share one slot; newest wins |
| Claude Code plugin          | none                   | No claim; never supersedes or is superseded    |
| Second Claude in a terminal | none                   | No claim; coexists with the first              |
| Neovim / Helix / JetBrains  | none                   | No claim; coexists                             |

### The VS Code token must be stable per workspace

The orphan and its respawn are two different extension-host
processes. The token must be identical for both, or the new
server never reaps the orphan. So the token must survive an
extension-host restart.

The extension sends `context.storageUri` as a string. VS Code
derives that URI from the workspace identity, so both hosts get
the same value after any reload or update, with nothing written
to disk. It is per workspace on that machine, which is the grain
the key needs. An empty window has no `storageUri` and sends no
scope. An earlier draft stored a random UUID in
`workspaceState`; that needed a fire-and-forget write and lost
reaping whenever the write failed.

`vscode.env.sessionId` is the obvious shortcut, but it is not
safe here. The API documents it as changing "each time the editor
is started," and it is injected per extension-host process. So a
leaked host and a fresh host may hold different session ids. That
would silently break reaping — the exact case the singleton
exists for. A workspace-derived id removes that doubt.

One known limit follows from the per-workspace grain. Two VS Code
windows on the *same* folder get the same storage URI, so the
newest still wins between them. VS Code already focuses an open
folder instead of opening a duplicate window, and today's
root-only key behaves the same way, so this is not a regression.

The converse limit: adding a second folder to a one-folder
window, or Save Workspace As, changes the storage URI while the
first folder stays. A host leaked across that change keeps the
old scope and is not reaped; the root-only key reaped it. The
leaked server cannot tell this from a second, legitimate window
on a workspace with the same first folder. See Follow-ups.

### Why a client token, not an inferred identity

| Identity               | Reaps the upgrade orphan?           | Two Claude terminals coexist?  |
| ---------------------- | ----------------------------------- | ------------------------------ |
| Workspace root only    | yes                                 | no — they supersede each other |
| `processId`            | no — orphan and respawn differ      | yes                            |
| `clientInfo.name`      | yes                                 | no — both report `claude-code` |
| `vscode.env.sessionId` | unclear — may change on host reload | yes (VS Code only)             |
| Workspace `storageUri` | yes — derived from the workspace    | yes                            |

Only a stable, client-supplied token reaps the orphan and
lets independent clients coexist. The mechanism is generic: any
future client with the leaked-host problem opts in with its own
stable token, with no name-specific branch in the server.

### One key function, one gate

`workspaceKey` takes the scope; no sibling function is added.
An empty scope hashes the root alone, the legacy key.

`EnableWorkspaceSingleton` stays the process capability: it
wires the registry seams and keeps unit tests hermetic. The
scope is only the key input and the claim gate, which fires
only for a non-empty scope.

### Backward compatibility

The key format changes from `sha256(root)` to
`sha256(root + "\x00" + scope)`. An empty scope reproduces the
old key byte for byte. A no-token server never claims, so it
never reads or writes that record. A scoped server writes its id
to the legacy record once at claim and never watches it. An
older root-only binary polling that record then sees a new owner
and steps aside, exactly as before scopes. Only the VS Code (now
scope-keyed) path watches a new key.

Keys are now per root and scope, and a deleted or moved
workspace leaves its record behind. So each scoped
start prunes, once after both claims, `.owner` records (and
leftover claim temp and quarantine files) older than 30 days.
The scan runs on the watcher goroutine, so the initialize
response never waits on it. A stale record is first renamed
to a quarantine path and its age checked again. A claim that
landed meanwhile is linked back, never over a newer one. A
pruned record of a live server reads as "no owner", which the
watcher treats as "still ours".

### Rollout

The extension bundles its own binary, so server and extension
ship together. Skew arises only when `mdsmith.path` points an
older extension at a newer binary. No token is sent, so the
orphan is not reaped until the extension updates.

The first update from a pre-scope build is covered. The leaked
host still runs the old binary. That binary watches the old
key. The new server writes that key once, so the old one exits.

### Documentation

[`lsp.md`](../docs/reference/cli/lsp.md) gains a short
"Multiple instances" section; room comes from re-wrapping its
prose to 72 columns. The "Two mdsmith servers running" note in
the [extension reference](../docs/reference/vscode-extension.md)
and the [VS Code guide](../docs/guides/editors/vscode.md) say
the scope is per VS Code workspace.

## Tasks

1. [x] Capture `initializationOptions` in `initializeParams`
   ([protocol.go](../internal/lsp/protocol.go)); read
   `mdsmith.singletonScope` as an optional string. Unit-test the
   unmarshal, including the absent / `null` / non-object case,
   which must decode to an empty scope (the opt-out path).
2. [x] Extend `workspaceKey` to take the scope and fold the
   empty-scope case in (empty scope hashes the root alone).
   Add `TestWorkspaceKey…` cases: same root + different scope →
   different key; same root + same scope → same key; empty scope
   → the legacy key. Do not add a second key function.
3. [x] Gate the claim and watcher on a non-empty scope in
   [`startSingletonWatch`](../internal/lsp/singleton.go), and
   thread the scope from `handleInitialize`
   ([server_lifecycle.go](../internal/lsp/server_lifecycle.go)).
   Drive the new empty-scope no-op red/green, distinct from the
   existing empty-root / empty-instanceID guard.
4. [x] VS Code: send `context.storageUri` as
   `initializationOptions.mdsmith.singletonScope`
   ([extension.ts](../editors/vscode/src/extension.ts) /
   [wiring.ts](../editors/vscode/src/wiring.ts)). Widen the
   injected context type to expose `storageUri`. A `bun:test`
   asserts the built client options carry the token, that two
   activations over equal storage URIs send one scope, and that a
   different workspace sends another.
5. [x] Unit-test the supersede logic on the existing seams
   (`watchSingleton` / the key): same scope → newest wins, older
   emits `mdsmith/superseded`; different scopes → both stay; no
   scope → never claims. Leave `decideClose` unchanged.
6. [x] Update [lsp.md](../docs/reference/cli/lsp.md),
   [vscode.md](../docs/guides/editors/vscode.md), and the
   troubleshooting note in
   [vscode-extension.md](../docs/reference/vscode-extension.md).
7. [x] On completion, flip the front-matter status and run
   `mdsmith fix PLAN.md`.

## Acceptance Criteria

- [x] Two `mdsmith lsp` servers on one workspace with different
      `singletonScope` tokens both stay alive; neither is
      superseded.
- [x] Two servers with the same `singletonScope`: the newest
      wins, the older sends `mdsmith/superseded` and exits.
- [x] A server that receives an empty or absent `singletonScope`
      never writes the owner registry and is never superseded;
      the absent / `null` `initializationOptions` decode is
      covered by a test.
- [x] The empty-scope no-op is driven red/green and is distinct
      from the pre-existing empty-root guard.
- [x] There is exactly one key function; an empty scope yields
      the legacy root-only key (a unit test pins this).
- [x] A scoped claim writes the legacy root-only record once
      and never watches it, so a pre-scope orphan exits on the
      first upgrade.
- [x] A scoped start prunes registry records older than 30 days,
      once, without deleting a record claimed mid-prune.
- [x] The VS Code extension sends
      `initializationOptions.mdsmith.singletonScope` = its
      workspace `storageUri`; a `bun:test` asserts the
      token is sent and depends only on the storage URI's
      value. VS Code keeps that value the same across hosts.
- [x] The VS Code upgrade hand-off still works — old server
      stops, new server starts — verified by a unit test that
      feeds one scope to two server instances.
- [x] [`docs/reference/cli/lsp.md`](../docs/reference/cli/lsp.md)
      documents multi-instance coexistence, and the
      [VS Code guide](../docs/guides/editors/vscode.md) and the
      [extension reference](../docs/reference/vscode-extension.md)
      note reflect the per-workspace scope.
- [x] All tests pass: `go test ./...` and the extension
      `bun:test` suite.
- [x] `go tool -modfile=tools/go.mod golangci-lint run` reports no
      issues.
- [x] `mdsmith check .` passes.

## Follow-ups

`PLAN.md` sits at its 300-line MDS022 cap, so, as plan 2610030438 did, these
are recorded here. File each as its own opus plan once `PLAN.md` has room.

1. **Workspace identity change.** Reap a server leaked across the storage URI
   change above (a restart-stable per-window id, or a one-shot claim of the old
   scope) while windows sharing a first folder coexist; drop the limit note.
2. **Fix-path prototype race.** `go test -race ./internal/lsp/` fails on
   origin/main (afd30920a): `Session.Fix` runs `toc.(*Rule).Check` on the
   registered prototype (`checker.ConfigureEnabledRules` returns it unchanged),
   whose lazy `engineOnce.Do` writes it while `engine.cloneRules` copies it.
   Catalog, build, conciseness-scoring and external-link share the pattern. Add
   a failing `-race` test of concurrent `Session.Fix` and `Session.Check`,
   private rule instances per config signature, and CI `go test -race` for
   `internal/{lsp,engine,fix}` and `pkg/mdsmith`.
3. **Shared temp-dir prune.** If `os.UserCacheDir` fails, the registry falls
   back to a shared `/tmp/mdsmith`, where another user can plant
   `lsp-singleton` as a symlink so prune removes old `*.tmp`, `*.owner` and
   `*.prune` files in its target. Skip prune on that fallback, or refuse a
   registry dir that is a symlink or owned by another user.
4. **Retire the legacy claim.** Each scoped start writes the root-only record,
   superseding a pre-scope client on that root; set a removal release.
5. **NUL root and seams.** Reject `%00` in `uriToPath`/`pickRoot` for every
   caller and drop the singleton-only guard. Replace `pruneStale`'s positional
   `hooks ...func(string)` with named seams, and read `initializationOptions`
   from the raw params map instead of the custom `UnmarshalJSON`, which made
   the other initialize fields case-sensitive.

## ...

<?allow-empty-section?>
