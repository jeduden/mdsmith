---
date: "2026-08-21"
scope: "diff since the last reviewed HEAD (2ab4b29) to b706d76 — the MDS072 external-link-check SSRF remediation, the in-tree runewidth vendoring, the tinygo CI bump, rule-config memoization, bounded workspace readers, the APM kind pack, and a broad high-performance-go.md perf sweep across the goldmark parser, lint, lsp, discovery, config, and rules packages"
method: "audit"
title: "mdsmith post-audit diff review — 2026-08-21"
summary: "Diff review of the 250 files merged since the last reviewed HEAD (2ab4b29). The headline is a net security improvement: MDS072 external-link-check gained a default-on SSRF guard (a net.Dialer.Control hook on the resolved IP that defeats DNS rebinding, a redirect guard, and a per-run probe ceiling), closing the prior review's S001/S002/S003 (tracking plans now done). One new Low finding: that guard does not decode the NAT64 well-known prefix 64:ff9b::/96, so on a DNS64/NAT64 host a NAT64-mapped internal IPv4 slips past isRestrictedIP. Everything else is bounds-preserving perf/refactor work, path containment and the symlink default-deny are intact, and no new exec/spawn/network sink appears anywhere in the diff. Section 0 baseline reconfirmed."
---
# mdsmith Security Review

- **Target:** jeduden/mdsmith @ `b706d76`
- **Mode:** audit
- **Scope:** Diff since the last reviewed HEAD (2ab4b29) to HEAD b706d76 — 250 files / 211 commits. Headline: the MDS072 external-link-check SSRF remediation (guarded client + redirect guard + per-run probe ceiling). Also the in-tree go-runewidth vendoring (eager LUT removed), the tinygo CI bump, rule-config memoization, bounded workspace readers, the APM kind pack, and a broad high-performance-go.md perf sweep across the goldmark parser, lint, lsp, discovery, config, and rules.
- **Date:** 2026-08-21

## Summary

Critical: 0 | High: 0 | Medium: 0 | Low: 1 | Info: 0

| ID   | Sev | Conf   | Title                                                                                                                             | Surface | Location                                         |
| ---- | --- | ------ | --------------------------------------------------------------------------------------------------------------------------------- | ------- | ------------------------------------------------ |
| S001 | low | likely | MDS072 SSRF guard does not decode the NAT64 well-known prefix (64:ff9b::/96), so a NAT64-mapped internal IPv4 bypasses the filter | cli     | `internal/rules/externallink/probe_net.go:41-68` |

## Findings

### S001 · MDS072 SSRF guard does not decode the NAT64 well-known prefix (64:ff9b::/96), so a NAT64-mapped internal IPv4 bypasses the filter

**Severity:** low · **Confidence:** likely · **Surface:** cli · **CWE-918**

**Location:** `internal/rules/externallink/probe_net.go:41-68`

- related: `internal/rules/externallink/probe_net.go:21`
- related: `internal/rules/externallink/probe_net.go:74`

**What.** The SSRF guard added this window to close the prior S001/S002 findings blocks restricted
destinations by checking the resolved remote IP in isRestrictedIP. It carefully decodes IPv4-mapped
(::ffff:x.x.x.x, via Unmap), IPv4-compatible (::x.x.x.x, via the Is6() high-96-bits-zero branch),
and 6to4 (the whole 2002::/16 prefix) forms so that an embedded loopback/metadata/RFC1918 IPv4 is
caught. It does NOT handle the NAT64 well-known prefix 64:ff9b::/96 (RFC 6052), which embeds an IPv4
in its low 32 bits. 64:ff9b:: has non-zero high bytes (b[1]=0x64, b[2]=0xff, b[3]=0x9b), so the
IPv4-compatible branch (which requires the first 12 bytes to be zero) does not fire, and
64:ff9b::/96 is absent from restrictedPrefixes. A throwaway test confirms isRestrictedIP returns
false for 64:ff9b::a9fe:a9fe (NAT64-mapped 169.254.169.254 cloud metadata), 64:ff9b::7f00:1
(127.0.0.1), and 64:ff9b::c0a8:1 (192.168.0.1). On a host on an IPv6-only network with a DNS64/NAT64
gateway, a document URL whose hostname the attacker resolves to a 64:ff9b:: address therefore
reaches the embedded internal IPv4 despite the guard.

**Impact.** On a DNS64/NAT64 host (increasingly common in IPv6-only cloud and mobile networks) with MDS072
explicitly enabled, a hostile Markdown file can steer an outbound probe to loopback, an RFC1918
host, or the cloud metadata endpoint (169.254.169.254 / fd00:ec2::254-class targets reachable via
NAT64) even though the default SSRF guard is on. Impact is bounded: MDS072 is opt-in (disabled by
default), the probe is a HEAD/GET whose body is discarded (blind SSRF — status/reachability oracle,
not response exfiltration), and reachability requires the victim's network to route 64:ff9b::/96.
This is a defense-in-depth gap in a now-guarded path, not a wide-open SSRF.

**Repro (sketch).** Add to a package test: `netip.MustParseAddr("64:ff9b::a9fe:a9fe")` passed to isRestrictedIP returns
false (guard misses it), whereas the equivalent bare IPv4 169.254.169.254 and the 6to4 form
2002:a9fe:a9fe:: both return true. End-to-end exploitation additionally requires a DNS64/NAT64
environment that routes 64:ff9b::/96 to the embedded IPv4, which is why confidence is `likely`
rather than `confirmed`.

**Fix.** In isRestrictedIP, decode the NAT64 well-known prefix the same way 6to4 and IPv4-compatible
addresses are already handled: when the address is in 64:ff9b::/96 (and, defensively, in any
locally-configured NAT64 prefix), extract the embedded IPv4 from the low 32 bits and re-run the
restricted-range check against it. Add 64:ff9b::/96 test cases (metadata, loopback, RFC1918
embeddings) to TestIsRestrictedIP_Blocked alongside the existing 6to4/IPv4-compatible cases.
Optionally document the residual Dialer.Control limitations (an env HTTP proxy relocates containment
to the proxy; enabling external-allow-internal disables the guard by config) next to the guard so
operators of NAT64/proxied networks understand the boundary.

## Coverage

The 250 files changed between the last reviewed HEAD (2ab4b29) and HEAD (b706d76) were mapped to the
threat-model surfaces by changed file; each security-relevant surface was read against
references/threat-model.md and traced in source. Parser-resilience/runewidth, filesystem
path/traversal/write-safety, and LSP/git/supply-chain were each traced by a dedicated pass in
addition to the directive/recipe and network review.

§0 baseline reconfirmed. No new exec/spawn/network sink appears in non-test code anywhere in the
diff: the only exec.Command/os-exec/sh matches are the prose of prior docs/security/ reports.
Recipes remain non-executed by the tool; recipesafety (MDS040) stays advisory-only (its change is a
cosmetic map[string]bool->map[string]struct{} refactor); the merge-driver and pre-merge-commit
source did not change this window; recipe commands still come from config, not documents; build
output-path validation is untouched.

MDS072 SSRF remediation (headline of the window, net improvement).
internal/rules/externallink/probe_net.go now routes probes through a guardedClient by default
(external-allow-internal is false by default and is re-reset to false at the top of ApplySettings,
so a partial links: override cannot silently disable it). The guard is a net.Dialer.Control hook
(ssrfControl) that runs on the RESOLVED remote IP of every TCP dial — initial hop and every redirect
hop — which correctly defeats DNS rebinding, plus a CheckRedirect (ssrfCheckRedirect) that caps the
chain at 10 and blocks redirects to restricted IP literals. isRestrictedIP blocks
loopback/unspecified/multicast, RFC1918, link-local (incl. 169.254.169.254 metadata), IPv6
link-local, ULA, CGN (Alibaba 100.100.100.200), 6to4 (2002::/16), and IPv4-mapped/IPv4-compatible
embeddings; it fails closed on an invalid Addr. A per-run ceiling (external-max-probes, default
1000) is enforced in checkURL and also defaults-reset in ApplySettings. This closes the prior
review's S001 (Medium blind-SSRF) and S002 (Low unbounded egress); the tracking plans 2607242010 and
2607242011 are now status ✅, and S003 (telemetry doc gap) is closed by telemetry.md carving out the
opt-in MDS072 exception. The one residual gap in this new control — the NAT64 well-known prefix
64:ff9b::/96 is not decoded, so a NAT64-mapped internal IPv4 slips past isRestrictedIP on a
DNS64/NAT64 host — is filed below as S001 (Low). Two known and accepted limitations of any
Dialer.Control IP guard are noted, not filed: an HTTP proxy set via the environment (Proxy:
http.ProxyFromEnvironment) moves containment to the proxy, and a hostile .mdsmith.yml that both
enables the opt-in rule and sets external-allow-internal: true disables the guard by configuration.

Parser resilience / DoS (dedicated pass). Every goldmark change in the window is a bounds-preserving
perf refactor or struct-field reorder: code_span.go/util.go byte-needle scans keep indices <
len(line) and strictly advance (no wedge); text/reader.go and segment.go bulk-append within the
pre-existing s.Stop/len(source) invariants; parser.go ids.Generate reuses one buffer with an
immutable string(buf) map key; extension/table.go adds a len(cols)==0 nil sentinel (hardening). The
vendored pkg/runewidth fork keeps the r<0 || r>0x10FFFF guard in RuneWidth and repoints to bounded
eastAsianWidth0[r] (r<0x300) / non-empty-table lookups after the eager LUT removal; the only
attacker-reachable call site (tablefmt.go StringWidth over cell text) stays bounded, and
CreateLUT/combinedLut is dead code in mdsmith. MDS060 occurrence compiles its pattern with stdlib
regexp (RE2, linear-time, no ReDoS) from config, not document content; MDS073 slidev editDistance
keeps its la/lb>64 guard fully protecting the [65]int stack buffers. The engine-level per-file
recover() still contains any single-file panic.

Filesystem path / traversal / write-safety (dedicated pass). No containment, symlink, or
write-target check regressed. The new bounded workspace readers (OSWorkspace/OverlayWorkspace
readFileLimited) mirror the existing ReadFile dispatch and still read rooted paths through
bytelimit.ReadFSFileLimited over the os.OpenRoot-derived FS, preserving RESOLVE_BENEATH containment
and symlink-escape refusal. discovery.go's filepath.Walk->WalkDir port preserves the symlink
default-deny (ModeSymlink gate + non-descent). rename.go, include_extract.go, fix.go, builddiag.go,
and the config merge refactors introduce no new attacker-reachable filepath.Join/Open/Write; the APM
pack (internal/pack/apm.go, user-invoked mdsmith init --apm) writes only hardcoded relative paths.

LSP / git / supply chain (dedicated pass). internal/lsp/server_diagnostics.go only batches
log/notification emission — nothing new runs on open/save, no file write, no exec, no network.
.github/workflows/ci.yml's only change is the checksum-verified tinygo bump (no new permissions
scope, no pull_request_target, no unpinned third-party action). go.mod/go.sum REMOVE
mattn/go-runewidth in favor of the in-tree fork and promote an already-transitive uax29 dep to
direct — no new external module, no replace directive. No postinstall/preinstall added to any
wrapper.

Not re-reviewed this window (unchanged source in the diff): the VS Code extension (editors/vscode),
the Obsidian plugin (editors/obsidian), the npm/PyPI/Homebrew/Flatpak distribution wrappers, and the
release publishing workflow — last covered by the 2026-06-19 and 2026-07-03 reviews.
