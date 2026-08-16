---
date: "2026-08-07"
scope: "diff since the 2026-07-24 post-audit diff review — the new MDS060 occurrence rule, the vendored goldmark html_block tag-lowering rewrite, the slidev MDS073 structural expansion, the corpus byte-limit cap, the ColumnOfOffset negative-offset clamp, the merge-driver/git-hook -merge to merge=text change, and a batch of high-performance-go.md perf refactors"
method: "audit"
title: "mdsmith post-audit diff review — 2026-08-07"
summary: "Diff review of the 62 commits merged since the 2026-07-24 review. The window is entirely high-performance-go.md perf refactors and one new opt-in rule, with zero new security findings. A whole-diff sink grep found no new exec, spawn, network, or file-write sink in non-test code. The new MDS060 occurrence rule is opt-in and compiles only config-supplied patterns via Go RE2 (no ReDoS). The goldmark tag-lowering rewrite writes a document-controlled tag name into a fixed 32-byte stack buffer but guards every store with a length check (safe truncation); the slidev Levenshtein [65]int buffers are guarded by an la/lb>64 bail. The build reserved-device-name length gate cannot skip a real reserved name (all are 3–4 chars). Two changes are net security improvements: the corpus byte-limit cap (bounds input read, closes a Stat/Read TOCTOU) and the ColumnOfOffset negative-offset clamp (panic hardening). §0 baseline holds: recipes still not executed, build output validation intact, symlink/containment logic unchanged, editor/CI/wrapper source untouched. The prior MDS072 external-link-check SSRF (2026-07-24 S001) is carried over unchanged — externallink saw no changes this window — and stays tracked by two open hardening plans."
---
# mdsmith Security Review

- **Target:** jeduden/mdsmith @ `2ab4b29`
- **Mode:** audit
- **Scope:** Diff review of the 62 commits merged since the 2026-07-24 post-audit diff review (base d4af5d5) — the new MDS060 occurrence rule, the vendored goldmark html_block tag-lowering rewrite, the slidev MDS073 structural expansion, the corpus byte-limit cap, the ColumnOfOffset negative-offset clamp, the merge-driver/git-hook -merge to merge=text change, and a large batch of high-performance-go.md perf refactors
- **Date:** 2026-08-07

## Summary

Critical: 0 | High: 0 | Medium: 0 | Low: 0 | Info: 0

| ID  | Sev | Conf | Title | Surface | Location |
| --- | --- | ---- | ----- | ------- | -------- |

## Coverage

Base d4af5d5 (2026-07-24 review HEAD) diffed to HEAD 2ab4b29. The 62-commit window is dominated by
high-performance-go.md perf refactors (struct field reordering, byte-needle gates, single-pass scans,
zero-alloc buffers) and their tests; the security-relevant subset was extracted with a sink grep over the
whole diff (exec.Command/os-exec/child_process/spawn/http.Do/net.Dial/fetch/WriteFile/OpenFile) and each hit
traced. No new exec, spawn, network, or file-write sink appears anywhere in non-test code this window.

Surfaces read against references/threat-model.md:

- Directive engine / path resolution (§1b): internal/rules/include/rule.go adds only a two-needle byte gate
  (mayContainIncludeDirective) in front of Check/Fix — a perf short-circuit; the path-resolution and
  containment logic downstream is unchanged. internal/rules/crossfilereferenceintegrity/rule.go changed with
  no new filepath.Join/Clean/Rel/EvalSymlinks call. No URL scheme accepted, no traversal introduced.

- Recipe / build surface (§1a): internal/rules/build/rule.go's only change gates the reserved-device-name
  check by segment length (skip len<3 || len>4 before ToUpper). All 22 names in reservedDeviceNames are
  exactly 3 (CON/PRN/AUX/NUL) or 4 (COM1-9/LPT1-9) chars, so the gate cannot skip a real reserved name — the
  build output-path validation baseline is preserved, no bypass. Recipes remain non-executed by the tool.

- New MDS060 occurrence rule (internal/rules/occurrence/rule.go, 458 lines): EnabledByDefault()=false
  (opt-in). Its only non-trivial primitive is regexp.Compile of a CONFIG-supplied pattern (.mdsmith.yml, not
  document content), cached in a package sync.Map keyed by pattern source. Go's regexp is RE2 (linear-time,
  no catastrophic backtracking), so no ReDoS. No os/filepath/exec/net reference in the file; it counts tokens
  and regex matches over document content and emits diagnostics only.

- Parser resilience / DoS (§2): pkg/goldmark/parser/html_block.go replaces strings.ToLower(string(b)) with a
  fixed 32-byte stack buffer (tagBuf.lowerInto) written per-character from a document-controlled tag name.
  The write loop has an explicit `if t.n >= len(t.buf) { break }` bound before every store, so oversized tag
  names truncate harmlessly rather than overflowing; the truncation cannot false-match a short allowed tag.
  internal/rules/slidevstructure/rule.go's editDistance uses two [65]int stack buffers sliced to lb+1, but is
  guarded by `if la > 64 || lb > 64 { return la + lb }` before the slice, so lb+1 maxes at exactly 65 — no
  out-of-range. Both fixed buffers are safe. internal/lint/file.go's ColumnOfOffset now clamps a negative
  offset to 0 before indexing (commit daae490) — a panic-hardening improvement, not a regression.

- Resource limits (§2): internal/corpus/collect.go now Stats each file, skips anything over
  bytelimit.DefaultMaxInputBytes, reads via bytelimit.ReadFileLimited (re-checks size on read to close the
  Stat/Read TOCTOU window), and skips (rather than aborts the whole corpus build on) an unreadable/racy
  file. This is a DoS/OOM improvement over the previous unbounded os.ReadFile, not a regression.

- Git integration (§7): cmd/mdsmith/mergedriver.go, internal/githooks/githooks.go, and
  internal/rules/githooksync/rule.go change the .gitattributes exclude attribute from `-merge` to
  `merge=text` (issue #755) and refactor hooks-dir resolution. The emitted attribute is a constant; exclude
  path patterns stay scoped to Markdown extensions; the joined hook path is filepath.Join(hooksDir,
  "pre-merge-commit") with a constant filename — no attacker-controlled path component and no execution sink.
  This is a merge-behaviour correctness fix, not a security change.

- LSP (§3): internal/lsp/protocol.go and server_codeaction.go changes are struct field reordering and small
  tweaks with no new command handler, applyEdit, file write, or network call.

One prior finding is carried over, unchanged and already tracked: the MDS072 external-link-check SSRF
(2026-07-24 S001, Medium) — internal/rules/externallink/ had zero changes this window (empty diff stat), so
the opt-in blind-SSRF probe remains as previously described. It is already scheduled by
plan/2607242010_mds072-ssrf-network-hardening.md and plan/2607242011_security-hardening-batch-2026-07-24.md
(both status 🔲, open), so this review files no new plan for it.

Not re-reviewed this window (unchanged source): the VS Code extension (editors/vscode), the Obsidian plugin
(editors/obsidian), the npm/PyPI/Homebrew/Flatpak distribution wrappers, and the release CI workflows — none
had source changes in the diff. They were last covered by the 2026-06-19 and 2026-07-03 reviews.

Conclusion: zero new findings. The window is entirely perf/refactor work; two changes (corpus byte-limit
cap, ColumnOfOffset negative-offset clamp) are net security improvements. Every §0 baseline defense holds:
recipes still not executed, no new exec/spawn/network sink, build output validation intact, symlink and
containment logic unchanged, editor/CI/wrapper source untouched.
