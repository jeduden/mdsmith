---
id: 2608212013
title: "Security hardening batch — 2026-08-21"
status: "🔲"
summary: >-
  Close S001 (Low) from the 2026-08-21 post-audit diff review: the
  MDS072 SSRF guard added to close the prior review's blind-SSRF
  finding decodes IPv4-mapped, IPv4-compatible, and 6to4 IPv6 forms
  but not the NAT64 well-known prefix 64:ff9b::/96, so a NAT64-mapped
  internal IPv4 (loopback, RFC1918, or 169.254.169.254 metadata)
  slips past isRestrictedIP on a DNS64/NAT64 host.
model: sonnet
depends-on: []
---
# Security hardening batch — 2026-08-21

## Goal

Close finding S001 (Low) from the [2026-08-21 post-audit diff
review](../docs/security/2026-08-21-post-audit-diff-review/report.md).
The MDS072 SSRF guard in
[`probe_net.go`](../internal/rules/externallink/probe_net.go) decodes a
restricted IPv4 hidden inside several IPv6 forms, but misses one. It
does not decode the NAT64 well-known prefix `64:ff9b::/96`. On an
IPv6-only host with a NAT64 gateway, that lets a hostname reach an
internal address the guard should block. The impact is low: MDS072 is
opt-in and the fix mirrors the existing 6to4 handling. The details and
the fix live in the tasks below.

## ...

<?allow-empty-section?>

## Tasks

1. Write a failing unit test in
   [`probe_net_test.go`](../internal/rules/externallink/probe_net_test.go):
   extend `TestIsRestrictedIP_Blocked` with the NAT64 cases
   `64:ff9b::a9fe:a9fe` (metadata `169.254.169.254`), `64:ff9b::7f00:1`
   (loopback `127.0.0.1`), and `64:ff9b::c0a8:1` (RFC1918
   `192.168.0.1`). Confirm it fails (they are currently allowed).
2. In `isRestrictedIP`
   ([`probe_net.go`](../internal/rules/externallink/probe_net.go)), when
   the unmapped address is in `64:ff9b::/96`, extract the embedded IPv4
   from the low 32 bits and re-run the restricted-range check against
   it — the same shape as the existing IPv4-compatible branch. Add a
   `64:ff9b::/96` entry near the `restrictedPrefixes` block or a
   dedicated well-known-prefix constant so the intent is documented.
3. Make the test pass. Keep the change allocation-free on the hot path
   (this runs per resolved dial, not per document).
4. Add a short comment next to the guard noting the two accepted
   `net.Dialer.Control` limitations the review recorded but did not
   file: an env-configured HTTP proxy (`Proxy:
   http.ProxyFromEnvironment`) moves containment to the proxy, and
   setting `external-allow-internal: true` disables the guard by
   configuration.
5. Run `go test ./internal/rules/externallink/...`, then
   `mdsmith check .`.

## ...

<?allow-empty-section?>

## Acceptance Criteria

- [ ] `isRestrictedIP` blocks a restricted IPv4 embedded in a
  `64:ff9b::/96` NAT64 address (metadata, loopback, RFC1918).
- [ ] `TestIsRestrictedIP_Blocked` covers the three NAT64 cases and
  passes.
- [ ] The guard comment documents the proxy and
  `external-allow-internal` limitations.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool -modfile=tools/go.mod golangci-lint run` reports no
  issues.

## ...

<?allow-empty-section?>
