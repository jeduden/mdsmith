---
id: 2608282011
title: "Security hardening batch — 2026-08-28"
status: "✅"
summary: >-
  Close the single informational finding from the 2026-08-28 post-audit
  diff review: the MDS072 guarded HTTP client sets Proxy:
  http.ProxyFromEnvironment, so a configured forward proxy can reach a
  destination the connect-time ssrfControl guard never inspects (S001).
model: sonnet
---
# Security hardening batch — 2026-08-28

## Goal

Close informational finding S001 from the [2026-08-28 post-audit diff
review](../docs/security/2026-08-28-post-audit-diff-review/report.md).

The MDS072 external-link-check SSRF guard lives in
[`ssrfControl`](../internal/rules/externallink/probe_net.go). It is a
`net.Dialer.Control` hook. It checks the resolved remote IP just before
connect. That is the right, DNS-rebinding-safe place.

But the guarded transport also sets `Proxy: http.ProxyFromEnvironment`.
When `HTTP_PROXY` or `HTTPS_PROXY` is set, Go dials the proxy. So the
guard checks the proxy IP, not the target. The proxy then connects to a
hostile document URL on the client's behalf. That URL may be an internal
host or `169.254.169.254`. The guard never sees it.

The preconditions are narrow. MDS072 must be enabled. A proxy must be
set. That proxy must be willing to forward internally.

MDS072 is also off by default. So this is defense-in-depth, not an
exposed default. The fix removes the proxy blind spot on the guarded
path and documents the caveat.

## ...

<?allow-empty-section?>

## Tasks

1. Add a failing test in
   [`probe_net_test.go`](../internal/rules/externallink/probe_net_test.go)
   that asserts the guarded client performs no proxied dial to a
   restricted destination: set a proxy on the guarded transport (or
   construct it via the same builder) pointing at a stub, and assert the
   request to a loopback/metadata URL is refused rather than forwarded.
   Confirm it is red against the current `buildGuardedClient`.
2. Make it green: on the guarded (`external-allow-internal: false`) path,
   remove the proxy blind spot. Preferred approach — set `Proxy: nil` on
   `guardedClient`'s transport so every dial is a direct connection the
   `Control` hook vets. If proxying must be retained for internal-corp
   probing, instead resolve the destination host up front and run
   `isRestrictedIP` over every resolved address before issuing the
   request, rejecting the URL when any resolves into a restricted range.
   Leave `permissiveClient` (opt-in `external-allow-internal: true`)
   unchanged.
3. Add a doc comment next to `buildGuardedClient` recording why the
   guarded path does not honor the environment proxy (or how the
   pre-resolution check compensates), so the property is not silently
   reintroduced.
4. Document the proxy caveat next to `external-allow-internal` in the
   MDS072 user docs and, if the guarded path keeps `Proxy: nil`, note
   that internal-corp URLs behind a required proxy are not probed.
5. Run `go test ./internal/rules/externallink/...`, then `mdsmith fix`
   on the edited docs and `mdsmith check .`.

## Acceptance Criteria

- [x] A red-then-green test proves the guarded client no longer forwards
      a restricted-destination request through an environment proxy.
- [x] `buildGuardedClient` no longer exposes the proxy blind spot on the
      default (`external-allow-internal: false`) path, with a comment
      recording the invariant.
- [x] `permissiveClient` behavior (opt-in `external-allow-internal:
      true`) is unchanged.
- [x] The MDS072 user docs state the proxy caveat.
- [x] `go test ./...` and `mdsmith check .` pass.
