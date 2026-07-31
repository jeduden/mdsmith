---
date: "2026-07-31"
scope: "diff since the 2026-07-24 post-audit diff review — the new MDS060 occurrence rule, MDS073 slidev-structure follow-up, a large high-performance-go.md perf refactor sweep (struct-field reordering, byte-scan gating, memoization), the corpus byte-limit/TOCTOU hardening, the merge-driver -merge -> merge=text fix, the ColumnOfOffset negative-offset clamp, and the goldmark html_block allocation refactor"
method: "audit"
title: "mdsmith post-audit diff review — 2026-07-31"
summary: "Diff review of the 98 files merged since the 2026-07-24 review (base d4af5d5 -> HEAD 2ab4b29). No new security findings: the window is overwhelmingly semantic-preserving perf refactoring (struct-field reordering, byte-scan gating, memoization), test additions, doc/merge-driver correctness fixes, and two content-only rules. §0 baseline reconfirmed — no new exec/network/spawn sink anywhere in the diff (the only exec/http grep hits are the prose of last week's own MDS072 report), recipes still not executed, and every fs/path sink added in the diff is in test files except a githooks refactor that only resolves an unchanged git-rev-parse hooks dir once instead of twice. Two changes are net security hardening: corpus/collect.go now caps read size and closes a Stat->read TOCTOU (skipping oversized/racy files instead of aborting the corpus build), and lint/file.go ColumnOfOffset adds a negative-offset clamp. MDS060 occurrence compiles config-supplied patterns with Go RE2 (linear-time, no ReDoS). The MDS072 external-link-check SSRF finding (S001 of 2026-07-24) remains open and out of this window's scope — its externallink code was untouched and plan 2607242010 is still 🔲."
---
# mdsmith Security Review

- **Target:** jeduden/mdsmith @ `2ab4b29`
- **Mode:** audit
- **Scope:** Diff review of everything merged since the 2026-07-24 post-audit diff review (base d4af5d5) — the new MDS060 occurrence rule, MDS073 slidev-structure follow-up work, a large high-performance-go.md perf refactor sweep (struct-field reordering, byte-scan gating, memoization), the corpus byte-limit/TOCTOU hardening, the merge-driver -merge -> merge=text correctness fix, the ColumnOfOffset negative-offset clamp, and the goldmark html_block allocation refactor
- **Date:** 2026-07-31

## Summary

Critical: 0 | High: 0 | Medium: 0 | Low: 0 | Info: 0

| ID  | Sev | Conf | Title | Surface | Location |
| --- | --- | ---- | ----- | ------- | -------- |

## Coverage

The 98 files changed between the 2026-07-24 review base (d4af5d5) and HEAD (2ab4b29) were
mapped to the threat-model surfaces by changed file, then each security-relevant surface was
read against references/threat-model.md and traced in the source.

Result: NO new security findings this window. The diff is overwhelmingly perf refactoring,
test additions, doc/merge-driver correctness fixes, and two content-only rules. Two changes
are net security hardening.

§0 baseline reconfirmed. No new exec/network/spawn sink appears anywhere in the diff: `git
diff | grep exec.Command|os/exec|child_process|spawn|http.|net.Dial|.Do(|fetch(|/bin/sh`
matched only the prose of last week's own MDS072 report under docs/security/, not code.
Recipes remain non-executed by the tool; the merge driver and pre-merge-commit source did
not change this window. Every fs/path sink (os.Open/ReadFile/WriteFile/Rename,
filepath.Join/Clean/Abs/Rel/EvalSymlinks, Lstat, O_CREATE) added in the diff is in _test.go
files, except the githooks refactor's `filepath.Join(hooksDir, "pre-merge-commit")` —
hooksDir still comes from githooks.ResolveHooksDir (a `git rev-parse` resolution); the only
change is that driftParts resolves it once and passes it to both peekHookSource and
preMergeCommitHookDrift instead of resolving twice. No path or symlink semantics changed.

Directive engine. internal/rules/include/rule.go gains a cheap byte-needle gate
(mayContainIncludeDirective: bytes.Contains for "<?include"/"<?/include") in front of
Check/Fix; path resolution and containment are unchanged behind the gate.
internal/archetype/gensection/engine.go replaces the hand-rolled SplitLines scan with
bytes.Split('\n') — verified semantically equivalent (both yield newlineCount+1 segments,
same on "", "a\n", "a\nb"). No traversal/SSRF/URL-scheme surface added.

Parser resilience. pkg/goldmark/parser/html_block.go swaps strings.ToLower(string(b)) tag
lookups for a 32-byte stack-buffer lowercase (tagBuf.lowerInto ->
tagInAllowedSet/isRawTextTag). The >32-byte truncation cannot cause a false match:
allowedBlockTags and the raw-text set hold only short names, and lookup is full-string
equality, so a truncated 32-byte prefix can never equal a <=10-char tag name. Matching
semantics preserved; type-1 close regexps unchanged. internal/lint/file.go ColumnOfOffset
adds an explicit `if offset < 0 { offset = 0 }` clamp and reuses the cached newline binary
search (newlineSearch) instead of a backward byte scan — a DoS/resilience hardening (defends
against a negative offset panic and turns per-diagnostic O(line-length) into O(log newlines)
on pathologically long lines). internal/corpus/collect.go now Stats then reads via
bytelimit.ReadFileLimited(DefaultMaxInputBytes) and skips (not aborts)
oversized/vanished/racy files, with the read re-checking the cap to close the Stat->read
TOCTOU window — a resilience/DoS hardening against a single oversized file in a cloned repo
aborting the whole corpus build.

New rules. MDS060 occurrence (internal/rules/occurrence/) is a pure content rule: it counts
token/regex occurrences per scope. Its pattern comes from .mdsmith.yml config and is
compiled with Go's regexp (RE2 — linear-time; no catastrophic backtracking is possible),
cached in a package sync.Map keyed by source; tokens and pattern are mutually exclusive. No
exec/network/path sink. MDS073 slidev-structure changes are follow-up to a rule already
reviewed on 2026-07-24 (pure AST/line parser, engine-level per-file recover() still contains
any panic).

Memoization refactors reviewed for correctness, not just perf: internal/lint/file.go
Memo/MemoFile add a Load-before-LoadOrStore fast path (panic-safety and once-semantics
preserved via the shared memoLoad body); internal/rules/crossfilereferenceintegrity/rule.go
caches the static Include/Exclude glob-validation verdict behind atomic.Bool+mutex and
correctly invalidates it (globSettingsDone.Store(false)) on every ApplySettings, so a
reconfigured Rule cannot serve a stale verdict. internal/foreignregion/scan.go single-pass
rewrite preserves the duplicate-start / unmatched-end / unclosed-start diagnostics of the
old per-region scanOne.

Git integration. The merge-driver exclude form changed from `-merge` to `merge=text`
(cmd/mdsmith/mergedriver.go, internal/githooks/githooks.go) so grandfathered Markdown falls
back to git's built-in 3-way text merge instead of binary-conflicting; ExtractGlobs reads
both forms for round-trip. This is a correctness/UX fix (#755); it does not reach any
execution sink or alter which paths the mdsmith driver claims.

Out of scope / carried over: the MDS072 external-link-check SSRF finding (S001 of the
2026-07-24 review) remains OPEN — internal/rules/externallink/ was not touched this window,
and its fix plan (plan/2607242010_mds072-ssrf-network-hardening.md) is still status 🔲. It is
not re-filed here to avoid duplicate tracking. Not re-reviewed this window (unchanged in the
diff): the VS Code extension, the Obsidian plugin, the npm/PyPI/Homebrew/Flatpak
distribution wrappers, and the release CI workflows (last covered by the 2026-06-19 and
2026-05-12 reviews).
