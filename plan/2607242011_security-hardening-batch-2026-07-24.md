---
id: 2607242011
title: "Security hardening batch — 2026-07-24"
status: "✅"
summary: >-
  Batch the informational finding from the 2026-07-24 post-audit diff
  review: telemetry.md still asserts zero runtime network egress and
  never carves out the opt-in MDS072 external-link-check, which does
  make outbound HTTP when enabled (S003).
model: haiku
---
# Security hardening batch — 2026-07-24

## Goal

Close the informational finding S003 from the [2026-07-24 post-audit
diff review](../docs/security/2026-07-24-post-audit-diff-review/report.md).
[`docs/reference/telemetry.md`](../docs/reference/telemetry.md) says the
CLI and LSP make zero outbound network calls in normal operation. It
also says `mdsmith check .` runs with no outbound access. MDS072
external-link-check breaks that claim when enabled: it makes `check`,
`fix`, and the LSP issue outbound HTTP. The page needs a carve-out, and
the rule docs need an SSRF caveat.

## ...

<?allow-empty-section?>

## Tasks

1. Add a section to
   [`docs/reference/telemetry.md`](../docs/reference/telemetry.md). Name
   MDS072 `external-link-check` as the one opt-in rule that makes
   runtime network calls. State it is off by default. Describe what it
   sends: a HEAD/GET to each http/https URL in the document.
2. Scope the "zero outbound network calls" and air-gapped wording to the
   default configuration.
3. Add an SSRF caveat to the MDS072 user docs. Cross-link the guard from
   [plan 2607242010](2607242010_mds072-ssrf-network-hardening.md).
4. Run `mdsmith fix` on the edited docs, then `mdsmith check .`.

## ...

<?allow-empty-section?>

## Acceptance Criteria

- [x] telemetry.md names MDS072 as the one opt-in networked rule.
- [x] The zero-egress claim is scoped to the default configuration.
- [x] MDS072 docs carry an SSRF caveat.
- [x] `mdsmith check .` passes.

## ...

<?allow-empty-section?>
