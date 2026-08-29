---
date: "2026-08-28"
scope: "diff since the 2026-08-14 post-audit diff review (base 2ab4b29) — 250 changed files: the newly vendored pkg/runewidth fork, the MDS072 external-link-check SSRF guard and per-run egress ceiling landing via #769, a bounded front-matter read path plus a MemFS directory-index rebuild in workspace.go, a memoized-line-index refactor in flavor/detect.go, the tinygo CI bump 0.39.0 → 0.41.1, and a broad batch of behavior-preserving high-performance-go perf refactors"
method: "audit"
title: "mdsmith post-audit diff review — 2026-08-28"
summary: "Diff review of the 250 files merged since the 2026-08-14 review. No new confirmed security defect. §0 baseline reconfirmed by exclusion: no exec/spawn/shell sink was added anywhere in the diff, so recipes remain non-executed and no zero-interaction path (LSP fix-on-save, merge driver, editor open) gained a command sink; no network sink was added outside the reviewed externallink rule. The window's substantive security-relevant changes are all clean or hardening: the new pkg/runewidth is a faithful, source-pinned vendored fork (pure Unicode-table binary search, no cgo/unsafe/exec/net, anchored linear regex over env only); workspace.go's readFileRootedLimited caps front-matter reads while preserving containment through the same os.OpenRoot handle; flavor/detect.go is a behavior-preserving refactor with a bounded binary-search line index; the tinygo bump keeps its checksum-pin intact. The MDS072 SSRF guard that landed via #769 (remediating the 2026-07-24 findings) is correctly built — enforced at net.Dialer.Control on the resolved IP (DNS-rebinding-safe), covering RFC1918/link-local/metadata/ULA/CGN and their IPv4-in-IPv6 encodings, with a redirect cap and a per-run egress ceiling, opt-in and off by default. One informational hardening item (S001) records that the guarded client's Proxy: http.ProxyFromEnvironment lets a configured forward proxy see a destination the connect-time guard cannot."
---
# mdsmith Security Review

- **Target:** jeduden/mdsmith @ `b706d76`
- **Mode:** audit
- **Scope:** Diff since the 2026-08-14 review (base 2ab4b29, HEAD b706d76), 250 files. Security-relevant: the new vendored pkg/runewidth fork; the MDS072 SSRF guard + egress ceiling landing via #769; a bounded front-matter read plus MemFS index rebuild in workspace.go; the flavor/detect.go line-index refactor; the tinygo CI bump 0.39.0 to 0.41.1; and a broad batch of behavior-preserving high-performance-go perf refactors.
- **Date:** 2026-08-28

## Summary

Critical: 0 | High: 0 | Medium: 0 | Low: 0 | Info: 1

| ID   | Sev  | Conf   | Title                                                                                        | Surface | Location                                           |
| ---- | ---- | ------ | -------------------------------------------------------------------------------------------- | ------- | -------------------------------------------------- |
| S001 | info | likely | SSRF guard cannot see the destination when a forward proxy is configured via the environment | cli     | `internal/rules/externallink/probe_net.go:118-128` |

## Hardening / Informational

### S001 · SSRF guard cannot see the destination when a forward proxy is configured via the environment

**Severity:** info · **Confidence:** likely · **Surface:** cli · **CWE-918**

**Location:** `internal/rules/externallink/probe_net.go:118-128`

- related: `internal/rules/externallink/probe_net.go:71`

**What.** The guarded HTTP client used by MDS072 (external-link-check, opt-in, off by default) enforces its SSRF containment in ssrfControl, a net.Dialer.Control hook that inspects the resolved remote IP right before connect. That is the correct, DNS-rebinding-safe place for the check — but the same transport also sets Proxy: http.ProxyFromEnvironment. When HTTP_PROXY/HTTPS_PROXY is set in the environment, Go dials the proxy's address rather than the document URL's resolved address, so ssrfControl validates the proxy IP and never sees the true destination. The forward proxy then
resolves and connects to the target host on the client's behalf, outside the guard. A hostile document URL pointing at an internal host or a cloud metadata endpoint (e.g. <http://169.254.169.254/>) would be blocked when dialed directly, but could be reached through a proxy that is itself willing to forward there. This is a known structural limitation of connect-time SSRF guards, not a coding error in isRestrictedIP (which is thorough).

**Impact.** Marginal and precondition-heavy: it requires (1) MDS072 explicitly enabled by the project, (2) a forward proxy configured in the environment where mdsmith runs, and (3) that proxy being willing to forward to the internal/metadata address in the document URL. Under those conditions the guard's block on private/link-local/metadata targets can be bypassed via the proxy, re-enabling the same limited internal-probe / metadata-reach the guard exists to prevent. No such bypass exists without a proxy.

**Repro (sketch).** Sketch (no working exploit): enable MDS072 with external-allow-internal:false, set HTTPS_PROXY to a permissive forward proxy, and lint a document containing a link to <http://169.254.169.254/latest/meta-data/.> ssrfControl sees only the proxy IP (public, allowed) and permits the dial; the proxy forwards to the metadata endpoint. With no proxy set, the same link is denied at connect.

**Fix.** Defense in depth for the guarded (external-allow-internal:false) path: either (a) disable proxy use on guardedClient (Proxy: nil) so every dial is a direct connection the Control hook can vet, accepting that internal-corp targets behind a required proxy simply won't be probed; or (b) when a proxy is in use, additionally resolve the destination hostname up front and run isRestrictedIP on every resolved address before issuing the request, rejecting the URL if any resolves into a restricted range (belt-and-suspenders against the proxy blind spot). Document the proxy caveat
next to external-allow-internal either way. Because MDS072 is opt-in and off by default, this is hardening rather than a fix for an exposed default.

## Coverage

The 250 files changed between the 2026-08-14 review base (2ab4b29) and HEAD (b706d76) were mapped to the threat-model surfaces and each in-scope surface read against references/threat-model.md.

§0 baseline reconfirmed. Grepping every added non-test line in the window for execution sinks (exec.Command/CommandContext, os/exec, sh -c, /bin/sh, syscall.Exec, .Run()) returned nothing — no new command/shell sink anywhere, so the recipes-are-never-executed property holds and no zero-interaction path (LSP fix-on-save, merge driver, editor open) gained an exec. Grepping added non-test lines for network sinks outside the reviewed externallink rule (http., net.Dial, .Get(, LookupHost) also returned nothing.

§1 directive engine: no changes to include/catalog/build path resolution or recipe handling in this window; MDS040 recipesafety unchanged (only 2 files touched, no logic change).

§2 parser resilience & workspace: pkg/runewidth is a faithful, source-pinned vendored fork of mattn/go-runewidth 0.0.27 — pure Unicode-table binary search (inTable/inWidthTable use bounded `for top >= bot` loops), no cgo, no unsafe, no exec, no network; the go:generate directive was stripped and the sole regexp (reLoc) is anchored, linear, and runs only over the locale env var (not document content). pkg/mdsmith/workspace.go adds readFileRootedLimited, which preserves ReadFile's containment by reading through the same os.OpenRoot(root) traversal-/symlink-safe handle and
merely caps the byte count via bytelimit — a DoS hardening, not a containment change. bytelimit reads max+1 and errors past max; the cap is correct. The MemFS buildDirIndex rewrite is behavior-preserving (empty-segment keys stop indexing exactly as the old per-call scan did). pkg/markdown/flavor/detect.go is a finding-construction refactor with a lazily-built, memoized lineIndex whose newlineSearch is a standard bounded binary search — no new unbounded loop, recursion, or allocation-DoS.

§3 LSP: internal/lsp/server_diagnostics.go changed for diagnostics perf only; no new write, exec, or network sink.

§6 supply chain: the tinygo bump keeps the checksum-pin pattern intact (TINYGO_SHA256 updated to the 0.41.1 .deb hash, still verified via `sha256sum -c`). Vendoring go-runewidth as in-repo source removes a module dependency in favor of pinned source — a supply-chain improvement, not a regression. No postinstall/preinstall script was added. The only other .github change is copilot-instructions.md (docs).

MDS072 SSRF guard (new this window via #769): reviewed in full. The guard is placed at net.Dialer.Control (ssrfControl), which fires on the resolved remote IP immediately before connect on every dial including redirect hops — the correct, DNS-rebinding-safe location. isRestrictedIP covers loopback/unspecified/multicast plus RFC1918, link-local (incl. 169.254.169.254 metadata), ULA, CGN (Alibaba 100.100.100.200), and the IPv6 encodings of those (IPv4-mapped via Unmap, IPv4-compatible ::x.x.x.x, and 6to4 2002::/16), after zone stripping. CheckRedirect caps the chain at 10 and
re-checks literal redirect targets. A per-run probe ceiling (external-max-probes) bounds egress. The rule is opt-in and off by default. One defense-in-depth limitation is recorded below (S001).

No confirmed security defect was found in this window. The single finding is informational hardening on the newly-landed SSRF guard.
