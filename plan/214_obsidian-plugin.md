---
id: 214
title: Obsidian plugin via hand-rolled LSP bridge
status: "⛔"
model: opus
summary: >-
  Superseded by plan 215 (WASM build target +
  Workspace abstraction) and plan 217 (Obsidian
  plugin shell). The pivot is WASM everywhere —
  one runtime for desktop and mobile — instead
  of a desktop-only LSP bridge plus a separate
  WASM follow-up.
depends-on: [121, 168]
---
# Obsidian plugin via hand-rolled LSP bridge

## Superseded

Replaced by two plans:

- [Plan 215](215_engine-api-wasm.md) — the
  mdsmith public engine API
  (`pkg/mdsmith.Session`) and its WASM
  bindings.
- [Plan 217](217_obsidian-plugin.md) — the
  Obsidian plugin shell that consumes the
  WASM runtime.

The pivot. LSP and a hand-rolled JSON-RPC
client buy nothing once the client and the
server share a JS process. A desktop-only
scope leaves mobile users out. WASM unifies
both targets behind one runtime.

The branch `plan-214-obsidian-plugin` has
code that plan 217 can reuse. The list:
`diagnostics.ts`, the settings tab, the code
actions, the styles, and the build shell.
Drop `lsp-client.ts` and `binary.ts`.

Its design notes informed plan 217. The
original plan body lives in git history under
the parent of the supersede commit. Run
`git log -p -1 -- plan/214_obsidian-plugin.md`
to find that revision.

## Goal

Superseded. See
[plan 217](217_obsidian-plugin.md).

## Tasks

Superseded. See
[plan 217](217_obsidian-plugin.md).

## Acceptance Criteria

- [ ] N/A — superseded.
