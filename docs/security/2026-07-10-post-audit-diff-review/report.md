---
date: "2026-07-10"
scope: "diff since the 2026-07-03 post-audit diff review — word-list file loading, the F001/F002 fixes, directive-engine workspace containment, CLI/engine parser, CI/supply chain"
method: "audit"
title: "mdsmith post-audit diff review — 2026-07-10"
summary: "Diff review of 85 commits merged since the 2026-07-03 post-audit diff review. No findings at any severity. The F001 (Workspace.ReadFile os.OpenRoot containment) and F002 (block-quote recursion depth cap) fixes both landed and hold. The one new file-resolution surface — word-list files under .mdsmith/wordlists/ — rejects symlinks, subdirectories, bad basenames, and YAML aliases, resolves extends by name in-memory with cycle detection, and is gated behind the mergeKinds flag so the WASM path does no filesystem discovery. Baseline §0 defenses reconfirmed: no new exec sink, no supply-chain change."
---
# mdsmith Security Review

- **Target:** jeduden/mdsmith @ `9c36a270861c4ad0c83a60188a6d96c758d766bf`
- **Mode:** audit
- **Scope:** Diff review of everything merged since the 2026-07-03 post-audit diff review (base bfa7803d), across the new word-list file-loading surface, the F001/F002 fixes, the directive-engine workspace containment, the CLI/engine parser, and the CI/supply-chain surface
- **Date:** 2026-07-10

## Summary

Critical: 0 | High: 0 | Medium: 0 | Low: 0 | Info: 0

| ID  | Sev | Conf | Title | Surface | Location |
| --- | --- | ---- | ----- | ------- | -------- |

## Coverage

85 commits since the 2026-07-03 diff review (base bfa7803d4f1dc68cd4cdfe375e93b12748417ae6) were mapped to the seven
threat-model surfaces by changed file; only Go source under internal/ and pkg/ carried security-relevant change (no
.github/, npm/, python/, or action.yml files changed this window). No Critical/High/Medium/Low/Info findings.

Prior-finding fixes verified as landed and holding. F001 (Workspace.ReadFile containment): OSWorkspace.ReadFile now
routes workspace-relative paths through readFileRooted (short-lived os.OpenRoot / RESOLVE_BENEATH handle, closed per
call), and OverlayWorkspace.ReadFile's disk fall-through reads through the sync.Once-cached ensureDiskFS/lint.OpenRootFS
handle; absolute paths keep their documented unconstrained passthrough (pkg/mdsmith/workspace.go:78-83, overlay.go:59-71,
workspace_fs.go:38-45). F002 (block-quote recursion): tryBlockquote now refuses to recurse past maxBlockquoteDepth=100
via the new scanLayer0Depth core (internal/lint/layer0.go:103-160,311-321), so a pathologically nested `>` line can no
longer exhaust the goroutine stack.

New surface — word-list file loading (internal/config/wordlist_files.go, internal/wordlist/wordlist.go, plan 210) —
reviewed as a fresh file-resolution boundary. discoverWordlists walks only .mdsmith/wordlists/*.{yaml,yml} at the
workspace root and rejects symlink entries (os.ModeSymlink), subdirectories, any basename not matching
`^[a-z][a-z0-9-]*$`, and a .yaml/.yml collision; reads go through the shared 1 MB readLimitedConfig cap; wordlist.Parse
rejects YAML anchors/aliases and uses strict KnownFields decoding. `extends:` is a list *name* resolved in-memory
against the discovered map (wordlist.Resolve/flatten), not a filesystem path, and cycles are caught by the per-chain
seen set — so extends cannot traverse the filesystem or infinite-loop. Discovery is gated behind the mergeKinds flag
(internal/config/load.go), so the in-memory ParseBytes/WASM path performs no filesystem discovery. Resolved entries
only feed rule word-lists (banned/required words) whose effects stay in local diagnostics; there is no exfiltration
channel. Symlinking the .mdsmith/wordlists directory itself would let os.ReadDir list an outside directory, but only
strict-YAML .yaml files with regex-valid basenames could be read, and their content only tunes local lint word-lists —
no read escapes to an output channel, and the behaviour matches the pre-existing kinds/conventions discovery. Not
treated as a finding.

CLI core / engine parser (rule changes across internal/rules/*, internal/schema/matchtree.go, internal/index/index.go):
the churned rule diffs are perf refactors (fmt.Sprintf→strconv, pre-sized/nil-return slices, per-File memoization) and
WordlistConsumer interface additions; no new unbounded recursion, slice-bounds risk, or panic path. matchtree.go's
ScopeMatch change is a GC struct-field reordering only. All dynamic regex compilation (internal/schema/*,
requiredtextpatterns, maxsectionlength) uses Go's RE2 engine, which is linear-time — no catastrophic backtracking /
ReDoS on config- or document-supplied patterns.

Baseline §0 defenses reconfirmed: every exec.Command/CommandContext sink is either a constant-arg git/go/gh invocation
or the documented CLI-only build-recipe runner (internal/build/exec.go, hooks.go); no exec was added to the LSP, WASM,
merge-driver, or pre-merge-commit path. No new child_process/spawn sink in the VS Code or Obsidian TypeScript.

Not re-reviewed (no changed files this window): the LSP server, VS Code extension, Obsidian plugin, Git
merge-driver/pre-merge-commit hooks, and the CI/distribution wrappers — all last covered by the 2026-07-03 and
2026-06-19 reviews.
