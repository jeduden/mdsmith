---
id: 2607170527
title: External link checking on WASM hosts (MDS072)
status: "🔳"
summary: >-
  Replace MDS072's fake-healthy WASM probe with honest behavior: never
  report a broken URL as OK, and make the rule functional on hosts that
  can do network I/O (Obsidian `requestUrl`, VS Code Node) through a
  host-provided async probe bridge.
model: opus
depends-on: [2606280208]
---
# External link checking on WASM hosts (MDS072)

## Goal

Make [MDS072 `external-link-check`][mds072] correct under WebAssembly.
The rule must never report a broken URL as healthy. Where the host can
reach the network, the rule must actually flag broken URLs. Where it
cannot, the rule must stay inert and say so.

[mds072]: ../internal/rules/MDS072-external-link-check/README.md

## Background

The WASM probe today returns HTTP `200` for every URL. See
[`probe_wasm.go`](../internal/rules/externallink/probe_wasm.go). So the
Obsidian plugin and every other WASM host silently pass every external
link. A broken URL is reported as fine. That is a false negative. It is
worse than the rule being absent, because it grants false confidence.

A browser sandbox cannot probe arbitrary URLs on its own. Cross-origin
`fetch` is blocked by CORS. There are no raw sockets. Go's `net/http` on
`js/wasm` runs on `fetch`, so a naive port would fail every cross-origin
request.

Those failures look identical to real transport errors. So a naive WASM
probe flips the failure mode: every URL becomes a false positive instead
of a false negative. Both are wrong.

Two facts make a real fix possible:

- The host often *can* reach the network. Obsidian's `requestUrl` API
  makes real HTTP requests that bypass CORS, on desktop and on mobile.
  The VS Code extension runs in Node with full network access.
- The engine already has a host seam. `pkg/mdsmith.Session` mirrors into
  JavaScript one-to-one, with a `capabilities()` feature-detect list.
  See [the engine API page](../docs/background/concepts/engine-api.md).

There is also a precedent for a sandbox-incapable rule.
[MDS040 recipe safety](../internal/rules/MDS040-recipe-safety/README.md)
needs shell access. It registers behind a `//go:build !wasm` tag. So it
is simply absent under WASM rather than faked.

## Design

### Stop faking a healthy result

The probe outcome becomes tri-state: probed-ok, probed-failed, and
not-probed. Only probed-failed emits a diagnostic. Not-probed emits
nothing. A URL the engine could not reach is never treated as healthy.

On a WASM build with no host bridge, every URL is not-probed. So the
rule emits nothing. That matches today's visible output. But it no
longer encodes a false `200`, and it composes with the bridge below.

### Host-provided async probe bridge

Network probing is asynchronous. `Session.Check` is synchronous. So the
host does the probing, and the engine reads the results. The flow has
three steps:

1. The engine exposes a file's external URLs and the probe config that
   applies to them (`external-skip`, `external-timeout`,
   `external-rate-limit`). A new mirrored method returns them.
2. The host probes each URL. Obsidian uses `requestUrl`. VS Code uses
   Node `fetch`. The host honors the skip list, the timeout, and the
   concurrency cap the engine supplied.
3. The host feeds each result back into the session's URL cache through
   a new method. A later `Check` reads the cache. A known-failed URL
   becomes a diagnostic. An OK or not-yet-probed URL does not.

This keeps the network I/O in the host, where it is allowed, and keeps
the rule logic in the shared engine. The host re-runs `Check` once the
probes resolve, so the diagnostics arrive asynchronously.

### One prober seam for every target

The probe becomes a pluggable seam:

- Native CLI keeps its in-process HTTP prober
  ([`probe_net.go`](../internal/rules/externallink/probe_net.go)).
- WASM and editor hosts use the bridge above.
- A host with no bridge leaves the rule inert.

The same cache-populate seam lets a long-lived native `mdsmith lsp`
session re-probe and evict. So it also closes the process-lifetime
staleness limitation noted in
[the MDS072 plan](2606280208_external-link-check.md).

### Advertise availability

The host and the user must be able to tell whether probing is live. The
session advertises external-link probing through `capabilities()` (or a
dedicated signal). When the rule is enabled in config but no bridge is
present, the host surfaces one informational note rather than silently
doing nothing.

## Tasks

1. [x] Make the probe outcome tri-state in the shared rule. Only a
   probed failure emits a diagnostic; a not-probed URL emits nothing.
   Add unit tests for all three states.
2. [x] Replace the fake-`200` WASM probe with a not-probed result, so
   the WASM build never reports a URL as healthy. Test that the WASM
   path emits no diagnostics for a broken URL.
3. [ ] Extract the prober into a seam (interface or func var) with three
   implementations: native HTTP, host bridge, and inert.
4. [ ] Add the engine method that returns a file's external URLs plus
   the applicable probe config, mirrored into JS.
5. [ ] Add the engine method that populates the URL-result cache from
   host-supplied outcomes, mirrored into JS. Cover it in the WASM smoke
   test.
6. [ ] Wire the Obsidian plugin prober to `requestUrl` and re-run
   `check` when results resolve. Add plugin tests.
7. [ ] Wire the VS Code extension prober to a Node HTTP client. Add
   extension tests.
8. [ ] Advertise external-link probing in `capabilities()` and surface a
   one-time "unavailable" note when the rule is enabled without a bridge.
9. [ ] Use the same populate/evict seam to re-probe in a long-lived
   native LSP session, closing the staleness follow-up from plan
   2606280208.
10. [ ] Update the rule README, the engine API page, and this plan to
    describe the host-bridge model. Confirm the WASM size budgets still
    hold.

## Acceptance Criteria

- [ ] The WASM build never reports a broken external URL as healthy.
- [ ] With a host bridge, a broken URL in a WASM host (Obsidian) is
  flagged with its HTTP status or transport error.
- [ ] Without a bridge, the rule emits no diagnostics and advertises
  itself as unavailable rather than passing silently.
- [ ] The host prober honors `external-skip`, `external-timeout`, and
  `external-rate-limit`.
- [ ] The native CLI behavior is unchanged.
- [ ] A long-lived native LSP session re-probes rather than serving a
  frozen result for the process lifetime.
- [ ] Both WASM size budgets still hold, and CI stays green.
