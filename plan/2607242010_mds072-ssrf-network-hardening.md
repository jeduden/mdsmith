---
id: 2607242010
title: "MDS072 external-link-check: SSRF and egress hardening"
status: "🔲"
summary: >-
  Close S001 (Medium) and S002 (Low) from the 2026-07-24 post-audit
  diff review: MDS072 probes document-supplied URLs with no
  private/loopback/link-local/metadata filtering and follows redirects
  with the default client (SSRF), and it caps probe concurrency but not
  the total request count per run. Add IP-range denial on the initial
  hop and every redirect, plus a per-run probe ceiling.
model: sonnet
---
# MDS072 external-link-check: SSRF and egress hardening

## Goal

Close findings S001 and S002 from the [2026-07-24 post-audit diff
review](../docs/security/2026-07-24-post-audit-diff-review/report.md).
MDS072 (`internal/rules/externallink/`) is the first rule that makes
outbound HTTP at lint time. When enabled it probes every http/https URL
from document content, with no defense against internal, link-local,
loopback, or metadata targets (S001, SSRF). It also follows up to 10
redirects with the zero-value `http.Client`, so containment cannot be
reasserted per hop. It caps concurrency but not the total number of
distinct URLs probed per run (S002, egress amplifier). This plan denies
internal targets by default and bounds total egress.

## ...

<?allow-empty-section?>

## Tasks

1. Add an SSRF guard in
   [`probe_net.go`](../internal/rules/externallink/probe_net.go). Build
   the shared `http.Client` with a `net.Dialer.Control` hook that
   inspects the resolved remote IP. Refuse loopback, private (RFC1918),
   link-local (`169.254.0.0/16`, IPv6 `fe80::/10`), ULA (`fc00::/7`),
   unspecified, and the cloud-metadata addresses. Write the failing
   test first: an httptest server bound to loopback must be refused.
2. Set `Client.CheckRedirect` to re-run the IP guard on every redirect
   hop. Test with a server that 302-redirects to `127.0.0.1`.
3. Make deny-internal the default. Add an opt-in
   `links.external-allow-internal` (bool, default false) for users who
   lint internal docs on purpose. Wire it through `applyLinks` and
   `ApplySettings` in
   [`rule.go`](../internal/rules/externallink/rule.go). Unit-test the
   parse and the default.
4. Add a per-run total-probe ceiling (a package-level counter beside the
   semaphore, configurable via `links.external-max-probes`). Once
   reached, further URLs return a not-probed result and emit no
   diagnostic. Surface that the cap was hit so coverage is not silently
   truncated. Failing test: N+1 distinct URLs issue at most N requests.
5. Leave the wasm prober (`probe_wasm.go`) as-is (probed=false). Add a
   guard test only if the shared config path needs one.
6. Update the MDS072 README and user docs. Describe the guard, the two
   new settings, and the SSRF caveat.

## ...

<?allow-empty-section?>

## Acceptance Criteria

- [ ] A link to a loopback, link-local, private, or metadata address is
      not probed when `external-allow-internal` is false (the default).
- [ ] The guard also fires on a redirect hop to such an address.
- [ ] `external-allow-internal: true` restores internal probing, with a
      test.
- [ ] A run over more than `external-max-probes` distinct URLs issues at
      most that many requests and reports the rest as unchecked.
- [ ] MDS072 README and docs describe the guard, settings, and caveat.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool -modfile=tools/go.mod golangci-lint run` is clean

## ...

<?allow-empty-section?>
