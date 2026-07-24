---
date: "2026-07-24"
scope: "diff since the 2026-07-17 post-audit diff review — the new MDS072 external-link-check network-probing rule, the MDS073 slidev slide-structure rule, foreign managed-region protection for fix, the merge-driver -merge scoping fix, and the init.go split"
method: "audit"
title: "mdsmith post-audit diff review — 2026-07-24"
summary: "Diff review of the 110 files merged since the 2026-07-17 review. The window's whole security story is MDS072 external-link-check — mdsmith's first rule that makes outbound HTTP at lint time. It is opt-in and off by default, but when enabled in .mdsmith.yml it probes every http/https URL taken from document content with no private/loopback/link-local/metadata filtering and follows redirects with the default client (S001, Medium: blind SSRF/internal-service oracle reachable from check, fix, and the LSP diagnostics-on-open path; the response body is discarded, so no direct exfil). The probe count per run is bounded only by concurrency, not in total, making a hostile document an egress/DoS amplifier (S002, Low). The telemetry doc still asserts zero runtime egress and omits this opt-in exception (S003, Info). The MDS073 slidev rule and the foreign managed-region fix logic add no path/exec/network sink; per-file panic recovery contains any parser panic. §0 baseline holds: no new exec/spawn sink, recipes still not executed, no editor/CI/wrapper source changed."
---
# mdsmith Security Review

- **Target:** jeduden/mdsmith @ `d4af5d5`
- **Mode:** audit
- **Scope:** Diff review of everything merged since the 2026-07-17 post-audit diff review (base d496596) — the new MDS072 external-link-check network-probing rule, the MDS073 slidev slide-structure rule, foreign managed-region protection for fix, the merge-driver -merge scoping fix, the init.go split, and CI pinned-baseline bump
- **Date:** 2026-07-24

## Summary

Critical: 0 | High: 0 | Medium: 1 | Low: 1 | Info: 1

| ID   | Sev    | Conf      | Title                                                                                                                                                                 | Surface | Location                                      |
| ---- | ------ | --------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------- | --------------------------------------------- |
| S001 | medium | confirmed | MDS072 external-link-check probes document-supplied URLs with no private/loopback/link-local/metadata filtering (SSRF), and follows redirects with the default client | cli     | `internal/rules/externallink/rule.go:192-200` |
| S002 | low    | confirmed | MDS072 caps probe concurrency but not the total number of outbound requests per run, so a hostile document is an egress/DoS amplifier                                 | cli     | `internal/rules/externallink/rule.go:300-334` |
| S003 | info   | confirmed | Telemetry / network-policy docs still assert zero runtime egress and do not disclose the opt-in MDS072 network access                                                 | cli     | `docs/reference/telemetry.md:10-26`           |

## Findings

### S001 · MDS072 external-link-check probes document-supplied URLs with no private/loopback/link-local/metadata filtering (SSRF), and follows redirects with the default client

**Severity:** medium · **Confidence:** confirmed · **Surface:** cli · **CWE-918**

**Location:** `internal/rules/externallink/rule.go:192-200`

- related: `internal/rules/externallink/rule.go:135-168`
- related: `internal/rules/externallink/rule.go:307-319`
- related: `internal/rules/externallink/probe_net.go:18-57`

**What.** When the opt-in external-link-check rule (MDS072) is enabled in .mdsmith.yml, Check walks the document AST
and extracts every http/https destination from links, autolinks, and images (externalURL, rule.go:174),
then issues a live HEAD (GET on 405) request to each one (checkURL -> probe -> http.Client.Do,
probe_net.go:41).

The only gate on the target is isExternalHTTP (rule.go:194), which checks solely that the scheme is
http/https and Host is non-empty. There is no allowlist and no rejection of RFC1918 / loopback / link-local
/ ULA hosts, so a document link to `http://169.254.169.254/latest/meta-data/`, `http://127.0.0.1:6379/`,
`http://[::1]/`, or an internal hostname is probed against the host running mdsmith. The shared client is
the zero-value http.Client (probe_net.go:18) with no CheckRedirect, so it follows up to 10 redirects by
default — an external URL that a reviewer or the external-skip denylist deems safe can 30x-bounce to an
internal target, and no per-hop containment is reapplied.

The rule runs anywhere Check runs: `mdsmith check` (CI), `mdsmith fix`, and the LSP diagnostics pipeline
(server_diagnostics.go schedules a lint on didOpen/didSave and the session runs rule.All()). The native
`mdsmith lsp` used by Neovim has no Workspace-Trust equivalent, so opening a hostile repo whose .mdsmith.yml
enables MDS072 fires the probes on file open. Both the enabling config and the malicious URLs are
attacker-controlled in the hostile-repo threat model, so 'requires non-default config' is weak mitigation —
the attacker ships the config.

**Impact.** A hostile repository (or a fork PR that edits .mdsmith.yml plus a Markdown file) turns the victim's mdsmith
run into an SSRF request generator against the victim's network position. On a CI runner or a developer's
LSP session this reaches internal-only services and cloud metadata endpoints (169.254.169.254).

Because the diagnostic reports the HTTP status code and the transport error, the attacker learns whether an
internal host/port exists and responds — internal service and port discovery from the victim's vantage. The
response body is drained to io.Discard and is never surfaced, so this is a blind SSRF probe
(existence/timing/status oracle), not a body-exfil primitive; that is the deciding factor keeping this
Medium rather than High. It would rise to High if the diagnostic ever echoed response bodies, or if MDS072
were enabled by a default convention (making it zero-config) instead of opt-in.

**Repro (sketch).** In a scratch repo, write .mdsmith.yml enabling the rule (rules: external-link-check: true) and a README.md
containing a link to `http://169.254.169.254/latest/meta-data/` and one to `http://127.0.0.1:6379/`. Run
`mdsmith check .` (or open the repo in Neovim with mdsmith lsp). Observe that mdsmith issues outbound
requests to the link-local and loopback targets and that the resulting diagnostic distinguishes 'unreachable'
from an HTTP status, i.e. it reveals whether the internal endpoint answered.

**Fix.** Before probing, resolve the target host and refuse any address in a private/loopback/link-local/ULA/
unspecified range and the cloud-metadata IPs. Guard both the initial connection and every redirect hop via
http.Client.CheckRedirect plus a net.Dialer.Control hook that re-validates the resolved IP, closing the
DNS-rebinding and redirect-bounce gaps. Consider making deny-internal the default and gating any internal
probing behind an explicit, documented opt-in. In the LSP, treat MDS072 as trust-sensitive (do not run it
against an untrusted workspace without an explicit trust signal).

### S002 · MDS072 caps probe concurrency but not the total number of outbound requests per run, so a hostile document is an egress/DoS amplifier

**Severity:** low · **Confidence:** confirmed · **Surface:** cli · **CWE-770**

**Location:** `internal/rules/externallink/rule.go:300-334`

- related: `internal/rules/externallink/rule.go:135-168`

**What.** The rate-limit semaphore (external-rate-limit, default 10) bounds only how many probes are in flight at once
(acquire/release, rule.go:323). Nothing bounds the total number of distinct URLs probed in a run. A single
document (or a workspace of documents) can contain an unbounded number of unique http/https URLs; the
per-URL cache collapses duplicates but not distinct hosts. With the default 5s per-request timeout and a slow
or non-responding attacker-chosen server, N unique URLs take roughly N/rate-limit * timeout seconds, all as
outbound requests the victim's host is made to send.

**Impact.** When MDS072 is enabled, a hostile file with tens of thousands of unique URLs makes mdsmith emit tens of
thousands of outbound requests and can stall a check/fix run or an LSP diagnostics pass for a long time (each
miss blocks up to the timeout behind the concurrency cap). Combined with S001 this makes the victim's CI a
low-rate request cannon against a third party or an internal network. Impact is bounded by the opt-in config
and the concurrency cap (so it is slow rather than a resource spike), hence Low.

**Repro (sketch).** With external-link-check enabled, generate a Markdown file with 20000 distinct `http://` links pointing at a
slow/blackholed server, then run `mdsmith check` on it and observe the run issue one request per unique URL
with no aggregate ceiling.

**Fix.** Add a per-run (and/or per-file) ceiling on the number of URLs actually probed, reporting the remaining links
as unchecked once the cap is hit (mirroring how other size caps degrade). Pair with a shorter default timeout
or a global deadline for the probing phase.

## Hardening / Informational

### S003 · Telemetry / network-policy docs still assert zero runtime egress and do not disclose the opt-in MDS072 network access

**Severity:** info · **Confidence:** confirmed · **Surface:** cli · **CWE-1059**

**Location:** `docs/reference/telemetry.md:10-26`

**What.** telemetry.md states 'The CLI and the LSP server make zero outbound network calls during normal operation'
and 'A locked-down or air-gapped CI runner can run mdsmith check . with no outbound access and the run
completes normally.' Since MDS072 external-link-check landed, that is no longer unconditionally true:
enabling the rule makes check/fix and the LSP issue outbound HTTP. The claim is defensible for the default
configuration (the rule is off by default), but the page never carves out the exception, and the rule's own
documentation does not warn that it performs network access against document-controlled URLs (the SSRF
surface of S001).

**Impact.** An operator relying on the telemetry page to certify mdsmith as network-silent (e.g. for an air-gapped or
egress-restricted environment) could enable external-link-check without realizing it introduces outbound
requests to attacker-influenced URLs. Documentation-accuracy / security-transparency gap; no direct exploit.

**Repro (sketch).** Read docs/reference/telemetry.md against the behavior of internal/rules/externallink: the doc's 'zero
outbound network calls' claim omits the one rule that makes them.

**Fix.** Add an explicit carve-out to telemetry.md naming MDS072 external-link-check as the single opt-in rule that
performs runtime network access, and add an SSRF/internal-target warning to the rule's user documentation
once the S001 filtering is in place.

## Coverage

The 110 files changed between the 2026-07-17 review base (d496596) and HEAD (d4af5d5) were mapped to the
threat-model surfaces by changed file, then each surface read against references/threat-model.md.

Headline surface — NEW runtime network egress (internal/rules/externallink/, MDS072 external-link-check).
This is the first mdsmith rule that makes outbound HTTP requests at lint time. It is opt-in
(EnabledByDefault=false; not enabled by any built-in convention — confirmed against internal/convention and
internal/rulelayer). When enabled via .mdsmith.yml it issues a HEAD/GET probe to every http/https URL found
in document content (inline links, autolinks, images) and reports transport errors and 4xx/5xx. The full
path document -> externalURL -> checkURL -> probe -> http.Client.Do(rawURL) was traced. Findings: S001 (SSRF:
no private/loopback/link-local/metadata target validation, and the default client follows redirects so
containment cannot be reasserted per hop), S002 (concurrency is bounded but the total probe count per run is
not), and S003 (the telemetry/network-policy docs still assert zero runtime egress and never mention this
opt-in exception).

MDS073 slidev slide-structure rule (internal/rules/slidevstructure/): a pure AST/line parser, no
package-level regexp, no exec. parseSlides indexes lines[0] without an explicit len>0 guard, but the engine
wraps every rule's Check in a per-file recover() (internal/engine/runner.go lintFile -> PanicDiagnostic), so
any panic is contained to a single-file diagnostic rather than crashing the run or the LSP — the panic
containment baseline is reconfirmed.

Foreign managed-region protection for fix (internal/foreignregion/, internal/fix/): marker-pair parsing over
document content; no path resolution, exec, or network sink introduced. The merge-driver -merge scoping fix
(PR #752) and the init.go split (PR #751) are refactors with no new exec/path/network sink. The one CI change
(.github/actions/setup-mdsmith-pinned-version/action.yml) is a version+SHA256 bump of the pinned baseline
binary; the download stays HTTPS + SHA256-verified before PATH exposure.

§0 baseline reconfirmed by exclusion: no recipe/exec sink was added anywhere in the diff (no new
exec.Command/child_process/spawn); recipes remain non-executed by the tool. The VS Code extension, Obsidian
plugin, npm/PyPI wrappers, and merge-driver/pre-merge-commit source did not change this window and were not
re-reviewed (last covered by the 2026-06-19 and 2026-07-03 reviews). The single new runtime-egress rule is
the whole security story of this window.
