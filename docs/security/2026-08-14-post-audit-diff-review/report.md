---
date: "2026-08-14"
scope: "diff since the 2026-07-24 post-audit diff review (base fe9bbae) — 91 changed files: performance optimizations across the include/crossfile/foreignregion/lint/lsp/schema/goldmark surfaces, the merge-driver -merge -> merge=text exclude change, the new MDS060 occurrence rule, the reworked MDS073 slidev slide-structure rule, and corpus-collection size limiting"
method: "audit"
title: "mdsmith post-audit diff review — 2026-08-14"
summary: "Diff review of the 91 files merged since the 2026-07-24 review. No new security defect: the window is dominated by behavior-preserving performance work (byte-needle gating in the include rule, memoization in crossfile/lint, single-pass foreign-region scanning, struct-layout and SIMD byte-scan tweaks in the LSP and goldmark parser, editDistance stack arrays in the slidev rule) plus two genuine hardenings — corpus collection now enforces bytelimit.DefaultMaxInputBytes and skips oversized/racy files instead of aborting (DoS reduction), and the merge-driver managed .gitattributes block now emits merge=text rather than -merge so ignore-derived Markdown paths keep git's 3-way text merge instead of binary-conflicting. The two new/changed rules (MDS060 occurrence, MDS073 slidev) use Go RE2 and length-guarded byte parsing, so no ReDoS or out-of-bounds. §0 baseline reconfirmed: no exec/spawn sink added anywhere in the diff, recipes remain non-executed, the include byte-needle fast-path drops no diagnostic. Carried forward and still open (already tracked by plans 2607242010 and 2607242011): the 2026-07-24 MDS072 external-link-check SSRF findings — externallink/ is unchanged this window."
---
# mdsmith Security Review

- **Target:** jeduden/mdsmith @ `2ab4b29`
- **Mode:** audit
- **Scope:** Diff review of everything merged since the 2026-07-24 post-audit diff review (base fe9bbae) — 91 changed files: performance optimizations across the include/crossfile/foreignregion/lint/lsp/schema/goldmark surfaces, the merge-driver -merge -> merge=text exclude change, the new MDS060 occurrence rule, the reworked MDS073 slidev slide-structure rule, and corpus-collection size limiting
- **Date:** 2026-08-14

## Summary

Critical: 0 | High: 0 | Medium: 0 | Low: 0 | Info: 0

| ID  | Sev | Conf | Title | Surface | Location |
| --- | --- | ---- | ----- | ------- | -------- |

## Coverage

The 91 files changed between the 2026-07-24 review base (fe9bbae) and HEAD (2ab4b29) were mapped to the
threat-model surfaces by changed file and each surface read against references/threat-model.md. No new
security defect was found in this window; the changes are overwhelmingly behavior-preserving performance
work plus one merge-driver hardening and one DoS hardening. Details by surface:

§0 baseline reconfirmed by exclusion. `git diff fe9bbae..HEAD | grep '^+.*(exec.Command|os/exec|sh -c|
/bin/sh)'` returns nothing: no execution sink was added anywhere in the diff. The recipe executor
(internal/build/exec.go, hooks.go) and every explicit-argv git/go/gh call site are untouched this window, so
recipes remain non-executed by the tool and no zero-interaction path (LSP fix-on-save, merge driver, editor
open) gained an exec sink.

Directive engine. internal/rules/include/rule.go added a mayContainIncludeDirective byte-needle fast-path
(bytes.Contains for "<?include" OR "<?/include") that skips Check/Fix on files carrying neither marker. Traced
for a false-negative bypass: the gate is a superset test (a file with either needle is still fully processed),
both needles are required precisely so a dangling <?/include?> orphan-end marker is not skipped, and a file
with neither substring provably cannot hold any include directive or end marker. No dropped diagnostic, no
containment change. Path resolution, symlink default-deny, and include depth/cycle handling were not modified.

Parser / DoS resilience. pkg/goldmark/parser/html_block.go replaced strings.ToLower(string(b)) tag lookups
with a 32-byte stack buffer (tagBuf.lowerInto); truncation past 32 bytes cannot cause a false match because
every allowed/raw-text tag name is far shorter than 32 bytes, so a truncated 32-byte string never equals a
short tag name. internal/foreignregion/scan.go was refactored from per-region passes to a single line walk
with per-region state machines; marker matching (trimmed-line equality) and the malformed-region diagnostics
are behavior-preserving. internal/lint/file.go's ColumnOfOffset now reuses the cached newline binary search
instead of an O(line-length) backward scan (a DoS-relevant improvement on single very long lines) and adds an
offset<0 clamp. No new backtracking regex; the two new rules use Go RE2 (linear time, no ReDoS).

New/changed rules. MDS060 occurrence (internal/rules/occurrence/, 458 new lines) compiles a user-supplied
regexp via regexp.Compile (RE2, so a hostile pattern cannot cause catastrophic backtracking) and counts
matches with FindAllStringIndex; config is bounded and the compiled-pattern sync.Map is keyed on bounded
config strings. MDS073 slidevstructure (273 lines changed) is line/byte parsing with explicit len>0 guards
before every index; its new editDistance helper guards la/lb>64 and slices fixed [65]int arrays to lb+1<=65,
so no out-of-bounds and O(64^2) worst case.

Corpus collection (DoS hardening, positive). internal/corpus/collect.go now os.Stat + bytelimit.ReadFileLimited
every file against bytelimit.DefaultMaxInputBytes and skips an oversized / vanished / permission-denied file
(reporting progress) instead of aborting the whole corpus build; the Stat->Read TOCTOU is closed by
ReadFileLimited re-checking the size on the actual read. This strictly reduces attacker-supplied-input DoS
exposure.

Git integration (merge-driver, hardening). The exclude form emitted into the managed .gitattributes block
changed from `-merge` to `merge=text` (internal/githooks/githooks.go RenderManagedBlock; issue #755). Both take
the mdsmith driver off the path via last-match-wins; merge=text is strictly safer because it keeps git's
built-in 3-way text merge instead of leaving the ignore-derived Markdown paths binary-conflicting. The excludes
remain markdown-extension-scoped (scopeExcludeToMarkdown, issue #750), so no non-Markdown file is affected;
ExtractGlobs still reads the legacy `-merge` form so an older block round-trips without false drift. No attacker
content is interpolated into the .gitattributes patterns. mergedriver.go's exec.Command sites remain
constant-arg `git` invocations.

LSP. internal/lsp/protocol.go reordered Diagnostic struct fields (layout-only, wire JSON unchanged) and
server_codeaction.go swapped a scalar newline count for bytes.Count (behavior-preserving). No change to the
fix-on-save / executeCommand handlers or their write scope.

Schema. internal/schema/validate.go renamed isClaimed -> exported IsClaimed (dedup, PR #756) with no logic
change; matchtree.go / validate_content.go follow the rename.

Carried forward (NOT re-reported, still open). The 2026-07-24 review's MDS072 external-link-check SSRF findings
(S001 no private/loopback/metadata target filtering + redirect-follow, S002 no per-run probe ceiling, S003
telemetry-doc gap) remain unremediated: internal/rules/externallink/ is unchanged in this window (empty diff),
and the tracking plans plan/2607242010_mds072-ssrf-network-hardening.md and
plan/2607242011_security-hardening-batch-2026-07-24.md are both still status 🔲. That surface is already
documented and queued; this review adds no new finding there.

Not re-reviewed this window (unchanged in the diff, last covered by the 2026-06-19 / 2026-07-03 reviews): the
VS Code extension (editors/vscode), the Obsidian plugin (editors/obsidian), the npm/PyPI distribution wrappers,
and the release workflow.
